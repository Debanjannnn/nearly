import type { Point } from "@/lib/hooks";
import { LIVE_MATCHES_HISTORY } from "@/lib/tokens";

const D = 86400e3;
// history keeps daily closes
export const RANGES = {
  "1W": { ms: 7 * D },
  "1M": { ms: 30 * D },
  "1Y": { ms: 365 * D },
  ALL: { ms: null },
} as const satisfies Record<string, { ms: number | null }>;
export type Range = keyof typeof RANGES;

// Today's holdings marked at each past close (legs forward-filled, back-filled with their first open); flat = stables etc.
// A leg with no history stays at its live px, so every point values the same set as live = flat + Σ amount × px.
export function holdingsSeries(legs: { amount: number; px: number; points: Point[] }[], flat: number, now: number, same = LIVE_MATCHES_HISTORY) {
  const times = [...new Set(legs.flatMap((l) => l.points.map((c) => c.t)))].filter((t) => t < now).sort((a, b) => a - b);
  const cur = legs.map(() => 0);
  const px = legs.map((l) => (l.points.length ? (l.points[0].o ?? l.points[0].v) : l.px));
  const pts = times.map((t) => {
    let v = flat;
    legs.forEach((l, i) => {
      while (cur[i] < l.points.length && l.points[cur[i]].t <= t) px[i] = l.points[cur[i]++].v;
      v += l.amount * px[i];
    });
    return { t, v };
  });
  // testnet live px isn't comparable to mainnet closes: end on the last close
  return same || !pts.length ? [...pts, { t: now, v: legs.reduce((v, l) => v + l.amount * l.px, flat) }] : pts;
}
