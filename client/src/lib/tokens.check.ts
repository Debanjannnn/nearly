import assert from "node:assert/strict";
import { anchor, fmtUnits, parseAmount, withLive } from "./tokens";

assert.equal(fmtUnits(0n, 6), "0");
assert.equal(fmtUnits(1_234_567_890n, 6), "1,234.5678");
assert.equal(fmtUnits("1000000000000000000000000", 24), "1");
assert.equal(fmtUnits(-1_500_000n, 6), "-1.5");
assert.equal(fmtUnits(1n, 6), "0");
assert.equal(fmtUnits(10n ** 21n, 24, 6), "0.001");

assert.equal(parseAmount("1.5", 6), 1_500_000n);
assert.equal(parseAmount(".5", 6), 500_000n);
assert.equal(parseAmount("2.", 24), 2n * 10n ** 24n);
assert.equal(parseAmount("0.0000001", 6), null);
assert.equal(parseAmount("", 6), null);
assert.equal(parseAmount(".", 6), null);
assert.equal(parseAmount("-1", 6), null);
assert.equal(parseAmount("1e3", 6), null);

assert.deepEqual(withLive([3.58, 3.59], 13.91, true), [3.58, 3.59, 13.91]);
assert.deepEqual(withLive([3.58, 3.59], 13.91, false), [3.58, 3.59]);
assert.deepEqual(withLive([3.58], null, true), [3.58]);
assert.deepEqual(withLive([], 13.91, true), []);

// testnet: every bar field scaled so the last close hits live; t/vol untouched
assert.deepEqual(anchor([{ t: 1, o: 1, h: 4, l: 1, v: 2, vol: 9 }, { t: 2, v: 4 }], 8, false), [{ t: 1, o: 2, h: 8, l: 2, v: 4, vol: 9 }, { t: 2, v: 8 }]);
const bars = [{ t: 1, v: 2 }];
assert.equal(anchor(bars, 8, true), bars);
assert.equal(anchor(bars, null, false), bars);
assert.deepEqual(anchor([], 8, false), []);

console.log("tokens ok");
