"use client";

import { ChevronDown, ShieldCheck } from "lucide-react";
import { useSession } from "@/lib/session";
import { Button, ErrorBox, Panel } from "@/components/ui";
import { cn } from "@/lib/utils";

const POLICY = [
  "Supply / withdraw on Rhea Lending",
  "Swap on the Rhea DEX USDC/wNEAR pool",
  "Stake / unstake on LiNEAR",
  "Send the open fee only to the nearly treasury",
  "Send funds back only to you, the owner",
];

// Executor function-call key on your NEAR account, with enable (create account) / revoke; compact = inline prompt used inside forms.
export function DelegationCard({ compact = false }: { compact?: boolean }) {
  const { delegated, delegate, revoke, delegating, delegationError, keyStatus, address } = useSession();
  const revoked = keyStatus === "revoked";
  const body = (
    <div className="space-y-4 p-5">
      <div className="flex items-start gap-3">
        <span className={cn("grid size-9 shrink-0 place-items-center rounded-full", delegated ? "bg-positive/15 text-positive" : "bg-secondary text-muted-foreground")}>
          <ShieldCheck className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-2 font-medium">
            Automation
            <span className={cn("rounded-md px-1.5 py-0.5 text-[11px]", delegated ? "bg-positive/15 text-positive" : revoked ? "bg-destructive/15 text-destructive" : "bg-secondary text-muted-foreground")}>
              {delegated ? "On" : revoked ? "Revoked" : "Off"}
            </span>
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {delegated
              ? "Executor key active on your account. Revoking deletes it on chain and stops every schedule."
              : revoked
                ? "Key deleted on chain. Buys, sells and schedules are off; your funds stay in your account."
                : "Lets DCA, rebalances and baskets run from your NEAR account while you're offline."}
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
          <li>Everything else is rejected by your account contract. Funds stay in your account. Revoke any time.</li>
        </ul>
      </details>
      {delegated ? (
        <Button variant="ghost" className="w-full" pending={delegating} onClick={() => void revoke()}>
          Revoke
        </Button>
      ) : (
        !revoked && (
          <Button className="w-full" pending={delegating} disabled={!!address && keyStatus === "pending"} onClick={() => void delegate()}>
            Enable automation
          </Button>
        )
      )}
      {delegationError && <ErrorBox message={delegationError} />}
    </div>
  );
  return compact ? <div className="rounded-xl border border-border bg-secondary/30">{body}</div> : <Panel>{body}</Panel>;
}
