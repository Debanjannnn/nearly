"use client";

import Link from "next/link";
import { fmtBps, getQuote, getRateSeries } from "@/lib/api";
import { WNEAR_POOL } from "@/lib/config";
import { dayOf, usePrices, useRates } from "@/lib/hooks";
import { useAsync } from "@/lib/session";
import { decimalsOf, fmtUnits, symbolOf, tokenBySymbol, toUnits, USDC } from "@/lib/tokens";
import type { Position, Strategy } from "@/lib/types";
import { cn } from "@/lib/utils";
import { ErrorBox, Panel, Skeleton } from "@/components/ui";
import { ProtocolIcon } from "@/components/ProtocolIcon";
import { Sparkline } from "@/components/Sparkline";
import { TokenIcon } from "@/components/TokenIcon";
import { pct } from "@/components/earn/Backtest";
import { defaultVenue, KIND_LABEL, STAKING_EXIT, venueName, protocolOf } from "@/components/earn/sources";

const D = 864e5;
const WNEAR = tokenBySymbol("wNEAR");
const LINEAR = tokenBySymbol("LiNEAR");
const PROBE = BigInt(1000) * BigInt(10) ** BigInt(USDC.decimals); // $1,000 USDC → wNEAR

const useSpark = (source: string, token: string) => useAsync(`rate30:${source}:${token}`, () => getRateSeries(source, token, dayOf(Date.now() - 30 * D)));

// "market-data + history" source line
function Source({ className }: { className?: string }) {
  return <p className={cn("text-[11px] text-muted-foreground", className)}>Live from nearly market-data (Rhea + LiNEAR contracts) · 30d from history</p>;
}

function Head({ protocol, title, children }: { protocol: string; title: React.ReactNode; children?: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="flex min-w-0 items-center gap-2 font-medium">
        <ProtocolIcon protocol={protocol} size={20} />
        <span className="truncate">{title}</span>
      </span>
      {children}
    </div>
  );
}

function Live({ stale }: { stale?: boolean }) {
  return stale ? (
    <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-warning/10 px-2 py-0.5 text-[11px] text-warning">Stale</span>
  ) : (
    <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-positive/10 px-2 py-0.5 text-[11px] text-positive">
      <span className="size-1.5 animate-pulse rounded-full bg-current" /> Live
    </span>
  );
}

function Kv({ k, v, note, tone }: { k: string; v: React.ReactNode; note?: React.ReactNode; tone?: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{k}</dt>
      <dd className={cn("tnum mt-0.5 truncate text-base font-medium", tone)}>{v}</dd>
      {note && <dd className="tnum mt-0.5 truncate text-[11px] text-muted-foreground">{note}</dd>}
    </div>
  );
}

function Spark({ source, token }: { source: string; token: string }) {
  const s = useSpark(source, token);
  if (s.error) return <p className="h-10 text-[11px] text-muted-foreground">No 30d history</p>;
  return s.data ? <Sparkline data={s.data.map((d) => Number(d.apy))} area className="h-10 w-full" /> : <Skeleton className="h-10 w-full" />;
}

function Tile({ protocol, title, children }: { protocol: string; title: string; children: React.ReactNode }) {
  return (
    <div className="space-y-3 rounded-2xl border border-border bg-secondary/30 p-4">
      <div className="flex items-center gap-2">
        <ProtocolIcon protocol={protocol} size={22} />
        <span className="font-medium">{title}</span>
      </div>
      {children}
    </div>
  );
}

