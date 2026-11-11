//! Sandbox: factory-made account drives lending, dex and staking mocks through execute_step.
use near_workspaces::network::Sandbox;
use near_workspaces::types::{Gas, KeyType, NearToken, SecretKey};
use near_workspaces::{Account, AccountId, Contract, Worker};
use serde_json::{json, Value};

const FACTORY_WASM: &[u8] = include_bytes!("../../res/nearly_factory.wasm");
const FT_WASM: &[u8] = include_bytes!("../../res/nearly_mocks_ft.wasm");
const LENDING_WASM: &[u8] = include_bytes!("../../res/nearly_mocks_lending.wasm");
const DEX_WASM: &[u8] = include_bytes!("../../res/nearly_mocks_dex.wasm");
const STAKING_WASM: &[u8] = include_bytes!("../../res/nearly_mocks_staking.wasm");
const TX_GAS: Gas = Gas::from_tgas(300);
const NEAR: u128 = 10u128.pow(24);

struct Env {
    worker: Worker<Sandbox>,
    alice: Account,
    usdc: Contract,
    wnear: Contract,
    lending: Contract,
    staking: Contract,
    exec: Account,
    id: AccountId,
}

async fn deploy(root: &Account, name: &str, wasm: &[u8]) -> anyhow::Result<Contract> {
    let acc = root
        .create_subaccount(name)
        .initial_balance(NearToken::from_near(10))
        .transact()
        .await?
        .into_result()?;
    Ok(acc.deploy(wasm).await?.into_result()?)
}

async fn setup() -> anyhow::Result<Env> {
    let worker = near_workspaces::sandbox().await?;
    let root = worker.root_account()?;
    let usdc = deploy(&root, "usdc", FT_WASM).await?;
    let wnear = deploy(&root, "wnear", FT_WASM).await?;
    let lending = deploy(&root, "lending", LENDING_WASM).await?;
    let dex = deploy(&root, "dex", DEX_WASM).await?;
    let staking = deploy(&root, "staking", STAKING_WASM).await?;
    let factory = deploy(&root, "factory", FACTORY_WASM).await?;
    for t in [&usdc, &wnear, &staking] {
        t.call("new").transact().await?.into_result()?;
    }
    staking
        .call("storage_deposit")
        .args_json(json!({"account_id": dex.id()}))
        .transact()
        .await?
        .into_result()?;
    // 1 USDC buys 2 wNEAR
    dex.call("new")
        .args_json(json!({"num": "2", "den": "1"}))
        .transact()
        .await?
        .into_result()?;
    // venues are registered on every token, as on the real nets; dex holds wNEAR liquidity
    for (t, who, amount) in [
        (&wnear, dex.id(), "1000000"),
        (&usdc, dex.id(), "0"),
        (&usdc, lending.id(), "0"),
    ] {
        t.call("mint")
            .args_json(json!({"account_id": who, "amount": amount}))
            .transact()
            .await?
            .into_result()?;
    }
    factory
        .call("new")
        .args_json(json!({"owner": root.id(), "config": {
            "treasury": root.id(),
            "venues": {"lending": lending.id(), "dex": dex.id(), "staking": staking.id()},
            "tokens": [[usdc.id(), "10000"], [wnear.id(), "10000"]],
            "near_cap": (5 * NEAR).to_string(),
        }}))
        .transact()
        .await?
        .into_result()?;

    let alice = root
        .create_subaccount("alice")
        .initial_balance(NearToken::from_near(30))
        .transact()
        .await?
        .into_result()?;
    let exec_sk = SecretKey::from_random(KeyType::ED25519);
    let need: NearToken = factory.view("required_deposit").await?.json()?;
    alice
        .call(factory.id(), "create_account")
        .args_json(json!({"owner": alice.id(), "executor_pubkey": exec_sk.public_key()}))
        .deposit(need)
        .max_gas()
        .transact()
        .await?
        .into_result()?;
    let id: AccountId = factory
        .view("get_account")
        .args_json(json!({"owner": alice.id()}))
        .await?
        .json()?;
    // alice funds the account: USDC via ft_transfer_call, NEAR for staking
    usdc.call("mint")
        .args_json(json!({"account_id": alice.id(), "amount": "5000"}))
        .transact()
        .await?
        .into_result()?;
    alice
        .call(usdc.id(), "ft_transfer_call")
        .args_json(json!({"receiver_id": id, "amount": "5000", "msg": ""}))
        .deposit(NearToken::from_yoctonear(1))
        .max_gas()
        .transact()
        .await?
        .into_result()?;
    alice
        .transfer_near(&id, NearToken::from_near(5))
        .await?
        .into_result()?;
    let exec = Account::from_secret_key(id.clone(), exec_sk, &worker);
    Ok(Env {
        worker,
        alice,
        usdc,
        wnear,
        lending,
        staking,
        exec,
        id,
    })
}

