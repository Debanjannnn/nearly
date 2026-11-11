"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { createBasket, deployBasket, errText, fmtUsd, pct, runBacktest } from "@/lib/api";
import { useHoldings, useRates } from "@/lib/hooks";
import { useSession } from "@/lib/session";
import { decimalsOf, KEYS, parseAmount, symbolOf, toUnits } from "@/lib/tokens";
import type { Backtest, BasketDef, Leg, Source } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Button, Divider, ErrorBox, Label, Notice, Panel } from "@/components/ui";
import { TokenIcon } from "@/components/TokenIcon";
import { ExecutorCard } from "@/components/automation/ExecutorCard";
import { Field, inputCls } from "@/components/automation/ScheduleForm";
import { BacktestView } from "./Backtest";
import { usdcOf } from "./BuyPanel";
import { RiskCard } from "./RiskCard";
import { RoutingFlow } from "./RoutingFlow";
import { blendedApy, evenSplit, legApy, setWeight, SOURCE_LABEL, sourcesFor, VENUE_NAME, weightsError, withPool } from "./legs";

const HINT: Record<Source, string> = {
  lending: "Supply to Rhea lending. Instant exit while the pool has liquidity.",
  staking: "Stake for LiNEAR. Unstake waits ~4 epochs.",
  hold: "Keep the token in your account. No venue, no yield.",
};

// every token × source the planner accepts
const OPTIONS = KEYS.flatMap((token) => sourcesFor(token).map((source) => ({ token, source })));
const same = (a: Pick<Leg, "token" | "source">, b: Pick<Leg, "token" | "source">) => a.token === b.token && a.source === b.source;

