//! Test-only Rhea DEX stand-in: instant swap via ft_transfer_call at a fixed rate, output sent back.
use near_sdk::json_types::U128;
use near_sdk::serde_json::{self, json};
use near_sdk::{
    env, near, require, AccountId, Gas, NearToken, PanicOnDefault, Promise, PromiseOrValue,
};

#[near(serializers = [json])]
pub struct SwapAction {
    pub pool_id: u64,
    pub token_in: AccountId,
    pub token_out: AccountId,
    pub amount_in: Option<U128>,
    pub min_amount_out: U128,
}

/// Unknown fields (force, skip_unwrap_near) are ignored, as on the real venue.
#[near(serializers = [json])]
pub struct SwapMsg {
    pub actions: Vec<SwapAction>,
}

#[near(contract_state)]
#[derive(PanicOnDefault)]
pub struct Contract {
    /// out = in * num / den
    num: u128,
    den: u128,
}

#[near]
impl Contract {
    #[init]
    pub fn new(num: U128, den: U128) -> Self {
        Self {
            num: num.0,
            den: den.0,
        }
    }

    pub fn get_return(&self, amount_in: U128) -> U128 {
        U128(amount_in.0 * self.num / self.den)
    }

    pub fn ft_on_transfer(
        &mut self,
        sender_id: AccountId,
        amount: U128,
        msg: String,
    ) -> PromiseOrValue<U128> {
        let m: SwapMsg = serde_json::from_str(&msg).unwrap_or_else(|_| env::panic_str("bad msg"));
        require!(m.actions.len() == 1, "one action only");
        let a = &m.actions[0];
        require!(
            a.token_in == env::predecessor_account_id(),
            "token_in mismatch"
        );
        let out = self.get_return(amount).0;
        require!(out >= a.min_amount_out.0, "E68: slippage error");
        Promise::new(a.token_out.clone())
            .function_call(
                "ft_transfer",
                json!({"receiver_id": sender_id, "amount": U128(out)})
                    .to_string()
                    .into_bytes(),
                NearToken::from_yoctonear(1),
                Gas::from_tgas(10),
            )
            .detach();
        PromiseOrValue::Value(U128(0))
    }
}
