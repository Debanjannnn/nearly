//! Test-only LiNEAR stand-in: 1:1 shares, no unstake delay.
use std::collections::HashMap;

use near_sdk::json_types::U128;
use near_sdk::{env, near, require, AccountId, NearToken, Promise};

#[near(contract_state)]
#[derive(Default)]
pub struct Contract {
    staked: HashMap<AccountId, u128>,
    unstaked: HashMap<AccountId, u128>,
}

#[near]
impl Contract {
    /// Registration is a no-op here; keeps the deposit.
    #[payable]
    #[allow(unused_variables)]
    pub fn storage_deposit(
        &mut self,
        account_id: Option<AccountId>,
        registration_only: Option<bool>,
    ) {
    }

    #[payable]
    pub fn deposit_and_stake(&mut self) {
        let amount = env::attached_deposit().as_yoctonear();
        require!(amount > 0, "zero stake");
        *self
            .staked
            .entry(env::predecessor_account_id())
            .or_default() += amount;
    }

    pub fn unstake(&mut self, amount: U128) {
        let who = env::predecessor_account_id();
        let s = self.staked.entry(who.clone()).or_default();
        require!(*s >= amount.0, "not enough staked");
        *s -= amount.0;
        *self.unstaked.entry(who).or_default() += amount.0;
    }

    pub fn withdraw_all(&mut self) -> Promise {
        let who = env::predecessor_account_id();
        let amount = self.unstaked.remove(&who).unwrap_or(0);
        Promise::new(who).transfer(NearToken::from_yoctonear(amount))
    }

    pub fn get_account_staked_balance(&self, account_id: AccountId) -> U128 {
        U128(self.staked.get(&account_id).copied().unwrap_or(0))
    }

    pub fn get_account_unstaked_balance(&self, account_id: AccountId) -> U128 {
        U128(self.unstaked.get(&account_id).copied().unwrap_or(0))
    }
}
