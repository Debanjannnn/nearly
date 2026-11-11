// bun run src/components/earn/basketIndex.check.ts
import assert from "node:assert/strict";
import { basketCandles } from "./basketIndex";
import type { Point } from "@/lib/hooks";

const bar = (t: number, o: number, h: number, l: number, c: number): Point => ({ t, o, h, l, v: c });

// 50% wNEAR (2 → 2.2), 50% USDC ($1): price = 0.5 × 1 + 0.5 × NEAR
const pts = basketCandles([
  { weightBps: 5000, points: [bar(1, 1.9, 2.1, 1.8, 2), bar(2, 2, 2.25, 1.9, 2.2)] },
  { weightBps: 5000, points: null },
]);
assert.equal(pts.length, 2);
assert.equal(pts[0].v, 1.5);
assert.ok(Math.abs(pts[1].v - 1.6) < 1e-12);
assert.equal(pts[1].h, 0.5 + 0.5 * 2.25);

// starts at the later first bar; a leg missing a bar holds its last close
const two = basketCandles([
  { weightBps: 5000, points: [bar(1, 1, 1, 1, 1), bar(2, 1, 1, 1, 2), bar(3, 2, 2, 2, 4)] },
  { weightBps: 5000, points: [bar(2, 10, 10, 10, 10)] },
]);
assert.deepEqual(two.map((p) => p.t), [2, 3]);
assert.equal(two[1].v, 0.5 * 4 + 0.5 * 10);

assert.deepEqual(basketCandles([{ weightBps: 10000, points: [] }]), []);
assert.deepEqual(basketCandles([{ weightBps: 10000, points: null }]), []);
console.log("basketIndex ok");
