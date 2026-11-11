"use client";

import Link from "next/link";
import { useState } from "react";
import { Pause, Play, Trash2 } from "lucide-react";
import { deleteSchedule, errText, fmtTime, fmtUsd, getExecutions, getSchedules, patchSchedule } from "@/lib/api";
import { useAsync } from "@/lib/hooks";
import { useSession } from "@/lib/session";
import { decimalsOf, toUnits } from "@/lib/tokens";
import type { Schedule } from "@/lib/types";
import { ErrorBox, Panel, Skeleton, StatusPill } from "@/components/ui";
import { KIND_LABEL } from "./ScheduleForm";

const every = (s: number) =>
  s % 604800 === 0 ? `${s / 604800}w` : s % 86400 === 0 ? `${s / 86400}d` : s % 3600 === 0 ? `${s / 3600}h` : s % 60 === 0 ? `${s / 60}m` : `${s}s`;

const describe = (s: Schedule) =>
  s.kind === "rebalance"
    ? `Check every ${every(s.every_seconds)}${s.params.drift_bps ? `, drift > ${s.params.drift_bps / 100}%` : ""}`
    : `Buy ${fmtUsd(toUnits(s.params.amount ?? "0", decimalsOf("usdc")))} every ${every(s.every_seconds)}`;

// schedules, optionally for one basket; refreshes every 15s
export function ScheduleList({ basketId, names, refresh = 0 }: { basketId?: string; names?: Record<string, string>; refresh?: number }) {
  const { authenticated, keyStatus } = useSession();
  const schedules = useAsync(authenticated ? `schedules:${refresh}` : null, getSchedules, 15_000);
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  if (!authenticated) return null;

  const mine = (schedules.data ?? []).filter((s) => !basketId || s.basket_id === basketId).sort((x, y) => Number(y.active) - Number(x.active));

  async function act(id: string, fn: () => Promise<unknown>) {
    setBusy(id);
    setErr(null);
    try {
      await fn();
      schedules.reload();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(null);
    }
  }

  return (
    <Panel>
      <div className="px-5 pt-4 text-sm font-medium">Schedules</div>
      {(schedules.error || err) && (
        <div className="px-5 pt-3">
          <ErrorBox message={err ?? errText(new Error(schedules.error!))} />
        </div>
      )}
      {schedules.loading && !schedules.data ? (
        <div className="space-y-3 p-5">
          <Skeleton className="h-4 w-2/3" />
          <Skeleton className="h-4 w-1/2" />
        </div>
      ) : mine.length === 0 ? (
        <p className="p-5 text-sm text-muted-foreground">No schedules yet.</p>
      ) : (
        <ul className="divide-y divide-border">
          {mine.map((s) => (
            <li key={s.id} className={s.active ? "p-5" : "p-5 opacity-60"}>
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className="rounded-md bg-secondary/60 px-2 py-0.5 text-[11px] text-muted-foreground">{KIND_LABEL[s.kind] ?? s.kind}</span>
                <span className="tnum text-sm">{describe(s)}</span>
                {!basketId && s.basket_id && (
                  <Link href={`/baskets/${s.basket_id}`} className="text-xs text-muted-foreground underline-offset-4 hover:underline">
                    {names?.[s.basket_id] ?? `basket ${s.basket_id.slice(0, 8)}`}
                  </Link>
                )}
                {s.position_id && <span className="text-xs text-muted-foreground">position {s.position_id.slice(0, 8)}</span>}
                <span className="ml-auto">
                  <StatusPill status={s.active ? "active" : "paused"} />
                </span>
                {(s.active || keyStatus === "active") && (
                  <button
                    type="button"
                    aria-label={s.active ? "Pause schedule" : "Resume schedule"}
                    disabled={busy === s.id}
                    onClick={() => void act(s.id, () => patchSchedule(s.id, { active: !s.active }))}
                    className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground disabled:opacity-50"
                  >
                    {s.active ? <Pause className="size-4" /> : <Play className="size-4" />}
                  </button>
                )}
                <button
                  type="button"
                  aria-label="Delete schedule"
                  disabled={busy === s.id}
                  onClick={() => void act(s.id, () => deleteSchedule(s.id))}
                  className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-destructive disabled:opacity-50"
                >
                  <Trash2 className="size-4" />
                </button>
              </div>
              <div className="tnum mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                {s.active && <span>next {fmtTime(s.next_run_at)}</span>}
                <span className="ml-auto">created {fmtTime(s.created_at)}</span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

// every api + scheduler run for the user, newest first
export function ExecutionLog() {
  const { authenticated } = useSession();
  const ex = useAsync(authenticated ? "executions" : null, getExecutions, 15_000);
  const runs = (ex.data ?? []).slice(0, 30);
  return (
    <Panel>
      <div className="px-5 pt-4 text-sm font-medium">Executions</div>
      {ex.error && (
        <div className="p-5">
          <ErrorBox message={errText(new Error(ex.error))} />
        </div>
      )}
      {ex.loading && !ex.data ? (
        <div className="p-5">
          <Skeleton className="h-4 w-1/2" />
        </div>
      ) : runs.length === 0 ? (
        <p className="p-5 text-sm text-muted-foreground">No executions yet.</p>
      ) : (
        <ul className="divide-y divide-border">
          {runs.map((e) => (
            <li key={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-5 py-3 text-sm">
              <StatusPill status={e.status} />
              <span className="capitalize">{e.kind.replace(/_/g, " ")}</span>
              {e.kind === "withdraw" && e.params.bps && <span className="tnum text-xs text-muted-foreground">{e.params.bps / 100}%</span>}
              {e.params.resume_of && <span className="rounded-md bg-secondary/60 px-1.5 py-0.5 text-[10px] text-muted-foreground">resume</span>}
              {e.params.schedule_id && <span className="rounded-md bg-secondary/60 px-1.5 py-0.5 text-[10px] text-muted-foreground">auto</span>}
              {e.kind === "deploy" && e.params.amount && (
                <span className="tnum text-xs text-muted-foreground">{fmtUsd(toUnits(e.params.amount, decimalsOf("usdc")))}</span>
              )}
              <span className="tnum text-xs text-muted-foreground">{e.steps_done} steps done</span>
              {e.error && (
                <span className="text-xs text-destructive" title={e.error}>
                  {errText(new Error(e.error))}
                </span>
              )}
              <span className="ml-auto text-xs text-muted-foreground">{fmtTime(e.created_at)}</span>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
