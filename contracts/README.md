# contracts

NEAR smart contracts (Rust, near-sdk).

- `account/`: per-user account contract (`<user>.nearly.<root>`). Holds basket funds and enforces the policy on chain: venue allowlist, per-step cap, withdrawals only to the owner. The executor drives it with a function-call access key.
- `factory/`: creates user sub-accounts and deploys `account` into them.
- `deployments/`: contract IDs the system may touch, per network.
