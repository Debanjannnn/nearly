"use client";

import { fmtUsd } from "@/lib/api";
import { decimalsOf, toUnits } from "@/lib/tokens";
import type { Execution } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Panel } from "@/components/ui";

// yield and fees from what the api returns; creator rev-share has no api route yet
export function EarningsCard({ executions, valueUsd, depositUsd, yieldPerYear }: { executions: Execution[]; valueUsd: number | null; depositUsd: number; yieldPerYear: number | null }) {
  const fees = executions
    .filter((e) => e.status === "succeeded" && e.params.fee)
    .reduce((s, e) => s + toUnits(e.params.fee!, decimalsOf("usdc")), 0);
  const pnl = valueUsd === null ? null : valueUsd - depositUsd;
  const items: [string, React.ReactNode][] = [
    ["Unrealized PnL", pnl === null ? "-" : <span className={cn(pnl >= 0 ? "text-positive" : "text-destructive")}>{`${pnl >= 0 ? "+" : ""}${fmtUsd(pnl)}`}</span>],
    ["Est. yield / year", yieldPerYear === null ? "-" : <span className="text-positive">{fmtUsd(yieldPerYear)}</span>],
    ["Open fees paid", fmtUsd(fees)],
  ];
  return (
    <section className="space-y-3">
      <h2 className="px-1 text-lg font-medium">Earnings</h2>
      <Panel className="p-5">
        <dl className="tnum grid grid-cols-3 gap-3 text-sm">
          {items.map(([k, v]) => (
            <div key={k}>
              <dt className="text-xs text-muted-foreground">{k}</dt>
              <dd className="mt-1 text-lg font-medium">{v}</dd>
            </div>
          ))}
        </dl>
        <p className="mt-3 text-[11px] text-muted-foreground">PnL marks open positions at spot against what you deposited. Yield uses live venue APYs.</p>
      </Panel>
    </section>
  );
}
