//! HTTP -> recheck -> sign -> send against a fake NEAR RPC.
use axum::{body::Body, extract::State, http::Request, routing::post, Json, Router};
use base64::{engine::general_purpose::STANDARD as B64, Engine};
use nearly_executor::{config::Deployments, exec::Executor, http, rpc::Rpc, signer::Signer};
use serde_json::{json, Value};
use std::{
    sync::{Arc, Mutex},
    time::Duration,
};
use tower::ServiceExt;

const ACCOUNT: &str = "alice.testnet.factory.nearly.testnet";
const USDC: &str = "usdcc.ft.ref-labs.testnet";

/// Fake RPC: logs calls; the first send_tx is refused with InvalidNonce, the rest succeed.
type Log = Arc<Mutex<Vec<(String, Value)>>>;

async fn fake_rpc(State(log): State<Log>, Json(req): Json<Value>) -> Json<Value> {
    let (method, params) = (
        req["method"].as_str().unwrap().to_string(),
        req["params"].clone(),
    );
    let sends = {
        let mut l = log.lock().unwrap();
        l.push((method.clone(), params.clone()));
        l.iter().filter(|(m, _)| m == "send_tx").count()
    };
    let res = match (method.as_str(), params["request_type"].as_str()) {
        ("query", Some("view_access_key")) => {
            json!({"result": {"nonce": 41, "permission": "FullAccess"}})
        }
        ("query", Some("call_function")) => json!({"result": {"result": b"\"3000000\"".to_vec()}}),
        ("block", _) => {
            json!({"result": {"header": {"hash": bs58::encode([9u8; 32]).into_string()}}})
        }
        ("send_tx", _) if sends == 1 => {
            json!({"error": {"name": "HANDLER_ERROR", "cause": {"name": "INVALID_TRANSACTION", "info": {"InvalidNonce": {"tx_nonce": 42, "ak_nonce": 50}}}}})
        }
        ("send_tx", _) if sends == 4 => {
            json!({"result": {"status": {"Failure": {"ActionError": {"index": 0, "kind": "FunctionCallError"}}}}})
        }
        ("send_tx", _) => json!({"result": {"status": {"SuccessValue": ""}}}),
        _ => json!({"error": {"name": "UNKNOWN"}}),
    };
    Json(res)
}

async fn setup() -> (Router, Log) {
    let log: Log = Default::default();
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let url = format!("http://{}", listener.local_addr().unwrap());
    let rpc = Router::new()
        .route("/", post(fake_rpc))
        .with_state(log.clone());
    tokio::spawn(async move { axum::serve(listener, rpc).await.unwrap() });

    let mut dep = Deployments::load(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../contracts/deployments/testnet.json"
    ))
    .unwrap();
    dep.factory = "factory.nearly.testnet".into();
    let exec = Executor {
        rpc: Rpc::new(url),
        signer: Signer::parse(&format!(
            "ed25519:{}",
            bs58::encode([7u8; 32]).into_string()
        ))
        .unwrap(),
        dep,
        max_step_usd: 1000,
        price_pool: 1900,
        nonces: Default::default(),
        backoff: Duration::from_millis(1),
    };
    let app = Arc::new(http::AppInner {
        exec,
        secret: "s3cret".into(),
        seen: Default::default(),
    });
    (http::router(app), log)
}

async fn post_exec(app: &Router, secret: &str, body: Value) -> (u16, Value) {
    let req = Request::post("/v1/execute")
        .header("content-type", "application/json")
        .header("x-internal-secret", secret)
        .body(Body::from(body.to_string()))
        .unwrap();
    let res = app.clone().oneshot(req).await.unwrap();
    let code = res.status().as_u16();
    let bytes = axum::body::to_bytes(res.into_body(), 1 << 20)
        .await
        .unwrap();
    (code, serde_json::from_slice(&bytes).unwrap_or(Value::Null))
}

fn body(id: &str, steps: Value) -> Value {
    json!({"account_id": ACCOUNT, "execution_id": id, "steps": steps})
}

/// nonce field of a signed tx: after borsh signer_id + 33-byte key.
fn tx_nonce(params: &Value) -> u64 {
    let b = B64
        .decode(params["signed_tx_base64"].as_str().unwrap())
        .unwrap();
    let at = 4 + ACCOUNT.len() + 33;
    u64::from_le_bytes(b[at..at + 8].try_into().unwrap())
}

#[tokio::test]
async fn execute_flow() {
    let (app, log) = setup().await;
    let steps = json!([
        {"kind": "supply", "token": USDC, "amount": "1000000"},
        {"kind": "stake", "amount": "1000000000000000000000000"},
        {"kind": "to_owner", "token": USDC, "amount": "5"},
        {"kind": "fee", "token": USDC, "amount": "5"}
    ]);

    let (code, _) = post_exec(&app, "wrong", body("e1", steps.clone())).await;
    assert_eq!(code, 401);

    let evil = json!([{"kind": "supply", "token": "evil.testnet", "amount": "1"}]);
    let (code, v) = post_exec(&app, "s3cret", body("e0", evil)).await;
    assert_eq!(code, 422, "{v}");
    let (code, _) = post_exec(
        &app,
        "s3cret",
        json!({"account_id": "bob.testnet", "execution_id": "e0", "steps": steps}),
    )
    .await;
    assert_eq!(code, 422);
    let over = json!([{"kind": "stake", "amount": "334000000000000000000000000"}]);
    let (code, _) = post_exec(&app, "s3cret", body("e0", over)).await;
    assert_eq!(code, 422);
    assert!(
        log.lock().unwrap().iter().all(|(m, _)| m != "send_tx"),
        "refused plan must not sign"
    );

    let (code, v) = post_exec(&app, "s3cret", body("e1", steps.clone())).await;
    assert_eq!(code, 200, "{v}");
    let r = v["results"].as_array().unwrap();
    assert_eq!(r.len(), 4);
    assert_eq!(
        (r[0]["status"].as_str(), r[1]["status"].as_str()),
        (Some("ok"), Some("ok"))
    );
    assert_eq!(r[2]["status"], "failed");
    assert!(
        r[2]["tx_hash"].is_string()
            && r[2]["error"]
                .as_str()
                .unwrap()
                .contains("FunctionCallError")
    );
    assert_eq!(
        (r[3]["status"].as_str(), r[3]["error"].as_str()),
        (Some("failed"), Some("skipped"))
    );
    assert!(r[3]["tx_hash"].is_null() && r[0]["error"].is_null());

    let nonces: Vec<u64> = log
        .lock()
        .unwrap()
        .iter()
        .filter(|(m, _)| m == "send_tx")
        .map(|(_, p)| tx_nonce(p))
        .collect();
    // 42 refused (InvalidNonce), refetch: 42 ok, 43 ok, 44 failed on chain
    assert_eq!(nonces, vec![42, 42, 43, 44]);

    let (code, _) = post_exec(&app, "s3cret", body("e1", steps)).await;
    assert_eq!(code, 409, "replay refused");
}
