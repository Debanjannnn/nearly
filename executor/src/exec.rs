//! Runs a step plan on one user account: recheck all, then sign + land each step in order.
use base64::{engine::general_purpose::STANDARD as B64, Engine};
use serde::Serialize;
use serde_json::{json, Value};
use std::time::Duration;

use crate::{
    config::Deployments,
    nonce::Nonces,
    policy,
    rpc::{Error, Rpc},
    signer::{Call, Signer},
    steps::Step,
};

const GAS: u64 = 300_000_000_000_000;
const ATTEMPTS: u32 = 4;

pub struct Executor {
    pub rpc: Rpc,
    pub signer: Signer,
    pub dep: Deployments,
    pub max_step_usd: u64,
    pub price_pool: u64,
    pub nonces: Nonces,
    /// time between retries; tests shrink it.
    pub backoff: Duration,
}

#[derive(Debug, Clone, PartialEq, Serialize)]
pub struct StepResult {
    pub index: usize,
    pub status: &'static str,
    pub tx_hash: Option<String>,
    pub error: Option<String>,
}

enum Fail {
    /// node refused before inclusion: safe to sign again.
    Rejected(Error),
    /// outcome unknown or final failure: never re-sign.
    Final(Option<String>, String),
}

impl Executor {
    /// Allowlist + cap for every step; nothing is signed unless all pass.
    pub async fn recheck(&self, account: &str, steps: &[Step]) -> Result<(), String> {
        policy::check_account(&self.dep, account)?;
        let price = match steps.iter().any(|s| policy::needs_price(&self.dep, s)) {
            true => Some(self.near_price().await?),
            false => None,
        };
        policy::check(&self.dep, steps, self.max_step_usd, price)
    }

    /// USDC atoms for 1 NEAR from the Rhea pool.
    async fn near_price(&self) -> Result<u128, String> {
        let t = &self.dep.tokens;
        let args = json!({"pool_id": self.price_pool, "token_in": t.wnear, "amount_in": "1000000000000000000000000", "token_out": t.usdc});
        let v = self
            .retry(|| {
                self.rpc
                    .view(&self.dep.venues.dex, "get_return", args.clone())
            })
            .await
            .map_err(|e| format!("price: {e}"))?;
        v.as_str()
            .and_then(|s| s.parse().ok())
            .ok_or_else(|| format!("price: bad get_return {v}"))
    }

    /// Stops at the first failed step; later steps report "skipped".
    pub async fn run(&self, account: &str, steps: &[Step]) -> Vec<StepResult> {
        let mut nonce = self.nonces.lease(account).await;
        let mut out = Vec::with_capacity(steps.len());
        let mut failed = false;
        for (index, step) in steps.iter().enumerate() {
            let r = if failed {
                Err((None, "skipped".into()))
            } else {
                self.step(account, step, &mut nonce).await
            };
            failed |= r.is_err();
            out.push(match r {
                Ok(h) => StepResult {
                    index,
                    status: "ok",
                    tx_hash: Some(h),
                    error: None,
                },
                Err((h, e)) => {
                    if e != "skipped" {
                        tracing::warn!(account, index, error = %e, "step failed");
                    }
                    StepResult {
                        index,
                        status: "failed",
                        tx_hash: h,
                        error: Some(e),
                    }
                }
            });
        }
        out
    }

