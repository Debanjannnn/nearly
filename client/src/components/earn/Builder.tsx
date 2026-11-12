"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { createStrategy, deployStrategy, errText, fmtUsd, runBacktest, toDef } from "@/lib/api";
import type { Backtest, Rules, Sleeve, SourceKind } from "@/lib/types";
import { useHoldings, useRates, useTickers } from "@/lib/hooks";
import { useSession } from "@/lib/session";
import { Button, Divider, ErrorBox, Label, Notice, Panel } from "@/components/ui";
import { TokenIcon } from "@/components/TokenIcon";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ProtocolIcon } from "@/components/ProtocolIcon";
import { Field, inputCls } from "@/components/automation/ScheduleForm";
import { evenSplit, setWeight, weightsError } from "./weights";
import { cn, decimalInput } from "@/lib/utils";
import { BacktestView, pct } from "./Backtest";
import { fmtAmt, StrategyFlow } from "./StrategyFlow";
import { RiskCard } from "./RiskCard";
import { toastError } from "@/components/TxToast";
import { DelegationCard } from "@/components/automation/Delegation";
import { AnyChainButton } from "@/components/deposit/AnyChainDeposit";
import { balanceOf } from "@/components/trade/MarketPanel";
import { ASSETS, blendedApy, defaultVenue, KIND_LABEL, kindsFor, lendingVenues, MAX_TOKENS, SOON, sourceApy, STAKING_EXIT, protocolOf, venueName } from "./sources";
import { toUnits, USDC } from "@/lib/tokens";

type Pick = { kind: SourceKind; venue: string; bps: number };

const MAX_SOURCES = 8; // planner: ≤8 legs total

const defaultPicks = (a: string): Pick[] => {
  const k = kindsFor(a).find((x) => x !== "hold") ?? "hold"; // default to a yield source; hold is opt-in
  return [{ kind: k, venue: defaultVenue(k), bps: 10000 }];
};
const toSources = (picks: Pick[]) => picks.map((p) => ({ kind: p.kind, venue: p.venue, weightBps: p.bps }));

const HINT: Record<SourceKind, string> = {
  hold: "Buy and keep the token in your account. No venue, no yield, instant exit.",
  lending: "Supply to Rhea Lending. Instant exit while the pool has liquidity.",
  staking: "Stake wNEAR for LiNEAR, the liquid staking token.",
  lp: "Rhea DEX liquidity pool. Coming soon.",
};

