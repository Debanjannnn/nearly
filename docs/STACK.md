# NEAR stack reference

Checked 2026-11-11. "RPC" means a `call_function` view run that day against `rpc.<net>.near.org`.
IDs used by code live in `contracts/deployments/{testnet,mainnet}.json`.

## Versions

| Item | Version | Source |
|---|---|---|
| near-sdk | 5.30.1 (MSRV 1.95) | https://crates.io/crates/near-sdk |
| cargo-near | 0.23.0 (MSRV 1.95), build with `cargo near build`, target `wasm32-unknown-unknown` | https://crates.io/crates/cargo-near |
| near-workspaces (tests) | 0.24.0 | https://crates.io/crates/near-workspaces |
| near-api (executor signing) | 0.8.7, edition 2024. `Signer::from_secret_key(sk)` then `Contract(id).call_function(m, args).transaction().with_signer(acc, signer)` | https://crates.io/crates/near-api, https://docs.rs/near-api/0.8.7 |
| near-jsonrpc-client (raw RPC, fallback) | 0.23.0 | https://crates.io/crates/near-jsonrpc-client |
| @hot-labs/near-connect | 0.12.0 | https://www.npmjs.com/package/@hot-labs/near-connect |
| near-api-js | 7.5.3 (has `near-api-js/nep413` `verifyMessage`) | https://www.npmjs.com/package/near-api-js |

## Contract and token IDs

| What | Testnet | Mainnet | Source |
|---|---|---|---|
| USDC | `usdcc.ft.ref-labs.testnet` (6 dec, "b-USDC") | `17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1` (native Circle USDC) | RPC `ft_metadata`; Rhea SDK docs use the mainnet id: https://github.com/rhea-finance/rhea-sdk-docs |
| wNEAR | `wrap.testnet` | `wrap.near` (24 dec) | RPC `ft_metadata`; `wnear_id` in Ref `metadata`; https://github.com/rhea-finance/rhea-sdk/blob/main/src/constant.ts |
| Rhea lending (Burrow) | `burrow.services.ref-labs.testnet` | `contract.main.burrow.near` | RPC `get_config` (mainnet `ref_exchange_id`=`v2.ref-finance.near`, testnet `ref_exchange_id`=`ref-finance-101.testnet`; Ref testnet `metadata.burrowland_id` points here) |
| Rhea DEX (Ref v2) | `ref-finance-101.testnet` (v1.9.19) | `v2.ref-finance.near` (v1.9.21) | https://github.com/rhea-finance/rhea-sdk/blob/main/src/constant.ts, RPC `metadata` |
| Rhea aggregator (optional) | none | `aggregatedex.near`, path API `GET https://smartx.rhea.finance/swapMultiDexPath` | https://github.com/rhea-finance/rhea-sdk-docs/blob/main/SmartRouterV2(Aggregated%20DEX).md |
| LST (LiNEAR) | `linear-protocol.testnet` | `linear-protocol.near` | https://github.com/linear-protocol/LiNEAR |
| USDC/wNEAR pool | `1900` (SIMPLE, thin: ~75 wNEAR / ~1.1k USDC) | `4512` (SIMPLE, 0.30% fee, top TVL) | RPC `get_pool`; https://api.ref.finance/list-top-pools |
| LiNEAR/wNEAR pool (LST exit) | UNCONFIRMED | `3515` (RATED_SWAP) | https://api.ref.finance/list-top-pools |

Older testnet Burrow `contract.1638481328.burrow.testnet` (uses `usdc.fakes.testnet`, from the Burrow README) still runs, but it is not wired to Ref testnet or Pyth, so it is not used.

## Rhea lending (Burrow)

| Action | Call | Source |
|---|---|---|
| Supply | `<token>.ft_transfer_call{receiver_id: burrow, amount, msg: ""}`, 1 yocto | https://github.com/NearDeFi/burrowland#readme |
| Supply as collateral | same, `msg: "{\"Execute\":{\"actions\":[{\"IncreaseCollateral\":{\"token_id\":\"<t>\"}}]}}"` | same |
| Withdraw | `burrow.execute{actions:[{"Withdraw":{"token_id":"<t>","amount":"<opt>"}}]}`, 1 yocto. No oracle needed from plain supply | same |
| Views | `get_asset{token_id}`, `get_assets_paged{}`, `get_account{account_id}`, `get_config{}` | same, RPC |
| APY | `get_asset().supply_apr` (decimal string, e.g. USDC 0.104 = 10.4%). Farm rewards in `farms` on top | RPC |

## Rhea DEX (Ref)

