#![allow(unused_must_use)]
use near_sdk::json_types::U128;
use near_sdk::serde_json::{self, json};
use near_sdk::test_utils::{get_logs, VMContextBuilder};
use near_sdk::{testing_env, AccountId, NearToken, PromiseResult, PublicKey, RuntimeFeesConfig};

use crate::executor::{lending_key, STAKING_KEY};
use crate::{Contract, Slot, Step, Venues};

fn acc(s: &str) -> AccountId {
    s.parse().unwrap()
}
fn exec_pk() -> PublicKey {
    "ed25519:6E8sCci9badyRkXb3JoRpBj5p8C6Tw41ELDZoiihKEtp"
        .parse()
        .unwrap()
}
fn other_pk() -> PublicKey {
    "ed25519:DcA2MzgpJbrUATQLLceocVckhhAqrkingax4oJ9kZ847"
        .parse()
        .unwrap()
}
const ME: &str = "alice.factory.testnet";
const USDC: &str = "usdc.testnet";
const WNEAR: &str = "wrap.testnet";

fn ctx(pred: &str, pk: PublicKey, deposit: u128) -> VMContextBuilder {
    let mut b = VMContextBuilder::new();
    b.current_account_id(acc(ME))
        .predecessor_account_id(acc(pred))
        .signer_account_pk(pk)
        .attached_deposit(NearToken::from_yoctonear(deposit))
        .account_balance(NearToken::from_near(10));
    b
}

fn set(pred: &str, pk: PublicKey, deposit: u128) {
    testing_env!(ctx(pred, pk, deposit).build());
}

fn setup() -> Contract {
    set("factory.testnet", other_pk(), 0);
    let mut c = Contract::new(
        acc("alice.testnet"),
        exec_pk(),
        acc("treasury.testnet"),
        Venues {
            lending: acc("lend.testnet"),
            dex: acc("dex.testnet"),
            staking: acc("lst.testnet"),
        },
        vec![(acc(USDC), U128(1_000)), (acc(WNEAR), U128(1_000))],
        U128(5_000),
    );
    set(USDC, other_pk(), 0);
    c.ft_on_transfer(acc("bob.testnet"), U128(2_000), String::new());
    c
}

fn as_executor() {
    set(ME, exec_pk(), 0);
}

fn bal(c: &Contract, t: &str) -> u128 {
    c.balances.get(&acc(t)).copied().unwrap_or(0)
}

fn settle_with(result: PromiseResult) {
    testing_env!(
        ctx(ME, exec_pk(), 0).build(),
        near_sdk::test_vm_config(),
        RuntimeFeesConfig::test(),
        Default::default(),
        vec![result]
    );
}

#[test]
fn deposit_credits_and_logs_event() {
    let c = setup();
    assert_eq!(bal(&c, USDC), 2_000);
    let log = get_logs().pop().unwrap();
    assert!(log.starts_with("EVENT_JSON:"));
    let v: serde_json::Value = serde_json::from_str(&log["EVENT_JSON:".len()..]).unwrap();
    assert_eq!(v["standard"], "nearly");
    assert_eq!(v["version"], "1.0.0");
    assert_eq!(v["event"], "deposit");
}

#[test]
#[should_panic(expected = "token not whitelisted")]
fn deposit_rejects_unknown_token() {
    let mut c = setup();
    set("scam.testnet", other_pk(), 0);
    c.ft_on_transfer(acc("bob.testnet"), U128(1), String::new());
}

#[test]
fn owner_withdraws() {
    let mut c = setup();
    set("alice.testnet", other_pk(), 1);
    c.withdraw(acc(USDC), U128(500));
    assert_eq!(bal(&c, USDC), 1_500);
}

#[test]
#[should_panic(expected = "only owner")]
fn withdraw_rejected_for_others() {
    let mut c = setup();
    set("mallory.testnet", other_pk(), 1);
    c.withdraw(acc(USDC), U128(500));
}

#[test]
#[should_panic(expected = "Requires attached deposit of exactly 1 yoctoNEAR")]
fn withdraw_needs_one_yocto() {
    let mut c = setup();
    set("alice.testnet", other_pk(), 0);
    c.withdraw(acc(USDC), U128(500));
}