    async fn step(
        &self,
        account: &str,
        step: &Step,
        nonce: &mut Option<u64>,
    ) -> Result<String, (Option<String>, String)> {
        let args = json!({ "step": step }).to_string();
        let pk = self.signer.public_key();
        for _ in 0..ATTEMPTS {
            let n = match *nonce {
                Some(n) => n,
                None => self
                    .retry(|| self.rpc.access_key_nonce(account, &pk))
                    .await
                    .map_err(|e| (None, e.to_string()))?,
            };
            let block_hash = self
                .retry(|| self.rpc.final_block_hash())
                .await
                .map_err(|e| (None, e.to_string()))?;
            let call = Call {
                account,
                nonce: n + 1,
                block_hash,
                method: "execute_step",
                args: args.as_bytes(),
                gas: GAS,
            };
            let signed = self.signer.sign(&call);
            *nonce = Some(n + 1);
            match self.land(account, &signed.hash, &signed.bytes).await {
                Ok(v) => return outcome(&signed.hash, &v),
                Err(Fail::Rejected(e)) if e.has("InvalidNonce") => *nonce = None,
                Err(Fail::Rejected(e)) if e.has("Expired") => *nonce = Some(n),
                Err(Fail::Rejected(e)) => {
                    *nonce = None;
                    return Err((None, e.to_string()));
                }
                Err(Fail::Final(h, e)) => {
                    *nonce = None;
                    return Err((h, e));
                }
            }
        }
        Err((None, "nonce retries exhausted".into()))
    }

    /// Send once; on an unclear answer poll status and resend the same bytes (same nonce, no double spend).
    async fn land(&self, account: &str, hash: &str, bytes: &[u8]) -> Result<Value, Fail> {
        let mut last = String::new();
        for attempt in 0..ATTEMPTS {
            if attempt > 0 {
                tokio::time::sleep(self.backoff).await;
                if let Ok(v) = self.rpc.tx_status(hash, account).await {
                    return Ok(v);
                }
            }
            match self.rpc.send_tx(bytes).await {
                Ok(v) => return Ok(v),
                Err(e) if matches!(e, Error::Transport(_)) || e.has("TIMEOUT_ERROR") => {
                    last = e.to_string()
                }
                Err(e) if attempt == 0 && e.has("INVALID_TRANSACTION") => {
                    return Err(Fail::Rejected(e))
                }
                // a resend may hit our own landed nonce: keep polling, never re-sign
                Err(e) if e.has("InvalidNonce") => last = e.to_string(),
                Err(e) => return Err(Fail::Final(Some(hash.into()), e.to_string())),
            }
        }
        Err(Fail::Final(
            Some(hash.into()),
            format!("outcome unknown: {last}"),
        ))
    }

    async fn retry<T, F, Fut>(&self, f: F) -> Result<T, Error>
    where
        F: Fn() -> Fut,
        Fut: std::future::Future<Output = Result<T, Error>>,
    {
        let mut r = f().await;
        for _ in 1..ATTEMPTS {
            if !matches!(r, Err(Error::Transport(_))) {
                break;
            }
            tokio::time::sleep(self.backoff).await;
            r = f().await;
        }
        r
    }
}

/// Final outcome -> ok or the failure the chain reported.
/// `on_settle` returns the amount the venue took: "0" = venue failed, funds refunded.
fn outcome(hash: &str, v: &Value) -> Result<String, (Option<String>, String)> {
    let used = v["status"]["SuccessValue"]
        .as_str()
        .and_then(|b| B64.decode(b).ok());
    if used.as_deref() == Some(b"\"0\"") {
        return Err((Some(hash.into()), "venue failed, refunded".into()));
    }
    match v["status"].get("Failure") {
        None if v["status"].is_object() => Ok(hash.into()),
        None => Err((
            Some(hash.into()),
            format!("no final status: {}", v["final_execution_status"]),
        )),
        Some(f) => Err((Some(hash.into()), f.to_string())),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn outcome_status() {
        assert!(outcome("h", &json!({"status": {"SuccessValue": ""}})).is_ok());
        assert!(outcome(
            "h",
            &json!({"status": {"SuccessValue": B64.encode("\"5\"")}})
        )
        .is_ok());
        let e = outcome(
            "h",
            &json!({"status": {"SuccessValue": B64.encode("\"0\"")}}),
        );
        assert_eq!(e.unwrap_err().1, "venue failed, refunded");
        let e = outcome(
            "h",
            &json!({"status": {"Failure": {"ActionError": {"kind": "x"}}}}),
        )
        .unwrap_err();
        assert_eq!(e.0.as_deref(), Some("h"));
        assert!(e.1.contains("ActionError"));
        assert!(outcome("h", &json!({"final_execution_status": "NONE"})).is_err());
    }
}