| Action | Call | Source |
|---|---|---|
| Swap | `<token_in>.ft_transfer_call{receiver_id: ref, amount, msg: "{\"force\":0,\"actions\":[{\"pool_id\":4512,\"token_in\":\"..\",\"token_out\":\"..\",\"amount_in\":\"..\",\"min_amount_out\":\"..\"}]}"}`, 1 yocto. Register `token_out` storage for the receiver first | https://github.com/rhea-finance/rhea-sdk/blob/main/src/v1-swap/instantSwap.ts |
| Quote | view `get_return{pool_id, token_in, amount_in, token_out}` (checked live on both nets) | RPC |
| Views | `get_pool{pool_id}`, `get_pools{from_index, limit}`, `metadata{}` | RPC |
| Pool list / TVL | `https://api.ref.finance/list-top-pools`, `https://indexer.ref.finance` (mainnet), `https://testnet-indexer.ref-finance.com` (testnet, UNCONFIRMED live) | rhea-sdk `constant.ts` |
| Where output lands | UNCONFIRMED: sent back to sender vs kept in Ref internal balance (then `withdraw{token_id, amount}`, 1 yocto). Test on testnet before relying on it | |

## LST: LiNEAR (pick)

Picked over stNEAR/NEARX: same id pattern on both nets, accepted as Burrow collateral on mainnet and testnet, deep Ref pool for instant exit.

| Action | Call | Source |
|---|---|---|
| Stake | `deposit_and_stake()` payable (attach NEAR) | https://github.com/linear-protocol/LiNEAR/blob/main/contracts/linear/src/stake.rs |
| Unstake (delayed, ~4 epochs) | `unstake{amount}` / `unstake_all()`, then `withdraw{amount}` / `withdraw_all()` | same |
| Instant exit | swap LiNEAR to wNEAR on Ref pool 3515 | api.ref.finance |
| Views | `ft_price()` (NEAR per LiNEAR, 24 dec), `get_summary()`, `get_account_details{account_id}` | https://github.com/linear-protocol/LiNEAR/blob/main/contracts/linear/src/view.rs, RPC |
| APY source | derive from `ft_price` growth between two snapshots (annualise). No official APY endpoint found: UNCONFIRMED | |

Note: `deposit_and_stake` needs attached NEAR, so the account contract calls it itself (see keys below).

## NEAR Intents 1Click (mainnet only)

| Item | Value | Source |
|---|---|---|
| Base | `https://1click.chaindefuser.com` | https://docs.near-intents.org/integration/distribution-channels/1click-api/quickstart/making-a-request.md |
| Endpoints | `GET /v0/tokens`, `POST /v0/quote`, `POST /v0/deposit/submit`, `GET /v0/status?depositAddress=..[&depositMemo=..]`, `GET /v0/any-input/withdrawals` | same, https://docs.near-intents.org/llms.txt |
| Asset ids | `nep141:<token account>`, e.g. `nep141:wrap.near`, `nep141:17208628..a1` | same |
| `dry: true` | validates + returns a quote, no `depositAddress`, nothing executes. Live dry USDC->wNEAR quote succeeded unauthenticated | same, live call |
| Real flow | `dry:false` quote -> `ft_transfer` to `depositAddress` (ORIGIN_CHAIN) -> optional `/v0/deposit/submit` -> poll status | same |
| Receive on NEAR account | `recipient: "<acct>.near"`, `recipientType: "DESTINATION_CHAIN"` (to the account), or `"INTENTS"` (stays in intents.near balance). `refundType`: `ORIGIN_CHAIN`/`INTENTS` | same |
| swapType | `EXACT_INPUT`, `EXACT_OUTPUT`, `FLEX_INPUT`, `ANY_INPUT` | https://docs.near-intents.org/integration/distribution-channels/1click-api/swap-types.md |
| Status values | `PENDING_DEPOSIT`, `KNOWN_DEPOSIT_TX`, `INCOMPLETE_DEPOSIT`, `PROCESSING`, `SUCCESS`, `REFUNDED`, `FAILED` | https://docs.near-intents.org/api-reference/oneclick/check-swap-execution-status.md |
| Auth (Q-006) | JWT from https://partners.near-intents.org, sent as `X-API-Key: <jwt>` or `Authorization: Bearer <jwt>` | https://docs.near-intents.org/integration/distribution-channels/1click-api/authentication.md |
| Fees (Q-007) | no JWT: +0.25% (25 bps) on every non-ANY_INPUT quote. With JWT, no appFees: 0.20%, or 0.01% on stablecoin / same-asset routes. Plus 0.0001% protocol fee. The 0.2% figure is the near-intents.org UI fee | https://docs.near-intents.org/resources/fees.md |
| Withdraw out of Intents balance | UNCONFIRMED: use `recipientType: DESTINATION_CHAIN` so nothing is left in intents.near; manual `ft_withdraw` on `intents.near` not checked | |
| Testnet | none (mainnet only) | issue #1 |

