"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { errText, getExecution, getHoldings, getPrices, getRates } from "./api";
import { useSession } from "./session";
import { tokenIdOf } from "./tokens";
import type { Execution, Prices, Rates } from "./types";
import { useTopic } from "./ws";

// copied = last copied value, cleared after ms
export function useCopy(ms = 1200) {
  const [copied, setCopied] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);
  const copy = useCallback(
    (v: string) => {
      void navigator.clipboard?.writeText(v).then(() => {
        setCopied(v);
        if (timer.current) clearTimeout(timer.current);
        timer.current = setTimeout(() => setCopied(null), ms);
      });
    },
    [ms],
  );
  return { copied, copy };
}

// key null = idle; a new key refetches; reload() refetches keeping data; pollMs = reload on an interval
export function useAsync<T>(key: string | null, fn: () => Promise<T>, pollMs?: number) {
  const [nonce, setNonce] = useState(0);
  const [s, setS] = useState<{ key: string | null; nonce: number; data: T | null; error: string | null }>({
    key: null,
    nonce: -1,
    data: null,
    error: null,
  });

  useEffect(() => {
    if (key === null) return;
    let live = true;
    fn()
      .then((data) => live && setS({ key, nonce, data, error: null }))
      .catch((e) => live && setS((p) => ({ key, nonce, data: p.key === key ? p.data : null, error: errText(e) })));
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, nonce]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  useEffect(() => {
    if (key === null || !pollMs) return;
    const t = setInterval(reload, pollMs);
    return () => clearInterval(t);
  }, [key, pollMs, reload]);
  const same = key !== null && s.key === key;
  return {
    data: same ? s.data : null,
    error: same ? s.error : null,
    loading: key !== null && !(same && s.nonce === nonce),
    reload,
  };
}

export function useDebounced<T>(v: T, ms = 400) {
  const [d, setD] = useState(v);
  useEffect(() => {
    const t = setTimeout(() => setD(v), ms);
    return () => clearTimeout(t);
  }, [v, ms]);
  return d;
}

// snapshot, then the market-data WS topic once it pushes
export function useRates() {
  const snap = useAsync("rates", getRates, 60_000);
  const live = useTopic<Rates["rates"]>("rates");
  return { rates: live ? { rates: live, stale: false } : snap.data, error: snap.error, loading: snap.loading };
}

export type PriceOf = (key: string) => number | null;

// USD per token key; usdc is the base at $1
export function usePrices() {
  const snap = useAsync("prices", getPrices, 60_000);
  const live = useTopic<Prices["prices"]>("prices");
  const list = live ?? snap.data?.prices ?? [];
  const priceOf: PriceOf = (key) => (key === "usdc" ? 1 : (list.find((p) => p.token === tokenIdOf(key))?.usd ?? null));
  return { prices: list, priceOf, stale: !live && !!snap.data?.stale, error: snap.error, loading: snap.loading };
}

// sub-account balances; idle until the account exists
export function useHoldings() {
  const { authenticated, account } = useSession();
  return useAsync(authenticated && account ? `holdings:${account}` : null, getHoldings, 30_000);
}

const DONE = new Set(["succeeded", "failed"]);
export const settled = (e: Execution | null | undefined) => !!e && DONE.has(e.status);

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
        setEx((p) => ({ id, data: p?.id === id ? p.data : null, error: errText(e) }));
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
