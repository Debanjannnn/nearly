//! Test-only Rhea lending stand-in: same supply/withdraw/storage surface, no interest, no oracle.
use std::collections::{HashMap, HashSet};

use near_sdk::json_types::U128;
use near_sdk::{
    assert_one_yocto, env, near, require, AccountId, Gas, NearToken, Promise, PromiseOrValue,
};

const MIN_STORAGE: NearToken = NearToken::from_millinear(100);

#[near(serializers = [json])]
pub struct AssetAmount {
    pub token_id: AccountId,
    pub amount: Option<U128>,
    pub max_amount: Option<U128>,
}

#[near(serializers = [json])]
pub enum Action {
    Withdraw(AssetAmount),
}

#[near(contract_state)]
#[derive(Default)]
pub struct Contract {
    registered: HashSet<AccountId>,
    /// (account, token) -> supplied
    supplied: HashMap<(AccountId, AccountId), u128>,
}

#[near]
impl Contract {
    #[payable]
    pub fn storage_deposit(
        &mut self,
        account_id: Option<AccountId>,
        #[allow(unused_variables)] registration_only: Option<bool>,
    ) {
        require!(
            env::attached_deposit() >= MIN_STORAGE,
            "storage deposit too low"
        );
        self.registered
            .insert(account_id.unwrap_or_else(env::predecessor_account_id));
    }

    /// Empty msg = plain supply, like the real venue.
    pub fn ft_on_transfer(
        &mut self,
        sender_id: AccountId,
        amount: U128,
        msg: String,
    ) -> PromiseOrValue<U128> {
        require!(
            self.registered.contains(&sender_id),
            "account not registered"
        );
        require!(msg.is_empty(), "only plain supply");
        *self
            .supplied
            .entry((sender_id, env::predecessor_account_id()))
            .or_default() += amount.0;
        PromiseOrValue::Value(U128(0))
    }

    #[payable]
    pub fn execute(&mut self, actions: Vec<Action>) {
        assert_one_yocto();
        let who = env::predecessor_account_id();
        for Action::Withdraw(a) in actions {
            let bal = self
                .supplied
                .entry((who.clone(), a.token_id.clone()))
                .or_default();
            let amount = match (a.amount, a.max_amount) {
                (Some(x), _) => x.0,
                (None, Some(m)) => m.0.min(*bal),
                (None, None) => *bal,
            };
            require!(amount <= *bal, "not enough supplied");
            *bal -= amount;
            Promise::new(a.token_id)
                .function_call(
                    "ft_transfer",
                    near_sdk::serde_json::json!({"receiver_id": who, "amount": U128(amount)})
                        .to_string()
                        .into_bytes(),
                    NearToken::from_yoctonear(1),
                    Gas::from_tgas(10),
                )
                .detach();
        }
    }

    pub fn get_supplied(&self, account_id: AccountId, token_id: AccountId) -> U128 {
        U128(
            self.supplied
                .get(&(account_id, token_id))
                .copied()
                .unwrap_or(0),
        )
    }
}
