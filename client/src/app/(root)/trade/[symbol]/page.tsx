"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useState } from "react";
import { motion } from "motion/react";
import { fmtNum, fmtUsd } from "@/lib/api";
import { addressUrl, NETWORK } from "@/lib/config";
import { usePrices, useHoldings, useSeries, useTickers } from "@/lib/hooks";
import { marketOf, pairsOf } from "@/lib/tokens";
import { Button, Card, Detail, Notice, Panel, Stat } from "@/components/ui";
import { TokenIcon } from "@/components/TokenIcon";
import { PriceChart, RANGES, type Range } from "@/components/chart/PriceChart";
import { MarketPanel } from "@/components/trade/MarketPanel";
import { ScheduleForm } from "@/components/automation/ScheduleForm";
import { ScheduleList } from "@/components/automation/ScheduleList";
import { AutoHeight, Segmented } from "@/components/motion";
import { ProtocolIcon } from "@/components/ProtocolIcon";

type Tab = "market" | "auto";

export default function TradePage() {
  const router = useRouter();
  const market = marketOf(decodeURIComponent(useParams<{ symbol: string }>().symbol));
  const [range, setRange] = useState<Range>("1M");
  const [mode, setMode] = useState<"line" | "candle">("candle");
  const [tab, setTab] = useState<Tab>("market");
  const [key, setKey] = useState(0);
  const chart = useSeries(market?.history, RANGES[range].ms);
  const { tickers, loading: tkLoading } = useTickers();
  const { stale } = usePrices();
  const holdings = useHoldings();

  if (!market)
    return (
      <main className="w-full px-4 pb-24 pt-8 md:px-6">
        <Panel className="mx-auto max-w-md p-8 text-center">
          <p className="text-lg font-medium">Market not found</p>
          <Link href="/" className="mt-5 inline-block">
            <Button variant="ghost">All markets</Button>
          </Link>
        </Panel>
      </main>
    );

  const t = market.token;
  const tk = tickers[market.symbol];
  const price = tk?.price ?? chart.points.at(-1)?.v ?? null;
  const points = chart.points;
  // history is daily: 24h range = yesterday's close vs now
  const day = tk?.series.slice(-2) ?? [];
  const held = holdings.data?.tokens.find((x) => x.symbol === t.symbol);
  const pairs = pairsOf(market);
  const q = market.quote.symbol;

  return (
    <main className="w-full px-4 pb-24 pt-10 md:px-6">
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_440px]">
        <div className="min-w-0 space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div className="flex min-w-0 items-center gap-3">
              <TokenIcon symbol={t.symbol} size={36} />
              <h1 className="truncate text-3xl font-medium tracking-tight">
                {t.name} <span className="text-muted-foreground">{t.symbol}</span>
              </h1>
            </div>
            {pairs.length > 1 && (
              <Segmented
                id="quote"
                className="w-auto shrink-0"
                value={market.symbol}
                onChange={(s) => router.replace(`/trade/${s}`)}
                options={pairs.map((p) => ({ value: p.symbol, label: p.quote.symbol }))}
              />
            )}
            {stale && <span className="rounded-full bg-warning/10 px-2 py-0.5 text-[11px] font-medium text-warning">Stale price</span>}
          </div>

          <PriceChart
            points={points}
            price={price}
            range={range}
            onRange={setRange}
            mode={mode}
            onMode={setMode}
            loading={chart.loading}
            error={chart.error}
            caption={`${t.symbol} / ${q}`}
            dataKey={`${market.symbol}:${range}`}
          />

          <Card title="Statistics">
            <dl className="grid grid-cols-2 gap-6 md:grid-cols-4">
              <Stat label="24h high" value={tkLoading ? null : day.length ? fmtUsd(Math.max(...day)) : "-"} />
              <Stat label="24h low" value={tkLoading ? null : day.length ? fmtUsd(Math.min(...day)) : "-"} />
              <Stat label="24h volume" value="-" />
              <Stat label="Your balance" value={held ? fmtNum(Number(held.amount), 6) : "-"} note={held && price ? fmtUsd(Number(held.amount) * price) : undefined} />
            </dl>
          </Card>

          <Card title="About">
            <dl className="grid gap-x-10 md:grid-cols-2">
              <Detail label="Token">
                <a href={addressUrl(market.swap.address)} target="_blank" rel="noreferrer" className="tnum hover:underline">
                  {market.swap.address.length > 20 ? `${market.swap.address.slice(0, 8)}…${market.swap.address.slice(-6)}` : market.swap.address}
                </a>
              </Detail>
              <Detail label="Rhea DEX pool">
                <span className="tnum inline-flex items-center gap-1.5">
                  <ProtocolIcon protocol="rhea-dex" size={16} />
                  {market.pool !== undefined ? `#${market.pool}` : "-"}
                </span>
              </Detail>
              <Detail label="Decimals">{t.decimals}</Detail>
              <Detail label="Network">NEAR {NETWORK}</Detail>
            </dl>
          </Card>
        </div>

        <aside className="space-y-3 lg:sticky lg:top-24 lg:self-start">
          <Segmented
            id="trade"
            size="lg"
            value={tab}
            onChange={setTab}
            options={[
              { value: "market", label: "Market" },
              { value: "auto", label: "Automate" },
            ]}
          />
          {q !== "USDC" && pairs.length > 1 && (
            <Notice>Thin {t.symbol}/{q} pool: less depth than {t.symbol}/USDC. Check the quote before trading size.</Notice>
          )}
          <AutoHeight className="rounded-[28px] border border-border bg-secondary/60">
            <motion.div key={tab} initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.2 }}>
              {tab === "market" ? (
                <MarketPanel market={market} holdings={holdings.data} onDone={holdings.reload} />
              ) : (
                <div className="space-y-4 p-5">
                  <ScheduleForm target={{ type: "market", id: market.symbol }} current={price} quote={q} onCreated={() => setKey((n) => n + 1)} />
                  <ScheduleList key={key} target={{ type: "market", id: market.symbol }} />
                </div>
              )}
            </motion.div>
          </AutoHeight>
        </aside>
      </div>
    </main>
  );
}
