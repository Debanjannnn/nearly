"use client";

import { useState } from "react";
import { deployBasket, errText, fmtUsd } from "@/lib/api";
import { FEE_BPS, TOTAL_BPS } from "@/lib/config";
import { useHoldings } from "@/lib/hooks";
import { useSession } from "@/lib/session";
import { decimalsOf, fmtUnits, parseAmount, toUnits } from "@/lib/tokens";
import type { Basket } from "@/lib/types";
import { Button, Notice, Row } from "@/components/ui";
import { TokenIcon } from "@/components/TokenIcon";
import { ExecutionProgress } from "@/components/Execution";
import { ExecutorCard } from "@/components/automation/ExecutorCard";

const DEC = decimalsOf("usdc");

// usdc balance of the sub-account, null while unknown
export const usdcOf = (h: ReturnType<typeof useHoldings>["data"]) => (h ? BigInt(h.tokens.usdc?.balance ?? "0") : null);

// Buy: USDC from your account, 0.4% open fee to treasury, executor routes the rest into every leg.
export function BuyPanel({ basket, initialExec = null, onSettled }: { basket: Basket; initialExec?: string | null; onSettled?: () => void }) {
  const { ready, authenticated, account, login, keyStatus, accountId } = useSession();
  const holdings = useHoldings();
  const [amount, setAmount] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [execId, setExecId] = useState<string | null>(initialExec);

  const raw = parseAmount(amount, DEC);
  const usdc = usdcOf(holdings.data);
  const n = raw !== null ? toUnits(raw, DEC) : 0;
  const fee = (n * FEE_BPS) / TOTAL_BPS;
  const own = accountId === basket.creator;

  const reason = !ready
    ? "Loading…"
    : keyStatus !== "active"
      ? "Automation key not active"
      : amount.trim() === ""
        ? "Enter an amount"
        : raw === null || raw === 0n
          ? "Invalid amount"
          : usdc !== null && raw > usdc
            ? "Insufficient USDC"
            : null;

  async function submit() {
    if (raw === null) return;
    setBusy(true);
    setErr(null);
    try {
      const r = await deployBasket(basket.id, raw.toString());
      setExecId(r.execution_id ?? null);
      setAmount("");
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-4 p-5">
      <span className="text-sm font-medium">Buy with USDC</span>
      <div className="rounded-2xl bg-card p-4">
        <div className="flex items-center justify-between text-sm text-muted-foreground">
          Amount
          {usdc !== null && (
            <button type="button" onClick={() => setAmount(fmtUnits(usdc, DEC, DEC).replace(/,/g, ""))} className="tnum text-xs hover:text-foreground">
              Max {fmtUnits(usdc, DEC, 2)}
            </button>
          )}
        </div>
        <div className="mt-2 flex items-center gap-3">
          <input
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            inputMode="decimal"
            aria-label="Amount in USDC"
            placeholder="0"
            className="tnum w-full min-w-0 bg-transparent text-3xl font-medium tracking-tight outline-none placeholder:text-muted-foreground/50"
          />
          <span className="inline-flex shrink-0 items-center gap-2 rounded-full bg-secondary/70 py-1.5 pr-3 pl-2 text-sm font-medium">
            <TokenIcon token="usdc" size={20} />
            USDC
          </span>
        </div>
      </div>
      <dl className="space-y-1.5 px-1 text-xs">
        <Row label={`Open fee (${FEE_BPS / 100}%)`}>{fmtUsd(fee)}</Row>
        <Row label="Deployed">{fmtUsd(Math.max(0, n - fee))}</Row>
        <Row label="Sell fee">none</Row>
      </dl>
      {usdc !== null && raw !== null && raw > usdc && account && (
        <Notice>
          Send USDC to your account <span className="tnum break-all text-foreground">{account}</span> first.
        </Notice>
      )}
      {authenticated && account && keyStatus !== "active" && <ExecutorCard compact />}
      {err && <Notice tone="bad">{err}</Notice>}
      {ready && !(authenticated && account) ? (
        <Button className="w-full py-3" onClick={() => void login()}>
          Sign in to buy
        </Button>
      ) : (
        <Button className="w-full py-3" pending={busy} disabled={reason !== null} onClick={() => void submit()}>
          {busy ? "Submitting…" : (reason ?? "Buy basket")}
        </Button>
      )}
      {execId && (
        <ExecutionProgress
          id={execId}
          onSettled={() => {
            holdings.reload();
            onSettled?.();
          }}
        />
      )}
      <p className="text-[11px] leading-4 text-muted-foreground">
        Funds stay in your own NEAR account contract; the executor key runs the steps (fee → swap → supply / stake). {own ? "" : "The creator earns half the open fee."}
      </p>
    </div>
  );
}
