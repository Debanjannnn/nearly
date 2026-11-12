//! Outgoing calls to tokens and venues. Each fn only builds the promise; accounting lives in ledger.
use near_sdk::json_types::U128;
use near_sdk::serde_json::json;
use near_sdk::{ext_contract, AccountId, Gas, NearToken, Promise};

// Gas per step's venue leg. Executor sends 300 TGas; leg + settle + observe must fit.
/// Fee / to_owner: plain ft_transfer.
pub const GAS_FT_TRANSFER: Gas = Gas::from_tgas(10);
/// Supply: ft_transfer_call -> lending ft_on_transfer -> ft_resolve_transfer.
pub const GAS_SUPPLY: Gas = Gas::from_tgas(100);
/// Withdraw: lending execute -> ft_transfer back + its own callback.
pub const GAS_WITHDRAW_LEND: Gas = Gas::from_tgas(120);
/// Swap: ft_transfer_call -> dex swap -> ft_transfer out -> ft_resolve_transfer.
pub const GAS_SWAP: Gas = Gas::from_tgas(150);
/// LiNEAR deposit_and_stake / unstake / withdraw_all.
pub const GAS_STAKE: Gas = Gas::from_tgas(50);
pub const GAS_UNSTAKE: Gas = Gas::from_tgas(50);
pub const GAS_STORAGE: Gas = Gas::from_tgas(10);
pub const GAS_VIEW: Gas = Gas::from_tgas(5);
/// NEP-145 registration_only cost for a standard FT (also LiNEAR).
pub const FT_STORAGE: NearToken = NearToken::from_micronear(1_250);
/// Rhea lending `storage_balance_bounds.min` (testnet + mainnet).
pub const LENDING_STORAGE: NearToken = NearToken::from_millinear(100);
const ONE_YOCTO: NearToken = NearToken::from_yoctonear(1);

#[allow(dead_code)]
#[ext_contract(ext_ft)]
trait Ft {
    fn ft_transfer(&mut self, receiver_id: AccountId, amount: U128, memo: Option<String>);
    fn ft_transfer_call(
        &mut self,
        receiver_id: AccountId,
        amount: U128,
        memo: Option<String>,
        msg: String,
    ) -> U128;
    fn ft_balance_of(&self, account_id: AccountId) -> U128;
    fn storage_deposit(&mut self, account_id: Option<AccountId>, registration_only: Option<bool>);
}

#[allow(dead_code)]
#[ext_contract(ext_wnear)]
trait WNear {
    fn near_withdraw(&mut self, amount: U128);
    fn near_deposit(&mut self);
}

#[allow(dead_code)]
#[ext_contract(ext_lending)]
trait Lending {
    fn execute(&mut self, actions: Vec<near_sdk::serde_json::Value>);
    fn get_asset(&self, token_id: AccountId) -> Option<near_sdk::serde_json::Value>;
}

#[allow(dead_code)]
#[ext_contract(ext_staking)]
trait Staking {
    fn deposit_and_stake(&mut self);
    fn unstake(&mut self, amount: U128);
    fn unstake_all(&mut self);
    fn withdraw_all(&mut self);
    fn ft_price(&self) -> U128;
}

/// wNEAR -> NEAR to this account; 1 yocto per NEP-141 convention.
pub fn unwrap(wnear: AccountId, amount: u128) -> Promise {
    ext_wnear::ext(wnear)
        .with_attached_deposit(ONE_YOCTO)
        .with_static_gas(GAS_FT_TRANSFER)
        .near_withdraw(U128(amount))
}

pub fn wrap(wnear: AccountId, amount: u128) -> Promise {
    ext_wnear::ext(wnear)
        .with_attached_deposit(NearToken::from_yoctonear(amount))
        .with_static_gas(GAS_FT_TRANSFER)
        .near_deposit()
}

pub fn ft_transfer(token: AccountId, to: AccountId, amount: u128) -> Promise {
    ext_ft::ext(token)
        .with_attached_deposit(ONE_YOCTO)
        .with_static_gas(GAS_FT_TRANSFER)
        .ft_transfer(to, U128(amount), None)
}