export function StrategyBuilder() {
  const router = useRouter();
  const { ready, address, delegated, ensureReady } = useSession();
  const holdings = useHoldings();
  const { rates } = useRates();
  const { tickers } = useTickers();
  const [name, setName] = useState("");
  // tokens[i] gets tokenBps[i] of the deposit; tab = token whose sources are being edited
  const [tokens, setTokens] = useState<string[]>(["USDC"]);
  const [tokenBps, setTokenBps] = useState<number[]>([10000]);
  const [picksBy, setPicksBy] = useState<Record<string, Pick[]>>({ USDC: [{ kind: "lending", venue: "rhea-lend", bps: 10000 }] });
  const [tab, setTab] = useState("USDC");
  const [amount, setAmount] = useState(""); // empty = publish only; a value also buys right after
  const [drift, setDrift] = useState("5");
  const [bt, setBt] = useState<{ data: Backtest | null; error: string | null; loading: boolean }>({ data: null, error: null, loading: false });
  const [busy, setBusy] = useState<"public" | "private" | null>(null);
  const [error, setError] = useState<string | null>(null);

  const asset = tab;
  const picks = picksBy[tab] ?? [];
  const setPicks = (next: Pick[]) => setPicksBy({ ...picksBy, [tab]: next });
  const multi = tokens.length > 1;
  const unit = multi ? "USDC" : tokens[0]; // preview amount: USDC deposit for multi, asset units for one token
  const sleeves: Sleeve[] = tokens.map((a, i) => ({ asset: a, weightBps: tokenBps[i], sources: toSources(picksBy[a] ?? []) }));
  const sources = sleeves[tokens.indexOf(tab)]?.sources ?? [];
  const prices: Record<string, number | null> = { ...Object.fromEntries(Object.entries(tickers).map(([k, t]) => [k, t.price])), USDC: 1 };

  const kinds = kindsFor(asset);
  const rules: Rules = { driftBps: Math.round(Number(drift) * 100) };
  const sleeveErr = (a: string) => {
    const err = weightsError((picksBy[a] ?? []).map((p) => p.bps));
    return err && multi ? `${a}: ${err}` : err;
  };
  const ruleErr = !(Number(drift) > 0 && Number(drift) <= 100) ? "Check the rules" : null;
  const splitErr = multi && weightsError(tokenBps);
  const invalid =
    (splitErr ? `Token split: ${splitErr}` : null) ??
    tokens.map(sleeveErr).find(Boolean) ??
    (sleeves.reduce((n, sl) => n + sl.sources.length, 0) > MAX_SOURCES ? `At most ${MAX_SOURCES} sources across tokens` : null) ??
    ruleErr ??
    (amount.trim() !== "" && !(Number(amount) > 0) ? "Invalid amount" : null);

  const resplit = (next: Omit<Pick, "bps">[]) => {
    const w = evenSplit(next.length);
    setPicks(next.map((p, i) => ({ ...p, bps: w[i] })));
    setBt({ data: null, error: null, loading: false });
  };
  const toggle = (kind: SourceKind) =>
    resplit(picks.some((p) => p.kind === kind) ? picks.filter((p) => p.kind !== kind) : [...picks, { kind, venue: defaultVenue(kind), bps: 0 }]);
  const update = (i: number, patch: Partial<Pick>) => setPicks(picks.map((p, j) => (j === i ? { ...p, ...patch } : p)));
  // chips multi-select; last token can't be removed. Adding/removing resplits token weights evenly.
  const toggleToken = (a: string) => {
    const on = tokens.includes(a);
    if (on ? tokens.length === 1 : tokens.length >= MAX_TOKENS) return;
    const next = on ? tokens.filter((t) => t !== a) : [...tokens, a];
    const nextUnit = next.length > 1 ? "USDC" : next[0];
    setTokens(next);
    setTokenBps(evenSplit(next.length));
    if (!on && !picksBy[a]) setPicksBy({ ...picksBy, [a]: defaultPicks(a) });
    setTab(on ? (tab === a ? next[0] : tab) : a);
    if (nextUnit !== unit) setAmount("");
    setBt({ data: null, error: null, loading: false });
  };

  async function backtest() {
    setBt({ data: null, error: null, loading: true });
    try {
      setBt({ data: await runBacktest(toDef(sleeves, rules), String(amtUsd)), error: null, loading: false });
    } catch (e) {
      setBt({ data: null, error: errText(e), loading: false });
    }
  }
  async function publish(pub: boolean) {
    if (name.trim() === "") return setError("Name your basket");
    if (!(await ensureReady())) return;
    setBusy(pub ? "public" : "private");
    setError(null);
    try {
      const s = await createStrategy({ name: name.trim(), sleeves, rules, public: pub });
      if (!(buyUsd > 0)) return router.push(`/baskets/${s.id}`);
      // basket saved; a failed buy lands on its page with the amount prefilled to retry
      try {
        const r = await deployStrategy(s.id, buyUsd.toFixed(2));
        router.push(`/baskets/${s.id}${r.execution_id ? `?exec=${r.execution_id}` : ""}`);
      } catch (e) {
        toastError(new Error(`Basket saved, buy failed: ${errText(e)}`));
        router.push(`/baskets/${s.id}?amount=${buyUsd.toFixed(2)}`);
      }
    } catch (e) {
      setError(errText(e));
      setBusy(null);
    }
  }

  const est = blendedApy(sleeves, rates);
  // empty amount previews (flow, backtest) at the placeholder
  const placeholder = unit === "USDC" ? "1000" : "1";
  const amt = Number(amount) > 0 ? Number(amount) : Number(placeholder);
  // history backtests in USD
  const amtUsd = unit === "USDC" ? amt : amt * (prices[unit] ?? 0) || 1000;
  // the buy, in USDC (single token: asset units at spot); 0 = publish only
  const buyUsd = Number(amount) > 0 ? (unit === "USDC" ? Number(amount) : (prices[unit] ?? 0) * Number(amount)) : 0;
  const usdc = balanceOf(holdings.data, "USDC");
  const have = usdc !== null ? toUnits(usdc, USDC.decimals) : null;
  const short = have !== null && buyUsd > have ? buyUsd - have : 0;
  const buyErr = !(Number(amount) > 0)
    ? null
    : !(buyUsd > 0)
      ? `No ${unit} price yet`
      : address && !delegated
        ? "Enable automation to buy"
        : short > 0
          ? "Insufficient USDC"
          : null;
  // flow shows the active token's part in its own units
  const px = tab === "USDC" ? 1 : prices[tab];
  const flowAmt = !multi ? amt : px ? (amt * tokenBps[tokens.indexOf(tab)]) / 10000 / px : 0;
  const flowLegs = sources.map((s) => ({ kind: s.kind, venue: s.venue, weightBps: s.weightBps, apy: sourceApy(s, asset, rates) }));

  return (
    <main className="w-full px-4 pb-24 pt-8 md:px-6">
      <div className="mx-auto w-full space-y-6">
        <div>
          <h1 className="text-xl font-medium sm:text-2xl">Build a basket</h1>
          <p className="mt-1 text-sm text-muted-foreground">Up to {MAX_TOKENS} tokens, each with its own mix of yield sources, plus shared rules. Backtest on NEAR mainnet history, then publish for others to buy.</p>
        </div>

        <div className="flex items-stretch gap-6 px-1">
          <div className="flex-1">
            <div className="text-sm text-muted-foreground">Blended APY</div>
            <div className="tnum mt-1 text-3xl font-medium tracking-tight text-positive">{est === null ? <span className="text-muted-foreground">-</span> : pct(est)}</div>
          </div>
          <div className="w-px self-stretch bg-border" />
          <div className="flex-1">
            <div className="text-sm text-muted-foreground">Rewards / year</div>
            <div className="tnum mt-1 text-3xl font-medium tracking-tight">{est !== null && amt > 0 ? `${fmtAmt(amt * est)} ${unit}` : "-"}</div>
          </div>
        </div>

        <div className="grid gap-6 lg:grid-cols-2 lg:items-start">
          <Panel>
            <div className="space-y-4 p-5 sm:p-6">
              <Step n={1}>Name &amp; tokens</Step>
              <input
                value={name}
                onChange={(e) => setName(e.target.value.slice(0, 60))}
                placeholder="NEAR yield basket"
                aria-label="Basket name"
                className="w-full bg-transparent text-2xl font-medium tracking-tight outline-none placeholder:text-muted-foreground/40"
              />
              <div className="flex flex-wrap gap-2">
                {ASSETS.map((a) => (
                  <button
                    key={a}
                    onClick={() => toggleToken(a)}
                    aria-pressed={tokens.includes(a)}
                    disabled={!tokens.includes(a) && tokens.length >= MAX_TOKENS}
                    className={cn(
                      "inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm transition-colors disabled:opacity-50",
                      tokens.includes(a) ? "border-primary/60 bg-primary/10" : "border-border bg-secondary/30 hover:bg-secondary/60",
                    )}
                  >
                    <TokenIcon symbol={a} size={18} />
                    {a}
                  </button>
                ))}
              </div>
              {multi && (
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">Token split (applied at deposit)</span>
                    <button
                      onClick={() => setTokenBps(evenSplit(tokens.length))}
                      className="rounded-lg px-2.5 py-1 text-xs text-muted-foreground hover:bg-secondary hover:text-foreground"
                    >
                      Split evenly
                    </button>
                  </div>
                  {tokens.map((a, i) => (
                    <div key={a} className="flex items-center gap-3">
                      <button onClick={() => setTab(a)} className="inline-flex w-20 shrink-0 items-center gap-1.5 text-sm hover:text-primary">
                        <TokenIcon symbol={a} size={16} />
                        {a}
                      </button>
                      <input
                        type="range"
                        min={0}
                        max={10000}
                        step={50}
                        value={tokenBps[i]}
                        onChange={(e) => setTokenBps(setWeight(tokenBps, i, Number(e.target.value)))}
                        aria-label={`${a} weight`}
                        className="range w-full"
                        style={{ "--pct": `${tokenBps[i] / 100}%` } as React.CSSProperties}
                      />
                      <span className="tnum w-14 text-right text-sm">{tokenBps[i] / 100}%</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
            <Divider />

            <div className="space-y-4 p-5 sm:p-6">
              <div className="flex items-center justify-between">
                <Step n={2}>Sources &amp; weights</Step>
                <button onClick={() => resplit(picks)} className="rounded-lg px-2.5 py-1 text-xs text-muted-foreground hover:bg-secondary hover:text-foreground">
                  Split evenly
                </button>
              </div>
              {multi && (
                <div role="tablist" className="flex flex-wrap gap-1.5">
                  {tokens.map((a) => (
                    <button
                      key={a}
                      role="tab"
                      aria-selected={tab === a}
                      onClick={() => setTab(a)}
                      className={cn(
                        "inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-sm transition-colors",
                        tab === a ? "bg-secondary text-foreground" : "text-muted-foreground hover:bg-secondary/60",
                        sleeveErr(a) && "text-warning",
                      )}
                    >
                      <TokenIcon symbol={a} size={16} />
                      {a}
                    </button>
                  ))}
                </div>
              )}
              {(["hold", "lending", "staking", "lp"] as SourceKind[]).map((kind) => {
                const i = picks.findIndex((p) => p.kind === kind);
                const p = picks[i];
                const soon = SOON.includes(kind);
                const ok = kinds.includes(kind);
                const apy = p ? sourceApy({ kind }, asset, rates) : null;
                return (
                  <div key={kind} className={cn("rounded-xl border p-4 transition-colors", p ? "border-primary/40 bg-primary/5" : "border-border bg-secondary/30", !ok && "opacity-50")}>
                    <label className="flex items-center gap-3">
                      <input type="checkbox" checked={!!p} disabled={!ok} onChange={() => toggle(kind)} className="accent-[var(--primary)]" />
                      <span className="text-sm font-medium">{KIND_LABEL[kind]}</span>
                      <span className="truncate text-xs text-muted-foreground">{ok || soon ? HINT[kind] : `not available for ${asset}`}</span>
                      {soon && <span className="ml-auto rounded-md bg-secondary px-1.5 py-0.5 text-[11px] text-muted-foreground">soon</span>}
                      {p && <span className="tnum ml-auto text-xs text-positive">{apy === null ? "-" : pct(apy)}</span>}
                    </label>
                    {p && (
                      <div className="mt-4 space-y-4 pl-7">
                        {kind === "lending" && (
                          <Select
                            items={lendingVenues().map((v) => ({ value: v.venue, label: v.name }))}
                            value={p.venue}
                            onValueChange={(v) => v && update(i, { venue: v })}
                          >
                            <SelectTrigger aria-label="Lending venue" className="h-10! w-full rounded-xl border-border bg-card px-3 dark:bg-card">
                              <SelectValue>
                                {(v: string) => (
                                  <span className="flex items-center gap-2">
                                    <ProtocolIcon protocol={protocolOf(v)} size={18} />
                                    {venueName(v)}
                                  </span>
                                )}
                              </SelectValue>
                            </SelectTrigger>
                            <SelectContent alignItemWithTrigger={false} sideOffset={6} className="rounded-xl p-1">
                              {lendingVenues().map((v) => (
                                <SelectItem key={v.venue} value={v.venue} className="rounded-lg px-2.5 py-2">
                                  <ProtocolIcon protocol={protocolOf(v.venue)} size={18} />
                                  {v.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        )}
                        {kind === "staking" && <Notice tone="muted">{STAKING_EXIT}</Notice>}
                        <div className="flex items-center gap-3">
                          <input
                            type="range"
                            min={0}
                            max={10000}
                            step={50}
                            value={p.bps}
                            onChange={(e) => {
                              const w = setWeight(picks.map((x) => x.bps), i, Number(e.target.value));
                              setPicks(picks.map((x, j) => ({ ...x, bps: w[j] })));
                            }}
                            aria-label={`${KIND_LABEL[kind]} weight`}
                            className="range w-full"
                            style={{ "--pct": `${p.bps / 100}%` } as React.CSSProperties}
                          />
                          <span className="tnum w-14 text-right text-sm">{p.bps / 100}%</span>
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
            <Divider />

            <div className="grid gap-3 p-5 sm:grid-cols-2 sm:p-6">
              <div className="sm:col-span-2">
                <Step n={3}>Rules</Step>
              </div>
              <Field label="Drift band (%)" hint="rebalance past this">
                <input className={inputCls} inputMode="decimal" value={drift} onChange={(e) => { const v = decimalInput(e.target.value); if (v !== null) setDrift(v); }} />
              </Field>
            </div>
          </Panel>

          <div className="space-y-4 lg:sticky lg:top-24">
            <Panel>
              <div className="flex items-center justify-between gap-3 px-5 pt-5">
                <Label>Amount</Label>
              </div>
              <div className="flex items-center gap-3 px-5 pt-3 pb-5">
                <input
                  value={amount}
                  onChange={(e) => { const v = decimalInput(e.target.value); if (v !== null) setAmount(v); }}
                  inputMode="decimal"
                  placeholder={placeholder}
                  aria-label={`Amount in ${unit}`}
                  className="tnum w-full min-w-0 bg-transparent text-4xl font-medium tracking-tight outline-none placeholder:text-muted-foreground/40"
                />
                <span className="inline-flex shrink-0 items-center gap-2 rounded-full bg-secondary/70 py-2 pl-3 pr-4 text-sm font-medium">
                  <TokenIcon symbol={unit} size={20} />
                  {unit}
                </span>
              </div>
              <Divider />
              <div className="px-5 pt-5">
                <Label>Routing</Label>
              </div>
              <StrategyFlow asset={asset} amount={flowAmt} legs={flowLegs} />
            </Panel>

            {tokens.some((t) => t !== "USDC") && <RiskCard def={toDef(sleeves, rules)} amount={buyUsd > 0 ? buyUsd : amtUsd} />}

            <Panel className="space-y-4 p-5 sm:p-6">
              {(invalid ?? buyErr) && <p className="text-xs text-warning">{invalid ?? buyErr}</p>}
              {error && <ErrorBox message={error} />}
              {buyUsd > 0 && address && !delegated && <DelegationCard compact />}
              {short > 0 && <AnyChainButton need={short} onArrived={holdings.reload} className="mx-auto flex w-fit" />}
              <Button className="w-full py-3" pending={busy === "public"} disabled={!ready || !!invalid || !!buyErr || busy !== null} onClick={() => void publish(true)}>
                {buyUsd > 0 ? `Publish & buy ${fmtUsd(buyUsd)}` : "Publish basket"}
              </Button>
              <Button variant="ghost" className="w-full" pending={busy === "private"} disabled={!!invalid || !!buyErr || busy !== null} onClick={() => void publish(false)}>
                {buyUsd > 0 ? "Save private & buy" : "Save private"}
              </Button>
              <p className="text-xs leading-relaxed text-muted-foreground">
                Buyers run the same allocation in their own wallets. Open fee 0.4%, you earn 0.2% on every buy.
              </p>
            </Panel>
          </div>
        </div>

        <Panel className="space-y-4 p-5 sm:p-6">
          <div className="flex items-center justify-between gap-3">
            <Step n={4}>Backtest</Step>
            <Button variant="ghost" pending={bt.loading} disabled={!!invalid} onClick={() => void backtest()}>
              Run backtest
            </Button>
          </div>
          {(bt.data || bt.loading || bt.error) && <BacktestView bt={bt.data} loading={bt.loading} error={bt.error} />}
        </Panel>
      </div>
    </main>
  );
}

function Step({ n, children }: { n: number; children: React.ReactNode }) {
  return (
    <h2 className="flex items-center gap-2 text-base font-medium">
      <span className="grid size-6 place-items-center rounded-full bg-primary/15 text-xs text-primary">{n}</span>
      {children}
    </h2>
  );
}