#[test]
#[should_panic(expected = "insufficient balance")]
fn withdraw_over_balance() {
    let mut c = setup();
    set("alice.testnet", other_pk(), 1);
    c.withdraw(acc(USDC), U128(2_001));
}

#[test]
fn failed_transfer_is_refunded() {
    let mut c = setup();
    set("alice.testnet", other_pk(), 1);
    c.withdraw(acc(USDC), U128(500));
    settle_with(PromiseResult::Failed);
    c.on_settle(Some(Slot::Balance(acc(USDC))), U128(500), None, false);
    assert_eq!(bal(&c, USDC), 2_000);
}

#[test]
fn long_success_result_is_not_refunded() {
    let mut c = setup();
    set("alice.testnet", other_pk(), 1);
    c.withdraw(acc(USDC), U128(500));
    settle_with(PromiseResult::Successful(vec![b'x'; 100]));
    c.on_settle(Some(Slot::Balance(acc(USDC))), U128(500), None, false);
    assert_eq!(bal(&c, USDC), 1_500);
}

#[test]
fn executor_fee_and_to_owner() {
    let mut c = setup();
    as_executor();
    c.execute_step(Step::Fee {
        token: acc(USDC),
        amount: U128(100),
    });
    c.execute_step(Step::ToOwner {
        token: acc(USDC),
        amount: U128(900),
    });
    assert_eq!(bal(&c, USDC), 1_000);
}

#[test]
#[should_panic(expected = "only self")]
fn executor_must_be_self() {
    let mut c = setup();
    set("alice.testnet", exec_pk(), 0);
    c.execute_step(Step::ToOwner {
        token: acc(USDC),
        amount: U128(1),
    });
}

#[test]
#[should_panic(expected = "not executor key")]
fn executor_wrong_key() {
    let mut c = setup();
    set(ME, other_pk(), 0);
    c.execute_step(Step::ToOwner {
        token: acc(USDC),
        amount: U128(1),
    });
}

#[test]
#[should_panic(expected = "over cap")]
fn over_cap_rejected() {
    let mut c = setup();
    as_executor();
    c.execute_step(Step::Supply {
        token: acc(USDC),
        amount: U128(1_001),
    });
}

#[test]
#[should_panic(expected = "over cap")]
fn stake_over_near_cap_rejected() {
    let mut c = setup();
    as_executor();
    c.execute_step(Step::Stake {
        amount: U128(5_001),
    });
}

#[test]
#[should_panic(expected = "zero amount")]
fn zero_amount_rejected() {
    let mut c = setup();
    as_executor();
    c.execute_step(Step::Fee {
        token: acc(USDC),
        amount: U128(0),
    });
}

#[test]
#[should_panic(expected = "token not whitelisted")]
fn non_whitelisted_token_rejected() {
    let mut c = setup();
    as_executor();
    c.execute_step(Step::Supply {
        token: acc("scam.testnet"),
        amount: U128(1),
    });
}

#[test]
#[should_panic(expected = "token not whitelisted")]
fn swap_to_non_whitelisted_rejected() {
    let mut c = setup();
    as_executor();
    c.execute_step(Step::Swap {
        pool_id: 1,
        token_in: acc(USDC),
        token_out: acc("scam.testnet"),
        amount_in: U128(1),
        min_out: U128(1),
    });
}

#[test]
#[should_panic(expected = "executor revoked")]
fn revoked_executor_rejected() {
    let mut c = setup();
    set("alice.testnet", other_pk(), 1);
    c.revoke_executor();
    as_executor();
    c.execute_step(Step::ToOwner {
        token: acc(USDC),
        amount: U128(1),
    });
}

#[test]
#[should_panic(expected = "only owner")]
fn revoke_only_owner() {
    let mut c = setup();
    set("mallory.testnet", other_pk(), 1);
    c.revoke_executor();
}

#[test]
#[should_panic(expected = "only owner")]
fn venues_only_owner() {
    let mut c = setup();
    set("mallory.testnet", other_pk(), 1);
    let v = Venues {
        lending: acc("evil.testnet"),
        dex: acc("evil.testnet"),
        staking: acc("evil.testnet"),
    };
    c.set_venues(v);
}

#[test]
fn owner_edits_policy() {
    let mut c = setup();
    set("alice.testnet", other_pk(), 1);
    c.set_token_cap(acc(USDC), None);
    c.set_near_cap(U128(7));
    let p = c.get_policy();
    assert_eq!(p.caps, vec![(acc(WNEAR), U128(1_000))]);
    assert_eq!(p.near_cap, U128(7));
}