pub fn ft_balance_of(token: AccountId, account: AccountId) -> Promise {
    ext_ft::ext(token)
        .with_static_gas(GAS_VIEW)
        .ft_balance_of(account)
}

/// NEP-145 registration; excess over the minimum is refunded by the callee.
pub fn register(contract: AccountId, account: AccountId, deposit: NearToken) -> Promise {
    ext_ft::ext(contract)
        .with_attached_deposit(deposit)
        .with_static_gas(GAS_STORAGE)
        .storage_deposit(Some(account), Some(true))
}

fn ft_transfer_call(
    token: AccountId,
    to: AccountId,
    amount: u128,
    msg: String,
    gas: Gas,
) -> Promise {
    ext_ft::ext(token)
        .with_attached_deposit(ONE_YOCTO)
        .with_static_gas(gas)
        .ft_transfer_call(to, U128(amount), None, msg)
}

/// Empty msg = plain supply (earns supply APR, not collateral). Needs lending registration.
pub fn supply(lending: AccountId, token: AccountId, amount: u128) -> Promise {
    ft_transfer_call(token, lending, amount, String::new(), GAS_SUPPLY)
}

/// Lending asset config; withdraw amounts are in its inner units (token * 10^extra_decimals).
pub fn lend_asset(lending: AccountId, token: AccountId) -> Promise {
    ext_lending::ext(lending)
        .with_static_gas(GAS_VIEW)
        .get_asset(token)
}

/// `inner` is in lending inner units. `max_amount` withdraws up to it, so share rounding can't fail it.
pub fn withdraw_lend(lending: AccountId, token: AccountId, inner: u128) -> Promise {
    let action = json!({"Withdraw": {"token_id": token, "max_amount": U128(inner)}});
    ext_lending::ext(lending)
        .with_attached_deposit(ONE_YOCTO)
        .with_static_gas(GAS_WITHDRAW_LEND)
        .execute(vec![action])
}

/// Instant swap: dex sends `token_out` back to us; it panics (full refund) below `min_out`.
pub fn swap(
    dex: AccountId,
    pool_id: u64,
    token_in: AccountId,
    token_out: AccountId,
    amount_in: u128,
    min_out: u128,
) -> Promise {
    let msg = json!({"force": 0, "skip_unwrap_near": true, "actions": [{
        "pool_id": pool_id,
        "token_in": token_in,
        "token_out": token_out,
        "amount_in": U128(amount_in),
        "min_amount_out": U128(min_out),
    }]});
    ft_transfer_call(token_in, dex, amount_in, msg.to_string(), GAS_SWAP)
}

pub fn stake(staking: AccountId, amount: u128) -> Promise {
    ext_staking::ext(staking)
        .with_attached_deposit(NearToken::from_yoctonear(amount))
        .with_static_gas(GAS_STAKE)
        .deposit_and_stake()
}

/// Delayed (~4 epochs on LiNEAR); NEAR is then pulled by `claim_unstaked`.
pub fn unstake(staking: AccountId, amount: u128) -> Promise {
    ext_staking::ext(staking)
        .with_static_gas(GAS_UNSTAKE)
        .unstake(U128(amount))
}

/// Full exit: LiNEAR rounds staked balance down, so unstake(amount) can miss by a few yocto.
pub fn unstake_all(staking: AccountId) -> Promise {
    ext_staking::ext(staking)
        .with_static_gas(GAS_UNSTAKE)
        .unstake_all()
}

/// NEAR per LiNEAR share, 24 decimals.
pub fn lst_price(staking: AccountId) -> Promise {
    ext_staking::ext(staking)
        .with_static_gas(GAS_VIEW)
        .ft_price()
}

pub fn withdraw_unstaked(staking: AccountId) -> Promise {
    ext_staking::ext(staking)
        .with_static_gas(GAS_UNSTAKE)
        .withdraw_all()
}