## Perps on NEAR

No NEAR-native on-chain perps venue a contract can call. near.com perps (Sept 2026) execute on Hyperliquid with Intents routing collateral; Veax perps run on Orderly. Both off-chain venues, not checked against an official doc: UNCONFIRMED. Decision: funding basis stays cut.

## Wallet: @hot-labs/near-connect 0.12.0

| API | Shape | Source |
|---|---|---|
| Create | `new NearConnector({ network?, features?, signIn?: {contractId, methods} })` | package `README.md`, `build/types/index.d.ts` |
| Connect | `await connector.connect({ signMessageParams?, addFunctionCallKey? })`; events `wallet:signIn`, `wallet:signInAndSignMessage`, `wallet:signOut` via `connector.on(..)`; `await connector.wallet()` | same |
| Tx | `wallet.signAndSendTransaction({receiverId, actions})`, `wallet.signAndSendTransactions({transactions})` (wallet-selector action format) | same |
| Sign | `wallet.signMessage({message, recipient, nonce: Uint8Array(32)})` -> NEP-413 `SignedMessage` | same |
| FC key | `addFunctionCallKey: {contractId, publicKey: "ed25519:..", allowMethods, gasAllowance?}` (default allowance 0.25 NEAR); only wallets with `signInWithFunctionCallKey` | same |

## NEP-413 verify (backend auth)

1. `payload = Payload{ message: string, nonce: [u8;32], recipient: string, callbackUrl: Option<string> }`, Borsh serialise.
2. Prepend Borsh `u32` tag `2147484061` (2^31 + 413).
3. `sha256(tag || payload)`; check ed25519 signature against `publicKey`.
4. Check `publicKey` is a **full-access** key of `accountId` (RPC `view_access_key`), and nonce/recipient match the issued challenge.

Source: https://github.com/near/NEPs/blob/master/neps/nep-0413.md, https://docs.near.org/web3-apps/backend/backend.md (or `verifyMessage` from `near-api-js/nep413`).

## Keys and deposits (design constraint)

| Fact | Source |
|---|---|
| Function-call keys cannot attach NEAR (deposit must be 0) and only call one contract (optionally listed methods) | https://docs.near.org/protocol/accounts-contracts/access-keys.md |
| `ft_transfer` / `ft_transfer_call` require exactly 1 yocto | https://github.com/near/NEPs/blob/master/neps/nep-0141.md |
| Contract-added FC key (`promise_batch_action_add_key_with_function_call`): allowance 0 means unlimited | access-keys doc |

So the executor key calls a non-payable method on the user's account contract, and the contract itself attaches the 1 yocto (and NEAR for `deposit_and_stake`, `storage_deposit`) in its outgoing promises. Burrow `execute` and Ref `withdraw` also want 1 yocto.

## RPC and data

| Item | Testnet | Mainnet | Source |
|---|---|---|---|
| RPC (dev) | `https://rpc.testnet.near.org` | `https://rpc.mainnet.near.org` | https://docs.near.org/api/rpc/providers.md |
| RPC (free, less limited) | `https://test.rpc.fastnear.com` | `https://free.rpc.fastnear.com` | same |
| Archival | `https://archival-rpc.testnet.near.org` (severely rate limited) | `https://archival-rpc.mainnet.near.org` | same |
| Account txns by method (free) | UNCONFIRMED (`api-testnet.nearblocks.io` returned 410) | `https://api.nearblocks.io/v1/account/<id>/txns?method=..` | https://docs.near.org/data-infrastructure/data-api.md |
| Balances (free) | UNCONFIRMED | `https://api.fastnear.com/v1/account/<id>/ft` | same |
| Events | contracts emit NEP-297 `EVENT_JSON:` logs; read from tx receipts or the indexers above | https://docs.near.org/smart-contracts/anatomy/events.md |

## UNCONFIRMED

- Testnet USDC faucet/mint for `usdcc.ft.ref-labs.testnet`.
- Ref swap via `ft_transfer_call`: output returned to sender vs kept in Ref internal balance.
- LiNEAR official APY endpoint (using `ft_price` deltas instead).
- Testnet LiNEAR/wNEAR pool id.
- 1Click: withdraw from an `INTENTS` balance; `/v0/any-input/withdrawals` path not exercised.
- Perps venues (near.com/Hyperliquid, Veax/Orderly): news only.
- Free testnet indexer API (NearBlocks testnet 410; FastNEAR testnet not tested); `testnet-indexer.ref-finance.com` liveness.
