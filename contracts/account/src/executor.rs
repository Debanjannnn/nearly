//! Executor entrypoint: policy check, then dispatch one step to its venue.
use near_sdk::json_types::U128;
use near_sdk::{env, near, require, AccountId, NearToken, Promise};

use crate::events::Event;
use crate::ledger::Slot;
use crate::step::Step;
use crate::venues;
use crate::{Contract, ContractExt};

/// NEAR kept back for storage of new state + gas of later executor txs.
const NEAR_RESERVE: NearToken = NearToken::from_millinear(500);
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
                    Some((pos, None)),
                    true,
                )
            }
            Step::WithdrawLend { token, amount } => {
                let pos = Slot::Position(lending_key(&token));
                self.debit(&pos, amount.0);
                let bal = Slot::Balance(token.clone());
                self.settle(
                    venues::withdraw_lend(v.lending, token, amount.0),
                    Some(pos),
                    amount.0,
                    Some((bal, None)),
                    false,
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
                // shortcut: books min_out, not the real output; #4 reads the actual amount
                let out = Some((Slot::Balance(token_out.clone()), Some(min_out)));
                let p = venues::swap(v.dex, pool_id, token_in, token_out, amount_in.0, min_out.0);
                self.settle(p, Some(bal_in), amount_in.0, out, true)
            }
            Step::Stake { amount } => {
                require!(amount.0 <= free_near(), "insufficient NEAR");
                let pos = Slot::Position(STAKING_KEY.into());
                self.settle(
                    venues::stake(v.staking, amount.0),
                    None,
                    amount.0,
                    Some((pos, None)),
                    false,
                )
            }
            Step::Unstake { amount } => {
                let pos = Slot::Position(STAKING_KEY.into());
                self.debit(&pos, amount.0);
                self.settle(
                    venues::unstake(v.staking, amount.0),
                    Some(pos),
                    amount.0,
                    None,
                    false,
                )
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
}

fn free_near() -> u128 {
    let locked = env::storage_usage() as u128 * env::storage_byte_cost().as_yoctonear();
    env::account_balance()
        .as_yoctonear()
        .saturating_sub(locked + NEAR_RESERVE.as_yoctonear())
}
