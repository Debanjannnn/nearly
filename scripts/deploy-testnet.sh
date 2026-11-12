#!/usr/bin/env bash
# Testnet only: deploys the factory under `root` of contracts/deployments/testnet.json, writes ids back.
# Needs near-cli-rs + root key in ~/.near-credentials/testnet.
set -euo pipefail
cd "$(dirname "$0")/.."
DEP=contracts/deployments/testnet.json
NET=(network-config testnet)
ROOT=$(jq -r .root "$DEP")
FACTORY=factory.$ROOT
TREASURY=treasury.$ROOT
[ -n "$ROOT" ] || { echo "root missing in $DEP"; exit 1; }

exists() { near account view-account-summary "$1" "${NET[@]}" now >/dev/null 2>&1; }
sub() {
  exists "$1" || near account create-account fund-myself "$1" "$2" autogenerate-new-keypair \
    save-to-legacy-keychain sign-as "$ROOT" "${NET[@]}" sign-with-legacy-keychain send
}

make contracts
sub "$TREASURY" '0.1 NEAR'
sub "$FACTORY" '5.5 NEAR'

# caps: 1k USDC (6 dec), 100 wNEAR (24 dec), 100 NEAR staking per step
INIT=$(jq -c --arg owner "$ROOT" --arg t "$TREASURY" '{owner: $owner, config: {
  treasury: $t, venues: .venues,
  tokens: [[.tokens.usdc, "1000000000"], [.tokens.wnear, "100000000000000000000000000"]],
  near_cap: "100000000000000000000000000"}}' "$DEP")
near contract deploy "$FACTORY" use-file contracts/res/nearly_factory.wasm \
  with-init-call new json-args "$INIT" prepaid-gas '100.0 Tgas' attached-deposit '0 NEAR' \
  "${NET[@]}" sign-with-legacy-keychain send

# fee step ft_transfers to treasury: unregistered = every deploy fails + refunds
for t in $(jq -r '.tokens[]' "$DEP"); do
  near contract call-function as-transaction "$t" storage_deposit json-args "{\"account_id\":\"$TREASURY\"}" \
    prepaid-gas '30 Tgas' attached-deposit '0.00125 NEAR' sign-as "$ROOT" "${NET[@]}" sign-with-legacy-keychain send
done

jq --arg f "$FACTORY" --arg t "$TREASURY" '.factory = $f | .treasury = $t' "$DEP" >"$DEP.tmp"
mv "$DEP.tmp" "$DEP"
echo "factory: https://testnet.nearblocks.io/address/$FACTORY"
