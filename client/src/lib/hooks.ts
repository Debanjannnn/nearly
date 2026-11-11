"use client";

import { useEffect, useState } from "react";
import { getExecution, getHoldings, getPriceSeries, getPrices, getRates } from "./api";
import { useAsync, useSession } from "./session";
import { MARKETS } from "./tokens";
import { useTopic } from "./ws";
import type { Execution, PricePoint, Prices, Rates } from "./types";

export { useAsync };

const D = 86400e3;
export const dayOf = (t: number) => new Date(t).toISOString().slice(0, 10);

export type Point = { t: number; v: number; o?: number; h?: number; l?: number; vol?: number };

// daily closes → bars: open = previous close, so candles show the day's move
export const toPoints = (rows: PricePoint[]): Point[] =>
  rows.map((r, i) => {
    const v = Number(r.usd);
    const o = i ? Number(rows[i - 1].usd) : v;
    return { t: Date.parse(r.day), v, o, h: Math.max(o, v), l: Math.min(o, v) };
  });

// USD per contract id; snapshot, then the market-data `prices` topic
export function usePrices() {
  const snap = useAsync("prices", getPrices, 60_000);
  const live = useTopic<Prices["prices"]>("prices");
  const list = live ?? snap.data?.prices ?? [];
  const priceOf = (id: string) => list.find((p) => p.token === id)?.usd ?? null;
  return { priceOf, stale: !live && !!snap.data?.stale, error: snap.error, loading: snap.loading && !live };
}

type Ticker = { price: number | null; series: number[]; chg: number | null; chgUsd: number | null };

// every listed token: a week of daily closes + live price; 24h change against the latest daily close
export function useTickers() {
  const hist = useAsync("tickers", async () => {
    const keys = [...new Set(MARKETS.flatMap((m) => (m.history ? [m.history] : [])))];
    const res = await Promise.allSettled(keys.map((k) => getPriceSeries(k, dayOf(Date.now() - 8 * D))));
    return Object.fromEntries(keys.map((k, i) => [k, res[i].status === "fulfilled" ? toPoints(res[i].value) : []]));
  });
  const prices = usePrices();
  const out: Record<string, Ticker> = {};
  for (const m of MARKETS) {
    const pts = (m.history && hist.data?.[m.history]) || [];
    const closes = pts.map((p) => p.v);
    const live = prices.priceOf(m.swap.address);
    const price = live ?? closes.at(-1) ?? null;
    // daily closes: live vs the last close, else the last close vs the one before
    const first = (live !== null ? closes.at(-1) : closes.at(-2)) ?? 0;
    out[m.symbol] = {
      price,
      series: live !== null && closes.length ? [...closes, live] : closes,
      chg: price !== null && first > 0 ? ((price - first) / first) * 100 : null,
      chgUsd: price !== null && first > 0 ? price - first : null,
    };
  }
  return { tickers: out, priceOf: prices.priceOf, loading: hist.loading || prices.loading, error: hist.error ?? prices.error };
}

// daily price points for a range; ms null = all history
export function useSeries(key: string | null | undefined, ms: number | null) {
  const s = useAsync(key ? `series:${key}:${ms}` : null, () => getPriceSeries(key!, ms === null ? "2000-01-01" : dayOf(Date.now() - ms)));
  return { points: s.data ? toPoints(s.data) : [], loading: s.loading, error: s.error };
}

// sub-account balances (FTs from the api, NEAR from RPC); idle until the account exists
export function useHoldings() {
  const { ready, authenticated, address } = useSession();
  return useAsync(ready && authenticated && address ? `holdings:${address}` : null, getHoldings, 30_000);
}

// snapshot, then the market-data WS topic once it pushes
export function useRates() {
  const snap = useAsync("rates", getRates, 60_000);
  const live = useTopic<Rates["rates"]>("rates");
  return { rates: live ? { rates: live, stale: false } : snap.data, error: snap.error, loading: snap.loading };
}

const DONE = new Set(["succeeded", "failed"]);

// polls GET /me/executions/{id} every 2s until it settles
export function useExecution(id: string | null) {
  const [ex, setEx] = useState<{ id: string; data: Execution | null; error: string | null } | null>(null);
  useEffect(() => {
    if (!id) return;
    let live = true;
    let t: ReturnType<typeof setTimeout>;
    const tick = async () => {
      try {
        const data = await getExecution(id);
        if (!live) return;
        setEx({ id, data, error: null });
        if (!DONE.has(data.status)) t = setTimeout(tick, 2000);
      } catch (e) {
        if (!live) return;
        setEx((p) => ({ id, data: p?.id === id ? p.data : null, error: e instanceof Error ? e.message : String(e) }));
        t = setTimeout(tick, 4000);
      }
    };
    void tick();
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [id]);
  return ex?.id === id ? ex : null;
}

export const settled = (e: Execution | null | undefined) => !!e && DONE.has(e.status);

export function useDebounced<T>(v: T, ms = 400) {
  const [d, setD] = useState(v);
  useEffect(() => {
    const t = setTimeout(() => setD(v), ms);
    return () => clearTimeout(t);
  }, [v, ms]);
  return d;
}

// clipboard write + which value was just copied (cleared after ms)
function fallback(v: string): boolean {
  const ta = Object.assign(document.createElement("textarea"), { value: v, readOnly: true });
  ta.style.cssText = "position:fixed;opacity:0";
  (document.querySelector("dialog[open]") ?? document.body).appendChild(ta);
  ta.select();
  const ok = document.execCommand("copy");
  ta.remove();
  return ok;
}

export function useCopy(ms = 1200) {
  const [copied, setCopied] = useState<string | null>(null);
  const done = (v: string) => (setCopied(v), setTimeout(() => setCopied(null), ms));
  const copy = (v: string) => {
    // clipboard API needs a secure context (https/localhost); else the old select + execCommand path.
    // The textarea goes inside an open modal <dialog>: everything outside the top layer is inert.
    if (navigator.clipboard?.writeText) return void navigator.clipboard.writeText(v).then(() => done(v), () => fallback(v) && done(v));
    if (fallback(v)) done(v);
  };
  return { copied, copy };
}
