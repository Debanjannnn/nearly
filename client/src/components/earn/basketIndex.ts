import type { Point } from "@/lib/hooks";

// Basket price = Σ weight × token price (USDC = $1), as OHLC from each token's daily bars, from the first common bar.
// h/l add up per-leg extremes (a bound: legs need not peak in the same bar). A leg without a bar at t holds its last close;
// points null = flat leg (USDC).
export function basketCandles(legs: { weightBps: number; points: Point[] | null }[]): Point[] {
  const live = legs.filter((l) => l.points);
  if (!live.length || live.some((l) => !l.points!.length)) return [];
  const flat = legs.filter((l) => !l.points).reduce((s, l) => s + l.weightBps / 10000, 0);
  const start = Math.max(...live.map((l) => l.points![0].t));
  const times = [...new Set(live.flatMap((l) => l.points!.map((c) => c.t)))].filter((t) => t >= start).sort((a, b) => a - b);
  const st = live.map(() => ({ i: 0, last: 0 }));
  return times.map((t) => {
    const p = { t, o: flat, h: flat, l: flat, v: flat, vol: 0 };
    live.forEach((leg, k) => {
      const s = st[k];
      const cs = leg.points!;
      while (s.i < cs.length && cs[s.i].t < t) s.last = cs[s.i++].v;
      const bar = s.i < cs.length && cs[s.i].t === t ? cs[s.i] : null;
      const [o, h, l, c] = bar ? [bar.o ?? bar.v, bar.h ?? bar.v, bar.l ?? bar.v, bar.v] : [s.last, s.last, s.last, s.last];
      const w = leg.weightBps / 10000;
      p.o += w * o;
      p.h += w * h;
      p.l += w * l;
      p.v += w * c;
    });
    return p;
  });
}
