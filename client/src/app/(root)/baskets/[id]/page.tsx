"use client";

import Link from "next/link";
import { useParams, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { fmtTime, getPositions, getPriceSeries, getStrategy, runBacktest, shortHash } from "@/lib/api";
import { dayOf, toPoints, useRates, useTickers } from "@/lib/hooks";
import { useAsync, useSession } from "@/lib/session";
import { addressUrl } from "@/lib/config";
import { Button, Card, Detail, ErrorBox, Panel, Skeleton, Stat } from "@/components/ui";
import { marketOf } from "@/lib/tokens";
import { PriceChart, RANGES, type Range } from "@/components/chart/PriceChart";
import { IconStack } from "@/components/IconStack";
import { AllocationDonut, BacktestView, pct } from "@/components/earn/Backtest";
import { DeployPanel } from "@/components/earn/DeployPanel";
import { basketCandles } from "@/components/earn/basketIndex";
import { StrategyFlow, type FlowLeg } from "@/components/earn/StrategyFlow";
import { Maximize2, Minimize2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { liveApy, parts, sourceApy } from "@/components/earn/sources";
import { ScheduleForm } from "@/components/automation/ScheduleForm";
import { ScheduleList } from "@/components/automation/ScheduleList";
import { PositionsTable } from "@/components/portfolio/Positions";

export default function BasketPage() {
  const id = useParams<{ id: string }>().id;
  const search = useSearchParams();
  // bumps ScheduleList after a create
  const [key, setKey] = useState(0);
  const { ready, authenticated } = useSession();
  const st = useAsync(ready ? `strategy:${id}:${authenticated}` : null, () => getStrategy(id));
  const s = st.data;
  // the viewer's live positions in this basket (closed/failed stay on the portfolio page)
  const positions = useAsync(ready && authenticated ? `positions:${id}` : null, getPositions);
  const mine = (positions.data ?? []).filter((p) => p.basket_id === id && p.state !== "closed");
  const { rates } = useRates();
  // backtest straight from history, $1,000 over the last year
  const sleeves = s ? parts(s) : [];
  const bt = useAsync(s ? `bt:${s.id}` : null, () => runBacktest(s!.definition, "1000"));
  const [range, setRange] = useState<Range>("1W");
  const [mode, setMode] = useState<"line" | "candle">("candle");
  const { ms } = RANGES[range];
  // aggregated chart: each token's daily closes weighted into one $100 index (USDC flat)
  const series = useAsync(s ? `bidx:${id}:${range}` : null, async () =>
    basketCandles(
      await Promise.all(
        sleeves.map(async (sl) => ({
          weightBps: sl.weightBps,
          points: sl.asset === "USDC" ? null : toPoints(await getPriceSeries(marketOf(sl.asset)!.history!, ms === null ? "2000-01-01" : dayOf(Date.now() - ms))),
        })),
      ),
    ),
  );
  const pts = series.data ?? [];
  // headline: live price = Σ weight × token price (USDC $1); last close until every token has a price
  const { tickers } = useTickers();
  const mids = sleeves.map((sl) => (sl.asset === "USDC" ? 1 : (tickers[sl.asset]?.price ?? null)));
  const livePx = s && mids.every((m) => m !== null) ? sleeves.reduce((a, sl, i) => a + (sl.weightBps / 10000) * mids[i]!, 0) : null;
  // split per $1,000: every source as a share of the whole basket
  const flowLegs = sleeves.flatMap((sl) =>
    sl.sources.map((src) => ({
      kind: src.kind,
      venue: src.venue,
      weightBps: (sl.weightBps * src.weightBps) / 10000,
      apy: sourceApy(src, sl.asset, rates),
      token: sl.asset,
    })),
  );

  if (st.error === "not_found")
    return (
      <main className="w-full px-4 pb-24 pt-8 md:px-6">
        <Panel className="mx-auto max-w-md p-8 text-center">
          <p className="text-lg font-medium">Basket not found</p>
          <Link href="/baskets" className="mt-5 inline-block">
            <Button variant="ghost">All baskets</Button>
          </Link>
        </Panel>
      </main>
    );

  const live = s ? liveApy(s, rates) : null;

  return (
    <main className="w-full px-4 pb-24 pt-10 md:px-6">
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_440px]">
        <div className="min-w-0 space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div className="flex min-w-0 items-center gap-3">
              {s ? <IconStack symbols={sleeves.map((sl) => sl.asset)} size={36} ring="ring-background" /> : <Skeleton className="size-9 rounded-full" />}
              {s ? (
                <h1 className="truncate text-3xl font-medium tracking-tight">
                  {s.name} <span className="text-muted-foreground">{sleeves.map((sl) => sl.asset).join(" · ")}</span>
                </h1>
              ) : (
                <Skeleton className="h-9 w-64" />
              )}
            </div>
            {s && !s.public && <span className="rounded-full bg-secondary px-2 py-0.5 text-[11px] text-muted-foreground">Private</span>}
          </div>
          {st.error && <ErrorBox message={st.error} />}

          <PriceChart
            points={pts}
            price={livePx ?? pts.at(-1)?.v ?? null}
            range={range}
            onRange={setRange}
            mode={mode}
            onMode={setMode}
            loading={!s || series.loading}
            error={series.error}
            caption="Σ weight × token price"
            dataKey={`${id}:${range}:${mode}`}
            empty={sleeves.every((sl) => sl.asset === "USDC") ? "All-USDC basket: price stays at $1, plus yield." : "No price history for every token in this range yet."}
          />

          <section className="rounded-[32px] bg-primary/[0.06] p-6">
            <dl className="grid grid-cols-2 gap-6 md:grid-cols-3">
              <Stat label="Live APY (est.)" value={s ? <span className="text-positive">{pct(live)}</span> : null} note={rates?.stale ? "rates stale" : "Σ weight × live rate"} />
              <Stat label="Backtest APY" value={s ? pct(bt.data?.apyUsd) : null} note="historical, mainnet data" />
              <Stat label="Max drawdown" value={s ? pct(bt.data?.maxDrawdownUsd) : null} />
            </dl>
          </section>

          {mine.length > 0 && (
            <section className="space-y-3">
              <h2 className="px-1 text-lg font-medium">Your positions</h2>
              <PositionsTable positions={mine} strategyOf={() => s ?? undefined} onChange={positions.reload} />
            </section>
          )}

          <div className="grid gap-4 xl:grid-cols-2">
            <Card title="Allocation" className="min-w-0">
              {s ? <AllocationDonut sleeves={sleeves} stacked /> : <Skeleton className="h-40 w-full" />}
            </Card>
            {s ? <SplitCard legs={flowLegs} /> : <Skeleton className="h-60 w-full rounded-3xl" />}
          </div>

          <Card title="Backtest">
            <BacktestView bt={bt.data} loading={bt.loading} error={bt.error} />
          </Card>

          <Card title="Rules">
            {s ? (
              <dl className="grid gap-x-10 md:grid-cols-2">
                <Detail label="Drift band">{(s.rules.driftBps ?? 500) / 100}%</Detail>
                <Detail label="Creator">
                  <a href={addressUrl(s.creator)} target="_blank" rel="noreferrer" className="tnum hover:underline">
                    {shortHash(s.creator)}
                  </a>
                </Detail>
                <Detail label="Created">{fmtTime(s.createdAt)}</Detail>
                <Detail label="Fees">0.4% on open · 0 on withdraw</Detail>
              </dl>
            ) : (
              <Skeleton className="h-24 w-full" />
            )}
          </Card>
        </div>

        <aside className="space-y-3 lg:sticky lg:top-24 lg:self-start">
          <div className="rounded-[28px] border border-border bg-secondary/60">{s ? <DeployPanel strategy={s} initialAmount={search.get("amount") ?? ""} initialExec={search.get("exec")} onSettled={positions.reload} /> : <Skeleton className="m-5 h-60" />}</div>
          {s && (
            <div className="space-y-4 rounded-[28px] border border-border bg-secondary/60 p-5">
              <span className="text-sm font-medium">Recurring buy</span>
              <ScheduleForm target={{ type: "strategy", id }} current={null} onCreated={() => setKey((n) => n + 1)} />
              <ScheduleList key={key} target={{ type: "strategy", id }} />
            </div>
          )}
        </aside>
      </div>
    </main>
  );
}

// routing diagram card; fullscreen via the browser's Fullscreen API (Esc exits), diagram fills the screen
function SplitCard({ legs }: { legs: FlowLeg[] }) {
  const ref = useRef<HTMLElement>(null);
  const [full, setFull] = useState(false);
  useEffect(() => {
    const sync = () => setFull(document.fullscreenElement === ref.current);
    document.addEventListener("fullscreenchange", sync);
    return () => document.removeEventListener("fullscreenchange", sync);
  }, []);
  const Icon = full ? Minimize2 : Maximize2;
  return (
    <section ref={ref} className={cn("min-w-0 rounded-3xl border border-border bg-card p-6", full && "flex flex-col rounded-none border-0")}>
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 className="text-2xl font-medium">Split</h2>
        <button
          type="button"
          aria-label={full ? "Exit full screen" : "Full screen"}
          onClick={() => void (full ? document.exitFullscreen() : ref.current?.requestFullscreen())}
          className="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
        >
          <Icon className="size-4" />
        </button>
      </div>
      <div className={cn(full && "min-h-0 flex-1")}>
        <StrategyFlow asset="USDC" amount={1000} legs={legs} compact={!full} fill={full} />
      </div>
    </section>
  );
}
