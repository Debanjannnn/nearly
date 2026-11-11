//! Factory: creates `<owner>.<factory>`, deploys the account contract, adds the executor key.
use near_sdk::json_types::U128;
use near_sdk::serde_json::json;
use near_sdk::store::LookupMap;
use near_sdk::{
    env, near, require, AccountId, Allowance, Gas, NearToken, PanicOnDefault, Promise, PublicKey,
};

mod name;

/// No upgrade path: accounts get no full-access key, so their code is fixed at creation.
/// Migrating = owner withdraws, then a new account is created from a newer factory.
const ACCOUNT_WASM: &[u8] = include_bytes!("../../res/nearly_account.wasm");
/// Gas the executor key may burn before it must be re-added.
const EXECUTOR_ALLOWANCE: NearToken = NearToken::from_near(1);
/// State, FT registrations and gas on top of the code storage.
const ACCOUNT_BUFFER: NearToken = NearToken::from_near(1);
const STATE_BYTES: u128 = 2_000;
const GAS_INIT: Gas = Gas::from_tgas(50);
const GAS_CB: Gas = Gas::from_tgas(10);

#[near(serializers = [borsh, json])]
#[derive(Clone)]
pub struct Venues {
    pub lending: AccountId,
    pub dex: AccountId,
    pub staking: AccountId,
}

/// Policy every new account starts with; the owner can edit it on the account later.
#[near(serializers = [borsh, json])]
#[derive(Clone)]
pub struct AccountConfig {
    pub treasury: AccountId,
    pub venues: Venues,
    pub tokens: Vec<(AccountId, U128)>,
    pub near_cap: U128,
}

#[near(contract_state)]
#[derive(PanicOnDefault)]
pub struct Factory {
    owner: AccountId,
    config: AccountConfig,
    /// owner -> created account
    accounts: LookupMap<AccountId, AccountId>,
}

#[near]
impl Factory {
    #[init]
    pub fn new(owner: AccountId, config: AccountConfig) -> Self {
        Self {
            owner,
            config,
            accounts: LookupMap::new(b"a"),
        }
    }

    pub fn set_config(&mut self, config: AccountConfig) {
        require!(env::predecessor_account_id() == self.owner, "only owner");
        self.config = config;
    }

    /// Callable by `owner` itself or the factory owner (backend). Attach `required_deposit`; excess is refunded.
    #[payable]
    pub fn create_account(&mut self, owner: AccountId, executor_pubkey: PublicKey) -> Promise {
        let payer = env::predecessor_account_id();
        require!(
            payer == owner || payer == self.owner,
            "only owner or factory owner"
        );
        require!(!self.accounts.contains_key(&owner), "account exists");
        let need = required();
        let deposit = env::attached_deposit();
        require!(deposit >= need, format!("attach at least {need}"));

        let id = name::sub_account(&owner, &env::current_account_id());
        self.accounts.insert(owner.clone(), id.clone());
        let c = &self.config;
        let args = json!({
            "owner": owner,
            "executor": executor_pubkey,
            "treasury": c.treasury,
            "venues": c.venues,
            "tokens": c.tokens,
            "near_cap": c.near_cap,
        });
        if let Some(excess) = deposit.checked_sub(need).filter(|e| !e.is_zero()) {
            Promise::new(payer.clone()).transfer(excess).detach();
        }
        Promise::new(id.clone())
            .create_account()
            .transfer(need)
            .deploy_contract(ACCOUNT_WASM)
            .function_call(
                "new",
                args.to_string().into_bytes(),
                NearToken::from_yoctonear(0),
                GAS_INIT,
            )
            .add_access_key_allowance(
                executor_pubkey,
                Allowance::limited(EXECUTOR_ALLOWANCE).unwrap(),
                id,
                "execute_step",
            )
            .then(
                Self::ext(env::current_account_id())
                    .with_static_gas(GAS_CB)
                    .on_create(owner, payer, need),
            )
    }

    /// Batch is atomic: on failure nothing exists, so free the name and refund the payer.
    #[private]
    pub fn on_create(&mut self, owner: AccountId, payer: AccountId, amount: NearToken) -> bool {
        let ok = env::promise_result_checked(0, 0).is_ok();
        if !ok {
            self.accounts.remove(&owner);
            Promise::new(payer).transfer(amount).detach();
        }
        ok
    }

    pub fn required_deposit(&self) -> NearToken {
        required()
    }

    pub fn get_account(&self, owner: AccountId) -> Option<&AccountId> {
        self.accounts.get(&owner)
    }

    pub fn get_config(&self) -> &AccountConfig {
        &self.config
    }
}

fn required() -> NearToken {
    let bytes = ACCOUNT_WASM.len() as u128 + STATE_BYTES;
    env::storage_byte_cost()
        .saturating_mul(bytes)
        .saturating_add(ACCOUNT_BUFFER)
}

#[cfg(test)]
mod tests;
