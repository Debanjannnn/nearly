use near_sdk::json_types::U128;
use near_sdk::{near, AccountId, PublicKey};

use crate::step::Step;

/// NEP-297 events, logged as `EVENT_JSON:{"standard":"nearly","version":"1.0.0",...}`.
#[near(event_json(standard = "nearly"))]
pub enum Event {
    #[event_version("1.0.0")]
    Deposit {
        token: AccountId,
        sender: AccountId,
        amount: U128,
    },
    #[event_version("1.0.0")]
    Step { step: Step },
    #[event_version("1.0.0")]
    Withdraw {
        token: AccountId,
        receiver: AccountId,
        amount: U128,
    },
    #[event_version("1.0.0")]
    Revoke { public_key: PublicKey },
}
