//! Step JSON shared with the planner and the account contract's `execute_step`.
use serde::{Deserialize, Deserializer, Serialize, Serializer};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case", deny_unknown_fields)]
pub enum Step {
    Supply {
        token: String,
        #[serde(with = "u128_str")]
        amount: u128,
    },
    WithdrawLend {
        token: String,
        #[serde(with = "u128_str")]
        amount: u128,
    },
    Swap {
        pool_id: u64,
        token_in: String,
        token_out: String,
        #[serde(with = "u128_str")]
        amount_in: u128,
        #[serde(with = "u128_str")]
        min_out: u128,
    },
    /// `token` = wNEAR the contract unwraps first.
    Stake {
        token: String,
        #[serde(with = "u128_str")]
        amount: u128,
    },
    Unstake {
        #[serde(with = "u128_str")]
        amount: u128,
    },
    Fee {
        token: String,
        #[serde(with = "u128_str")]
        amount: u128,
    },
    ToOwner {
        token: String,
        #[serde(with = "u128_str")]
        amount: u128,
    },
}

impl Step {
    /// Token moved and how much; `None` token = native NEAR (staking).
    pub fn spend(&self) -> (Option<&str>, u128) {
        match self {
            Step::Supply { token, amount }
            | Step::WithdrawLend { token, amount }
            | Step::Fee { token, amount }
            | Step::ToOwner { token, amount } => (Some(token), *amount),
            Step::Swap {
                token_in,
                amount_in,
                ..
            } => (Some(token_in), *amount_in),
            // wNEAR is priced as NEAR either way
            Step::Stake { amount, .. } | Step::Unstake { amount } => (None, *amount),
        }
    }
}

/// u128 as a decimal string, like near-sdk `U128`.
mod u128_str {
    use super::*;

    pub fn serialize<S: Serializer>(v: &u128, s: S) -> Result<S::Ok, S::Error> {
        s.serialize_str(&v.to_string())
    }

    pub fn deserialize<'de, D: Deserializer<'de>>(d: D) -> Result<u128, D::Error> {
        let s = String::deserialize(d)?;
        if s.is_empty() || !s.bytes().all(|b| b.is_ascii_digit()) {
            return Err(serde::de::Error::custom(
                "amount is not a u128 decimal string",
            ));
        }
        s.parse().map_err(serde::de::Error::custom)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn wire_shape() {
        let s: Step = serde_json::from_value(json!({"kind":"swap","pool_id":0,"token_in":"a","token_out":"b","amount_in":"340282366920938463463374607431768211455","min_out":"1"})).unwrap();
        assert_eq!(s.spend(), (Some("a"), u128::MAX));
        assert_eq!(
            serde_json::to_value(Step::Stake {
                token: "w".into(),
                amount: 5
            })
            .unwrap(),
            json!({"kind":"stake","token":"w","amount":"5"})
        );
        for bad in [
            json!({"kind":"stake","amount":5}),
            json!({"kind":"stake","amount":"-1"}),
            json!({"kind":"stake","amount":"+1"}),
            json!({"kind":"stake","amount":"340282366920938463463374607431768211456"}),
            json!({"kind":"stake","amount":"1"}),
            json!({"kind":"unstake","amount":"1","token":"x"}),
            json!({"kind":"transfer","token":"x","amount":"1"}),
        ] {
            assert!(
                serde_json::from_value::<Step>(bad.clone()).is_err(),
                "{bad}"
            );
        }
    }
}
