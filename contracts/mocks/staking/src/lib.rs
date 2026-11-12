//! Test-only LiNEAR stand-in: shares are an NEP-141 token, 1:1 with NEAR, no unstake delay.
use near_contract_standards::fungible_token::core::FungibleTokenCore;
use near_contract_standards::fungible_token::resolver::FungibleTokenResolver;
use near_contract_standards::fungible_token::FungibleToken;
use near_sdk::json_types::U128;
use near_sdk::store::LookupMap;
use near_sdk::{env, near, require, AccountId, NearToken, PanicOnDefault, Promise, PromiseOrValue};

#[near(contract_state)]
#[derive(PanicOnDefault)]
pub struct Contract {
    token: FungibleToken,
    unstaked: LookupMap<AccountId, u128>,
}

#[near]
impl Contract {
    #[init]
    pub fn new() -> Self {
        Self {
            token: FungibleToken::new(b"t"),
            unstaked: LookupMap::new(b"u"),
        }
    }

    /// Registers for free; keeps the deposit.
    #[payable]
    #[allow(unused_variables)]
    pub fn storage_deposit(
        &mut self,
        account_id: Option<AccountId>,
        registration_only: Option<bool>,
    ) {
        let who = account_id.unwrap_or_else(env::predecessor_account_id);
        if !self.token.accounts.contains_key(&who) {
            self.token.internal_register_account(&who);
        }
    }

    #[payable]
    pub fn deposit_and_stake(&mut self) {
        let amount = env::attached_deposit().as_yoctonear();
        require!(amount > 0, "zero stake");
        let who = env::predecessor_account_id();
        if !self.token.accounts.contains_key(&who) {
            self.token.internal_register_account(&who);
        }
        self.token.internal_deposit(&who, amount);
    }

    pub fn unstake(&mut self, amount: U128) {
        let who = env::predecessor_account_id();
        self.token.internal_withdraw(&who, amount.0);
        *self.unstaked.entry(who).or_default() += amount.0;
    }

    pub fn unstake_all(&mut self) {
        let all = self.token.ft_balance_of(env::predecessor_account_id());
        self.unstake(all);
    }

    pub fn withdraw_all(&mut self) -> Promise {
        let who = env::predecessor_account_id();
        let amount = self.unstaked.remove(&who).unwrap_or(0);
        Promise::new(who).transfer(NearToken::from_yoctonear(amount))
    }

    pub fn ft_price(&self) -> U128 {
        U128(10u128.pow(24))
    }

    pub fn get_account_staked_balance(&self, account_id: AccountId) -> U128 {
        self.token.ft_balance_of(account_id)
    }

    pub fn get_account_unstaked_balance(&self, account_id: AccountId) -> U128 {
        U128(self.unstaked.get(&account_id).copied().unwrap_or(0))
    }
}

#[near]
impl FungibleTokenCore for Contract {
    #[payable]
    fn ft_transfer(&mut self, receiver_id: AccountId, amount: U128, memo: Option<String>) {
        self.token.ft_transfer(receiver_id, amount, memo)
    }

    #[payable]
    fn ft_transfer_call(
        &mut self,
        receiver_id: AccountId,
        amount: U128,
        memo: Option<String>,
        msg: String,
    ) -> PromiseOrValue<U128> {
        self.token.ft_transfer_call(receiver_id, amount, memo, msg)
    }

    fn ft_total_supply(&self) -> U128 {
        self.token.ft_total_supply()
    }

    fn ft_balance_of(&self, account_id: AccountId) -> U128 {
        self.token.ft_balance_of(account_id)
    }
}

#[near]
impl FungibleTokenResolver for Contract {
    #[private]
    fn ft_resolve_transfer(
        &mut self,
        sender_id: AccountId,
        receiver_id: AccountId,
        amount: U128,
    ) -> U128 {
        self.token
            .internal_ft_resolve_transfer(&sender_id, receiver_id, amount)
            .0
            .into()
    }
}
