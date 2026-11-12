//! One-off state upgrade: config gained `pools`.
use near_sdk::borsh::BorshDeserialize;
#[cfg(test)]
use near_sdk::borsh::BorshSerialize;
use near_sdk::json_types::U128;
use near_sdk::store::LookupMap;
use near_sdk::{env, near, AccountId};

use crate::{AccountConfig, Factory, FactoryExt, Venues};

#[derive(BorshDeserialize)]
#[cfg_attr(test, derive(BorshSerialize))]
#[borsh(crate = "near_sdk::borsh")]
pub(crate) struct OldConfig {
    pub(crate) treasury: AccountId,
    pub(crate) venues: Venues,
    pub(crate) tokens: Vec<(AccountId, U128)>,
    pub(crate) near_cap: U128,
}

#[derive(BorshDeserialize)]
#[cfg_attr(test, derive(BorshSerialize))]
#[borsh(crate = "near_sdk::borsh")]
pub(crate) struct OldFactory {
    pub(crate) owner: AccountId,
    pub(crate) config: OldConfig,
    pub(crate) accounts: LookupMap<AccountId, AccountId>,
}

#[near]
impl Factory {
    #[private]
    #[init(ignore_state)]
    pub fn migrate(pools: Vec<u64>) -> Self {
        let old: OldFactory = env::state_read().unwrap_or_else(|| env::panic_str("no state"));
        let c = old.config;
        Self {
            owner: old.owner,
            config: AccountConfig {
                treasury: c.treasury,
                venues: c.venues,
                tokens: c.tokens,
                near_cap: c.near_cap,
                pools,
            },
            accounts: old.accounts,
        }
    }
}
