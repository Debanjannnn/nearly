"use client";

import { useEffect } from "react";
import { Check, ExternalLink } from "lucide-react";
import { errText, shortId } from "@/lib/api";
import { txUrl } from "@/lib/config";
import { settled, useExecution } from "@/lib/hooks";
import { decimalsOf, fmtUnits, KEYS, symbolOf, tokenIdOf } from "@/lib/tokens";
import type { Execution, Step } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Spinner, StatusPill } from "./ui";

const LABEL: Record<Step["kind"], string> = {
  fee: "Open fee → treasury",
  swap: "Swap",
  supply: "Supply to lending",
  withdraw_lend: "Withdraw from lending",
  stake: "Stake to LiNEAR",
  unstake: "Unstake from LiNEAR",
  to_owner: "Send to your wallet",
};

// step tokens are contract ids; stake/unstake move wnear
const keyOf = (id?: string) => KEYS.find((k) => tokenIdOf(k) === id) ?? "wnear";
const amt = (raw: string | undefined, id?: string) => (raw ? `${fmtUnits(raw, decimalsOf(keyOf(id)), 4)} ${symbolOf(keyOf(id))}` : "");

export const stepLabel = (s: Step) =>
  s.kind === "swap"
    ? `Swap ${amt(s.amount_in, s.token_in)} → ${symbolOf(keyOf(s.token_out))}`
    : [LABEL[s.kind] ?? s.kind, amt(s.amount, s.token)].filter(Boolean).join(" · ");

// live step-by-step progress of an executor run (polls until settled)
export function ExecutionProgress({ id, onSettled }: { id: string; onSettled?: (e: Execution) => void }) {
  const ex = useExecution(id);
  const e = ex?.data;
  const done = settled(e);
  useEffect(() => {
    if (e && done) onSettled?.(e);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [done]);
  if (!e) return ex?.error ? <p className="text-xs text-destructive">{errText(new Error(ex.error))}</p> : <Spinner label="Submitting…" />;
  const steps = e.steps ?? [];
  const width = steps.length ? (e.steps_done / steps.length) * 100 : 0;
  return (
    <div className="space-y-3 rounded-2xl border border-border bg-card p-4 text-sm">
      <div className="flex items-center gap-2">
        <span className="font-medium capitalize">{e.kind.replace(/_/g, " ")}</span>
        <StatusPill status={e.status} />
        <span className="tnum ml-auto text-xs text-muted-foreground">
          {e.steps_done}/{steps.length} steps
        </span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-secondary">
        <div className={cn("h-full rounded-full transition-[width] duration-500", e.status === "failed" ? "bg-destructive" : "bg-primary")} style={{ width: `${width}%` }} />
      </div>
      <ol className="space-y-1.5 text-xs">
        {steps.map((s) => (
          <li key={s.index} className={cn("flex items-center gap-2", s.status === "ok" ? "text-foreground" : "text-muted-foreground")}>
            {s.status === "ok" ? (
              <Check className="size-3.5 text-positive" />
            ) : s.index === e.steps_done && !done ? (
              <span className="size-3.5 animate-spin rounded-full border-2 border-border border-t-primary" />
            ) : (
              <span className={cn("size-3.5 rounded-full border", s.status === "failed" ? "border-destructive" : "border-border")} />
            )}
            {stepLabel(s.step)}
            {s.tx_hash && (
              <a href={txUrl(s.tx_hash)} target="_blank" rel="noreferrer" className="ml-auto inline-flex items-center gap-1 hover:text-foreground">
                {shortId(s.tx_hash)}
                <ExternalLink className="size-3" />
              </a>
            )}
          </li>
        ))}
      </ol>
      {e.error && <p className="text-xs break-words text-destructive">{errText(new Error(e.error))}</p>}
    </div>
  );
}
