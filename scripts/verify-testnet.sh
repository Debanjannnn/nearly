#!/usr/bin/env bash
# Checks every contract ID in contracts/deployments/testnet.json has code on testnet.
# root/treasury are plain accounts: they only need to exist.
set -euo pipefail
cd "$(dirname "$0")/.."
DEP=contracts/deployments/testnet.json
RPC=${NEAR_RPC:-https://rpc.testnet.near.org}

view() {
  curl -sf "$RPC" -H 'content-type: application/json' -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"query\",
    \"params\":{\"request_type\":\"view_account\",\"finality\":\"final\",\"account_id\":\"$1\"}}"
}

fail=0
check() {
  local name=$1 id=$2 want_code=$3 res hash
  if [ -z "$id" ]; then echo "MISS $name: empty"; fail=1; return; fi
  res=$(view "$id") || { echo "FAIL $name $id: rpc"; fail=1; return; }
  hash=$(jq -r '.result.code_hash // empty' <<<"$res")
  if [ -z "$hash" ]; then
    echo "FAIL $name $id: $(jq -c '.error.cause.name // .error' <<<"$res")"; fail=1
  elif [ "$want_code" = 1 ] && [ "$hash" = 11111111111111111111111111111111 ]; then
    echo "FAIL $name $id: no code"; fail=1
  else
    echo "ok   $name $id"
  fi
}

check root "$(jq -r .root "$DEP")" 0
check treasury "$(jq -r .treasury "$DEP")" 0
check factory "$(jq -r .factory "$DEP")" 1
while IFS=$'\t' read -r k id; do check "$k" "$id" 1; done < <(
  jq -r '(.tokens, .venues) | to_entries[] | "\(.key)\t\(.value)"' "$DEP")
exit $fail
