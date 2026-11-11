"use client";

import { ChevronDown, ShieldCheck } from "lucide-react";
import { useSession } from "@/lib/session";
import { cn } from "@/lib/utils";
import { Button, ErrorBox, Notice, Panel } from "@/components/ui";

const POLICY = [
  "Supply / withdraw on Rhea lending",
  "Swap on the Rhea USDC/wNEAR pool",
  "Stake / unstake on LiNEAR",
  "Send the open fee only to the nearly treasury",
  "Send funds back only to you, the owner",
];

// executor function-call key status + revoke; compact = inline prompt inside buy / schedule forms
export function ExecutorCard({ compact = false }: { compact?: boolean }) {
  const { keyStatus, revoke, pending, error, account } = useSession();
  const on = keyStatus === "active";
  const revoked = keyStatus === "revoked";
  const body = (
    <div className="space-y-4 p-5">
      <div className="flex items-start gap-3">
        <span className={cn("grid size-9 shrink-0 place-items-center rounded-full", on ? "bg-positive/15 text-positive" : "bg-secondary text-muted-foreground")}>
          <ShieldCheck className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-2 font-medium">
            Automation key
            <span className={cn("rounded-md px-1.5 py-0.5 text-[11px]", on ? "bg-positive/15 text-positive" : revoked ? "bg-destructive/15 text-destructive" : "bg-secondary text-muted-foreground")}>
              {on ? "On" : revoked ? "Revoked" : (keyStatus ?? "Off")}
            </span>
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {on
              ? "Runs buys, sells and schedules from your account. Revoking deletes the key on chain and stops every schedule."
              : revoked
                ? "Key deleted on chain. Buys, sells and schedules are off; your funds stay in your account and you can withdraw them with your wallet."
                : "Your account is created with a limited executor key. Sign in to set it up."}
          </p>
        </div>
      </div>
      <details className="group text-xs text-muted-foreground">
        <summary className="flex cursor-pointer list-none items-center gap-1 hover:text-foreground [&::-webkit-details-marker]:hidden">
          What can it do?
          <ChevronDown className="size-3.5 transition-transform group-open:rotate-180" />
        </summary>
        <ul className="mt-2 space-y-1 pl-4 leading-5 [&>li]:list-disc">
          {POLICY.map((l) => (
            <li key={l}>{l}</li>
          ))}
          <li>Everything else is rejected by your account contract. Per-token caps apply.</li>
        </ul>
      </details>
      {on && account && (
        <Button variant="ghost" className="w-full" pending={pending} onClick={() => void revoke()}>
          Revoke executor key
        </Button>
      )}
      {revoked && compact && <Notice tone="bad">Automation is revoked for this account.</Notice>}
      {error && <ErrorBox message={error} />}
    </div>
  );
  return compact ? <div className="rounded-xl border border-border bg-secondary/30">{body}</div> : <Panel>{body}</Panel>;
}
