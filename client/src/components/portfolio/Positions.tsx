"use client";

import Link from "next/link";
import { Fragment, useState } from "react";
import { ChevronDown } from "lucide-react";
import { errText, fmtTime, fmtUsd, withdrawPosition } from "@/lib/api";
import { FEE_BPS } from "@/lib/config";
import { useTickers } from "@/lib/hooks";
import { decimalsOf, fmtUnits, keyOf, toUnits, USDC } from "@/lib/tokens";
import type { Position, Strategy } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Button, Notice, StatusPill } from "@/components/ui";
import { TokenIcon } from "@/components/TokenIcon";
import { IconStack } from "@/components/IconStack";
import { ScheduleForm } from "@/components/automation/ScheduleForm";
import { ExecutionProgress } from "@/components/Execution";
import { defaultVenue, KIND_LABEL, parts, STAKING_EXIT, venueName, protocolOf } from "@/components/earn/sources";
import { ProtocolIcon } from "@/components/ProtocolIcon";
import { Segmented } from "@/components/motion";

const SELL_PCTS = ["25", "50", "75", "100"] as const;
export const STABLES = new Set(["USDC"]);

type PriceOf = (sym: string) => number | null;

// spot USD price per token symbol from live tickers (stables = $1); null = no quote yet
export function usePriceOf(): PriceOf {
  const { tickers } = useTickers();
  return (sym) => (STABLES.has(sym) ? 1 : (tickers[sym]?.price ?? null));
}

// basket legs grouped by token, each with what the position holds (atoms, same order as the legs)
const groupsOf = (p: Position, s: Strategy | undefined) =>
  parts(s ?? { sleeves: [] }).map((sl) => {
    const key = keyOf(sl.asset);
    const legs = (s?.definition.legs ?? []).flatMap((l, i) => (l.token === key ? [{ ...l, held: p.held[i] ?? "0" }] : []));
    return { token: sl.asset, legs, dec: decimalsOf(key) };
  });

const principalOf = (p: Position) => toUnits(p.deposit, USDC.decimals);

// Entry / mark on the basket's own index (Σ weight × token price, as the basket chart). A token's entry is its effective
// fill: USDC put into it (deposit after the open fee × weight) ÷ tokens received.
// shortcut: legs change on rebalance, so entry drifts after one; store fill prices at deploy if exact entry matters
export function positionPrices(p: Position, s: Strategy | undefined, priceOf: PriceOf) {
  if (!s) return { entry: null, mark: null };
  const net = principalOf(p) * (1 - FEE_BPS / 10000);
  const groups = groupsOf(p, s);
  let entry: number | null = 0;
  let mark: number | null = 0;
  for (const sl of parts(s)) {
    const w = sl.weightBps / 10000;
    const px = priceOf(sl.asset);
    mark = mark === null || px === null ? null : mark + w * px;
    if (STABLES.has(sl.asset)) {
      entry = entry === null ? null : entry + w;
      continue;
    }
    const g = groups.find((x) => x.token === sl.asset);
    const held = g?.legs.reduce((a, l) => a + toUnits(l.held, g.dec), 0) ?? 0;
    entry = entry === null || held <= 0 ? null : entry + (w * (net * w)) / held;
  }
  return { entry, mark };
}

// spot value: held tokens at spot
// shortcut: venue yield accrued since the last run is not marked; read venue balances via RPC if exact value matters
export function valueOf(p: Position, s: Strategy | undefined, priceOf: PriceOf) {
  if (!s) return null;
  return heldOf(p, s).reduce<number | null>((v, h) => {
    const px = priceOf(h.token);
    return v === null || px === null ? null : v + h.amount * px;
  }, 0);
}

// token units the position holds, per token symbol
export const heldOf = (p: Position, s: Strategy) =>
  groupsOf(p, s).map((g) => ({ token: g.token, amount: g.legs.reduce((a, l) => a + toUnits(l.held, g.dec), 0) }));

const signed = (n: number) => (n >= 0 ? "text-positive" : "text-destructive");

