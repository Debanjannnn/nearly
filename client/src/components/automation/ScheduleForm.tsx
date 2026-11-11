"use client";

import { useState } from "react";
import { createSchedule, errText } from "@/lib/api";
import { useSession } from "@/lib/session";
import { decimalsOf, parseAmount } from "@/lib/tokens";
import type { ScheduleInput, ScheduleKind } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Button, ErrorBox, Label, Notice } from "@/components/ui";
import { Segmented } from "@/components/motion";
import { ExecutorCard } from "./ExecutorCard";

export const KIND_LABEL: Record<ScheduleKind, string> = {
  recurring_buy: "Recurring buy",
  dca: "DCA",
  rebalance: "Rebalance",
};
const HINT: Record<ScheduleKind, string> = {
  recurring_buy: "Each run buys the full basket as a new position.",
  dca: "Each run buys the basket tokens and holds them in your account.",
  rebalance: "Each run moves legs back to target weights once they drift past the band.",
};

const INTERVALS = [
  { value: "3600", label: "1h" },
  { value: "86400", label: "1d" },
  { value: "604800", label: "1w" },
  { value: "custom", label: "Custom" },
] as const;
type IntervalOpt = (typeof INTERVALS)[number]["value"];

export const inputCls =
  "tnum w-full rounded-xl border border-border bg-card px-3 py-2.5 text-sm outline-none transition-colors placeholder:text-muted-foreground/60 focus:border-ring";

export function Field({ label, hint, children }: { label: string; hint?: React.ReactNode; children: React.ReactNode }) {
  return (
    <label className="block space-y-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <Label>{label}</Label>
        {hint && <span className="text-xs text-muted-foreground">{hint}</span>}
      </div>
      {children}
    </label>
  );
}

// target: a basket (recurring buy / DCA) or an open position (rebalance)
export function ScheduleForm({ target, onCreated }: { target: { basket_id: string } | { position_id: string }; onCreated: () => void }) {
  const { authenticated, login, keyStatus } = useSession();
  const kinds: ScheduleKind[] = "basket_id" in target ? ["recurring_buy", "dca"] : ["rebalance"];
  const [kind, setKind] = useState<ScheduleKind>(kinds[0]);
  const [amount, setAmount] = useState("25");
  const [every, setEvery] = useState<IntervalOpt>("86400");
  const [customMin, setCustomMin] = useState("30");
  const [drift, setDrift] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  const everySeconds = every === "custom" ? Math.round(Number(customMin) * 60) : Number(every);

  function build(): ScheduleInput | string {
    if (!(everySeconds >= 60)) return "Interval must be at least 1 minute.";
    if (kind === "rebalance") {
      const bps = Math.round(Number(drift) * 100);
      if (drift.trim() && !(bps > 0 && bps <= 10_000)) return "Drift band must be 0.01%–100%.";
      return { kind, position_id: (target as { position_id: string }).position_id, every_seconds: everySeconds, params: drift.trim() ? { drift_bps: bps } : {} };
    }
    const raw = parseAmount(amount, decimalsOf("usdc"));
    if (!raw) return "Amount must be a positive USDC amount.";
    return { kind, basket_id: (target as { basket_id: string }).basket_id, every_seconds: everySeconds, params: { amount: raw.toString() } };
  }

  async function submit() {
    const input = build();
    setError(typeof input === "string" ? input : null);
    if (typeof input === "string") return;
    setPending(true);
    setDone(false);
    try {
      await createSchedule(input);
      setDone(true);
      onCreated();
    } catch (e) {
      setError(errText(e));
    } finally {
      setPending(false);
    }
  }

  if (!authenticated)
    return (
      <div className="flex items-center justify-between gap-3 text-sm">
        <span className="text-muted-foreground">Sign in to automate.</span>
        <Button onClick={() => void login()}>Sign in</Button>
      </div>
    );

  return (
    <div className="space-y-4">
      {kinds.length > 1 && (
        <Segmented id={`schedule-kind-${kinds[0]}`} value={kind} onChange={(k) => (setKind(k), setError(null))} options={kinds.map((k) => ({ value: k, label: KIND_LABEL[k] }))} />
      )}
      <p className="text-xs text-muted-foreground">{HINT[kind]}</p>

      {kind === "rebalance" ? (
        <Field label="Drift band (%)" hint="blank = basket rule">
          <input className={inputCls} inputMode="decimal" placeholder="5" value={drift} onChange={(e) => setDrift(e.target.value)} />
        </Field>
      ) : (
        <Field label="Buy per run (USDC)">
          <input className={inputCls} inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} />
        </Field>
      )}

      <Field label={kind === "rebalance" ? "Check every" : "Every"}>
        <div className="flex flex-wrap gap-1.5">
          {INTERVALS.map((o) => (
            <button
              key={o.value}
              type="button"
              onClick={() => setEvery(o.value)}
              className={cn(
                "rounded-full px-3 py-1.5 text-xs font-medium transition-colors",
                every === o.value ? "bg-primary text-primary-foreground" : "bg-secondary/60 text-muted-foreground hover:text-foreground",
              )}
            >
              {o.label}
            </button>
          ))}
        </div>
      </Field>
      {every === "custom" && (
        <Field label="Custom interval (minutes, min 1)">
          <input className={inputCls} inputMode="decimal" value={customMin} onChange={(e) => setCustomMin(e.target.value)} />
        </Field>
      )}

      {error && <ErrorBox message={error} />}
      {done && <Notice tone="good">{KIND_LABEL[kind]} scheduled.</Notice>}

      {keyStatus === "active" ? (
        <Button className="w-full" pending={pending} onClick={() => void submit()}>
          Create {KIND_LABEL[kind].toLowerCase()}
        </Button>
      ) : (
        <ExecutorCard compact />
      )}
    </div>
  );
}
