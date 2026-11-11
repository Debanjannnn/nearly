"use client";

import Link from "next/link";
import { errText, fmtUsd, getBaskets, getExecutions, getMyBaskets, getPositions, shortId } from "@/lib/api";
import { useAsync, useHoldings, usePrices, useRates } from "@/lib/hooks";
import { useSession } from "@/lib/session";
import { Button, ErrorBox, Notice, Panel, Skeleton } from "@/components/ui";
import { Flash } from "@/components/motion";
import { ExecutorCard } from "@/components/automation/ExecutorCard";
import { ExecutionLog, ScheduleList } from "@/components/automation/ScheduleList";
import { blendedApy } from "@/components/basket/legs";
import { EarningsCard } from "./Earnings";
import { Holdings, holdingsUsd } from "./Holdings";
import { depositUsd, PositionsTable, positionValue } from "./Positions";

export default function PortfolioPage() {
  const { ready, authenticated, account, accountId, login, pending, keyStatus } = useSession();
  const on = ready && authenticated && !!account;
  const holdings = useHoldings();
  const { priceOf } = usePrices();
  const { rates } = useRates();
  const positions = useAsync(on ? "positions" : null, getPositions, 15_000);
  const executions = useAsync(on ? "executions:earn" : null, getExecutions, 30_000);
  const pub = useAsync("baskets", getBaskets);
  const mine = useAsync(on ? "my-baskets" : null, getMyBaskets);

  if (ready && !on) {
    return (
      <main className="w-full px-4 pt-16 pb-24 md:px-6">
        <Panel className="mx-auto w-full max-w-md p-8 text-center">
          <h1 className="text-lg font-medium">Your portfolio</h1>
          <p className="mt-2 text-sm text-muted-foreground">Sign in to see your account, positions and schedules.</p>
          <Button className="mt-6 w-full" pending={pending} onClick={() => void login()}>
            {!accountId ? "Connect wallet" : !authenticated ? "Sign in" : "Create account"}
          </Button>
        </Panel>
      </main>
    );
  }

  const baskets = new Map([...(pub.data ?? []), ...(mine.data ?? [])].map((b) => [b.id, b]));
  const names = Object.fromEntries([...baskets.values()].map((b) => [b.id, b.name]));
  const open = (positions.data ?? []).filter((p) => p.state !== "closed");
  const values = open.map((p) => positionValue(p, baskets.get(p.basket_id), priceOf));
  const posUsd = values.every((v) => v !== null) ? values.reduce<number>((s, v) => s + v!, 0) : null;
  const depUsd = open.reduce((s, p) => s + depositUsd(p), 0);
  const free = holdingsUsd(holdings.data, priceOf);
  const total = posUsd === null ? null : free + posUsd;
  const yieldYr = values.every((v) => v !== null)
    ? open.reduce((s, p, i) => s + values[i]! * (blendedApy(baskets.get(p.basket_id)?.definition ?? { legs: [] }, rates) ?? 0), 0)
    : null;

  return (
    <main className="w-full px-4 pt-8 pb-24 md:px-6">
      <div className="mx-auto w-full space-y-6">
        <header className="flex flex-wrap items-baseline justify-between gap-3">
          <h1 className="truncate text-xl font-medium sm:text-2xl">
            <span className="text-muted-foreground">Welcome, </span>
            {accountId ? shortId(accountId) : <Skeleton className="inline-block h-6 w-32 align-middle" />}
          </h1>
          <div className="text-right">
            <div className="text-xs text-muted-foreground">Total value</div>
            {total === null ? (
              <Skeleton className="mt-1 h-9 w-36" />
            ) : (
              <Flash value={total} className="tnum text-3xl font-medium tracking-tight">
                {fmtUsd(total)}
              </Flash>
            )}
          </div>
        </header>

        {keyStatus === "revoked" && <Notice tone="bad">Executor key revoked: buys, sells and schedules are off. Funds stay in your account.</Notice>}
        {holdings.error && <ErrorBox message={`Balances unavailable: ${errText(new Error(holdings.error))}`} />}

        <Holdings h={holdings.data} loading={holdings.loading} priceOf={priceOf} baskets={pub.data} />

        <section className="space-y-3">
          <div className="flex items-center justify-between px-1">
            <h2 className="text-lg font-medium">Positions</h2>
            <Link href="/baskets" className="text-sm text-muted-foreground hover:text-foreground">
              Explore baskets →
            </Link>
          </div>
          {positions.error && <ErrorBox message={errText(new Error(positions.error))} />}
          {positions.loading && !positions.data ? (
            <Skeleton className="h-28 w-full rounded-3xl" />
          ) : (positions.data ?? []).length === 0 ? (
            <Panel className="p-5 text-sm text-muted-foreground">No positions yet.</Panel>
          ) : (
            <PositionsTable positions={positions.data ?? []} basketOf={(id) => baskets.get(id)} priceOf={priceOf} onChange={positions.reload} />
          )}
        </section>

        <EarningsCard executions={executions.data ?? []} valueUsd={posUsd} depositUsd={depUsd} yieldPerYear={yieldYr} />

        <section className="space-y-3">
          <h2 className="px-1 text-lg font-medium">Automation</h2>
          <ExecutorCard />
          <ScheduleList names={names} />
          <ExecutionLog />
        </section>
      </div>
    </main>
  );
}
