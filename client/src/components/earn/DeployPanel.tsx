"use client";

import { useState } from "react";
import { deployStrategy, errText, fmtUsd } from "@/lib/api";
import { FEE_BPS, NETWORK } from "@/lib/config";
import { useHoldings } from "@/lib/hooks";
import { useSession } from "@/lib/session";
import { fmtUnits, formatUnits, parseAmount, toUnits, USDC } from "@/lib/tokens";
import type { Strategy } from "@/lib/types";
import { Button, Notice, Row } from "@/components/ui";
import { ExecutionProgress } from "@/components/Execution";
import { DelegationCard } from "@/components/automation/Delegation";
import { actionBtn, balanceOf, pill } from "@/components/trade/MarketPanel";
import { AnyChainButton } from "@/components/deposit/AnyChainDeposit";

// Buy: USDC in, 0.4% open fee to treasury, the executor key routes the rest into every source from your account.
export function DeployPanel({
  strategy,
  initialAmount = "",
  initialExec = null,
  onSettled,
}: {
  strategy: Strategy;
  initialAmount?: string;
  initialExec?: string | null;
  onSettled?: () => void;
}) {
  const { ready, address, delegated, keyStatus, mainAddress, ensureReady } = useSession();
  const holdings = useHoldings();
  const [amount, setAmount] = useState(initialAmount);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [execId, setExecId] = useState<string | null>(initialExec); // set when the builder bought on publish

  const raw = parseAmount(amount, USDC.decimals);
  const usdc = balanceOf(holdings.data, "USDC");
  const n = raw !== null ? toUnits(raw, USDC.decimals) : 0;
  const fee = (n * FEE_BPS) / 10000;
  const own = mainAddress === strategy.creator;

  const reason = !ready
    ? "Loading…"
    : address && !delegated
      ? "Automation key not active"
      : amount.trim() === ""
        ? "Enter an amount"
        : raw === null || raw === BigInt(0)
          ? "Invalid amount"
          : usdc !== null && raw > usdc
            ? "Insufficient USDC"
            : null;

  async function submit() {
    if (!(await ensureReady())) return;
    setBusy(true);
    setErr(null);
    try {
      const r = await deployStrategy(strategy.id, amount.trim());
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
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-medium">Buy with USDC</span>
      </div>
      <div className="rounded-2xl bg-card p-4">
        <div className="flex items-center justify-between text-sm text-muted-foreground">
          Amount
          {usdc !== null && (
            <button type="button" onClick={() => setAmount(formatUnits(usdc, USDC.decimals))} className="tnum text-xs hover:text-foreground">
              Max {fmtUnits(usdc, USDC.decimals, 2)}
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
          {pill("USDC")}
        </div>
      </div>
      <dl className="space-y-1.5 px-1 text-xs">
        <Row label={`Open fee (${FEE_BPS / 100}%)`}>{fmtUsd(fee)}</Row>
        <Row label="Deployed">{fmtUsd(Math.max(0, n - fee))}</Row>
        <Row label="Withdraw fee">none</Row>
      </dl>
      {address && keyStatus !== "active" && <DelegationCard compact />}
      {err && <Notice tone="bad">{err}</Notice>}
      <Button className={actionBtn} pending={busy} disabled={reason !== null} onClick={() => void submit()}>
        {busy ? "Submitting…" : (reason ?? "Buy basket")}
      </Button>
      {raw !== null && usdc !== null && raw > usdc && <AnyChainButton need={toUnits(raw - usdc, USDC.decimals)} onArrived={holdings.reload} className="mx-auto flex w-fit" />}
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
        NEAR {NETWORK}{NETWORK === "mainnet" ? ", real funds" : ""}. Everything stays in your own NEAR account contract&apos;s venue positions; nearly&apos;s executor key runs the steps (fee → swap → supply / stake). {own ? "" : "The creator earns half the open fee."}
      </p>
    </div>
  );
}
