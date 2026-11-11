import assert from "node:assert/strict";
import { DEPLOYMENTS } from "@/lib/config";
import { blendedApy, evenSplit, legApy, setWeight, weightsError, withPool } from "./legs";

assert.deepEqual(evenSplit(3), [3334, 3333, 3333]);
assert.deepEqual(setWeight([5000, 5000], 0, 8000), [5000, 5000]);
assert.deepEqual(setWeight([2000, 5000], 0, 8000), [5000, 5000]);
assert.equal(weightsError([5000, 5000]), null);
assert.match(weightsError([5000, 4000]) ?? "", /90.00%/);
assert.equal(withPool({ token: "usdc", source: "lending", weight_bps: 1, pool_id: 7 }).pool_id, undefined);
assert.ok(withPool({ token: "wnear", source: "staking", weight_bps: 1 }).pool_id! > 0);

const usdc = DEPLOYMENTS.tokens!.usdc!;
const rates = {
  stale: false,
  rates: [
    { venue: "lending" as const, contract: "l", token: usdc, apy: 0.1, ts: 0 },
    { venue: "staking" as const, contract: "s", token: "x", apy: 0.06, ts: 0 },
  ],
};
assert.equal(legApy({ token: "wnear", source: "hold" }, null), 0);
assert.equal(legApy({ token: "wnear", source: "lending" }, rates), null);
const apy = blendedApy(
  { legs: [{ token: "usdc", source: "lending", weight_bps: 5000 }, { token: "wnear", source: "staking", weight_bps: 5000 }] },
  rates,
);
assert.ok(Math.abs(apy! - 0.08) < 1e-9);
assert.equal(blendedApy({ legs: [{ token: "wnear", source: "lending", weight_bps: 10000 }] }, rates), null);

console.log("legs ok");
