"use client";

import { useState } from "react";
import { createSchedule, errText } from "@/lib/api";
import { parseAmount, USDC } from "@/lib/tokens";
import { useSession } from "@/lib/session";
import type { ScheduleInput, ScheduleKind, Target } from "@/lib/types";
import { Button, ErrorBox, Label, Notice } from "@/components/ui";
import { Segmented } from "@/components/motion";
import { showToast } from "@/components/TxToast";
import { DelegationCard } from "./Delegation";
import { cn } from "@/lib/utils";

const KINDS: Record<Target["type"], ScheduleKind[]> = {
  // token schedules have no backend yet
  market: [],
  position: ["rebalance"],
  // each run deploys the amount into the basket (recurring buy: new position; DCA: tokens held in your account)
  strategy: ["recurring_buy", "dca"],
};
export const KIND_LABEL: Record<string, string> = {
  dca: "DCA",
  recurring_buy: "Recurring buy",
  rebalance: "Rebalance",
};

const INTERVALS = [
  { value: "3600", label: "1h" },
  { value: "86400", label: "1d" },
  { value: "604800", label: "1w" },
  { value: "custom", label: "Custom" },
] as const;
type IntervalOpt = (typeof INTERVALS)[number]["value"];

const DEC = /^\d+(\.\d+)?$/;
const pos = (s: string) => DEC.test(s.trim()) && Number(s) > 0;

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

// current / quote kept for the shared call shape (token pages pass a live price)
export function ScheduleForm({ target, onCreated }: { target: Target; current?: number | null; onCreated: () => void; quote?: string }) {
  const { address, delegated, ensureReady } = useSession();
  const kinds = KINDS[target.type];
  const [kind, setKind] = useState<ScheduleKind>(kinds[0] ?? "dca");
  const [amount, setAmount] = useState("25");
  const [every, setEvery] = useState<IntervalOpt>("86400");
  const [customMin, setCustomMin] = useState("30");
  const [drift, setDrift] = useState("5");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const everySeconds = every === "custom" ? Math.round(Number(customMin) * 60) : Number(every);
  const driftBps = Math.round(Number(drift) * 100);

  function build(): ScheduleInput | string {
    if (!(everySeconds >= 60)) return kind === "rebalance" ? "Check cadence must be at least 1 minute." : "Interval must be at least 1 minute.";
    if (kind === "rebalance") {
      if (drift.trim() && !(driftBps > 0 && driftBps <= 10_000)) return "Drift band must be 0.01%–100%.";
      return { kind, position_id: target.id, every_seconds: everySeconds, params: drift.trim() ? { drift_bps: driftBps } : {} };
    }
    const raw = pos(amount) ? parseAmount(amount, USDC.decimals) : null;
    if (!raw) return "Amount must be a positive USDC amount.";
    return { kind, basket_id: target.id, every_seconds: everySeconds, params: { amount: raw.toString() } };
  }

  async function submit() {
    const input = build();
    setError(typeof input === "string" ? input : null);
    if (typeof input === "string") return;
    if (!(await ensureReady())) return;
    setPending(true);
    try {
      await createSchedule(input);
      showToast({ tone: "success", title: `${KIND_LABEL[kind]} scheduled` });
      onCreated();
    } catch (e) {
      setError(errText(e));
    } finally {
      setPending(false);
    }
  }

  if (kinds.length === 0)
    return <Notice tone="muted">Token DCA and take profit / stop loss are coming soon. Set a recurring buy on a basket today.</Notice>;

  const intervalPicker = (label: string) => (
    <>
      <Field label={label}>
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
    </>
  );

  return (
    <div className="space-y-4">
      {kinds.length > 1 && (
        <Segmented
          id={`schedule-kind-${target.type}`}
          value={kind}
          onChange={(k) => (setKind(k), setError(null))}
          options={kinds.map((k) => ({ value: k, label: KIND_LABEL[k] }))}
        />
      )}

      {kind !== "rebalance" && (
        <>
          <Field label="Buy per run (USDC)">
            <input className={inputCls} inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} />
          </Field>
          {intervalPicker("Every")}
        </>
      )}

      {kind === "rebalance" && (
        <>
          <Field label="Drift band (%)" hint="blank = basket rules">
            <input className={inputCls} inputMode="decimal" value={drift} onChange={(e) => setDrift(e.target.value)} />
          </Field>
          {intervalPicker("Check every")}
        </>
      )}

      {error && <ErrorBox message={error} />}

      {delegated || !address ? (
        <Button className="w-full" pending={pending} onClick={() => void submit()}>
          Create {KIND_LABEL[kind].toLowerCase()}
        </Button>
      ) : (
        <DelegationCard compact />
      )}
    </div>
  );
}
