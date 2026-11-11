"use client";

import Link from "next/link";
import { Fragment, useState } from "react";
import { ChevronDown } from "lucide-react";
import { errText, fmtTime, fmtUsd, withdrawPosition } from "@/lib/api";
import type { PriceOf } from "@/lib/hooks";
import { decimalsOf, fmtUnits, symbolOf, toUnits } from "@/lib/tokens";
import type { Basket, Position } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Button, Notice, StatusPill } from "@/components/ui";
import { IconStack, TokenIcon } from "@/components/TokenIcon";
import { Segmented } from "@/components/motion";
import { ExecutionProgress } from "@/components/Execution";
import { ScheduleForm } from "@/components/automation/ScheduleForm";
import { SOURCE_LABEL, tokensOf, VENUE_NAME } from "@/components/basket/legs";

const SELL_PCTS = ["25", "50", "75", "100"] as const;

export const depositUsd = (p: Position) => toUnits(p.deposit, decimalsOf("usdc"));

// held legs at spot; null = basket or a price unknown
// shortcut: venue yield accrued since the last run is not marked, read venue balances via RPC if exact value matters
export function positionValue(p: Position, b: Basket | undefined, priceOf: PriceOf): number | null {
  if (!b || p.state === "closed") return p.state === "closed" ? 0 : null;
  return b.definition.legs.reduce<number | null>((v, l, i) => {
    const px = priceOf(l.token);
    return v === null || px === null ? null : v + toUnits(p.held[i] ?? "0", decimalsOf(l.token)) * px;
  }, 0);
}

const signed = (n: number) => (n >= 0 ? "text-positive" : "text-destructive");

export function PositionsTable({
  positions,
  basketOf,
  priceOf,
  onChange,
}: {
  positions: Position[];
  basketOf: (id: string) => Basket | undefined;
  priceOf: PriceOf;
  onChange: () => void;
}) {
  const [open, setOpen] = useState<string | null>(null);
  return (
    <div className="overflow-x-auto rounded-3xl border border-border bg-card">
      <table className="w-full min-w-[760px] text-sm">
        <thead className="text-left text-xs text-muted-foreground">
          <tr className="border-b border-border">
            <th className="py-3 pl-5 font-normal">Basket</th>
            <th className="px-3 py-3 text-right font-normal">Deposit</th>
            <th className="px-3 py-3 text-right font-normal">Value</th>
            <th className="px-3 py-3 text-right font-normal">PnL</th>
            <th className="px-3 py-3 font-normal">Opened</th>
            <th className="px-3 py-3 font-normal">Status</th>
            <th className="py-3 pr-5" />
          </tr>
        </thead>
        <tbody className="tnum">
          {positions.map((p) => {
            const b = basketOf(p.basket_id);
            const dep = depositUsd(p);
            const value = p.state === "closed" ? null : positionValue(p, b, priceOf);
            const pnl = value !== null ? value - dep : null;
            const expanded = open === p.id;
            return (
              <Fragment key={p.id}>
                <tr
                  onClick={() => setOpen(expanded ? null : p.id)}
                  className={cn("cursor-pointer border-b border-border transition-colors hover:bg-secondary/30", expanded && "bg-secondary/20")}
                >
                  <td className="py-3 pl-5">
                    <div className="flex items-center gap-3">
                      <IconStack tokens={b ? tokensOf(b.definition) : ["usdc"]} size={22} ring="ring-card" />
                      <Link href={`/baskets/${p.basket_id}`} onClick={(e) => e.stopPropagation()} className="truncate font-medium hover:underline">
                        {p.basket_name}
                      </Link>
                    </div>
                  </td>
                  <td className="px-3 py-3 text-right">{fmtUsd(dep)}</td>
                  <td className="px-3 py-3 text-right">{value !== null ? fmtUsd(value) : "-"}</td>
                  <td className={cn("px-3 py-3 text-right", pnl !== null && signed(pnl))}>
                    {pnl !== null && dep > 0 ? `${pnl >= 0 ? "+" : ""}${fmtUsd(pnl)} (${pnl >= 0 ? "+" : ""}${((pnl / dep) * 100).toFixed(2)}%)` : "-"}
                  </td>
                  <td className="px-3 py-3 text-xs whitespace-nowrap text-muted-foreground">{fmtTime(p.created_at)}</td>
                  <td className="px-3 py-3">
                    <StatusPill status={p.state} />
                  </td>
                  <td className="py-3 pr-5 text-right">
                    <ChevronDown className={cn("inline size-4 text-muted-foreground transition-transform", expanded && "rotate-180")} />
                  </td>
                </tr>
                {expanded && (
                  <tr className="border-b border-border bg-secondary/10">
                    <td colSpan={7} className="px-5 py-4">
                      <PositionDetail p={p} b={b} onChange={onChange} />
                    </td>
                  </tr>
                )}
              </Fragment>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// expanded row: legs, partial / resume sell, rebalance schedule, live execution
function PositionDetail({ p, b, onChange }: { p: Position; b: Basket | undefined; onChange: () => void }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [execId, setExecId] = useState<string | null>(null);
  const [auto, setAuto] = useState(false);
  const [sellPct, setSellPct] = useState<(typeof SELL_PCTS)[number]>("100");
  // withdrawing = a sell that stalled; the api resumes it from its last step
  const stalled = p.state === "withdrawing";

  async function sell() {
    setBusy(true);
    setErr(null);
    try {
      const r = await withdrawPosition(p.id, Number(sellPct) * 100);
      if (r.execution_id) setExecId(r.execution_id);
      else onChange(); // nothing left on chain: settled at once
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      {b ? (
        <ul className="grid gap-x-8 gap-y-1 text-xs md:grid-cols-2">
          {b.definition.legs.map((l, i) => (
            <li key={i} className="flex items-center gap-2">
              <TokenIcon token={l.token} size={14} />
              <span className="rounded-md bg-secondary/60 px-1.5 py-0.5 text-[10px] text-muted-foreground">{SOURCE_LABEL[l.source]}</span>
              {VENUE_NAME[l.source]}
              <span className="tnum ml-auto text-muted-foreground">
                {fmtUnits(p.held[i] ?? "0", decimalsOf(l.token), 4)} {symbolOf(l.token)}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-xs text-muted-foreground">Basket details unavailable.</p>
      )}
      <p className="text-[11px] text-muted-foreground">
        Value marks held tokens at spot; venue yield accrues on the venue. Selling unwinds each leg and sends the tokens to your wallet as they are, no fee.
      </p>
      {stalled && <Notice>A sell stopped part way. Resume runs only the steps still left on chain.</Notice>}
      {err && <Notice tone="bad">{err}</Notice>}
      {(p.state === "open" || stalled) && (
        <div className="flex flex-wrap items-center gap-2">
          {!stalled && (
            <Segmented id={`sell-${p.id}`} value={sellPct} onChange={setSellPct} options={SELL_PCTS.map((v) => ({ value: v, label: `${v}%` }))} className="w-auto" />
          )}
          <Button variant="ghost" pending={busy} onClick={() => void sell()}>
            {stalled ? "Resume sell" : `Sell ${sellPct}%`}
          </Button>
          {!stalled && (
            <Button variant="ghost" onClick={() => setAuto((a) => !a)}>
              {auto ? "Hide rebalance" : "Rebalance schedule"}
            </Button>
          )}
        </div>
      )}
      {auto && <ScheduleForm target={{ position_id: p.id }} onCreated={() => setAuto(false)} />}
      {execId && <ExecutionProgress id={execId} onSettled={onChange} />}
    </div>
  );
}
