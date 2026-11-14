# Mainnet plan

Nearly runs on NEAR testnet today. This is the planned path to mainnet after the hackathon. None of it is live yet.

The contracts don't depend on the network, so mainnet is a config change plus hardening.

## 1. Deploy config

Add `contracts/deployments/mainnet.json` with the mainnet venues:

| Role | Mainnet account |
|---|---|
| Rhea lending | `contract.main.burrow.near` |
| Rhea DEX | `v2.ref-finance.near` |
| Liquid staking (LiNEAR) | `linear-protocol.near` |
| wNEAR | `wrap.near` |
| USDC | `17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1` |

Whitelist only audited pools with deep liquidity.

## 2. Reproducible builds

Build with `cargo near build reproducible-wasm` so anyone can match the deployed code hash to the source.

## 3. Audit

Get an external review of `contracts/account` (policy checks, rolling 24h limit, settle callbacks) before it holds real funds.

## 4. Conservative defaults

Ship low per-step caps and a low `daily_steps`. Each owner raises them for their own account.

## 5. NEAR Intents 1Click

1Click already runs on mainnet, so any-chain deposit and withdraw work as they are. A partner JWT removes the 0.2% fee.
