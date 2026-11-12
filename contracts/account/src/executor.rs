//! Executor entrypoint: policy check, then dispatch one step to its venue.
use near_sdk::json_types::U128;
use near_sdk::serde_json;
use near_sdk::{env, near, AccountId, Gas, Promise, PromiseError, PromiseOrValue};

use crate::events::Event;
use crate::ledger::Slot;
use crate::step::Step;
use crate::venues;
use crate::{Contract, ContractExt};

/// Withdraw leg + settle + observe, issued from `on_lend_asset`.
const GAS_LEND_CB: Gas = Gas::from_tgas(160);
/// Swap leg + settle + observe, issued from `on_lst_exit`.
const GAS_LST_CB: Gas = Gas::from_tgas(190);
const GAS_LST_SETTLE: Gas = Gas::from_tgas(10);
/// Stake leg + `on_stake`, issued from `on_unwrap`.
const GAS_STAKE_CB: Gas = Gas::from_tgas(150);
/// Re-wrap + observe on a failed stake.
const GAS_REWRAP_CB: Gas = Gas::from_tgas(50);
/// ft_price scale; price is cut to 10 decimals so amount * 1e10 fits u128 up to ~34k NEAR.
const PRICE_CUT: u128 = 100_000_000_000_000;
const SHARE_ONE: u128 = 10_000_000_000;
pub const STAKING_KEY: &str = "staking";

pub fn lending_key(token: &AccountId) -> String {
    format!("lending:{token}")
}

#[near]
impl Contract {
    /// Zero deposit (non-payable); only via the executor function-call key on this account.
    pub fn execute_step(&mut self, step: Step) -> Promise {
        self.assert_executor();
        self.check_step(&step);
        Event::Step { step: step.clone() }.emit();
        let v = self.venues.clone();
        match step {
            Step::Supply { token, amount } => {
                let bal = Slot::Balance(token.clone());
                self.debit(&bal, amount.0);
                let pos = Slot::Position(lending_key(&token));
                self.settle(
                    venues::supply(v.lending, token, amount.0),
                    Some(bal),
                    amount.0,
                    Some(pos),
                    true,
                )
            }
            Step::WithdrawLend { token, amount } => {
                self.debit(&Slot::Position(lending_key(&token)), amount.0);
                venues::lend_asset(v.lending, token.clone()).then(
                    Self::ext(env::current_account_id())
                        .with_static_gas(GAS_LEND_CB)
                        .on_lend_asset(token, amount),
                )
            }
            // LST exit: instant swap of LiNEAR shares instead of the ~4 epoch unstake
            Step::Swap {
                pool_id,
                token_in,
                token_out,
                amount_in,
                min_out,
            } if token_in == v.staking => {
                let pos = Slot::Position(STAKING_KEY.into());
                self.debit(&pos, amount_in.0);
                let all = self.slot(&pos) == 0;
                venues::lst_price(v.staking.clone())
                    .and(venues::ft_balance_of(v.staking, env::current_account_id()))
                    .then(
                        Self::ext(env::current_account_id())
                            .with_static_gas(GAS_LST_CB)
                            .on_lst_exit(pool_id, token_out, amount_in, min_out, all),
                    )
            }
            Step::Swap {
                pool_id,
                token_in,
                token_out,
                amount_in,
                min_out,
            } => {
                let bal_in = Slot::Balance(token_in.clone());
                self.debit(&bal_in, amount_in.0);
                let p = venues::swap(
                    v.dex,
                    pool_id,
                    token_in,
                    token_out.clone(),
                    amount_in.0,
                    min_out.0,
                );
                let p = self.settle(p, Some(bal_in), amount_in.0, None, true);
                Self::observe(p, token_out)
            }
            Step::Stake { token, amount } => {
                self.debit(&Slot::Balance(token.clone()), amount.0);
                venues::unwrap(token.clone(), amount.0).then(
                    Self::ext(env::current_account_id())
                        .with_static_gas(GAS_STAKE_CB)
                        .on_unwrap(token, amount),
                )
            }
            Step::Unstake { amount } => {
                let pos = Slot::Position(STAKING_KEY.into());
                self.debit(&pos, amount.0);
                let p = match self.slot(&pos) {
                    0 => venues::unstake_all(v.staking),
                    _ => venues::unstake(v.staking, amount.0),
                };
                self.settle(p, Some(pos), amount.0, None, false)
            }
            Step::Fee { token, amount } => self.send(token, self.treasury.clone(), amount.0),
            Step::ToOwner { token, amount } => {
                let owner = self.owner.clone();
                Event::Withdraw {
                    token: token.clone(),
                    receiver: owner.clone(),
                    amount: U128(amount.0),
                }
                .emit();
                self.send(token, owner, amount.0)
            }
        }
    }

