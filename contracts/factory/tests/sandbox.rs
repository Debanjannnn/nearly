//! Sandbox: factory creates an account; the executor key can call `execute_step` and nothing else.
use near_workspaces::types::{Gas, KeyType, NearToken, SecretKey};
use near_workspaces::{Account, AccountId};
use serde_json::json;

const FACTORY_WASM: &[u8] = include_bytes!("../../res/nearly_factory.wasm");
/// What the executor sends per tx; must fit the key allowance.
const TX_GAS: Gas = Gas::from_tgas(300);

#[tokio::test]
async fn executor_key_is_scoped_and_revocable() -> anyhow::Result<()> {
    let worker = near_workspaces::sandbox().await?;
    let root = worker.root_account()?;
    let factory = root
        .create_subaccount("factory")
        .initial_balance(NearToken::from_near(50))
        .transact()
        .await?
        .into_result()?;
    let alice = root
        .create_subaccount("alice")
        .initial_balance(NearToken::from_near(20))
        .transact()
        .await?
        .into_result()?;
    factory.deploy(FACTORY_WASM).await?.into_result()?;
    let venue = root.id().to_string();
    factory
        .call(factory.id(), "new")
        .args_json(json!({"owner": root.id(), "config": {
            "treasury": root.id(),
            "venues": {"lending": venue, "dex": venue, "staking": venue},
            "tokens": [],
            "near_cap": "1000",
        }}))
        .transact()
        .await?
        .into_result()?;

    let exec_sk = SecretKey::from_random(KeyType::ED25519);
    let need: NearToken = factory
        .view(factory.id(), "required_deposit")
        .await?
        .json()?;
    let args = json!({"owner": alice.id(), "executor_pubkey": exec_sk.public_key()});
    let res = alice
        .call(factory.id(), "create_account")
        .args_json(&args)
        .deposit(need)
        .max_gas()
        .transact()
        .await?;
    assert!(res.is_success(), "{res:?}");
    let dup = alice
        .call(factory.id(), "create_account")
        .args_json(&args)
        .deposit(need)
        .max_gas()
        .transact()
        .await?;
    assert!(dup.is_failure());

    let id: AccountId = factory
        .view(factory.id(), "get_account")
        .args_json(json!({"owner": alice.id()}))
        .await?
        .json()?;
    assert_eq!(id.as_str(), "alice-test-near.factory.test.near");
    let exec = Account::from_secret_key(id.clone(), exec_sk, &worker);
    let step = json!({"step": {"kind": "stake", "amount": "1"}});

    // key accepted, contract runs: stake within cap succeeds against a no-op venue
    let ok = exec
        .call(&id, "execute_step")
        .args_json(&step)
        .gas(TX_GAS)
        .transact()
        .await?;
    assert!(format!("{ok:?}").contains("EVENT_JSON"), "{ok:?}");

    let other = exec
        .call(&id, "set_near_cap")
        .args_json(json!({"cap": "1"}))
        .transact()
        .await;
    assert!(
        other.is_err() || other?.is_failure(),
        "executor key called another method"
    );
    let paid = exec
        .call(&id, "execute_step")
        .args_json(&step)
        .deposit(NearToken::from_yoctonear(1))
        .transact()
        .await;
    assert!(
        paid.is_err() || paid?.is_failure(),
        "executor key attached deposit"
    );
    let over = exec
        .call(&id, "execute_step")
        .args_json(json!({"step": {"kind": "stake", "amount": "1001"}}))
        .gas(TX_GAS)
        .transact()
        .await?;
    let err = format!("{:?}", over.into_result().unwrap_err());
    assert!(err.contains("over cap"), "{err}");

    alice
        .call(&id, "revoke_executor")
        .deposit(NearToken::from_yoctonear(1))
        .transact()
        .await?
        .into_result()?;
    let (_, revoked): (serde_json::Value, bool) = worker.view(&id, "get_executor").await?.json()?;
    assert!(revoked);
    let after = exec
        .call(&id, "execute_step")
        .args_json(&step)
        .gas(TX_GAS)
        .transact()
        .await;
    assert!(
        after.is_err() || after?.is_failure(),
        "revoked key still works"
    );
    Ok(())
}
