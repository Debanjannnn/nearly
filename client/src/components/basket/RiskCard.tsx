"use client";

import { ShieldAlert } from "lucide-react";
import { getQuote } from "@/lib/api";
import { FEE_BPS, TOTAL_BPS, WNEAR_POOL } from "@/lib/config";
import { useAsync, useDebounced } from "@/lib/hooks";
import { decimalsOf, fmtUnits, symbolOf, tokenIdOf } from "@/lib/tokens";
import type { BasketDef } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Panel, Row } from "@/components/ui";

const NOTES = {
  lending: "Rhea lending: smart-contract risk; a fully borrowed pool can delay withdraws.",
  staking: "LiNEAR: unstake waits ~4 epochs (about 2-3 days) and can trade off peg.",
  hold: "Held in your account: no venue risk, no yield.",
} as const;

// price exposure, venue notes and the live swap impact of the buy (market-data quote on the Rhea pool)
export function RiskCard({ def, amount }: { def: BasketDef; amount: number }) {
  const net = amount * (1 - FEE_BPS / TOTAL_BPS);
  const swapped = def.legs.filter((l) => l.token !== "usdc");
  const share = swapped.reduce((a, l) => a + l.weight_bps, 0) / TOTAL_BPS;
  const swapIn = BigInt(Math.floor(useDebounced(net * share) * 1e6)); // usdc atoms
  const q = useAsync(swapIn > 0n ? `quote:${swapIn}` : null, () => getQuote(tokenIdOf("usdc"), tokenIdOf("wnear"), swapIn.toString(), WNEAR_POOL));
  const tooBig = q.data ? swapIn > BigInt(q.data.max_safe_in) : false;
  const sources = [...new Set(def.legs.map((l) => l.source))];

  return (
    <Panel className="space-y-4 p-5 sm:p-6">
      <div className="flex items-center gap-2 text-sm font-medium">
        <ShieldAlert className="size-4 text-warning" /> Risk
      </div>
      <dl className="space-y-1.5 text-xs">
        <Row label="Price exposure">{share > 0 ? `${+(share * 100).toFixed(2)}% in ${symbolOf("wnear")}` : "none (all USDC)"}</Row>
        {swapIn > 0n && (
          <>
            <Row label="Swap impact">
              {q.data ? <span className={cn(tooBig && "text-warning")}>{(q.data.impact_bps / 100).toFixed(2)}%</span> : q.error ? "quote unavailable" : "…"}
            </Row>
            {q.data && (
              <Row label="Max safe swap">
                {fmtUnits(q.data.max_safe_in, decimalsOf("usdc"), 2)} {symbolOf("usdc")}
              </Row>
            )}
          </>
        )}
        <Row label="Custody">your own account contract</Row>
      </dl>
      {tooBig && <p className="text-xs text-warning">This buy is above the pool&apos;s safe size; expect a worse price or a revert.</p>}
      <ul className="space-y-1.5 pl-4 text-xs leading-5 text-muted-foreground [&>li]:list-disc">
        {sources.map((s) => (
          <li key={s}>{NOTES[s]}</li>
        ))}
        <li>The executor key can only call this account&apos;s allowlisted venues. Revoke it any time.</li>
      </ul>
    </Panel>
  );
}
