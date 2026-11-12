//! NEAR JSON-RPC: the few methods the executor needs.
use base64::{engine::general_purpose::STANDARD as B64, Engine};
use serde_json::{json, Value};
use std::time::Duration;

pub struct Rpc {
    http: reqwest::Client,
    url: String,
}

#[derive(Debug)]
pub enum Error {
    /// no answer (network, 5xx, bad body): the request may or may not have landed.
    Transport(String),
    /// the node's `error` object.
    Node(Value),
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
        match self {
            Error::Transport(e) => write!(f, "rpc transport: {e}"),
            Error::Node(e) => write!(f, "rpc: {e}"),
        }
    }
}

impl Error {
    /// error name anywhere in the node's error (`TIMEOUT_ERROR`, `InvalidNonce`, ...).
    pub fn has(&self, name: &str) -> bool {
        matches!(self, Error::Node(v) if v.to_string().contains(name))
    }
}

impl Rpc {
    pub fn new(url: String) -> Self {
        // send_tx FINAL can hold the request ~10s before the node times out
        let http = reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
            .build()
            .expect("reqwest client");
        Self { http, url }
    }

    pub async fn call(&self, method: &str, params: Value) -> Result<Value, Error> {
        let body = json!({"jsonrpc": "2.0", "id": "nearly", "method": method, "params": params});
        let res = self
            .http
            .post(&self.url)
            .json(&body)
            .send()
            .await
            .map_err(|e| Error::Transport(e.to_string()))?;
        let status = res.status();
        let v: Value = res
            .json()
            .await
            .map_err(|e| Error::Transport(format!("{status}: {e}")))?;
        if let Some(e) = v.get("error") {
            return Err(Error::Node(e.clone()));
        }
        v.get("result")
            .cloned()
            .ok_or_else(|| Error::Transport(format!("{status}: no result")))
    }

    pub async fn access_key_nonce(&self, account: &str, public_key: &str) -> Result<u64, Error> {
        let p = json!({"request_type": "view_access_key", "finality": "final", "account_id": account, "public_key": public_key});
        let v = self.call("query", p).await?;
        v["nonce"].as_u64().ok_or(Error::Node(v))
    }

    pub async fn final_block_hash(&self) -> Result<[u8; 32], Error> {
        let v = self.call("block", json!({"finality": "final"})).await?;
        let h = v["header"]["hash"]
            .as_str()
            .and_then(|h| bs58::decode(h).into_vec().ok());
        h.and_then(|b| b.try_into().ok()).ok_or(Error::Node(v))
    }

    /// view call with JSON args and a JSON result.
    pub async fn view(&self, contract: &str, method: &str, args: Value) -> Result<Value, Error> {
        let p = json!({"request_type": "call_function", "finality": "final", "account_id": contract, "method_name": method, "args_base64": B64.encode(args.to_string())});
        let v = self.call("query", p).await?;
        let bytes: Vec<u8> =
            serde_json::from_value(v["result"].clone()).map_err(|_| Error::Node(v.clone()))?;
        serde_json::from_slice(&bytes).map_err(|_| Error::Node(v))
    }

    pub async fn send_tx(&self, signed: &[u8]) -> Result<Value, Error> {
        self.call(
            "send_tx",
            json!({"signed_tx_base64": B64.encode(signed), "wait_until": "FINAL"}),
        )
        .await
    }

    pub async fn tx_status(&self, hash: &str, sender: &str) -> Result<Value, Error> {
        self.call(
            "tx",
            json!({"tx_hash": hash, "sender_account_id": sender, "wait_until": "FINAL"}),
        )
        .await
    }
}
