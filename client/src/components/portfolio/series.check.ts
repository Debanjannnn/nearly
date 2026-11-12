// bun run src/components/portfolio/series.check.ts
import assert from "node:assert/strict";
import { holdingsSeries } from "./series";

const c = (t: number, o: number, v: number) => ({ t, o, v });
const s = holdingsSeries([{ amount: 2, px: 9, points: [c(10, 5, 6), c(30, 6, 8)] }, { amount: 1, px: 120, points: [c(20, 100, 110)] }], 50, 40, true);
// t10: 50 + 2*6 + 1*100(backfilled open); t20: 50+12+110; t30: 50+16+110; live: 50+18+120
assert.deepEqual(s, [{ t: 10, v: 162 }, { t: 20, v: 172 }, { t: 30, v: 176 }, { t: 40, v: 188 }]);
assert.deepEqual(holdingsSeries([], 7, 5, true), [{ t: 5, v: 7 }]);
// leg without history (e.g. a position token) is in every point at live px: no jump at the live point
assert.deepEqual(holdingsSeries([{ amount: 1, px: 4, points: [c(10, 4, 4)] }, { amount: 10, px: 5, points: [] }], 0, 20, true), [{ t: 10, v: 54 }, { t: 20, v: 54 }]);
// testnet: closes scaled per leg so the last close meets live px (×2 here); no jump at the live point
assert.deepEqual(holdingsSeries([{ amount: 1, px: 8, points: [c(10, 2, 3), c(20, 3, 4)] }], 1, 30, false), [{ t: 10, v: 7 }, { t: 20, v: 9 }, { t: 30, v: 9 }]);
assert.deepEqual(holdingsSeries([], 7, 5, false), [{ t: 5, v: 7 }]);

console.log("series ok");
