use std::{collections::HashMap, sync::Arc};
use tokio::sync::{Mutex, OwnedMutexGuard};

/// Last nonce per access key (one executor key, so keyed by account); `None` = refetch.
// shortcut: in-process only; replicas need a shared lock (pg advisory) before scaling out
#[derive(Default)]
pub struct Nonces(Mutex<HashMap<String, Arc<Mutex<Option<u64>>>>>);

impl Nonces {
    pub async fn lease(&self, account: &str) -> OwnedMutexGuard<Option<u64>> {
        let slot = self
            .0
            .lock()
            .await
            .entry(account.to_string())
            .or_default()
            .clone();
        slot.lock_owned().await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;

    #[tokio::test]
    async fn same_key_serializes_and_keeps_nonce() {
        let n = Nonces::default();
        let mut a = n.lease("a").await;
        *a = Some(7);
        assert!(
            tokio::time::timeout(Duration::from_millis(30), n.lease("a"))
                .await
                .is_err()
        );
        assert!(
            tokio::time::timeout(Duration::from_millis(30), n.lease("b"))
                .await
                .is_ok()
        );
        drop(a);
        assert_eq!(*n.lease("a").await, Some(7));
    }
}
