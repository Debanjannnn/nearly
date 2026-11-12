![Nearly: The Yield Bearing Basket, live on NEAR](docs/banner.png)

# Nearly

![NEAR](https://img.shields.io/badge/NEAR-testnet-000000?logo=near&logoColor=white)
![Rust](https://img.shields.io/badge/contracts-Rust-B7410E?logo=rust&logoColor=white)
![Go](https://img.shields.io/badge/services-Go-00ADD8?logo=go&logoColor=white)
![Next.js](https://img.shields.io/badge/app-Next.js-000000?logo=nextdotjs&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-7-DC382D?logo=redis&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-compose-2496ED?logo=docker&logoColor=white)
![License: MIT](https://img.shields.io/badge/license-MIT-blue)
![Contract tests](https://img.shields.io/badge/contract%20tests-71%20passing-2EA043)

Nearly lets you put money into yield baskets on NEAR. You pick a mix of lending, liquidity and liquid staking. A bot runs and rebalances the basket, but it **cannot take your funds**. The rules are enforced by your own NEAR smart-contract account, not by our server.

Built at NEAR HACKATHON (2026-11-01 → 2026-11-14). Tracks: **`defi`** and **`security`**.

## For judges: check it in 2 minutes

No Docker or secrets needed for these:

```sh
scripts/verify-testnet.sh    # every deployed account exists and has code
make contracts-test          # 71 contract tests: policy, caps, pools, 24h limit, fee budget, revoke, migrate, sandbox runs
```

| Track | What to look at |
|---|---|
| `defi` | Baskets across Rhea lending, the Rhea DEX and liquid staking; backtests; deposit and withdraw from any chain via NEAR Intents 1Click |
| `security` | The bot holds a function-call key limited to `execute_step`, and the contract checks each step: token and pool whitelists, per-step cap, **rolling 24h limit**, no free-form receiver, owner kill switch |

## The problem

Automated DeFi strategies need a bot that signs transactions. Usually that means handing it your keys or depositing into a shared vault, and either way you're trusting the operator. One bug or leaked server key and the money is gone.

## What Nearly does

- **One account per user.** The factory creates `alice-testnet.factory.nearly-dev-c7dc68.testnet` (for `alice.testnet`), a contract account that only you own.
- **The bot gets a function-call key, not your keys.** That key can call exactly one method, `execute_step`, with a limited gas allowance.
- **The contract checks every step.** A step can only move whitelisted tokens into Rhea lending, whitelisted Rhea DEX pools, liquid staking, the treasury fee, or back to you. Each step has a cap, each token has a rolling 24-hour limit, and swaps need a nonzero `min_out`. No step can name an arbitrary receiver.
- **You can switch the bot off at any time.** `revoke_executor` deletes its key on chain.
- **Deposit or withdraw from any chain** through NEAR Intents 1Click.

## Threat model: what a leaked bot key can do

| An attacker with the bot's key… | Result | Enforced at |
|---|---|---|
| sends funds to their own account | ❌ impossible: no step type has a receiver field | `contracts/account/src/step.rs:4` |
| calls any other method (`withdraw`, `set_*`, …) | ❌ the key only allows `execute_step` | `contracts/factory/src/lib.rs:108` |
| moves a non-whitelisted token | ❌ rejected | `contracts/account/src/policy.rs:29` |
| swaps through an attacker's pool | ❌ `pool not whitelisted`; swaps also need `min_out` > 0 | `contracts/account/src/policy.rs:64` |
| moves more than the cap in one step | ❌ `over cap` | `contracts/account/src/policy.rs:29` |
| repeats steps to drain over time | ❌ `over daily cap`: at most `daily_steps` × cap per token in **any** 24 hours (default 10), and at most 64 steps | `contracts/account/src/limit.rs:12` |
| keeps acting after you notice | ❌ `revoke_executor` deletes the key, so the bot can't sign at all; a contract flag (`executor revoked`) blocks it as a backup | `contracts/account/src/owner.rs:109` |
| pays itself through treasury fees | ❌ fees are capped at 0.4% of the token's 24h limit, a constant nobody can raise | `contracts/account/src/policy.rs:54` |
| accepts a poor price on a whitelisted pool | ⚠️ price slippage on whitelisted pools, bounded by the per-step and 24h caps (see Known limits) | — |

## How NEAR is used

| NEAR feature | Where | Why it matters |
|---|---|---|
| Sub-accounts + factory | `contracts/factory/src/lib.rs:72` `create_account` | Each user gets their own on-chain account; there's no shared vault |
| Function-call access keys | `contracts/factory/src/lib.rs:108` `add_access_key_allowance(…, "execute_step")` | The bot can call one method, with capped gas |
| On-chain policy engine | `contracts/account/src/policy.rs:16` `assert_executor`, `:29` `assert_cap`, `:45` `check_step`; `limit.rs:12` rolling 24h window | Limits are enforced by the contract, not the server |
| No-receiver step type | `contracts/account/src/step.rs:4` | Fund destinations are fixed in code |
| Owner controls | `contracts/account/src/owner.rs:94` `set_daily_steps`, `:102` `set_pools`, `:109` `revoke_executor` | The owner sets the limits and pools, and can remove the bot's key in one call |
| NEAR Intents 1Click | `internal/shared/intents/`, `services/api/internal/api/intents.go` | Deposit from or withdraw to other chains |
| Rhea lending + DEX, liquid staking | `contracts/deployments/testnet.json` | Where the yield comes from |
| NEP-413 sign-in | `services/api/internal/auth/` | Wallet login without passwords |
| NEAR Connect wallet | `client/` (`@hot-labs/near-connect`) | Connect an existing NEAR wallet |

## Architecture

```mermaid
flowchart TD
  U([User · NEAR wallet])
  U -->|NEP-413 sign-in| C[client · Next.js]
  U -->|signs create_account| F[factory contract]
  C --> API[api · Go]
  API --> S[scheduler · Go]
  API --> M[market-data + history · Go]
  API <-->|any-chain in/out| I[NEAR Intents 1Click]
  S --> X[executor · Rust<br/>function-call key]
  F -->|deploys + adds bot key| A[your account contract<br/>token + pool whitelist · caps · 24h limit]
  X -->|execute_step only| A
  A --> L[(Rhea lending)]
  A --> D[(Rhea DEX)]
  A --> ST[(LiNEAR staking)]
  A --> T[(treasury fee)]
  A -.->|events| IX[indexer · Go]
  IX --> DB[(PostgreSQL)]
```

Funds leave your account only to these venues, the treasury fee, or back to you (`to_owner`, `withdraw`).

| Path | Role |
|---|---|
| `contracts/account` | Per-user account contract: holds funds, enforces the policy on chain |
| `contracts/factory` | Creates user sub-accounts, deploys `account`, adds the bot's function-call key |
| `contracts/mocks/*` | Sandbox-only venue mocks for tests |
| `executor/` | Rust signer: turns planned steps into `execute_step` transactions; adds an off-chain USD cap (`MAX_STEP_USD`) and requires `min_out` on swaps |
| `services/api` | User API: auth, baskets, deposit/withdraw, Intents quotes, schedules |
| `services/scheduler` | DCA, recurring buys, rebalancing |
| `services/market-data` | Prices, rates, quotes, WebSocket feed |
| `services/history` | Historical rates and backtests |
| `services/indexer` | Indexes account contract events |
| `internal/shared` | Shared Go packages: config, HTTP, Postgres, NEAR RPC, planner, Intents |
| `client/` | Next.js app |
| `migrations/` | Postgres schema |

## Contract addresses (NEAR testnet)

| Role | Address | Explorer |
|---|---|---|
| Factory (ours) | `factory.nearly-dev-c7dc68.testnet` | [nearblocks](https://testnet.nearblocks.io/address/factory.nearly-dev-c7dc68.testnet) |
| Treasury (ours) | `treasury.nearly-dev-c7dc68.testnet` | [nearblocks](https://testnet.nearblocks.io/address/treasury.nearly-dev-c7dc68.testnet) |
| Root / deployer (ours) | `nearly-dev-c7dc68.testnet` | [nearblocks](https://testnet.nearblocks.io/address/nearly-dev-c7dc68.testnet) |
| User accounts (ours) | `<owner>.factory.nearly-dev-c7dc68.testnet` | one per user, created by the factory |
| Rhea lending | `burrow.services.ref-labs.testnet` | [nearblocks](https://testnet.nearblocks.io/address/burrow.services.ref-labs.testnet) |
| Rhea DEX | `ref-finance-101.testnet` | [nearblocks](https://testnet.nearblocks.io/address/ref-finance-101.testnet) |
| Liquid staking (LiNEAR) | `linear-protocol.testnet` | [nearblocks](https://testnet.nearblocks.io/address/linear-protocol.testnet) |
| USDC (test) | `usdcc.ft.ref-labs.testnet` | [nearblocks](https://testnet.nearblocks.io/address/usdcc.ft.ref-labs.testnet) |
| wNEAR | `wrap.testnet` | [nearblocks](https://testnet.nearblocks.io/address/wrap.testnet) |

Source of truth: `contracts/deployments/testnet.json`. Check with `scripts/verify-testnet.sh`; redeploy with `scripts/deploy-testnet.sh`.

## On-chain proof (NEAR testnet)

Real transactions from the demo account `nearly-dev-c7dc68-testnet.factory.nearly-dev-c7dc68.testnet`, signed by the bot's function-call key:

| What | Result | Transaction |
|---|---|---|
| Bot runs a `stake` step (0.1 wNEAR → LiNEAR) | ✅ succeeds | [`GtzHERVS…`](https://testnet.nearblocks.io/txns/GtzHERVSbqpHvtmB2PFtHzEdZeS7HkRAE7YCyDKU4UnA) |
| Bot tries to unstake 1000 NEAR (cap is 100) | ❌ `Smart contract panicked: over cap` | [`36tY4UZf…`](https://testnet.nearblocks.io/txns/36tY4UZfR3U9BU3BannDU9RkQbErjD7qZm9EAxayMJSP) |
| Factory upgrade: new code + `migrate` adding the pool whitelist, in one transaction | ✅ succeeds | [`EWDdvv9n…`](https://testnet.nearblocks.io/txns/EWDdvv9nHfLEugj2vNuUkB99tSb1hCaGNYR6kN3RNhZN) |

To check the rejection yourself:

```sh
curl -s https://archival-rpc.testnet.near.org -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tx","params":["36tY4UZfR3U9BU3BannDU9RkQbErjD7qZm9EAxayMJSP","nearly-dev-c7dc68-testnet.factory.nearly-dev-c7dc68.testnet"]}' \
  | jq '.result.receipts_outcome[0].outcome.status'
```

This demo account was created before the pool whitelist, rolling limit and fee budget existed, and accounts keep the code they were created with. New accounts from the factory get all of them.

## Try the app (testnet)

Prerequisites: Docker, Go 1.25+, Rust + `cargo-near`, Bun, and a NEAR testnet wallet.

```sh
cp .env.example .env          # fill EXECUTOR_*, INTERNAL_SECRET, SESSION_SECRET, NEARLY_ROOT_ACCOUNT
make up                       # postgres, redis, api, executor, scheduler, market-data, history, indexer
make smoke                    # every service answers GET /health
cd client && cp .env.example .env.local && bun install && bun run dev   # http://localhost:3000
```

Then in the app:

1. **Connect your wallet**, then sign in by signing a message (no password).
   → Nearly asks you to sign the factory `create_account` transaction. Your account (for example `alice-testnet.factory.nearly-dev-c7dc68.testnet`) appears on testnet.
2. **Go to Baskets → New** and build a mix (for example USDC lending + wNEAR staking), then click **Backtest**.
   → You see historical APY and a chart for that mix.
3. **Open the basket, deposit, and deploy.**
   → The executor runs the steps. **Portfolio → Executions** lists each run with its transaction hash.
4. **Go to Portfolio → Revoke.**
   → The bot's key is deleted from your account, so the bot can no longer sign any transaction for it.
5. **Go to Risk.**
   → You see your exposure by venue and token.

Deposit and withdraw **from any chain** (in the wallet menu) use NEAR Intents 1Click. Intents has no testnet, so live transfers run on mainnet only (HACKATHON.md Q-007). Dry-run quotes work without moving funds.

## Verify the security claims

Every claim maps to a test you can run:

| Claim | Test (`contracts/account/src/tests.rs`) |
|---|---|
| No step can send funds to an arbitrary account | `no_step_can_name_a_receiver` (line 440) |
| Only the bot's key, signing on the account itself, can run steps | `executor_must_be_self`, `executor_wrong_key` (line 179) |
| Steps over the cap are rejected | `over_cap_rejected` (line 190), `stake_over_near_cap_rejected` |
| Repeated steps stop at the 24h limit, measured as a rolling window, not calendar days | `over_daily_cap_rejected` (line 310), `daily_cap_is_rolling_not_calendar` (line 342), `daily_cap_resets_next_day`, `spend_log_is_bounded` |
| Swaps only through whitelisted pools, never with `min_out` of 0 | `swap_on_unknown_pool_rejected` (line 376), `swap_without_min_out_rejected` (line 384) |
| Treasury fees stay within 0.4% of the 24h limit | `fees_over_budget_rejected` (line 415), `fee_budget_scales_with_daily_steps` |
| Only the owner can change the limit or the pools | `daily_steps_only_owner`, `pools_only_owner` |
| Non-whitelisted tokens are rejected | `non_whitelisted_token_rejected`, `swap_to_non_whitelisted_rejected` |
| Revoke works and only the owner can do it | `revoked_executor_rejected` (line 248), `revoke_only_owner` |
| Failed venue calls refund and book nothing | `failed_transfer_is_refunded`, `failed_swap_refunds_and_books_nothing` |
| Contract against real venue contracts | `contracts/factory/tests/sandbox.rs`, `contracts/factory/tests/venues.rs` (near-workspaces sandbox) |

```sh
make contracts-test   # 71 contract tests: 56 account, 9 factory, 6 sandbox
go test ./...         # Go services and shared packages
cd executor && cargo test
```

CI (`.github/workflows/ci.yml`) runs Go vet + tests (unit and integration), executor tests, contract tests, a Docker build, and client typecheck + lint.

## Known limits

- The bot sets each swap's `min_out` from a live quote. A compromised bot could accept a worse price on a whitelisted pool, but only within the per-step and 24h limits.
- NEAR Intents has no testnet, so live cross-chain transfers are mainnet-only. On testnet you get dry-run quotes.
- Perps and limit orders are out of scope.

## Built with

near-sdk (Rust), near-workspaces, Go, PostgreSQL, Redis, Next.js 16, React 19, `@hot-labs/near-connect`, Rhea, NEAR Intents 1Click. Parts of the code were written with AI coding assistants (Claude Code). All code was written during the hackathon window.

## License

[MIT](LICENSE)
