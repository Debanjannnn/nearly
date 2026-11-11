use serde::Deserialize;
use std::env;

#[derive(Debug, Clone, Deserialize)]
pub struct Tokens {
    pub usdc: String,
    pub wnear: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct Venues {
    pub lending: String,
    pub dex: String,
    pub staking: String,
}

/// contracts/deployments/<network>.json: the only source of contract IDs.
#[derive(Debug, Clone, Deserialize)]
pub struct Deployments {
    pub root: String,
    pub factory: String,
    pub treasury: String,
    pub tokens: Tokens,
    pub venues: Venues,
}

impl Deployments {
    pub fn load(path: &str) -> Result<Self, String> {
        let raw = std::fs::read_to_string(path).map_err(|e| format!("{path}: {e}"))?;
        serde_json::from_str(&raw).map_err(|e| format!("{path}: {e}"))
    }
}

pub struct Config {
    pub addr: String,
    pub network: String,
    pub rpc_url: String,
    pub executor_account_id: String,
    pub executor_private_key: String,
    pub internal_secret: String,
    pub max_step_usd: u64,
    /// Rhea USDC/wNEAR pool used to price NEAR amounts against MAX_STEP_USD.
    pub price_pool: u64,
    pub deployments: Deployments,
}

pub fn opt(key: &str) -> Option<String> {
    env::var(key)
        .ok()
        .map(|v| v.trim().to_string())
        .filter(|v| !v.is_empty())
}

pub fn require(key: &str) -> Result<String, String> {
    opt(key).ok_or_else(|| format!("missing env {key}"))
}

fn num<T: std::str::FromStr>(key: &str) -> Result<Option<T>, String> {
    opt(key)
        .map(|v| v.parse().map_err(|_| format!("{key} is not a number")))
        .transpose()
}

impl Config {
    pub fn from_env() -> Result<Self, String> {
        let network = opt("NEAR_NETWORK").unwrap_or_else(|| "testnet".into());
        let default_pool = match network.as_str() {
            "testnet" => 1900,
            "mainnet" => 4512,
            n => return Err(format!("NEAR_NETWORK {n}: want testnet or mainnet")),
        };
        let dir = opt("DEPLOYMENTS_DIR").unwrap_or_else(|| "../contracts/deployments".into());
        Ok(Self {
            addr: opt("EXECUTOR_ADDR").unwrap_or_else(|| "0.0.0.0:8083".into()),
            rpc_url: opt("RPC_URL")
                .unwrap_or_else(|| format!("https://rpc.{network}.near.org")),
            executor_account_id: require("EXECUTOR_ACCOUNT_ID")?,
            executor_private_key: require("EXECUTOR_PRIVATE_KEY")?,
            internal_secret: require("INTERNAL_SECRET")?,
            max_step_usd: num("MAX_STEP_USD")?.ok_or("missing env MAX_STEP_USD")?,
            price_pool: num("PRICE_POOL_ID")?.unwrap_or(default_pool),
            deployments: Deployments::load(&format!("{dir}/{network}.json"))?,
            network,
        })
    }
}
