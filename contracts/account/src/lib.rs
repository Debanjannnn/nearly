//! Per-user account contract. Holds funds; the executor moves them only within the on-chain policy.
use near_sdk::json_types::U128;
use near_sdk::store::IterableMap;
use near_sdk::{near, AccountId, BorshStorageKey, PanicOnDefault, PublicKey};

mod events;
mod executor;
mod ledger;
mod owner;
mod policy;
pub mod step;
mod venues;

#[cfg(test)]
mod tests;

pub use ledger::Slot;
pub use step::Step;

#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq)]
pub struct Venues {
    pub lending: AccountId,
    pub dex: AccountId,
    pub staking: AccountId,
}

#[near(serializers = [json])]
pub struct Policy {
    pub treasury: AccountId,
    pub venues: Venues,
    /// token -> per-step cap; a token is whitelisted iff it has a cap.
    pub caps: Vec<(AccountId, U128)>,
    pub near_cap: U128,
}

#[near]
#[derive(BorshStorageKey)]
enum Key {
    Caps,
    Balances,
    Positions,
}

#[near(contract_state)]
#[derive(PanicOnDefault)]
pub struct Contract {
    owner: AccountId,
    executor: PublicKey,
    revoked: bool,
    treasury: AccountId,
    venues: Venues,
    caps: IterableMap<AccountId, u128>,
    near_cap: u128,
    balances: IterableMap<AccountId, u128>,
    positions: IterableMap<String, u128>,
}

#[near]
impl Contract {
    #[init]
    pub fn new(
        owner: AccountId,
        executor: PublicKey,
        treasury: AccountId,
        venues: Venues,
        tokens: Vec<(AccountId, U128)>,
        near_cap: U128,
    ) -> Self {
        let mut c = Self {
            owner,
            executor,
            revoked: false,
            treasury,
            venues,
            caps: IterableMap::new(Key::Caps),
            near_cap: near_cap.0,
            balances: IterableMap::new(Key::Balances),
            positions: IterableMap::new(Key::Positions),
        };
        for (token, cap) in tokens {
            c.add_token(token, cap.0);
        }
        c.register_venues(None);
        c
    }

    pub fn get_owner(&self) -> &AccountId {
        &self.owner
    }

    pub fn get_executor(&self) -> (&PublicKey, bool) {
        (&self.executor, self.revoked)
    }

    pub fn get_policy(&self) -> Policy {
        Policy {
            treasury: self.treasury.clone(),
            venues: self.venues.clone(),
            caps: self
                .caps
                .iter()
                .map(|(t, c)| (t.clone(), U128(*c)))
                .collect(),
            near_cap: U128(self.near_cap),
        }
    }

    pub fn get_balances(&self) -> Vec<(&AccountId, U128)> {
        self.balances.iter().map(|(t, b)| (t, U128(*b))).collect()
    }

    /// Ledger balance of `token` the owner can `withdraw` now.
    pub fn get_balance(&self, token: AccountId) -> U128 {
        U128(self.balances.get(&token).copied().unwrap_or(0))
    }

    pub fn get_positions(&self) -> Vec<(&String, U128)> {
        self.positions.iter().map(|(k, b)| (k, U128(*b))).collect()
    }
}
