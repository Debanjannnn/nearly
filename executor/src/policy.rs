//! Off-chain recheck before signing: same rules as the account contract, plus a USD cap per step.
use crate::{config::Deployments, steps::Step};

pub const MAX_STEPS: usize = 32;
const USDC_ONE: u128 = 1_000_000;
const NEAR_ONE: u128 = 1_000_000_000_000_000_000_000_000;

/// Only `<owner>.<factory>` accounts carry the executor key.
pub fn check_account(dep: &Deployments, account: &str) -> Result<(), String> {
    if dep.factory.is_empty() {
        return Err("factory unset in deployments".into());
    }
    let owner = account
        .strip_suffix(&format!(".{}", dep.factory))
        .unwrap_or_default();
    let valid = account.len() <= 64
        && !owner.is_empty()
        && owner.split('.').all(|p| {
            !p.is_empty()
                && p.bytes()
                    .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-' || b == b'_')
        });
    if !valid {
        return Err(format!(
            "account {account} is not a {} sub-account",
            dep.factory
        ));
    }
    Ok(())
}

/// Steps moving wNEAR or NEAR need a USD price for the cap.
pub fn needs_price(dep: &Deployments, s: &Step) -> bool {
    s.spend().0.is_none_or(|t| t == dep.tokens.wnear)
}

/// `near_usdc`: USDC atoms per 1 NEAR, required when any step `needs_price`.
pub fn check(
    dep: &Deployments,
    steps: &[Step],
    max_step_usd: u64,
    near_usdc: Option<u128>,
) -> Result<(), String> {
    if steps.is_empty() || steps.len() > MAX_STEPS {
        return Err(format!("want 1..={MAX_STEPS} steps"));
    }
    let cap = max_step_usd as u128 * USDC_ONE;
    for (i, s) in steps.iter().enumerate() {
        check_step(dep, s, cap, near_usdc).map_err(|e| format!("step {i}: {e}"))?;
    }
    Ok(())
}

fn check_step(
    dep: &Deployments,
    s: &Step,
    cap: u128,
    near_usdc: Option<u128>,
) -> Result<(), String> {
    let allowed = |t: &str| t == dep.tokens.usdc || t == dep.tokens.wnear;
    if let Step::Swap {
        token_in,
        token_out,
        min_out,
        ..
    } = s
    {
        if token_in == token_out || !allowed(token_out) {
            return Err(format!("swap {token_in} -> {token_out} not allowed"));
        }
        if *min_out == 0 {
            return Err("swap without min_out".into());
        }
    }
    let (token, amount) = s.spend();
    if amount == 0 {
        return Err("zero amount".into());
    }
    let usd = match token {
        Some(t) if t == dep.tokens.usdc => Some(amount),
        Some(t) if !allowed(t) => return Err(format!("token {t} not allowed")),
        _ => amount
            .checked_mul(near_usdc.ok_or("no NEAR price")?)
            .map(|v| v / NEAR_ONE),
    };
    match usd {
        Some(v) if v <= cap => Ok(()),
        _ => Err("over MAX_STEP_USD".into()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    pub fn dep() -> Deployments {
        let mut d = Deployments::load(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/../contracts/deployments/testnet.json"
        ))
        .unwrap();
        d.factory = "factory.nearly.testnet".into();
        d
    }

    const USDC: &str = "usdcc.ft.ref-labs.testnet";
    const P: Option<u128> = Some(3 * USDC_ONE);

    #[test]
    fn account_must_be_factory_child() {
        let d = dep();
        assert!(check_account(&d, "alice.testnet.factory.nearly.testnet").is_ok());
        for bad in [
            "factory.nearly.testnet",
            ".factory.nearly.testnet",
            "evil.testnet",
            "a..factory.nearly.testnet",
            "Bob.factory.nearly.testnet",
            "xfactory.nearly.testnet",
        ] {
            assert!(check_account(&d, bad).is_err(), "{bad}");
        }
        let mut d = dep();
        d.factory.clear();
        assert!(check_account(&d, "alice.testnet.").is_err());
    }

    #[test]
    fn refuses_outside_allowlist_and_cap() {
        let d = dep();
        let ok = |s: Step| check(&d, &[s], 1000, P);
        assert!(ok(Step::Supply {
            token: USDC.into(),
            amount: 1000 * USDC_ONE
        })
        .is_ok());
        assert!(ok(Step::Supply {
            token: USDC.into(),
            amount: 1000 * USDC_ONE + 1
        })
        .is_err());
        assert!(ok(Step::Supply {
            token: "evil.testnet".into(),
            amount: 1
        })
        .is_err());
        assert!(ok(Step::Fee {
            token: USDC.into(),
            amount: 0
        })
        .is_err());
        assert!(ok(Step::Stake {
            amount: 333 * NEAR_ONE
        })
        .is_ok());
        assert!(ok(Step::Stake {
            amount: 334 * NEAR_ONE
        })
        .is_err());
        assert!(ok(Step::Unstake { amount: u128::MAX }).is_err());
        assert!(check(&d, &[Step::Stake { amount: 1 }], 1000, None).is_err());
        let swap = |out: &str, min_out| Step::Swap {
            pool_id: 1,
            token_in: USDC.into(),
            token_out: out.into(),
            amount_in: 1,
            min_out,
        };
        assert!(ok(swap("wrap.testnet", 1)).is_ok());
        assert!(ok(swap("wrap.testnet", 0)).is_err());
        assert!(ok(swap(USDC, 1)).is_err());
        assert!(ok(swap("evil.testnet", 1)).is_err());
        assert!(check(&d, &[], 1000, P).is_err());
    }
}
