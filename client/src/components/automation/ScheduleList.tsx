"use client";

import Link from "next/link";
import { useState } from "react";
import { ExternalLink, Pause, Play, Trash2 } from "lucide-react";
import { deleteSchedule, errText, fmtBps, fmtTime, fmtUsd, getExecutions, getSchedules, patchSchedule, shortHash } from "@/lib/api";
import { toUnits, USDC } from "@/lib/tokens";
import { useAsync, useSession } from "@/lib/session";
import { txUrl } from "@/lib/config";
import type { Schedule, Target } from "@/lib/types";
import { ErrorBox, Panel, Skeleton, StatusPill } from "@/components/ui";
import { toastError } from "@/components/TxToast";
import { stepLabel } from "@/components/Execution";
import { KIND_LABEL } from "./ScheduleForm";

const every = (s: number) =>
  s % 604800 === 0 ? `${s / 604800}w` : s % 86400 === 0 ? `${s / 86400}d` : s % 3600 === 0 ? `${s / 3600}h` : s % 60 === 0 ? `${s / 60}m` : `${s}s`;

const usdc = (raw?: string) => fmtUsd(toUnits(raw ?? "0", USDC.decimals));
const describe = (s: Schedule) =>
  s.kind === "rebalance"
    ? `Check every ${every(s.every_seconds)}${s.params.drift_bps ? `, drift > ${fmtBps(s.params.drift_bps)}` : ""}`
    : `Buy ${usdc(s.params.amount)} every ${every(s.every_seconds)}`;

const targetOf = (s: Schedule): Target | null =>
  s.basket_id ? { type: "strategy", id: s.basket_id } : s.position_id ? { type: "position", id: s.position_id } : null;
const targetHref = (t: Target | null) => (!t ? null : t.type === "strategy" ? `/baskets/${t.id}` : "/portfolio");

// Schedules (optionally for one target); refreshes every 15s.
export function ScheduleList({ target, names }: { target?: Target; names?: Record<string, string> }) {
  const { authenticated, delegated } = useSession();
  const schedules = useAsync(authenticated ? "schedules" : null, getSchedules, 15_000);
  const [deleting, setDeleting] = useState<string | null>(null);
  const { reload } = schedules;

  const mine = (schedules.data ?? [])
    .filter((s) => !target || (targetOf(s)?.type === target.type && targetOf(s)?.id === target.id))
    .sort((x, y) => Number(y.active) - Number(x.active));

  // delete, or pause / resume (active flag)
  async function act(id: string, fn: () => Promise<unknown>) {
    setDeleting(id);
    try {
      await fn();
      reload();
    } catch (e) {
      toastError(new Error(errText(e)));
    } finally {
      setDeleting(null);
    }
  }

  return (
    <Panel>
      <div className="px-5 pt-4 text-sm font-medium">Automations</div>
      {schedules.error && (
        <div className="p-5">
          <ErrorBox message={errText(new Error(schedules.error))} />
        </div>
      )}
      {schedules.loading && !schedules.data ? (
        <div className="space-y-3 p-5">
          <Skeleton className="h-4 w-2/3" />
          <Skeleton className="h-4 w-1/2" />
        </div>
      ) : mine.length === 0 ? (
        <p className="p-5 text-sm text-muted-foreground">No automations yet.</p>
      ) : (
        <ul className="divide-y divide-border">
          {mine.map((s) => {
            const t = targetOf(s);
            const href = !target ? targetHref(t) : null;
            return (
              <li key={s.id} className={s.active ? "p-5" : "p-5 opacity-60"}>
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <span className="rounded-md bg-secondary/60 px-2 py-0.5 text-[11px] text-muted-foreground">{KIND_LABEL[s.kind] ?? s.kind}</span>
                  <span className="tnum text-sm">{describe(s)}</span>
                  {href && t && (
                    <Link href={href} className="text-xs text-muted-foreground underline-offset-4 hover:underline">
                      {names?.[t.id] ?? `${t.type === "strategy" ? "basket" : t.type} ${t.id.slice(0, 8)}`}
                    </Link>
                  )}
                  <span className="ml-auto">
                    <StatusPill status={s.active ? "active" : "paused"} />
                  </span>
                  {(s.active || delegated) && (
                    <button
                      type="button"
                      aria-label={s.active ? "Pause automation" : "Resume automation"}
                      disabled={deleting === s.id}
                      onClick={() => void act(s.id, () => patchSchedule(s.id, { active: !s.active }))}
                      className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground disabled:opacity-50"
                    >
                      {s.active ? <Pause className="size-4" /> : <Play className="size-4" />}
                    </button>
                  )}
                  <button
                    type="button"
                    aria-label="Delete automation"
                    disabled={deleting === s.id}
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
            );
          })}
        </ul>
      )}
    </Panel>
  );
}

// Every api + scheduler execution for the user, newest first.
export function ExecutionLog() {
  const { authenticated } = useSession();
  const ex = useAsync(authenticated ? "executions" : null, getExecutions, 15_000);
  const runs = [...(ex.data ?? [])].sort((x, y) => y.created_at.localeCompare(x.created_at)).slice(0, 30);
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
          {runs.map((e) => {
            const hash = e.steps?.findLast((x) => x.tx_hash)?.tx_hash;
            return (
              <li key={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-5 py-3 text-sm">
                <StatusPill status={e.status} />
                <span className="capitalize">{e.kind.replace(/_/g, " ")}</span>
                <span className="truncate text-xs text-muted-foreground">
                  {e.kind === "deploy" && e.params.amount ? usdc(e.params.amount) : e.kind === "withdraw" && e.params.bps ? `${e.params.bps / 100}%` : ""}
                  {(e.steps ?? []).length ? ` ${(e.steps ?? []).map((x) => stepLabel(x.step)).slice(0, 3).join(", ")}` : ""}
                </span>
                <span className="tnum text-xs text-muted-foreground">
                  {e.steps_done}/{e.steps?.length ?? e.steps_done}
                </span>
                {e.params.resume_of && <span className="rounded-md bg-secondary/60 px-1.5 py-0.5 text-[10px] text-muted-foreground">resume</span>}
                {e.params.schedule_id && <span className="rounded-md bg-secondary/60 px-1.5 py-0.5 text-[10px] text-muted-foreground">auto</span>}
                {e.error && (
                  <span className="text-xs text-destructive" title={e.error}>
                    {errText(new Error(e.error))}
                  </span>
                )}
                <span className="ml-auto text-xs text-muted-foreground">{fmtTime(e.created_at)}</span>
                {hash && (
                  <a href={txUrl(hash)} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
                    {shortHash(hash)}
                    <ExternalLink className="size-3" />
                  </a>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </Panel>
  );
}