#[test]
fn no_step_can_name_a_receiver() {
    let with_receiver =
        json!({"kind": "to_owner", "token": USDC, "amount": "1", "receiver_id": "mallory.testnet"});
    assert!(serde_json::from_value::<Step>(with_receiver).is_err());
    let transfer = json!({"kind": "transfer", "token": USDC, "amount": "1"});
    assert!(serde_json::from_value::<Step>(transfer).is_err());
}

#[test]
fn step_json_shape() {
    let s: Step = serde_json::from_value(json!({
        "kind": "swap", "pool_id": 7, "token_in": USDC, "token_out": WNEAR, "amount_in": "10", "min_out": "9"
    }))
    .unwrap();
    assert_eq!(
        s,
        Step::Swap {
            pool_id: 7,
            token_in: acc(USDC),
            token_out: acc(WNEAR),
            amount_in: U128(10),
            min_out: U128(9)
        }
    );
    let w: Step =
        serde_json::from_value(json!({"kind": "withdraw_lend", "token": USDC, "amount": "1"}))
            .unwrap();
    assert_eq!(
        w,
        Step::WithdrawLend {
            token: acc(USDC),
            amount: U128(1)
        }
    );
}

#[test]
fn supply_books_position_on_partial_use() {
    let mut c = setup();
    as_executor();
    c.execute_step(Step::Supply {
        token: acc(USDC),
        amount: U128(1_000),
    });
    assert_eq!(bal(&c, USDC), 1_000);
    settle_with(PromiseResult::Successful(b"\"600\"".to_vec()));
    let pos = Slot::Position(lending_key(&acc(USDC)));
    c.on_settle(Some(Slot::Balance(acc(USDC))), U128(1_000), Some(pos), true);
    assert_eq!(bal(&c, USDC), 1_400);
    assert_eq!(
        c.positions.get(&lending_key(&acc(USDC))).copied(),
        Some(600)
    );
}

#[test]
fn stake_failure_books_nothing() {
    let mut c = setup();
    as_executor();
    c.execute_step(Step::Stake { amount: U128(100) });
    settle_with(PromiseResult::Failed);
    c.on_settle(
        None,
        U128(100),
        Some(Slot::Position(STAKING_KEY.into())),
        false,
    );
    assert!(c.positions.get(STAKING_KEY).is_none());
}

fn swap_step(min_out: u128) -> Step {
    Step::Swap {
        pool_id: 1,
        token_in: acc(USDC),
        token_out: acc(WNEAR),
        amount_in: U128(500),
        min_out: U128(min_out),
    }
}

#[test]
fn swap_books_actual_output() {
    let mut c = setup();
    as_executor();
    c.execute_step(swap_step(900));
    assert_eq!(bal(&c, USDC), 1_500);
    settle_with(PromiseResult::Successful(b"\"500\"".to_vec()));
    c.on_settle(Some(Slot::Balance(acc(USDC))), U128(500), None, true);
    // dex sent 1_000 wNEAR, more than min_out
    settle_with(PromiseResult::Successful(b"\"1000\"".to_vec()));
    assert_eq!(c.on_observe(acc(WNEAR)), U128(1_000));
    assert_eq!(bal(&c, WNEAR), 1_000);
    assert_eq!(bal(&c, USDC), 1_500);
}

#[test]
fn failed_swap_refunds_and_books_nothing() {
    let mut c = setup();
    as_executor();
    c.execute_step(swap_step(10_000));
    settle_with(PromiseResult::Successful(b"\"0\"".to_vec()));
    c.on_settle(Some(Slot::Balance(acc(USDC))), U128(500), None, true);
    settle_with(PromiseResult::Successful(b"\"0\"".to_vec()));
    assert_eq!(c.on_observe(acc(WNEAR)), U128(0));
    assert_eq!(bal(&c, USDC), 2_000);
    assert_eq!(bal(&c, WNEAR), 0);
}

#[test]
fn failed_balance_read_books_nothing() {
    let mut c = setup();
    settle_with(PromiseResult::Failed);
    assert_eq!(c.on_observe(acc(USDC)), U128(0));
    assert_eq!(bal(&c, USDC), 2_000);
}

