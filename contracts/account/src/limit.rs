//! Rolling 24h spend limit per token, exact: keeps each executor spend in the window.
use near_sdk::{env, require};

use crate::Contract;

const WINDOW_NS: u64 = 86_400 * 1_000_000_000;
/// Bounds the log; more steps than this per token per 24h are rejected.
pub(crate) const MAX_SPENDS: usize = 64;

impl Contract {
    /// Books `amount` under `key`, or panics if the last 24h would exceed `limit`.
    pub(crate) fn book_spend(&mut self, key: String, amount: u128, limit: u128) {
        let now = env::block_timestamp();
        let mut log = self.spent.get(&key).cloned().unwrap_or_default();
        log.retain(|&(t, _)| now.saturating_sub(t) < WINDOW_NS);
        let used: u128 = log.iter().map(|&(_, a)| a).sum();
        require!(used.saturating_add(amount) <= limit, "over daily cap");
        require!(log.len() < MAX_SPENDS, "too many steps in 24h");
        log.push((now, amount));
        self.spent.insert(key, log);
    }
}
