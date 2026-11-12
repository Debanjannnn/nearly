//! Owner-only methods. Each needs 1 yocto so it must be signed with a full-access key.
use near_sdk::json_types::U128;
use near_sdk::{assert_one_yocto, env, near, require, AccountId, Gas, Promise};

use crate::events::Event;
use crate::venues;
use crate::{Contract, ContractExt, Venues};

const GAS_WITHDRAW_CB: Gas = Gas::from_tgas(5);

impl Contract {
    /// Whitelists `token`; registers this account on it the first time.
    pub(crate) fn add_token(&mut self, token: AccountId, cap: u128) {
        if self.caps.insert(token.clone(), cap).is_none() {
            venues::register(token, env::current_account_id(), venues::FT_STORAGE).detach();
        }
    }

    /// Lending needs storage before the first supply; LiNEAR is also an FT we hold.
    pub(crate) fn register_venues(&self, old: Option<&Venues>) {
        let me = env::current_account_id();
        let v = &self.venues;
        if old.is_none_or(|o| o.lending != v.lending) {
            venues::register(v.lending.clone(), me.clone(), venues::LENDING_STORAGE).detach();
        }
        if old.is_none_or(|o| o.staking != v.staking) {
            venues::register(v.staking.clone(), me, venues::FT_STORAGE).detach();
        }
    }

    fn assert_owner_call(&self) {
        assert_one_yocto();
        self.assert_owner();
    }
}

#[near]
impl Contract {
    /// Always pays out to `owner`; there is no receiver argument. The tx fails if the transfer
    /// failed (ledger refunded by `on_settle` first), so callers can trust a success.
    #[payable]
    pub fn withdraw(&mut self, token: AccountId, amount: U128) -> Promise {
        self.assert_owner_call();
        require!(amount.0 > 0, "zero amount");
        let owner = self.owner.clone();
        Event::Withdraw {
            token: token.clone(),
            receiver: owner.clone(),
            amount,
        }
        .emit();
        self.send(token, owner, amount.0).then(
            Self::ext(env::current_account_id())
                .with_static_gas(GAS_WITHDRAW_CB)
                .on_withdraw(amount),
        )
    }

    /// Runs after `on_settle`, so panicking here keeps its refund.
    #[private]
    pub fn on_withdraw(&self, amount: U128) -> U128 {
        let sent = env::promise_result_checked(0, 64)
            .ok()
            .and_then(|b| near_sdk::serde_json::from_slice::<U128>(&b).ok());
        require!(sent == Some(amount), "transfer failed, refunded to ledger");
        amount
    }

    #[payable]
    pub fn set_venues(&mut self, venues: Venues) {
        self.assert_owner_call();
        let old = std::mem::replace(&mut self.venues, venues);
        self.register_venues(Some(&old));
    }

    /// `cap = None` removes the token from the whitelist.
    #[payable]
    pub fn set_token_cap(&mut self, token: AccountId, cap: Option<U128>) {
        self.assert_owner_call();
        match cap {
            Some(c) => self.add_token(token, c.0),
            None => drop(self.caps.remove(&token)),
        }
    }

    #[payable]
    pub fn set_near_cap(&mut self, cap: U128) {
        self.assert_owner_call();
        self.near_cap = cap.0;
    }

    /// Limit per token in any 24h = `steps` × per-step cap.
    #[payable]
    pub fn set_daily_steps(&mut self, steps: u32) {
        self.assert_owner_call();
        require!(steps > 0, "zero steps");
        self.daily_steps = steps;
    }

    /// Replaces the Rhea pools swaps may use.
    #[payable]
    pub fn set_pools(&mut self, pools: Vec<u64>) {
        self.assert_owner_call();
        self.pools = pools;
    }

    /// Deletes the executor key; every later executor call fails.
    #[payable]
    pub fn revoke_executor(&mut self) -> Promise {
        self.assert_owner_call();
        require!(!self.revoked, "already revoked");
        self.revoked = true;
        Event::Revoke {
            public_key: self.executor.clone(),
        }
        .emit();
        Promise::new(env::current_account_id()).delete_key(self.executor.clone())
    }
}
