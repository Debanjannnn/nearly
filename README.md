# Nearly

Nearly lets you put money into yield baskets on NEAR. You pick a mix of lending, liquidity and liquid staking. A bot runs and rebalances the basket, but it **cannot take your funds**. The rules are enforced by your own NEAR smart-contract account, not by our server.

Built at NEAR HACKATHON (2026-11-01 → 2026-11-14). Tracks: **`defi`** and **`security`**.

## The problem

Automated DeFi strategies need a bot that signs transactions. Usually that means handing it your keys or depositing into a shared vault, and either way you're trusting the operator. One bug or leaked server key and the money is gone.

## What Nearly does

- **One account per user.** The factory creates `alice-testnet.factory.nearly-dev-c7dc68.testnet` (for `alice.testnet`), a contract account that only you own.
- **The bot gets a function-call key, not your keys.** That key can call exactly one method, `execute_step`, with a limited gas allowance.
- **The contract checks every step.** A step can only move whitelisted tokens, under a per-step cap, into Rhea lending, the Rhea DEX, liquid staking, the treasury fee, or back to you. No step can name an arbitrary receiver.
- **You can switch the bot off at any time.** `revoke_executor` deletes its key on chain.
- **Deposit or withdraw from any chain** through NEAR Intents 1Click.

## How NEAR is used

| NEAR feature | Where | Why it matters |
|---|---|---|
| Sub-accounts + factory | `contracts/factory/src/lib.rs:69` `create_account` | Each user gets their own on-chain account; there's no shared vault |
| Function-call access keys | `contracts/factory/src/lib.rs:104` `add_access_key_allowance(…, "execute_step")` | The bot can call one method, with capped gas |
| On-chain policy engine | `contracts/account/src/policy.rs:13` `assert_executor`, `:26` `assert_cap`, `:38` `check_step` | Limits are enforced by the contract, not the server |
| No-receiver step type | `contracts/account/src/step.rs:4` | Fund destinations are fixed in code |
| Owner kill switch | `contracts/account/src/owner.rs:94` `revoke_executor` | Removes the bot's key in one call |
| NEAR Intents 1Click | `internal/shared/intents/`, `services/api/internal/api/intents.go` | Deposit from or withdraw to other chains |
| Rhea lending + DEX, liquid staking | `contracts/deployments/testnet.json` (venues) | Where the yield actually comes from |
| NEP-413 sign-in | `services/api/internal/auth/` | Wallet login without passwords |
| NEAR Connect wallet | `client/` (`@hot-labs/near-connect`) | Connect an existing NEAR wallet |

## Try it (testnet)

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
   → The bot's key is deleted. Any later step fails on chain with `executor revoked`.
5. **Go to Risk.**
   → You see your exposure by venue and token.

Deposit and withdraw **from any chain** (in the wallet menu) use NEAR Intents 1Click. Intents has no testnet, so live transfers run on mainnet only (HACKATHON.md Q-007). Dry-run quotes work without moving funds.

## Testnet deployment

| Role | Account |
|---|---|
| Factory | [`factory.nearly-dev-c7dc68.testnet`](https://testnet.nearblocks.io/address/factory.nearly-dev-c7dc68.testnet) |
| Treasury | `treasury.nearly-dev-c7dc68.testnet` |
| Lending (Rhea) | `burrow.services.ref-labs.testnet` |
| DEX (Rhea) | `ref-finance-101.testnet` |
| Liquid staking | `linear-protocol.testnet` |
| Tokens | `usdcc.ft.ref-labs.testnet`, `wrap.testnet` |

To check that every account exists and has code, run `scripts/verify-testnet.sh`. To redeploy, run `scripts/deploy-testnet.sh`.

## Verify the security claims

Every claim maps to a test you can run:

| Claim | Test (`contracts/account/src/tests.rs`) |
|---|---|
| No step can send funds to an arbitrary account | `no_step_can_name_a_receiver` (line 291) |
| Only the bot's key, signing on the account itself, can run steps | `executor_must_be_self`, `executor_wrong_key` (line 178) |
| Steps over the cap are rejected | `over_cap_rejected` (line 189), `stake_over_near_cap_rejected` |
| Non-whitelisted tokens are rejected | `non_whitelisted_token_rejected`, `swap_to_non_whitelisted_rejected` |
| Revoke works and only the owner can do it | `revoked_executor_rejected` (line 247), `revoke_only_owner` |
| Failed venue calls refund and book nothing | `failed_transfer_is_refunded`, `failed_swap_refunds_and_books_nothing` |
| Contract against real venue contracts | `contracts/factory/tests/sandbox.rs`, `contracts/factory/tests/venues.rs` (near-workspaces sandbox) |

```sh
make contracts-test   # 58 contract tests: 44 account, 8 factory, 6 sandbox
go test ./...         # Go services and shared packages
cd executor && cargo test
```

CI (`.github/workflows/ci.yml`) runs Go vet + tests (unit and integration), executor tests, contract tests, a Docker build, and client typecheck + lint.

## Architecture

```
wallet ──sign──▶ client (Next.js) ──▶ api (Go) ──▶ scheduler (Go) ──▶ executor (Rust)
                                       │                               │ function-call key
                                       ▼                               ▼
                                   postgres ◀── indexer ◀──── <owner>.factory… (account contract)
                                                                       │ policy-checked steps
                                                                       ▼
                                                Rhea lending · Rhea DEX · liquid staking · treasury
```

| Path | Role |
|---|---|
| `contracts/account` | Per-user account contract: holds funds, enforces the policy on chain |
| `contracts/factory` | Creates user sub-accounts, deploys `account`, adds the bot's function-call key |
| `contracts/mocks/*` | Sandbox-only venue mocks for tests |
| `executor/` | Rust signer: turns planned steps into `execute_step` transactions, with its own USD cap (`MAX_STEP_USD`) |
| `services/api` | User API: auth, baskets, deposit/withdraw, Intents quotes, schedules |
| `services/scheduler` | DCA, recurring buys, rebalancing |
| `services/market-data` | Prices, rates, quotes, WebSocket feed |
| `services/history` | Historical rates and backtests |
| `services/indexer` | Indexes account contract events |
| `internal/shared` | Shared Go packages: config, HTTP, Postgres, NEAR RPC, planner, Intents |
| `client/` | Next.js app |
| `migrations/` | Postgres schema |

## Known limits

- The cap applies to each step; there is no rolling daily limit yet.
- NEAR Intents has no testnet, so live cross-chain transfers are mainnet-only. On testnet you get dry-run quotes.
- Perps and limit orders are out of scope.

## Built with

near-sdk (Rust), near-workspaces, Go, Postgres, Redis, Next.js 16, React 19, `@hot-labs/near-connect`, Rhea, NEAR Intents 1Click. Parts of the code were written with AI coding assistants (Claude Code). All code was written during the hackathon window.

## License

[PolyForm Strict 1.0.0](LICENSE)
