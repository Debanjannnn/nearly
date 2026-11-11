use near_sdk::{env, AccountId};

/// `alice.testnet` -> `alice-testnet.<factory>`; falls back to a hash when that is not a valid id.
pub fn sub_account(owner: &AccountId, factory: &AccountId) -> AccountId {
    let readable = format!("{}.{factory}", owner.as_str().replace('.', "-"));
    readable.parse().unwrap_or_else(|_| {
        let hash: String = env::sha256(owner.as_bytes())[..8]
            .iter()
            .map(|b| format!("{b:02x}"))
            .collect();
        format!("{hash}.{factory}").parse().unwrap()
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn readable_and_hashed_names() {
        let f: AccountId = "factory.testnet".parse().unwrap();
        let alice: AccountId = "alice.testnet".parse().unwrap();
        assert_eq!(
            sub_account(&alice, &f).as_str(),
            "alice-testnet.factory.testnet"
        );
        let implicit: AccountId = "a".repeat(64).parse().unwrap();
        let s = sub_account(&implicit, &f);
        assert_eq!(s.as_str().len(), 16 + 1 + f.as_str().len());
        assert!(s.as_str().ends_with(".factory.testnet"));
    }
}
