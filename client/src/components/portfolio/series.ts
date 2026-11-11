import type { Point } from "@/lib/hooks";

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
export function holdingsSeries(legs: { amount: number; points: Point[] }[], flat: number, now: number, live: number) {
  const withData = legs.filter((l) => l.points.length);
  const times = [...new Set(withData.flatMap((l) => l.points.map((c) => c.t)))].filter((t) => t < now).sort((a, b) => a - b);
  const cur = withData.map(() => 0);
  const px = withData.map((l) => l.points[0].o ?? l.points[0].v);
  const pts = times.map((t) => {
    let v = flat;
    withData.forEach((l, i) => {
      while (cur[i] < l.points.length && l.points[cur[i]].t <= t) px[i] = l.points[cur[i]++].v;
      v += l.amount * px[i];
    });
    return { t, v };
  });
  return [...pts, { t: now, v: live }];
}
