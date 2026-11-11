"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { Maximize2, Minimize2 } from "lucide-react";
import { fmtTime, getBasket, getPositions, pct, runBacktest, shortId } from "@/lib/api";
import { accountUrl } from "@/lib/config";
import { useAsync, usePrices, useRates } from "@/lib/hooks";
import { useSession } from "@/lib/session";
import { symbolOf } from "@/lib/tokens";
import type { Leg, Rates } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Button, Card, Detail, ErrorBox, Panel, Skeleton, Stat } from "@/components/ui";
import { IconStack } from "@/components/TokenIcon";
import { ScheduleForm } from "@/components/automation/ScheduleForm";
import { ScheduleList } from "@/components/automation/ScheduleList";
import { PositionsTable } from "@/components/portfolio/Positions";
import { AllocationDonut, BacktestView } from "./Backtest";
import { BuyPanel } from "./BuyPanel";
import { RiskCard } from "./RiskCard";
import { RoutingFlow } from "./RoutingFlow";
import { blendedApy, tokensOf } from "./legs";

export function BasketDetail({ id }: { id: string }) {
  const search = useSearchParams();
  const [refresh, setRefresh] = useState(0); // bumps ScheduleList after a create
  const { ready, authenticated, account } = useSession();
  const st = useAsync(ready ? `basket:${id}:${authenticated}` : null, () => getBasket(id));
  const b = st.data;
  const positions = useAsync(ready && authenticated && account ? `positions:${id}` : null, getPositions);
  const mine = (positions.data ?? []).filter((p) => p.basket_id === id && p.state !== "closed");
  const { rates } = useRates();
  const { priceOf } = usePrices();
  // shortcut: backtest runs per view straight from history; cache server-side if traffic grows
  const bt = useAsync(b ? `bt:${b.id}` : null, () => runBacktest(b!.definition));

  if (st.error === "not_found")
    return (
      <main className="w-full px-4 pt-8 pb-24 md:px-6">
        <Panel className="mx-auto max-w-md p-8 text-center">
          <p className="text-lg font-medium">Basket not found</p>
          <Link href="/baskets" className="mt-5 inline-block">
            <Button variant="ghost">All baskets</Button>
          </Link>
        </Panel>
      </main>
    );

  const tokens = b ? tokensOf(b.definition) : [];
  const live = b ? blendedApy(b.definition, rates) : null;

  return (
    <main className="w-full px-4 pt-10 pb-24 md:px-6">
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_440px]">
        <div className="min-w-0 space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div className="flex min-w-0 items-center gap-3">
              {b ? <IconStack tokens={tokens} size={36} ring="ring-background" /> : <Skeleton className="size-9 rounded-full" />}
              {b ? (
                <h1 className="truncate text-3xl font-medium tracking-tight">
                  {b.name} <span className="text-muted-foreground">{tokens.map(symbolOf).join(" · ")}</span>
                </h1>
              ) : (
                <Skeleton className="h-9 w-64" />
              )}
            </div>
            {b && !b.public && <span className="rounded-full bg-secondary px-2 py-0.5 text-[11px] text-muted-foreground">Private</span>}
          </div>
          {st.error && <ErrorBox message={st.error} />}

          <section className="rounded-[32px] bg-primary/[0.06] p-6">
            <dl className="grid grid-cols-2 gap-6 md:grid-cols-3">
              <Stat label="Live APY (est.)" value={b ? <span className="text-positive">{pct(live)}</span> : null} note={rates?.stale ? "rates stale" : "Σ weight × live rate"} />
              <Stat label="Backtest APY" value={b ? (bt.loading ? "…" : pct(bt.data?.apy)) : null} note="historical" />
              <Stat label="Max drawdown" value={b ? (bt.loading ? "…" : pct(bt.data?.max_drawdown)) : null} />
            </dl>
          </section>

          {mine.length > 0 && (
            <section className="space-y-3">
              <h2 className="px-1 text-lg font-medium">Your positions</h2>
              <PositionsTable positions={mine} basketOf={() => b ?? undefined} priceOf={priceOf} onChange={positions.reload} />
            </section>
          )}

          <div className="grid gap-4 xl:grid-cols-2">
            <Card title="Allocation" className="min-w-0">
              {b ? <AllocationDonut def={b.definition} stacked /> : <Skeleton className="h-40 w-full" />}
            </Card>
            {b ? <SplitCard legs={b.definition.legs} rates={rates} /> : <Skeleton className="h-60 w-full rounded-3xl" />}
          </div>

          <Card title="Backtest">
            <BacktestView bt={bt.data} loading={bt.loading} error={bt.error} />
          </Card>

          {b && <RiskCard def={b.definition} amount={1000} />}

          <Card title="Rules">
            {b ? (
              <dl className="grid gap-x-10 md:grid-cols-2">
                <Detail label="Drift band">{b.definition.drift_bps ? `${b.definition.drift_bps / 100}%` : "never rebalance"}</Detail>
                <Detail label="Creator">
                  <a href={accountUrl(b.creator)} target="_blank" rel="noreferrer" className="tnum hover:underline">
                    {shortId(b.creator)}
                  </a>
                </Detail>
                <Detail label="Created">{fmtTime(b.created_at)}</Detail>
                <Detail label="Fees">0.4% on open · 0 on sell</Detail>
              </dl>
            ) : (
              <Skeleton className="h-24 w-full" />
            )}
          </Card>
        </div>

        <aside className="space-y-3 lg:sticky lg:top-24 lg:self-start">
          <div className="rounded-[28px] border border-border bg-secondary/60">
            {b ? <BuyPanel basket={b} initialExec={search.get("exec")} onSettled={positions.reload} /> : <Skeleton className="m-5 h-60" />}
          </div>
          {b && (
            <div className="space-y-4 rounded-[28px] border border-border bg-secondary/60 p-5">
              <span className="text-sm font-medium">Recurring buy</span>
              <ScheduleForm target={{ basket_id: id }} onCreated={() => setRefresh((n) => n + 1)} />
              <ScheduleList basketId={id} refresh={refresh} />
            </div>
          )}
        </aside>
      </div>
    </main>
  );
}

// routing diagram per $1,000; fullscreen via the Fullscreen API (Esc exits)
function SplitCard({ legs, rates }: { legs: Leg[]; rates: Rates | null | undefined }) {
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
        <h2 className="text-2xl font-medium">Routing</h2>
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
        <RoutingFlow amount={1000} legs={legs} rates={rates} compact={!full} fill={full} />
      </div>
    </section>
  );
}