#[test]
fn withdraw_lend_moves_position_to_balance() {
    let mut c = setup();
    let key = lending_key(&acc(USDC));
    c.positions.insert(key.clone(), 800);
    as_executor();
    c.execute_step(Step::WithdrawLend {
        token: acc(USDC),
        amount: U128(300),
    });
    assert_eq!(c.positions.get(&key).copied(), Some(500));
    settle_with(asset(12));
    c.on_lend_asset(acc(USDC), U128(300));
    settle_with(PromiseResult::Successful(vec![]));
    c.on_settle(Some(Slot::Position(key.clone())), U128(300), None, false);
    // 2_000 tracked + 300 arrived
    settle_with(PromiseResult::Successful(b"\"2300\"".to_vec()));
    c.on_observe(acc(USDC));
    assert_eq!(bal(&c, USDC), 2_300);
    assert_eq!(c.positions.get(&key).copied(), Some(500));
}

#[test]
fn failed_withdraw_lend_restores_position() {
    let mut c = setup();
    let key = lending_key(&acc(USDC));
    c.positions.insert(key.clone(), 800);
    as_executor();
    c.execute_step(Step::WithdrawLend {
        token: acc(USDC),
        amount: U128(300),
    });
    settle_with(PromiseResult::Failed);
    c.on_settle(Some(Slot::Position(key.clone())), U128(300), None, false);
    assert_eq!(c.positions.get(&key).copied(), Some(800));
}

fn asset(extra_decimals: u64) -> PromiseResult {
    let a = json!({"config": {"extra_decimals": extra_decimals}});
    PromiseResult::Successful(a.to_string().into_bytes())
}

#[test]
fn withdraw_lend_scales_to_inner_units() {
    let mut c = setup();
    let key = lending_key(&acc(USDC));
    c.positions.insert(key.clone(), 800);
    settle_with(asset(12));
    c.on_lend_asset(acc(USDC), U128(300));
    let args = String::from_utf8(
        near_sdk::test_utils::get_created_receipts()[0]
            .actions
            .iter()
            .find_map(|a| match a {
                near_sdk::mock::MockAction::FunctionCallWeight { args, .. } => Some(args.clone()),
                _ => None,
            })
            .unwrap(),
    )
    .unwrap();
    assert!(
        args.contains("\"max_amount\":\"300000000000000\""),
        "{args}"
    );
}

#[test]
fn unknown_lend_asset_restores_position() {
    let mut c = setup();
    let key = lending_key(&acc(USDC));
    c.positions.insert(key.clone(), 500);
    settle_with(PromiseResult::Successful(b"null".to_vec()));
    c.on_lend_asset(acc(USDC), U128(300));
    assert_eq!(c.positions.get(&key).copied(), Some(800));
}

#[test]
fn unstake_failure_restores_position() {
    let mut c = setup();
    c.positions.insert(STAKING_KEY.into(), 1_000);
    as_executor();
    c.execute_step(Step::Unstake { amount: U128(400) });
    assert_eq!(c.positions.get(STAKING_KEY).copied(), Some(600));
    settle_with(PromiseResult::Failed);
    c.on_settle(
        Some(Slot::Position(STAKING_KEY.into())),
        U128(400),
        None,
        false,
    );
    assert_eq!(c.positions.get(STAKING_KEY).copied(), Some(1_000));
}

fn results(r: Vec<PromiseResult>) {
    testing_env!(
        ctx(ME, exec_pk(), 0).build(),
        near_sdk::test_vm_config(),
        RuntimeFeesConfig::test(),
        Default::default(),
        r
    );
}

fn ok(v: &str) -> PromiseResult {
    PromiseResult::Successful(format!("\"{v}\"").into_bytes())
}

/// (method, args) of every function call this test made.
fn calls() -> Vec<(String, String)> {
    near_sdk::test_utils::get_created_receipts()
        .iter()
        .flat_map(|r| r.actions.iter())
        .filter_map(|a| match a {
            near_sdk::mock::MockAction::FunctionCallWeight {
                method_name, args, ..
            } => Some((
                String::from_utf8(method_name.clone()).unwrap(),
                String::from_utf8(args.clone()).unwrap(),
            )),
            _ => None,
        })
        .collect()
}

