#![allow(unused_must_use)]
use near_sdk::json_types::U128;
use near_sdk::test_utils::VMContextBuilder;
use near_sdk::{testing_env, AccountId, NearToken, PromiseResult, PublicKey, RuntimeFeesConfig};

use crate::{required, AccountConfig, Factory, Venues};

fn acc(s: &str) -> AccountId {
    s.parse().unwrap()
}
fn pk() -> PublicKey {
    "ed25519:6E8sCci9badyRkXb3JoRpBj5p8C6Tw41ELDZoiihKEtp"
        .parse()
        .unwrap()
}

fn ctx(pred: &str, deposit: NearToken) -> VMContextBuilder {
    let mut b = VMContextBuilder::new();
    b.current_account_id(acc("factory.testnet"))
        .predecessor_account_id(acc(pred))
        .attached_deposit(deposit)
        .account_balance(NearToken::from_near(100));
    b
}

fn setup() -> Factory {
    testing_env!(ctx("admin.testnet", NearToken::from_yoctonear(0)).build());
    Factory::new(
        acc("admin.testnet"),
        AccountConfig {
            treasury: acc("treasury.testnet"),
            venues: Venues {
                lending: acc("l.testnet"),
                dex: acc("d.testnet"),
                staking: acc("s.testnet"),
            },
            tokens: vec![(acc("usdc.testnet"), U128(1_000))],
            near_cap: U128(1),
        },
    )
}

fn call_as(pred: &str, deposit: NearToken) {
    testing_env!(ctx(pred, deposit).build());
}

#[test]
fn creates_and_records_account() {
    let mut f = setup();
    call_as("alice.testnet", required());
    f.create_account(acc("alice.testnet"), pk());
    assert_eq!(
        f.get_account(acc("alice.testnet")).unwrap().as_str(),
        "alice-testnet.factory.testnet"
    );
}

#[test]
fn factory_owner_may_create_for_user() {
    let mut f = setup();
    call_as("admin.testnet", required());
    f.create_account(acc("alice.testnet"), pk());
    assert!(f.get_account(acc("alice.testnet")).is_some());
}

#[test]
#[should_panic(expected = "account exists")]
fn rejects_duplicate() {
    let mut f = setup();
    call_as("alice.testnet", required());
    f.create_account(acc("alice.testnet"), pk());
    f.create_account(acc("alice.testnet"), pk());
}

#[test]
#[should_panic(expected = "only owner or factory owner")]
fn rejects_third_party() {
    let mut f = setup();
    call_as("mallory.testnet", required());
    f.create_account(acc("alice.testnet"), pk());
}

#[test]
#[should_panic(expected = "attach at least")]
fn rejects_low_deposit() {
    let mut f = setup();
    call_as("alice.testnet", NearToken::from_near(1));
    f.create_account(acc("alice.testnet"), pk());
}

#[test]
fn failed_create_frees_name() {
    let mut f = setup();
    call_as("alice.testnet", required());
    f.create_account(acc("alice.testnet"), pk());
    testing_env!(
        ctx("factory.testnet", NearToken::from_yoctonear(0)).build(),
        near_sdk::test_vm_config(),
        RuntimeFeesConfig::test(),
        Default::default(),
        vec![PromiseResult::Failed]
    );
    assert!(!f.on_create(acc("alice.testnet"), acc("alice.testnet"), required()));
    assert!(f.get_account(acc("alice.testnet")).is_none());
}

#[test]
#[should_panic(expected = "only owner")]
fn config_only_owner() {
    let mut f = setup();
    call_as("mallory.testnet", NearToken::from_yoctonear(0));
    let c = f.get_config().clone();
    f.set_config(c);
}
