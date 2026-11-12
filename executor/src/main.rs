use nearly_executor::{config::Config, exec::Executor, http, rpc::Rpc, signer::Signer};
use std::{sync::Arc, time::Duration};

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_env("LOG_LEVEL")
                .unwrap_or_else(|_| "info".into()),
        )
        .init();
    if let Err(e) = run().await {
        eprintln!("fatal: {e}");
        std::process::exit(1);
    }
}

async fn run() -> Result<(), String> {
    let cfg = Config::from_env()?;
    let signer = Signer::parse(&cfg.executor_private_key)
        .map_err(|e| format!("EXECUTOR_PRIVATE_KEY: {e}"))?;
    tracing::info!(network = %cfg.network, executor = %cfg.executor_account_id, key = %signer.public_key(), "executor key");
    let exec = Executor {
        rpc: Rpc::new(cfg.rpc_url),
        signer,
        dep: cfg.deployments,
        max_step_usd: cfg.max_step_usd,
        price_pool: cfg.price_pool,
        nonces: Default::default(),
        backoff: Duration::from_secs(2),
    };
    let app = Arc::new(http::AppInner {
        exec,
        secret: cfg.internal_secret,
        seen: Default::default(),
    });
    let listener = tokio::net::TcpListener::bind(&cfg.addr)
        .await
        .map_err(|e| format!("bind {}: {e}", cfg.addr))?;
    tracing::info!(addr = %cfg.addr, "listening");
    axum::serve(listener, http::router(app))
        .with_graceful_shutdown(async {
            let _ = tokio::signal::ctrl_c().await;
        })
        .await
        .map_err(|e| e.to_string())
}