export function BasketBuilder() {
  const router = useRouter();
  const { ready, authenticated, account, login, keyStatus } = useSession();
  const holdings = useHoldings();
  const { rates } = useRates();
  const [name, setName] = useState("");
  const [legs, setLegs] = useState<Leg[]>([
    { token: "usdc", source: "lending", weight_bps: 5000 },
    { token: "wnear", source: "staking", weight_bps: 5000 },
  ]);
  const [drift, setDrift] = useState("5");
  const [amount, setAmount] = useState(""); // empty = publish only; a value also buys right after
  const [bt, setBt] = useState<{ data: Backtest | null; error: string | null; loading: boolean }>({ data: null, error: null, loading: false });
  const [busy, setBusy] = useState<"public" | "private" | null>(null);
  const [error, setError] = useState<string | null>(null);

  const def: BasketDef = { legs: legs.map(withPool), drift_bps: Math.round(Number(drift) * 100) };
  const raw = amount.trim() === "" ? 0n : parseAmount(amount, decimalsOf("usdc"));
  const amt = raw ? toUnits(raw, decimalsOf("usdc")) : 1000; // preview size
  const usdc = usdcOf(holdings.data);
  const est = blendedApy(def, rates);

  const invalid =
    weightsError(legs.map((l) => l.weight_bps)) ??
    (!(Number(drift) >= 0 && Number(drift) <= 100) ? "Drift band must be 0–100%" : null) ??
    (raw === null ? "Invalid amount" : null);
  const buyErr = !raw
    ? null
    : !(authenticated && account)
      ? null
      : keyStatus !== "active"
        ? "Automation key not active"
        : usdc !== null && raw > usdc
          ? "Insufficient USDC in your account"
          : null;

  const resplit = (next: Leg[]) => {
    const w = evenSplit(next.length);
    setLegs(next.map((l, i) => ({ ...l, weight_bps: w[i] })));
    setBt({ data: null, error: null, loading: false });
  };
  const toggle = (o: Pick<Leg, "token" | "source">) =>
    resplit(legs.some((l) => same(l, o)) ? legs.filter((l) => !same(l, o)) : [...legs, { ...o, weight_bps: 0 }]);

  async function backtest() {
    setBt({ data: null, error: null, loading: true });
    try {
      setBt({ data: await runBacktest(def, String(amt)), error: null, loading: false });
    } catch (e) {
      setBt({ data: null, error: errText(e), loading: false });
    }
  }

  async function publish(pub: boolean) {
    if (!(authenticated && account)) return login();
    if (name.trim() === "") return setError("Name your basket");
    setBusy(pub ? "public" : "private");
    setError(null);
    try {
      const b = await createBasket(name.trim(), def, pub);
      if (!raw) return router.push(`/baskets/${b.id}`);
      // basket saved; a failed buy still lands on its page
      try {
        const r = await deployBasket(b.id, raw.toString());
        router.push(`/baskets/${b.id}${r.execution_id ? `?exec=${r.execution_id}` : ""}`);
      } catch (e) {
        setError(`Basket saved, buy failed: ${errText(e)}`);
        router.push(`/baskets/${b.id}`);
      }
    } catch (e) {
      setError(errText(e));
      setBusy(null);
    }
  }

  return (
    <main className="w-full px-4 pt-8 pb-24 md:px-6">
      <div className="mx-auto w-full space-y-6">
        <div>
          <h1 className="text-xl font-medium sm:text-2xl">Build a basket</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Split USDC across Rhea lending, LiNEAR staking and plain holds. Backtest on history, then publish for others to buy.
          </p>
        </div>

        <div className="flex items-stretch gap-6 px-1">
          <div className="flex-1">
            <div className="text-sm text-muted-foreground">Blended APY</div>
            <div className="tnum mt-1 text-3xl font-medium tracking-tight text-positive">{est === null ? <span className="text-muted-foreground">-</span> : pct(est)}</div>
          </div>
          <div className="w-px self-stretch bg-border" />
          <div className="flex-1">
            <div className="text-sm text-muted-foreground">Yield / year</div>
            <div className="tnum mt-1 text-3xl font-medium tracking-tight">{est !== null ? fmtUsd(amt * est) : "-"}</div>
          </div>
        </div>

        <div className="grid gap-6 lg:grid-cols-2 lg:items-start">
          <Panel>
            <div className="space-y-4 p-5 sm:p-6">
              <StepTitle n={1}>Name</StepTitle>
              <input
                value={name}
                onChange={(e) => setName(e.target.value.slice(0, 80))}
                placeholder="Half stable, half staked NEAR"
                aria-label="Basket name"
                className="w-full bg-transparent text-2xl font-medium tracking-tight outline-none placeholder:text-muted-foreground/40"
              />
            </div>
            <Divider />

            <div className="space-y-4 p-5 sm:p-6">
              <div className="flex items-center justify-between">
                <StepTitle n={2}>Legs &amp; weights</StepTitle>
                <button onClick={() => resplit(legs)} className="rounded-lg px-2.5 py-1 text-xs text-muted-foreground hover:bg-secondary hover:text-foreground">
                  Split evenly
                </button>
              </div>
              {OPTIONS.map((o) => {
                const i = legs.findIndex((l) => same(l, o));
                const l = legs[i];
                const apy = legApy(o, rates);
                return (
                  <div key={`${o.token}:${o.source}`} className={cn("rounded-xl border p-4 transition-colors", l ? "border-primary/40 bg-primary/5" : "border-border bg-secondary/30")}>
                    <label className="flex items-center gap-3">
                      <input type="checkbox" checked={!!l} onChange={() => toggle(o)} className="accent-[var(--primary)]" />
                      <TokenIcon token={o.token} size={18} />
                      <span className="text-sm font-medium">
                        {symbolOf(o.token)} · {SOURCE_LABEL[o.source]}
                      </span>
                      <span className="hidden truncate text-xs text-muted-foreground sm:inline">{o.source === "hold" ? HINT.hold : `${VENUE_NAME[o.source]}. ${HINT[o.source]}`}</span>
                      <span className="tnum ml-auto text-xs text-positive">{apy === null ? "-" : pct(apy)}</span>
                    </label>
                    {l && (
                      <div className="mt-4 flex items-center gap-3 pl-7">
                        <input
                          type="range"
                          min={0}
                          max={10000}
                          step={50}
                          value={l.weight_bps}
                          onChange={(e) => {
                            const w = setWeight(legs.map((x) => x.weight_bps), i, Number(e.target.value));
                            setLegs(legs.map((x, j) => ({ ...x, weight_bps: w[j] })));
                          }}
                          aria-label={`${symbolOf(o.token)} ${SOURCE_LABEL[o.source]} weight`}
                          className="range w-full"
                          style={{ "--pct": `${l.weight_bps / 100}%` } as React.CSSProperties}
                        />
                        <span className="tnum w-14 text-right text-sm">{l.weight_bps / 100}%</span>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
            <Divider />

            <div className="grid gap-3 p-5 sm:grid-cols-2 sm:p-6">
              <div className="sm:col-span-2">
                <StepTitle n={3}>Rules</StepTitle>
              </div>
              <Field label="Drift band (%)" hint="0 = never rebalance">
                <input className={inputCls} inputMode="decimal" value={drift} onChange={(e) => setDrift(e.target.value)} />
              </Field>
            </div>
          </Panel>

          <div className="space-y-4 lg:sticky lg:top-24">
            <Panel>
              <div className="px-5 pt-5">
                <Label>Amount</Label>
              </div>
              <div className="flex items-center gap-3 px-5 pt-3 pb-5">
                <input
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  inputMode="decimal"
                  placeholder="1000"
                  aria-label="Amount in USDC"
                  className="tnum w-full min-w-0 bg-transparent text-4xl font-medium tracking-tight outline-none placeholder:text-muted-foreground/40"
                />
                <span className="inline-flex shrink-0 items-center gap-2 rounded-full bg-secondary/70 py-2 pr-4 pl-3 text-sm font-medium">
                  <TokenIcon token="usdc" size={20} />
                  USDC
                </span>
              </div>
              <Divider />
              <div className="px-5 pt-5">
                <Label>Routing</Label>
              </div>
              <RoutingFlow amount={amt} legs={def.legs} rates={rates} />
            </Panel>

            <RiskCard def={def} amount={amt} />

            <Panel className="space-y-4 p-5 sm:p-6">
              {(invalid ?? buyErr) && <p className="text-xs text-warning">{invalid ?? buyErr}</p>}
              {error && <ErrorBox message={error} />}
              {raw && authenticated && account && keyStatus !== "active" ? <ExecutorCard compact /> : null}
              {raw && usdc !== null && raw > usdc && account ? (
                <Notice>
                  Send USDC to <span className="tnum break-all text-foreground">{account}</span> to buy.
                </Notice>
              ) : null}
              <Button className="w-full py-3" pending={busy === "public"} disabled={!ready || !!invalid || !!buyErr || busy !== null} onClick={() => void publish(true)}>
                {!(authenticated && account) ? "Sign in to publish" : raw ? `Publish & buy ${fmtUsd(amt)}` : "Publish basket"}
              </Button>
              {authenticated && account && (
                <Button variant="ghost" className="w-full" pending={busy === "private"} disabled={!!invalid || !!buyErr || busy !== null} onClick={() => void publish(false)}>
                  {raw ? "Save private & buy" : "Save private"}
                </Button>
              )}
              <p className="text-xs leading-relaxed text-muted-foreground">
                Buyers run the same allocation in their own accounts. Open fee 0.4%, you earn 0.2% on every buy.
              </p>
            </Panel>
          </div>
        </div>

        <Panel className="space-y-4 p-5 sm:p-6">
          <div className="flex items-center justify-between gap-3">
            <StepTitle n={4}>Backtest</StepTitle>
            <Button variant="ghost" pending={bt.loading} disabled={!!weightsError(legs.map((l) => l.weight_bps))} onClick={() => void backtest()}>
              Run backtest
            </Button>
          </div>
          {(bt.data || bt.loading || bt.error) && <BacktestView bt={bt.data} loading={bt.loading} error={bt.error} />}
        </Panel>
      </div>
    </main>
  );
}

function StepTitle({ n, children }: { n: number; children: React.ReactNode }) {
  return (
    <h2 className="flex items-center gap-2 text-base font-medium">
      <span className="grid size-6 place-items-center rounded-full bg-primary/15 text-xs text-primary">{n}</span>
      {children}
    </h2>
  );
}
