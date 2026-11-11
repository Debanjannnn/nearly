"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { Plus } from "lucide-react";
import { motion } from "motion/react";
import { fmtUsd, getBaskets, getPriceSeries, pct, shortId } from "@/lib/api";
import { useAsync, usePrices, useRates } from "@/lib/hooks";
import { KEYS, symbolOf, tokenBySymbol } from "@/lib/tokens";
import { cn } from "@/lib/utils";
import { ErrorBox, Notice, Skeleton } from "@/components/ui";
import { HeroSearch } from "@/components/BasketSearch";
import { Sparkline } from "@/components/Sparkline";
import { EASE, Flash, Reveal, TabPill } from "@/components/motion";
import { IconStack, TokenIcon } from "@/components/TokenIcon";
import { blendedApy, legApy, legLabel, tokensOf } from "@/components/basket/legs";

type Row = { key: string; href: string; name: string; sub: string; icons: string[]; series: number[]; price: number | null; chg: number | null; lend: number | null; stake: number | null; apy: number | null; creator?: string };

const MONTH = 30 * 86400e3;

export function HomePage() {
  const router = useRouter();
  const params = useSearchParams();
  const [tab, setTab] = useState<"markets" | "baskets">(params.get("tab") === "baskets" ? "baskets" : "markets");
  const { rates, error: ratesErr } = useRates();
  const { priceOf, error: pxErr } = usePrices();
  const baskets = useAsync("baskets", getBaskets);
  // 30d daily closes per non-base token from history (usdc is flat)
  const hist = useAsync("home-series", async () => {
    const from = new Date(Date.now() - MONTH).toISOString().slice(0, 10);
    const res = await Promise.allSettled(KEYS.filter((k) => k !== "usdc").map((k) => getPriceSeries(k, from).then((d) => [k, d.map((p) => Number(p.usd))] as const)));
    return Object.fromEntries(res.flatMap((r) => (r.status === "fulfilled" ? [r.value] : [])));
  });

  const marketRows: Row[] = KEYS.map((k) => {
    const series = hist.data?.[k] ?? [];
    const price = priceOf(k) ?? series.at(-1) ?? null;
    return {
      key: k,
      href: `/baskets?token=${k}`,
      name: tokenBySymbol(symbolOf(k))?.name ?? symbolOf(k),
      sub: symbolOf(k),
      icons: [k],
      series,
      price,
      chg: price !== null && series[0] ? ((price - series[0]) / series[0]) * 100 : null,
      lend: legApy({ token: k, source: "lending" }, rates),
      stake: k === "wnear" ? legApy({ token: k, source: "staking" }, rates) : null,
      apy: null,
    };
  });
  const basketRows: Row[] = (baskets.data ?? []).map((b) => ({
    key: b.id,
    href: `/baskets/${b.id}`,
    name: b.name,
    sub: b.definition.legs.map((l) => `${legLabel(l)} ${l.weight_bps / 100}%`).join(" · "),
    icons: tokensOf(b.definition),
    series: [],
    price: null,
    chg: null,
    lend: null,
    stake: null,
    apy: blendedApy(b.definition, rates),
    creator: b.creator,
  }));

  const top = (list: Row[], f: (r: Row) => number | null) => list.reduce<Row | null>((m, r) => (f(r) !== null && (!m || f(r)! > f(m)!) ? r : m), null);
  const near = marketRows.find((r) => r.key === "wnear") ?? null;
  const bestLend = top(marketRows, (r) => Math.max(r.lend ?? -1, r.stake ?? -1));
  const topBasket = top(basketRows, (r) => r.apy);
  const rows = tab === "markets" ? marketRows : basketRows;
  const loading = tab === "markets" ? !rates && !ratesErr && !hist.data : baskets.loading && !baskets.data;

  return (
    <main className="w-full px-4 pb-24 md:px-6">
      <section className="pt-16 pb-20 text-center md:pt-24 md:pb-28">
        <h1 className="text-[40px] leading-tight font-medium tracking-tight md:text-[60px]">Yield baskets on NEAR</h1>
        <p className="mx-auto mt-4 max-w-lg text-pretty text-muted-foreground">Lending, staking and holds in one buy, from your own account contract.</p>
        <div className="mt-8 md:mt-10">
          <HeroSearch />
        </div>
      </section>

      {(ratesErr || pxErr || baskets.error) && (
        <div className="mx-auto mb-4 max-w-7xl">
          <ErrorBox message={`Some data unavailable: ${ratesErr ?? pxErr ?? baskets.error}`} />
        </div>
      )}
      {rates?.stale && (
        <div className="mx-auto mb-4 max-w-7xl">
          <Notice>Venue rates are stale; APYs may lag.</Notice>
        </div>
      )}

      <section className="mx-auto grid max-w-7xl grid-cols-1 gap-4 pb-16 md:grid-cols-2 md:pb-24 lg:grid-cols-3">
        <Reveal>
          <Highlight title="NEAR (30d)" row={near} loading={!near?.price && !hist.data} value={(r) => (r.price ? <Flash value={r.price}>{fmtUsd(r.price)}</Flash> : "-")} />
        </Reveal>
        <Reveal delay={0.05}>
          <Highlight title="Best venue APY" row={bestLend} loading={!rates} value={(r) => <span className="text-positive">{pct(Math.max(r.lend ?? 0, r.stake ?? 0))}</span>} />
        </Reveal>
        <Reveal delay={0.1}>
          <Highlight title="Top basket (live APY)" row={topBasket} loading={baskets.loading && !baskets.data} value={(r) => <span className="text-positive">{pct(r.apy)}</span>} />
        </Reveal>
      </section>

      <section className="mx-auto max-w-7xl space-y-4">
        <div className="flex flex-wrap items-center gap-1">
          <div role="tablist" className="flex flex-wrap items-center gap-1">
            {(["markets", "baskets"] as const).map((t) => (
              <button
                key={t}
                role="tab"
                aria-selected={tab === t}
                onClick={() => setTab(t)}
                className={cn("relative rounded-full px-3 py-1 text-[22px] transition-colors", tab === t ? "text-foreground" : "text-muted-foreground hover:text-foreground")}
              >
                {tab === t && <TabPill id="home-cat" className="rounded-full bg-secondary" />}
                <span className="relative">{t === "markets" ? "Markets" : "Baskets"}</span>
              </button>
            ))}
          </div>
          {tab === "baskets" && (
            <Link href="/baskets/new" className="ml-auto inline-flex items-center gap-1.5 rounded-full bg-secondary px-3.5 py-2 text-sm font-medium hover:bg-accent">
              <Plus className="size-4" /> New basket
            </Link>
          )}
        </div>

        <div className="overflow-hidden rounded-3xl border border-white/10 bg-card shadow-[0_8px_40px_rgba(0,0,0,0.25)]">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm">
              <thead>
                <tr className="border-b border-border bg-secondary/40 text-[13px] text-muted-foreground [&>th]:px-4 [&>th]:py-3 [&>th]:font-medium">
                  {tab === "markets" ? (
                    <>
                      <th className="w-[30%] pl-6 text-left">Token</th>
                      <th className="text-right">Price</th>
                      <th className="text-right">30d</th>
                      <th className="hidden text-center lg:table-cell">Last 30d</th>
                      <th className="text-right">Lending APY</th>
                      <th className="hidden pr-6 text-right sm:table-cell">Staking APY</th>
                    </>
                  ) : (
                    <>
                      <th className="w-[40%] pl-6 text-left">Basket</th>
                      <th className="text-right">Live APY</th>
                      <th className="hidden pr-6 text-right md:table-cell">Creator</th>
                    </>
                  )}
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {loading ? (
                  [0, 1, 2].map((i) => (
                    <tr key={i} className="h-16">
                      <td colSpan={6} className="px-6">
                        <Skeleton className="h-8 w-full" />
                      </td>
                    </tr>
                  ))
                ) : rows.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="py-10 text-center text-muted-foreground">
                      No baskets yet.{" "}
                      <Link href="/baskets/new" className="text-foreground underline underline-offset-4">
                        Create one
                      </Link>
                      .
                    </td>
                  </tr>
                ) : (
                  rows.map((r, i) => {
                    const open = () => router.push(r.href);
                    return (
                      <motion.tr
                        key={r.key}
                        initial={{ opacity: 0, y: 4 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.25, delay: Math.min(i, 10) * 0.03, ease: EASE }}
                        tabIndex={0}
                        onClick={open}
                        onKeyDown={(e) => e.key === "Enter" && open()}
                        className="h-16 cursor-pointer transition-colors duration-150 hover:bg-secondary/40 focus-visible:bg-secondary/40 focus-visible:outline-none [&>td]:px-4"
                      >
                        <td className="pl-6">
                          <div className="flex items-center gap-3">
                            {r.icons.length > 1 ? <IconStack tokens={r.icons} size={26} ring="ring-card" /> : <TokenIcon token={r.icons[0]} size={26} />}
                            <span className="truncate text-[15px] font-medium">{r.name}</span>
                            <span className="hidden truncate text-muted-foreground sm:inline">{r.sub}</span>
                          </div>
                        </td>
                        {tab === "markets" ? (
                          <>
                            <td className="tnum text-right font-medium">{r.price !== null ? <Flash value={r.price}>{fmtUsd(r.price)}</Flash> : "-"}</td>
                            <td className={cn("tnum text-right", r.chg === null ? "text-muted-foreground" : r.chg >= 0 ? "text-positive" : "text-destructive")}>
                              {r.chg === null ? "-" : `${r.chg >= 0 ? "+" : ""}${r.chg.toFixed(2)}%`}
                            </td>
                            <td className="hidden lg:table-cell">
                              <Sparkline data={r.series} area className="mx-auto h-9 w-[140px]" />
                            </td>
                            <td className="tnum text-right text-positive">{pct(r.lend)}</td>
                            <td className="tnum hidden pr-6 text-right text-positive sm:table-cell">{pct(r.stake)}</td>
                          </>
                        ) : (
                          <>
                            <td className="tnum text-right font-medium text-positive">{pct(r.apy)}</td>
                            <td className="tnum hidden pr-6 text-right text-muted-foreground md:table-cell">{r.creator ? shortId(r.creator) : "-"}</td>
                          </>
                        )}
                      </motion.tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </div>
      </section>
    </main>
  );
}

function Highlight({ title, row, value, loading }: { title: string; row: Row | null; value: (r: Row) => React.ReactNode; loading?: boolean }) {
  const body = (
    <article className="rounded-[22px] bg-secondary/40">
      <p className="px-6 pt-2.5 text-[15px] text-muted-foreground">{title}</p>
      <div className="mt-3 rounded-[22px] border border-white/10 bg-card p-6 transition-[box-shadow,scale,translate,border-color] duration-200 ease-out hover:-translate-y-0.5 hover:border-white/15 hover:ring-2 hover:ring-white/10 active:scale-[0.98]">
        {row && !loading ? (
          <>
            {row.series.length ? (
              <Sparkline data={row.series} className="h-32 w-full" />
            ) : (
              <p className="grid h-32 place-items-center text-center text-sm text-muted-foreground">{row.sub}</p>
            )}
            <div className="mt-1 flex items-center justify-between gap-3">
              <span className="flex min-w-0 items-center gap-2.5">
                <IconStack tokens={row.icons} size={26} ring="ring-card" />
                <span className="truncate text-[22px] leading-none">{row.name}</span>
              </span>
              <span className="tnum shrink-0 text-[18px] leading-none font-medium">{value(row)}</span>
            </div>
          </>
        ) : loading ? (
          <>
            <Skeleton className="h-32 w-full" />
            <Skeleton className="mt-1 h-7 w-40" />
          </>
        ) : (
          <p className="grid h-40 place-items-center text-sm text-muted-foreground">Nothing yet.</p>
        )}
      </div>
    </article>
  );
  return row ? (
    <Link href={row.href} aria-label={`View ${row.name}`} className="block">
      {body}
    </Link>
  ) : (
    body
  );
}
