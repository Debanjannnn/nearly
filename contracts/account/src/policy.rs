//! On-chain policy: who may call what, and how much a single step may move.
use near_sdk::{env, require, AccountId};

use crate::step::Step;
use crate::Contract;

impl Contract {
    pub(crate) fn assert_owner(&self) {
        require!(env::predecessor_account_id() == self.owner, "only owner");
    }

    /// Executor txs are signed by the account's own function-call key, so predecessor == self.
    pub(crate) fn assert_executor(&self) {
        require!(
            env::predecessor_account_id() == env::current_account_id(),
            "only self"
        );
        require!(!self.revoked, "executor revoked");
        require!(
            env::signer_account_pk() == self.executor,
            "not executor key"
        );
    }

    /// `None` token = native NEAR (stake/unstake). Also books the amount against the daily limit.
    pub(crate) fn assert_cap(&mut self, token: Option<&AccountId>, amount: u128) {
        require!(amount > 0, "zero amount");
        let cap = match token {
            Some(t) => *self
                .caps
                .get(t)
                .unwrap_or_else(|| env::panic_str("token not whitelisted")),
            None => self.near_cap,
        };
        require!(amount <= cap, "over cap");
        let key = token.map_or("near".to_string(), |t| t.to_string());
        self.book_spend(key, amount, cap.saturating_mul(self.daily_steps.into()));
    }

    pub(crate) fn check_step(&mut self, step: &Step) {
        match step {
            Step::Supply { token, amount }
            | Step::WithdrawLend { token, amount }
            | Step::Fee { token, amount }
            | Step::ToOwner { token, amount } => self.assert_cap(Some(token), amount.0),
            Step::Swap {
                pool_id,
                token_in,
                token_out,
                amount_in,
                min_out,
            } => {
                require!(token_in != token_out, "same token");
                require!(self.pools.contains(pool_id), "pool not whitelisted");
                require!(min_out.0 > 0, "zero min_out");
                require!(self.caps.contains_key(token_out), "token not whitelisted");
                // LST exit spends the staking position (NEAR value), capped like stake
                let t = Some(token_in).filter(|t| **t != self.venues.staking);
                self.assert_cap(t, amount_in.0);
            }
            Step::Stake { token, amount } => {
                require!(self.caps.contains_key(token), "token not whitelisted");
                self.assert_cap(None, amount.0)
            }
            Step::Unstake { amount } => self.assert_cap(None, amount.0),
        }
    }
}