impl Env {
    async fn step(&self, step: Value) -> anyhow::Result<()> {
        self.exec
            .call(&self.id, "execute_step")
            .args_json(json!({"step": step}))
            .gas(TX_GAS)
            .transact()
            .await?
            .into_result()?;
        Ok(())
    }

    async fn ledger(&self, method: &str, key: &str) -> anyhow::Result<u128> {
        let rows: Vec<(String, String)> = self.worker.view(&self.id, method).await?.json()?;
        Ok(rows
            .into_iter()
            .find(|(k, _)| k == key)
            .map_or(0, |(_, v)| v.parse().unwrap()))
    }

    async fn ft_balance(&self, token: &Contract) -> anyhow::Result<u128> {
        let v: String = token
            .view("ft_balance_of")
            .args_json(json!({"account_id": self.id}))
            .await?
            .json()?;
        Ok(v.parse()?)
    }

    async fn venue_u128(&self, c: &Contract, method: &str, args: Value) -> anyhow::Result<u128> {
        let v: String = c.view(method).args_json(args).await?.json()?;
        Ok(v.parse()?)
    }
}

#[tokio::test]
async fn supply_withdraw_lending() -> anyhow::Result<()> {
    let e = setup().await?;
    let usdc = e.usdc.id().to_string();
    let pos = format!("lending:{usdc}");
    let supplied = || {
        e.venue_u128(
            &e.lending,
            "get_supplied",
            json!({"account_id": e.id, "token_id": usdc}),
        )
    };

    e.step(json!({"kind": "supply", "token": usdc, "amount": "3000"}))
        .await?;
    assert_eq!(e.ledger("get_positions", &pos).await?, 3000);
    assert_eq!(e.ledger("get_balances", &usdc).await?, 2000);
    assert_eq!(supplied().await?, 3000);

    e.step(json!({"kind": "withdraw_lend", "token": usdc, "amount": "1200"}))
        .await?;
    assert_eq!(e.ledger("get_positions", &pos).await?, 1800);
    assert_eq!(e.ledger("get_balances", &usdc).await?, 3200);
    assert_eq!(supplied().await?, 1800);
    assert_eq!(e.ft_balance(&e.usdc).await?, 3200);
    Ok(())
}

#[tokio::test]
async fn swap_books_actual_output_and_respects_min_out() -> anyhow::Result<()> {
    let e = setup().await?;
    let (usdc, wnear) = (e.usdc.id().to_string(), e.wnear.id().to_string());
    let swap = |min_out: &str| {
        json!({"kind": "swap", "pool_id": 1, "token_in": usdc, "token_out": wnear,
               "amount_in": "1000", "min_out": min_out})
    };

    e.step(swap("1500")).await?;
    assert_eq!(e.ledger("get_balances", &usdc).await?, 4000);
    assert_eq!(e.ledger("get_balances", &wnear).await?, 2000);
    assert_eq!(e.ft_balance(&e.wnear).await?, 2000);

    // min_out above the 2x rate: dex panics, input refunded, nothing booked
    e.step(swap("2001")).await?;
    assert_eq!(e.ledger("get_balances", &usdc).await?, 4000);
    assert_eq!(e.ledger("get_balances", &wnear).await?, 2000);
    assert_eq!(e.ft_balance(&e.usdc).await?, 4000);
    Ok(())
}