    /// Scales the withdraw to lending inner units; no asset config = position refunded.
    #[private]
    pub fn on_lend_asset(&mut self, token: AccountId, amount: U128) -> PromiseOrValue<U128> {
        let pos = Slot::Position(lending_key(&token));
        let scaled = env::promise_result_checked(0, 16_384)
            .ok()
            .and_then(|b| serde_json::from_slice::<serde_json::Value>(&b).ok())
            .and_then(|a| a["config"]["extra_decimals"].as_u64())
            .and_then(|d| 10u128.checked_pow(d as u32))
            .and_then(|m| amount.0.checked_mul(m));
        let Some(inner) = scaled else {
            self.credit(&pos, amount.0);
            return PromiseOrValue::Value(U128(0));
        };
        let p = venues::withdraw_lend(self.venues.lending.clone(), token.clone(), inner);
        let p = self.settle(p, Some(pos), amount.0, None, false);
        PromiseOrValue::Promise(Self::observe(p, token))
    }

    /// `amount` is NEAR value off the staking position. Full exit sells every share (rewards + dust),
    /// partial sells `amount` worth at the current price. Unreadable price/balance = position refunded.
    #[private]
    pub fn on_lst_exit(
        &mut self,
        pool_id: u64,
        token_out: AccountId,
        amount: U128,
        min_out: U128,
        all: bool,
    ) -> PromiseOrValue<U128> {
        let read = |i| {
            env::promise_result_checked(i, 64)
                .ok()
                .and_then(|b| serde_json::from_slice::<U128>(&b).ok())
                .map(|v| v.0)
        };
        let shares = match (read(0).map(|p| p / PRICE_CUT), read(1)) {
            (_, Some(bal)) if all => Some(bal),
            (Some(p), Some(bal)) if p > 0 => {
                amount.0.checked_mul(SHARE_ONE).map(|v| (v / p).min(bal))
            }
            _ => None,
        };
        let pos = Slot::Position(STAKING_KEY.into());
        let Some(shares) = shares.filter(|s| *s > 0) else {
            self.credit(&pos, amount.0);
            return PromiseOrValue::Value(U128(0));
        };
        let v = self.venues.clone();
        let p = venues::swap(
            v.dex,
            pool_id,
            v.staking,
            token_out.clone(),
            shares,
            min_out.0,
        )
        .then(
            Self::ext(env::current_account_id())
                .with_static_gas(GAS_LST_SETTLE)
                .on_lst_swap(amount),
        );
        PromiseOrValue::Promise(Self::observe(p, token_out))
    }

    /// Unwrap failed = wNEAR refunded. Else stake; a failed stake re-wraps (see `on_stake`).
    #[private]
    pub fn on_unwrap(&mut self, token: AccountId, amount: U128) -> PromiseOrValue<U128> {
        if failed() {
            self.credit(&Slot::Balance(token), amount.0);
            return PromiseOrValue::Value(U128(0));
        }
        let v = self.venues.clone();
        PromiseOrValue::Promise(
            venues::stake(v.staking, amount.0).then(
                Self::ext(env::current_account_id())
                    .with_static_gas(GAS_REWRAP_CB)
                    .on_stake(token, amount),
            ),
        )
    }

    /// Staked = position booked. Failed = NEAR wrapped back and credited via observe.
    #[private]
    pub fn on_stake(&mut self, token: AccountId, amount: U128) -> PromiseOrValue<U128> {
        if !failed() {
            self.credit(&Slot::Position(STAKING_KEY.into()), amount.0);
            return PromiseOrValue::Value(amount);
        }
        let p = venues::wrap(token.clone(), amount.0);
        PromiseOrValue::Promise(Self::observe(p, token))
    }

    /// Ref swaps all or nothing: no shares used = position refunded.
    #[private]
    pub fn on_lst_swap(&mut self, amount: U128) -> U128 {
        let used = env::promise_result_checked(0, 64)
            .ok()
            .and_then(|b| serde_json::from_slice::<U128>(&b).ok())
            .map_or(0, |u| u.0);
        if used == 0 {
            self.credit(&Slot::Position(STAKING_KEY.into()), amount.0);
        }
        U128(used)
    }

    /// Pulls unstaked NEAR back once the staking delay passed. Funds only come here, so anyone may call.
    pub fn claim_unstaked(&self) -> Promise {
        venues::withdraw_unstaked(self.venues.staking.clone())
    }
}

fn failed() -> bool {
    matches!(env::promise_result_checked(0, 0), Err(PromiseError::Failed))
}
