use near_sdk::json_types::U128;
use near_sdk::{near, AccountId};

/// One executor instruction. No variant carries a receiver: funds go only to venues, treasury or owner.
#[near(serializers = [json])]
#[serde(tag = "kind", rename_all = "snake_case", deny_unknown_fields)]
#[derive(Clone, Debug, PartialEq)]
pub enum Step {
    Supply {
        token: AccountId,
        amount: U128,
    },
    WithdrawLend {
        token: AccountId,
        amount: U128,
    },
    Swap {
        pool_id: u64,
        token_in: AccountId,
        token_out: AccountId,
        amount_in: U128,
        min_out: U128,
    },
    /// `token` = wNEAR; contract unwraps it, then stakes the NEAR.
    Stake {
        token: AccountId,
        amount: U128,
    },
    Unstake {
        amount: U128,
    },
    Fee {
        token: AccountId,
        amount: U128,
    },
    ToOwner {
        token: AccountId,
        amount: U128,
    },
}