#[tokio::test]
async fn stake_unstake_matches_venue() -> anyhow::Result<()> {
    let e = setup().await?;
    let staked = || {
        e.venue_u128(
            &e.staking,
            "get_account_staked_balance",
            json!({"account_id": e.id}),
        )
    };

    e.step(json!({"kind": "stake", "amount": (2 * NEAR).to_string()}))
        .await?;
    assert_eq!(e.ledger("get_positions", "staking").await?, 2 * NEAR);
    assert_eq!(staked().await?, 2 * NEAR);

    e.step(json!({"kind": "unstake", "amount": (NEAR / 2).to_string()}))
        .await?;
    let left = 2 * NEAR - NEAR / 2;
    assert_eq!(e.ledger("get_positions", "staking").await?, left);
    assert_eq!(staked().await?, left);

    // more than the position: rejected before any venue call
    e.step(json!({"kind": "unstake", "amount": (left + 1).to_string()}))
        .await
        .unwrap_err();
    assert_eq!(e.ledger("get_positions", "staking").await?, left);

    // mock has no delay; real LiNEAR needs ~4 epochs before claim
    let before = e.worker.view_account(&e.id).await?.balance;
    let root = e.worker.root_account()?;
    root.call(&e.id, "claim_unstaked")
        .gas(TX_GAS)
        .transact()
        .await?
        .into_result()?;
    let after = e.worker.view_account(&e.id).await?.balance;
    assert!(after > before, "unstaked NEAR not returned");
    Ok(())
}

#[tokio::test]
async fn lst_exit_swaps_shares_to_wnear() -> anyhow::Result<()> {
    let e = setup().await?;
    let (lst, wnear) = (e.staking.id().to_string(), e.wnear.id().to_string());
    let exit = |amount: u128| {
        json!({"kind": "swap", "pool_id": 3515, "token_in": lst, "token_out": wnear,
               "amount_in": amount.to_string(), "min_out": "1"})
    };
    e.step(json!({"kind": "stake", "amount": "1000"})).await?;
    assert_eq!(e.ft_balance(&e.staking).await?, 1000);

    // partial: 400 NEAR value at price 1 = 400 shares, dex pays 2x
    e.step(exit(400)).await?;
    assert_eq!(e.ledger("get_positions", "staking").await?, 600);
    assert_eq!(e.ledger("get_balances", &wnear).await?, 800);

    // full exit sells every share left, then to_owner can pay out at once
    e.step(exit(600)).await?;
    assert_eq!(e.ledger("get_positions", "staking").await?, 0);
    assert_eq!(e.ft_balance(&e.staking).await?, 0);
    assert_eq!(e.ledger("get_balances", &wnear).await?, 2000);
    Ok(())
}

#[tokio::test]
async fn owner_withdraw_fails_loudly_when_transfer_fails() -> anyhow::Result<()> {
    let e = setup().await?;
    let usdc = e.usdc.id().to_string();
    let wnear = e.wnear.id().to_string();
    e.step(
        json!({"kind": "swap", "pool_id": 1, "token_in": usdc, "token_out": wnear,
                  "amount_in": "100", "min_out": "1"}),
    )
    .await?;
    assert_eq!(e.ledger("get_balances", &wnear).await?, 200);
    let withdraw = |token: &str, amount: &str| {
        e.alice
            .call(&e.id, "withdraw")
            .args_json(json!({"token": token, "amount": amount}))
            .deposit(NearToken::from_yoctonear(1))
            .gas(TX_GAS)
            .transact()
    };

    // alice is not registered on wNEAR: ft_transfer fails, tx fails, ledger refunded
    let r = withdraw(&wnear, "200").await?;
    assert!(r.is_failure(), "withdraw reported success");
    let got: String = e
        .worker
        .view(&e.id, "get_balance")
        .args_json(json!({"token": wnear}))
        .await?
        .json()?;
    assert_eq!(got, "200");

    let r = withdraw(&usdc, "100").await?;
    assert_eq!(r.into_result()?.json::<String>()?, "100");
    assert_eq!(e.ledger("get_balances", &usdc).await?, 4800);
    Ok(())
}
