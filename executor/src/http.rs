use axum::{
    extract::{rejection::JsonRejection, Request, State},
    http::StatusCode,
    middleware::{self, Next},
    response::{IntoResponse, Response},
    routing::{get, post},
    Json, Router,
};
use serde::Deserialize;
use serde_json::json;
use std::{collections::HashSet, sync::Arc};
use tokio::sync::Mutex;

use crate::{exec::Executor, steps::Step};

pub struct AppInner {
    pub exec: Executor,
    pub secret: String,
    /// execution ids already accepted: a replay must not move funds twice.
    // shortcut: in memory, lost on restart; persist when the api retries across executor restarts
    pub seen: Mutex<HashSet<String>>,
}

pub type App = Arc<AppInner>;

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ExecuteReq {
    pub account_id: String,
    pub execution_id: String,
    pub steps: Vec<Step>,
}

fn fail(code: StatusCode, e: impl std::fmt::Display) -> Response {
    (code, Json(json!({ "error": e.to_string() }))).into_response()
}

pub fn router(app: App) -> Router {
    let api = Router::new()
        .route("/v1/execute", post(execute))
        .route_layer(middleware::from_fn_with_state(app.clone(), guard));
    Router::new()
        .route("/health", get(|| async { Json(json!({"status": "ok"})) }))
        .merge(api)
        .with_state(app)
}

fn eq_ct(a: &[u8], b: &[u8]) -> bool {
    a.len() == b.len() && a.iter().zip(b).fold(0u8, |acc, (x, y)| acc | (x ^ y)) == 0
}

async fn guard(State(app): State<App>, req: Request, next: Next) -> Response {
    let got = req
        .headers()
        .get("x-internal-secret")
        .map(|v| v.as_bytes())
        .unwrap_or_default();
    if !eq_ct(got, app.secret.as_bytes()) {
        return fail(StatusCode::UNAUTHORIZED, "unauthorized");
    }
    next.run(req).await
}

/// 422 = refused before signing; 200 = per-step results, possibly with failures.
async fn execute(
    State(app): State<App>,
    body: Result<Json<ExecuteReq>, JsonRejection>,
) -> Response {
    let Json(req) = match body {
        Ok(b) => b,
        Err(e) => return fail(StatusCode::BAD_REQUEST, e.body_text()),
    };
    if req.execution_id.is_empty() || req.execution_id.len() > 64 {
        return fail(StatusCode::BAD_REQUEST, "bad execution_id");
    }
    if let Err(e) = app.exec.recheck(&req.account_id, &req.steps).await {
        tracing::warn!(account = %req.account_id, execution = %req.execution_id, error = %e, "refused");
        return fail(StatusCode::UNPROCESSABLE_ENTITY, e);
    }
    if !app.seen.lock().await.insert(req.execution_id.clone()) {
        return fail(StatusCode::CONFLICT, "execution_id already run");
    }
    tracing::info!(account = %req.account_id, execution = %req.execution_id, steps = req.steps.len(), "execute");
    // own task: a caller hanging up must not stop a plan half-way
    let task = tokio::spawn(async move { app.exec.run(&req.account_id, &req.steps).await });
    match task.await {
        Ok(results) => Json(json!({ "results": results })).into_response(),
        Err(e) => fail(StatusCode::INTERNAL_SERVER_ERROR, e),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn constant_time_eq() {
        assert!(eq_ct(b"abc", b"abc"));
        assert!(!eq_ct(b"abc", b"abd"));
        assert!(!eq_ct(b"abc", b"ab"));
        assert!(!eq_ct(b"", b"x"));
    }
}