// Venues every basket routes into: live APY, LiNEAR rate, pool depth from a $1,000 quote
export function VenueRisk() {
  const { rates, error } = useRates();
  const prices = usePrices();
  const probe = useAsync(WNEAR ? "risk-probe" : null, () => getQuote(USDC.address, WNEAR!.address, PROBE, 50, WNEAR_POOL), 60_000);
  const lend = (sym: string) => rates?.rates.find((r) => r.venue === "lending" && r.token === tokenBySymbol(sym)?.address)?.apy ?? null;
  const stake = rates?.rates.find((r) => r.venue === "staking")?.apy ?? null;
  const near = WNEAR ? prices.priceOf(WNEAR.address) : null;
  const lin = LINEAR ? prices.priceOf(LINEAR.address) : null;
  const ratio = near && lin ? lin / near : null;
  const p = probe.data;

  return (
    <Panel className="space-y-5 p-5 sm:p-6">
      <Head protocol="rhea" title="Venues">
        <Live stale={!!rates?.stale || prices.stale} />
      </Head>
      {error && <ErrorBox message={`Rates unavailable: ${error}`} />}
      <div className="grid gap-3 md:grid-cols-3">
        <Tile protocol="rhea-lending" title="Rhea Lending">
          <dl className="grid grid-cols-2 gap-3">
            <Kv k="USDC supply APY" v={rates ? pct(lend("USDC")) : <Skeleton className="h-5 w-16" />} tone="text-positive" />
            <Kv k="wNEAR supply APY" v={rates ? pct(lend("wNEAR")) : <Skeleton className="h-5 w-16" />} tone="text-positive" />
          </dl>
          <Spark source="lending" token="usdc" />
          <p className="text-[11px] text-muted-foreground">30d USDC supply APY · exit: withdraw from the pool, same tx</p>
        </Tile>
        <Tile protocol="linear" title="LiNEAR staking">
          <dl className="grid grid-cols-2 gap-3">
            <Kv k="Staking APY" v={rates ? pct(stake) : <Skeleton className="h-5 w-16" />} tone="text-positive" />
            <Kv k="LiNEAR / NEAR" v={ratio !== null ? ratio.toFixed(4) : "-"} note={lin !== null ? `$${lin.toFixed(3)}` : undefined} />
          </dl>
          <Spark source="staking" token="wnear" />
          <p className="text-[11px] text-muted-foreground">{STAKING_EXIT}</p>
        </Tile>
        <Tile protocol="rhea-dex" title={`Rhea DEX pool #${WNEAR_POOL}`}>
          {probe.error ? (
            <ErrorBox message={`Quote unavailable: ${probe.error}`} />
          ) : (
            <dl className="grid grid-cols-2 gap-3">
              <Kv k="Max safe size" v={p ? `${fmtUnits(p.max_safe_in, USDC.decimals, 0)} USDC` : <Skeleton className="h-5 w-20" />} note="before impact cap" />
              <Kv
                k="Impact, $1,000"
                v={p ? fmtBps(p.impact_bps) : <Skeleton className="h-5 w-12" />}
                tone={p && p.impact_bps > 100 ? "text-warning" : undefined}
                note={p ? `→ ${fmtUnits(p.amount_out, decimalsOf("wnear"))} wNEAR` : undefined}
              />
            </dl>
          )}
          <p className="text-[11px] text-muted-foreground">USDC / wNEAR. Every non-USDC leg buys and sells here; liquidity sources are coming soon.</p>
        </Tile>
      </div>
      <Source />
    </Panel>
  );
}

const EXIT: Record<string, string> = {
  lending: "Withdraw from Rhea Lending",
  staking: STAKING_EXIT,
  hold: "Already in your account",
};

// one row per basket leg: where it sits and how it comes back
export function Exposures({ positions, strategyOf }: { positions: Position[]; strategyOf: (id: string) => Strategy | undefined }) {
  const prices = usePrices();
  const rows = positions.flatMap((p) => {
    const legs = strategyOf(p.basket_id)?.definition.legs ?? [];
    const vals = legs.map((l, i) => {
      const sym = l.source === "staking" ? "LiNEAR" : symbolOf(l.token);
      const amt = toUnits(p.held[i] ?? "0", decimalsOf(l.token));
      const px = sym === "USDC" ? 1 : prices.priceOf(tokenBySymbol(sym)?.address ?? "");
      return { l, sym, amt, usd: px === null ? null : amt * px };
    });
    const total = vals.reduce((a, v) => a + (v.usd ?? 0), 0);
    return vals.map((v, i) => ({ p, i, ...v, share: total > 0 && v.usd !== null ? v.usd / total : null }));
  });
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[760px] text-sm">
        <thead className="text-left text-xs text-muted-foreground">
          <tr className="border-b border-border">
            <th className="py-3 font-normal">Basket</th>
            <th className="px-3 py-3 font-normal">Venue</th>
            <th className="px-3 py-3 font-normal">Token</th>
            <th className="px-3 py-3 text-right font-normal">Amount</th>
            <th className="px-3 py-3 text-right font-normal">Share</th>
            <th className="py-3 pl-3 font-normal">Exit route</th>
          </tr>
        </thead>
        <tbody className="tnum divide-y divide-border">
          {rows.map(({ p, i, l, sym, amt, share }) => {
            const venue = defaultVenue(l.source);
            return (
              <tr key={`${p.id}:${i}`}>
                <td className="py-3">
                  <Link href={`/baskets/${p.basket_id}`} className="font-medium hover:underline">
                    {p.basket_name}
                  </Link>
                </td>
                <td className="px-3 py-3">
                  <span className="flex items-center gap-2">
                    <ProtocolIcon protocol={protocolOf(venue)} size={18} />
                    {venueName(venue)}
                    <span className="text-xs text-muted-foreground">{KIND_LABEL[l.source]}</span>
                  </span>
                </td>
                <td className="px-3 py-3">
                  <span className="flex items-center gap-2">
                    <TokenIcon symbol={sym} size={18} />
                    {sym}
                  </span>
                </td>
                <td className="px-3 py-3 text-right">{amt.toLocaleString(undefined, { maximumFractionDigits: 4 })}</td>
                <td className="px-3 py-3 text-right">{share === null ? "-" : pct(share, 1)}</td>
                <td className="py-3 pl-3 text-xs text-muted-foreground">
                  {EXIT[l.source]}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