export function PositionsTable({
  positions,
  strategyOf,
  onChange,
}: {
  positions: Position[];
  strategyOf: (id: string) => Strategy | undefined;
  onChange: () => void;
}) {
  const priceOf = usePriceOf();
  const [open, setOpen] = useState<string | null>(null);
  return (
    <div className="overflow-x-auto rounded-3xl border border-border bg-card">
      <table className="w-full min-w-[860px] text-sm">
        <thead className="text-left text-xs text-muted-foreground">
          <tr className="border-b border-border">
            <th className="py-3 pl-5 font-normal">Basket</th>
            <th className="px-3 py-3 text-right font-normal">Size</th>
            <th className="px-3 py-3 text-right font-normal">Entry price</th>
            <th className="px-3 py-3 text-right font-normal">Mark price</th>
            <th className="px-3 py-3 text-right font-normal">Value</th>
            <th className="px-3 py-3 text-right font-normal">PnL (ROE)</th>
            <th className="px-3 py-3 font-normal">Opened</th>
            <th className="px-3 py-3 font-normal">Status</th>
            <th className="py-3 pr-5" />
          </tr>
        </thead>
        <tbody className="tnum">
          {positions.map((p) => {
            const s = strategyOf(p.basket_id);
            const { entry, mark } = positionPrices(p, s, priceOf);
            const value = p.state === "closed" ? null : valueOf(p, s, priceOf);
            const principal = principalOf(p);
            const pnl = value !== null ? value - principal : null;
            const expanded = open === p.id;
            return (
              <Fragment key={p.id}>
                <tr
                  onClick={() => setOpen(expanded ? null : p.id)}
                  className={cn("cursor-pointer border-b border-border transition-colors hover:bg-secondary/30", expanded && "bg-secondary/20")}
                >
                  <td className="py-3 pl-5">
                    <div className="flex items-center gap-3">
                      <IconStack symbols={s ? parts(s).map((sl) => sl.asset) : ["USDC"]} size={22} ring="ring-card" />
                      <Link href={`/baskets/${p.basket_id}`} onClick={(e) => e.stopPropagation()} className="truncate font-medium hover:underline">
                        {p.basket_name}
                      </Link>
                    </div>
                  </td>
                  <td className="px-3 py-3 text-right">{fmtUsd(principal)}</td>
                  <td className="px-3 py-3 text-right">{entry !== null ? fmtUsd(entry) : "-"}</td>
                  <td className="px-3 py-3 text-right">{mark !== null ? fmtUsd(mark) : "-"}</td>
                  <td className="px-3 py-3 text-right">{value !== null ? fmtUsd(value) : "-"}</td>
                  <td className={cn("px-3 py-3 text-right", pnl !== null && signed(pnl))}>
                    {pnl !== null && principal > 0 ? `${pnl >= 0 ? "+" : ""}${fmtUsd(pnl)} (${pnl >= 0 ? "+" : ""}${((pnl / principal) * 100).toFixed(2)}%)` : "-"}
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
                    <td colSpan={9} className="px-5 py-4">
                      <PositionDetail p={p} s={s} onChange={onChange} />
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

// expanded row: legs, sell, rebalance schedule, live execution
function PositionDetail({ p, s, onChange }: { p: Position; s: Strategy | undefined; onChange: () => void }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [execId, setExecId] = useState<string | null>(null);
  const [auto, setAuto] = useState(false);
  const [sellPct, setSellPct] = useState<(typeof SELL_PCTS)[number]>("100");
  // withdrawing = a sell that stalled; the api resumes it from its last step
  const closing = p.state === "withdrawing";
  const groups = groupsOf(p, s);

  async function withdraw() {
    setBusy(true);
    setErr(null);
    try {
      const r = await withdrawPosition(p.id, closing ? 10000 : Number(sellPct) * 100);
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
      <ul className="grid gap-x-8 gap-y-1 text-xs md:grid-cols-2">
        {groups.flatMap((g) =>
          g.legs.map((l, i) => (
            <li key={`${g.token}${i}`} className="flex items-center gap-2">
              <TokenIcon symbol={g.token} size={14} />
              <span className="rounded-md bg-secondary/60 px-1.5 py-0.5 text-[10px] text-muted-foreground">{KIND_LABEL[l.source]}</span>
              <ProtocolIcon protocol={protocolOf(defaultVenue(l.source))} size={14} />
              {venueName(defaultVenue(l.source))}
              <span className="tnum ml-auto text-muted-foreground">
                {fmtUnits(l.held, g.dec)} {g.token}
              </span>
            </li>
          )),
        )}
      </ul>
      <p className="text-[11px] text-muted-foreground">
        Value marks held tokens at spot; lending/staking yield accrues on-venue. Selling unwinds each leg and sends the tokens to your wallet, no fee.
      </p>
      {s?.definition.legs.some((l) => l.source === "staking") && <Notice tone="muted">{STAKING_EXIT}</Notice>}
      {closing && <Notice>A sell stopped part way. Resume runs only the steps still left on chain.</Notice>}
      {err && <Notice tone="bad">{err}</Notice>}
      {(p.state === "open" || closing) && (
        <div className="flex flex-wrap items-center gap-2">
          {!closing && (
            <Segmented id={`sell-${p.id}`} value={sellPct} onChange={setSellPct} options={SELL_PCTS.map((v) => ({ value: v, label: `${v}%` }))} className="w-auto" />
          )}
          <Button variant="ghost" pending={busy} onClick={() => void withdraw()}>
            {closing ? "Resume sell" : "Sell"} to wallet
          </Button>
          {!closing && (
            <Button variant="ghost" onClick={() => setAuto((a) => !a)}>
              {auto ? "Hide automation" : "Rebalance"}
            </Button>
          )}
        </div>
      )}
      {auto && <ScheduleForm target={{ type: "position", id: p.id }} onCreated={() => setAuto(false)} />}
      {execId && <ExecutionProgress id={execId} onSettled={onChange} />}
    </div>
  );
}