#[test]
fn owner_withdraw_ok_returns_amount() {
    let c = setup();
    settle_with(ok("500"));
    assert_eq!(c.on_withdraw(U128(500)), U128(500));
}

#[test]
#[should_panic(expected = "transfer failed, refunded to ledger")]
fn owner_withdraw_fails_when_transfer_refunded() {
    let c = setup();
    settle_with(ok("0"));
    c.on_withdraw(U128(500));
}

#[test]
fn get_balance_reads_ledger() {
    let c = setup();
    assert_eq!(c.get_balance(acc(USDC)), U128(2_000));
    assert_eq!(c.get_balance(acc(WNEAR)), U128(0));
}

fn lst_exit(amount: u128) -> Step {
    Step::Swap {
        pool_id: 3515,
        token_in: acc("lst.testnet"),
        token_out: acc(WNEAR),
        amount_in: U128(amount),
        min_out: U128(1),
    }
}

#[test]
fn lst_exit_debits_staking_position() {
    let mut c = setup();
    c.positions.insert(STAKING_KEY.into(), 1_000);
    as_executor();
    c.execute_step(lst_exit(400));
    assert_eq!(c.positions.get(STAKING_KEY).copied(), Some(600));
    let methods: Vec<_> = calls().into_iter().map(|(m, _)| m).collect();
    assert_eq!(methods, ["ft_price", "ft_balance_of", "on_lst_exit"]);
}

#[test]
#[should_panic(expected = "over cap")]
fn lst_exit_uses_near_cap() {
    let mut c = setup();
    c.positions.insert(STAKING_KEY.into(), 10_000);
    as_executor();
    c.execute_step(lst_exit(5_001));
}

fn swapped_shares() -> String {
    let (_, args) = calls()
        .into_iter()
        .find(|(m, _)| m == "ft_transfer_call")
        .unwrap();
    let v: serde_json::Value = serde_json::from_str(&args).unwrap();
    let msg: serde_json::Value = serde_json::from_str(v["msg"].as_str().unwrap()).unwrap();
    assert_eq!(v["amount"], msg["actions"][0]["amount_in"]);
    v["amount"].as_str().unwrap().into()
}

#[test]
fn lst_partial_exit_sells_value_at_price() {
    let mut c = setup();
    // 1 share = 1.25 NEAR: 1e24 NEAR value = 0.8e24 shares
    results(vec![
        ok("1250000000000000000000000"),
        ok("5000000000000000000000000"),
    ]);
    c.on_lst_exit(3515, acc(WNEAR), U128(10u128.pow(24)), U128(1), false);
    assert_eq!(swapped_shares(), "800000000000000000000000");
}

#[test]
fn lst_full_exit_sells_whole_balance() {
    let mut c = setup();
    results(vec![
        ok("1250000000000000000000000"),
        ok("799999999999999999999998"),
    ]);
    c.on_lst_exit(3515, acc(WNEAR), U128(10u128.pow(24)), U128(1), true);
    assert_eq!(swapped_shares(), "799999999999999999999998");
}

#[test]
fn lst_exit_unreadable_price_refunds_position() {
    let mut c = setup();
    c.positions.insert(STAKING_KEY.into(), 600);
    results(vec![PromiseResult::Failed, ok("5")]);
    assert!(matches!(
        c.on_lst_exit(3515, acc(WNEAR), U128(400), U128(1), false),
        near_sdk::PromiseOrValue::Value(U128(0))
    ));
    assert_eq!(c.positions.get(STAKING_KEY).copied(), Some(1_000));
}

#[test]
fn lst_swap_failure_refunds_position() {
    let mut c = setup();
    c.positions.insert(STAKING_KEY.into(), 600);
    settle_with(ok("0"));
    assert_eq!(c.on_lst_swap(U128(400)), U128(0));
    assert_eq!(c.positions.get(STAKING_KEY).copied(), Some(1_000));
    settle_with(ok("320"));
    assert_eq!(c.on_lst_swap(U128(400)), U128(320));
    assert_eq!(c.positions.get(STAKING_KEY).copied(), Some(1_000));
}

#[test]
fn full_unstake_uses_unstake_all() {
    let mut c = setup();
    c.positions.insert(STAKING_KEY.into(), 1_000);
    as_executor();
    c.execute_step(Step::Unstake {
        amount: U128(1_000),
    });
    assert_eq!(calls()[0].0, "unstake_all");
}
