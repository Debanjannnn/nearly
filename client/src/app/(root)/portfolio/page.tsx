"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { errText, fmtNum, getExecutions, getMyStrategies, getPositions, getStrategies } from "@/lib/api";
import { useHoldings, useRates, useTickers } from "@/lib/hooks";
import { useAsync, useSession } from "@/lib/session";
import { tokenBySymbol } from "@/lib/tokens";
import { ErrorBox, Panel, Skeleton } from "@/components/ui";
import { Holdings, type Row } from "@/components/portfolio/Holdings";
import { ValueChart } from "@/components/portfolio/ValueChart";
import { EarningsCard } from "@/components/portfolio/Earnings";
import { DelegationCard } from "@/components/automation/Delegation";
import { ExecutionLog, ScheduleList } from "@/components/automation/ScheduleList";
import { heldOf, PositionsTable, STABLES, usePriceOf, valueOf } from "@/components/portfolio/Positions";
import { liveApy } from "@/components/earn/sources";

export default function PortfolioPage() {
  const { ready, authenticated, mainAddress, address } = useSession();
  // logged out: same layout with empty states; per-account data waits for the sub-account
  const on = ready && authenticated && !!address;
  const holdings = useHoldings();
  const { tickers, loading: pxLoading } = useTickers();
  const priceOf = usePriceOf();
  const { rates } = useRates();
  const positions = useAsync(on ? "positions" : null, getPositions);
  const executions = useAsync(on ? "executions" : null, getExecutions);
  const baskets = useAsync("strategies", () => getStrategies());
  const mine = useAsync(on ? "my-strategies" : null, getMyStrategies);

  const h = holdings.data;
  const raw = (sym: string) => (h ? BigInt(h.tokens.find((t) => t.symbol === sym)?.raw ?? 0) : undefined);
  const rows: Row[] = [];
  for (const t of h?.tokens ?? []) {
    if (t.symbol === "NEAR" || t.symbol === "USDC" || Number(t.amount) === 0) continue;
    const price = priceOf(t.symbol);
    rows.push({
      key: t.symbol,
      symbol: t.symbol,
      name: tokenBySymbol(t.symbol)?.name ?? t.symbol,
      cls: STABLES.has(t.symbol) ? "Stablecoin" : "Crypto",
      icons: [t.symbol],
      href: tickers[t.symbol] ? `/trade/${t.symbol}` : undefined,
      price,
      balance: fmtNum(Number(t.amount), 6),
      value: price === null ? null : Number(t.amount) * price,
    });
  }
  const tokensUsd = (h?.tokens ?? []).reduce((s, t) => s + Number(t.amount) * (priceOf(t.symbol) ?? 0), 0);
  const strats = new Map([...(baskets.data ?? []), ...(mine.data ?? [])].map((b) => [b.id, b]));
  const names = Object.fromEntries([...(baskets.data ?? []), ...(mine.data ?? [])].map((b) => [b.id, b.name]));
  const open = (positions.data ?? []).filter((p) => p.state !== "closed");
  const vals = open.map((p) => valueOf(p, strats.get(p.basket_id), priceOf));
  const posUsd = vals.reduce<number>((s, v) => s + (v ?? 0), 0);
  const depositUsd = open.reduce((s, p) => s + Number(p.deposit) / 1e6, 0);
  const apyOf = (id: string) => (strats.get(id) ? (liveApy(strats.get(id)!, rates) ?? 0) : 0);
  const yieldPerYear = open.length ? open.reduce((a, p, i) => a + (vals[i] ?? 0) * apyOf(p.basket_id), 0) : null;
  const total = tokensUsd + posUsd;
  // chart marks the same set as total: wallet tokens + valued positions' held, stables kept flat
  const held = new Map<string, number>();
  const add = (m: string, n: number) => held.set(m, (held.get(m) ?? 0) + n);
  for (const t of h?.tokens ?? []) add(t.symbol, Number(t.amount));
  open.forEach((p, i) => vals[i] !== null && heldOf(p, strats.get(p.basket_id)!).forEach((x) => add(x.token, x.amount)));
  const legs = [...held]
    .map(([market, amount]) => ({ market, amount, px: STABLES.has(market) ? 0 : (priceOf(market) ?? 0) }))
    .filter((l) => l.amount > 0 && l.px > 0);
  const loading = !ready || (on && (holdings.loading || pxLoading));
  const near = raw("NEAR");

  return (
    <main className="w-full px-4 pb-24 pt-8 md:px-6">
      <div className="mx-auto w-full space-y-6">
        <header className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <h1 className="truncate text-xl font-medium sm:text-2xl">
              <span className="text-muted-foreground">Welcome, </span>
              {!ready ? <Skeleton className="inline-block h-6 w-32 align-middle" /> : (mainAddress ?? "-")}
            </h1>
          </div>
          {ready && <Clock />}
        </header>

        <ValueChart legs={legs} flat={total - legs.reduce((s, l) => s + l.amount * l.px, 0)} total={total} loading={loading} />

        {holdings.error && <ErrorBox message={`Wallet balances unavailable: ${errText(new Error(holdings.error))}`} />}

        <Holdings rows={rows} loading={loading} usdc={raw("USDC")} near={near} lowGas={near !== undefined && near < LOW_GAS} baskets={baskets.data} />

        <section className="space-y-3">
          <div className="flex items-center justify-between px-1">
            <h2 className="text-lg font-medium">My baskets</h2>
            <Link href="/baskets" className="text-sm text-muted-foreground hover:text-foreground">
              Explore baskets →
            </Link>
          </div>
          {positions.error && <ErrorBox message={errText(new Error(positions.error))} />}
          {positions.loading && !positions.data ? (
            <Skeleton className="h-28 w-full rounded-3xl" />
          ) : (positions.data ?? []).length === 0 ? (
            <Panel className="p-5 text-sm text-muted-foreground">No baskets yet.</Panel>
          ) : (
            <PositionsTable positions={positions.data ?? []} strategyOf={(id) => strats.get(id)} onChange={positions.reload} />
          )}
        </section>

        <EarningsCard executions={executions.data ?? []} valueUsd={vals.some((v) => v === null) ? null : posUsd} depositUsd={depositUsd} yieldPerYear={yieldPerYear} />

        <section className="space-y-3">
          <h2 className="px-1 text-lg font-medium">Automation</h2>
          <DelegationCard />
          <ScheduleList names={names} />
          <ExecutionLog />
        </section>
      </div>
    </main>
  );
}

// 0.1 NEAR (24 decimals)
const LOW_GAS = BigInt(10) ** BigInt(23);

function useNow() {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  return now;
}

function Clock() {
  const now = useNow();
  return (
    <time className="tnum text-sm text-muted-foreground sm:text-base" dateTime={new Date(now).toISOString()}>
      {new Date(now).toLocaleString("en-GB", { day: "numeric", month: "long", year: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" })}
    </time>
  );
}
