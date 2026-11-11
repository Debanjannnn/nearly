//! Outgoing calls to tokens and venues. Each fn only builds the promise; accounting lives in lib/callbacks.
use near_sdk::json_types::U128;
use near_sdk::serde_json::json;
use near_sdk::{ext_contract, AccountId, Gas, NearToken, Promise};

pub const GAS_FT_TRANSFER: Gas = Gas::from_tgas(10);
pub const GAS_FT_TRANSFER_CALL: Gas = Gas::from_tgas(80);
pub const GAS_VENUE: Gas = Gas::from_tgas(80);
pub const GAS_STORAGE: Gas = Gas::from_tgas(10);
/// NEP-145 registration_only cost for a standard FT.
pub const FT_STORAGE: NearToken = NearToken::from_micronear(1_250);
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
    fn storage_deposit(&mut self, account_id: Option<AccountId>, registration_only: Option<bool>);
}

#[allow(dead_code)]
#[ext_contract(ext_lending)]
trait Lending {
    fn execute(&mut self, actions: Vec<near_sdk::serde_json::Value>);
}

#[allow(dead_code)]
#[ext_contract(ext_staking)]
trait Staking {
    fn deposit_and_stake(&mut self);
    fn unstake(&mut self, amount: U128);
}

pub fn ft_transfer(token: AccountId, to: AccountId, amount: u128) -> Promise {
    ext_ft::ext(token)
        .with_attached_deposit(ONE_YOCTO)
        .with_static_gas(GAS_FT_TRANSFER)
        .ft_transfer(to, U128(amount), None)
}

pub fn register(token: AccountId, account: AccountId) -> Promise {
    ext_ft::ext(token)
        .with_attached_deposit(FT_STORAGE)
        .with_static_gas(GAS_STORAGE)
        .storage_deposit(Some(account), Some(true))
}

fn ft_transfer_call(token: AccountId, to: AccountId, amount: u128, msg: String) -> Promise {
    ext_ft::ext(token)
        .with_attached_deposit(ONE_YOCTO)
        .with_static_gas(GAS_FT_TRANSFER_CALL)
        .ft_transfer_call(to, U128(amount), None, msg)
}

// shortcut: empty msg = plain supply, no collateral; #4 tunes msg + venue registration.
pub fn supply(lending: AccountId, token: AccountId, amount: u128) -> Promise {
    ft_transfer_call(token, lending, amount, String::new())
}

pub fn withdraw_lend(lending: AccountId, token: AccountId, amount: u128) -> Promise {
    let action = json!({"Withdraw": {"token_id": token, "max_amount": U128(amount)}});
    ext_lending::ext(lending)
        .with_attached_deposit(ONE_YOCTO)
        .with_static_gas(GAS_VENUE)
        .execute(vec![action])
}

pub fn swap(
    dex: AccountId,
    pool_id: u64,
    token_in: AccountId,
    token_out: AccountId,
    amount_in: u128,
    min_out: u128,
) -> Promise {
    let msg = json!({"force": 0, "actions": [{
        "pool_id": pool_id,
        "token_in": token_in,
        "token_out": token_out,
        "amount_in": U128(amount_in),
        "min_amount_out": U128(min_out),
    }]});
    ft_transfer_call(token_in, dex, amount_in, msg.to_string())
}

pub fn stake(staking: AccountId, amount: u128) -> Promise {
    ext_staking::ext(staking)
        .with_attached_deposit(NearToken::from_yoctonear(amount))
        .with_static_gas(GAS_VENUE)
        .deposit_and_stake()
}

pub fn unstake(staking: AccountId, amount: u128) -> Promise {
    ext_staking::ext(staking)
        .with_static_gas(GAS_VENUE)
        .unstake(U128(amount))
}
