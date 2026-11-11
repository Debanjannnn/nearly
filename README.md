# nearly

## Layout

| Path | Role |
|------|------|
| `contracts/account` | Per-user account contract: holds funds, enforces policy on chain |
| `contracts/factory` | Creates user sub-accounts, deploys `account` |
| `executor/` | Rust signer; drives user accounts with a function-call access key |
| `executor/src/venues` | Rhea lending/DEX, liquid staking, NEAR Intents 1Click |
| `services/api` | User API: baskets, deposit, withdraw, schedules |
| `services/scheduler` | DCA, recurring buys, rebalancing |
| `services/market-data` | Prices, rates, quotes |
| `services/history` | Historical rates and backtests |
| `internal/shared` | Shared Go packages (config, http, postgres, NEAR RPC, planner) |
| `indexer/` | Indexes user account events |
| `client/` | Next.js app (NEAR wallet selector) |
| `migrations/` | Postgres schema |

## Status

Scaffold only.
