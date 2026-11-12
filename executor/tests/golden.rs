//! Planner golden fixtures: every planned step must parse, round-trip byte-equal and pass the recheck.
use nearly_executor::{config::Deployments, policy, steps::Step};
use serde_json::Value;

const ROOT: &str = concat!(env!("CARGO_MANIFEST_DIR"), "/..");

#[test]
fn planner_fixtures_pass_recheck() {
    let mut dep = Deployments::load(&format!("{ROOT}/contracts/deployments/testnet.json")).unwrap();
    dep.factory = "factory.nearly.testnet".into();
    let files: Vec<_> = std::fs::read_dir(format!("{ROOT}/testdata/planner"))
        .unwrap()
        .map(|e| e.unwrap().path())
        .collect();
    assert!(files.len() >= 4, "planner fixtures missing");
    for f in files {
        let g: Value = serde_json::from_str(&std::fs::read_to_string(&f).unwrap()).unwrap();
        for key in ["steps", "plan"] {
            let Some(raw) = g.get(key) else { continue };
            let steps: Vec<Step> =
                serde_json::from_value(raw.clone()).unwrap_or_else(|e| panic!("{f:?} {key}: {e}"));
            assert_eq!(
                &serde_json::to_value(&steps).unwrap(),
                raw,
                "{f:?} {key} round trip"
            );
            // fixture prices 1 wNEAR at 3 USDC
            policy::check(&dep, &steps, 1000, Some(3_000_000))
                .unwrap_or_else(|e| panic!("{f:?} {key}: {e}"));
            assert!(
                policy::check(&dep, &steps, 1, Some(3_000_000)).is_err(),
                "{f:?} {key}: cap not enforced"
            );
        }
    }
}
