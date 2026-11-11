"use client";

import { CartesianGrid, Cell, Line, LineChart, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { errText, fmtUsd, pct } from "@/lib/api";
import { symbolOf } from "@/lib/tokens";
import type { Backtest, BasketDef, Source } from "@/lib/types";
import { cn } from "@/lib/utils";
import { ErrorBox, Skeleton, Stat } from "@/components/ui";
import { legLabel, SOURCE_LABEL } from "./legs";

const COLORS = ["var(--primary)", "var(--chart-1)", "var(--chart-2)", "var(--warning)", "var(--chart-3)", "var(--chart-4)"];

// series "price:wnear" | "apy:lending:usdc", contribution keys "lending:usdc"
function seriesLabel(series: string) {
  const parts = series.split(":");
  const tok = parts.at(-1)!;
  if (parts[0] === "price") return `${symbolOf(tok)} price`;
  const src = parts.find((p): p is Source => p in SOURCE_LABEL);
  return src ? `${symbolOf(tok)} ${SOURCE_LABEL[src].toLowerCase()}` : series;
}

// equity curve over the window + headline stats + coverage notes
export function BacktestView({ bt, loading, error }: { bt: Backtest | null; loading: boolean; error: string | null }) {
  if (error) return <ErrorBox message={errText(new Error(error))} />;
  if (!bt) return loading ? <Skeleton className="h-80 w-full" /> : <p className="text-sm text-muted-foreground">Run a backtest to see history.</p>;
  const clamped = bt.coverage.filter((c) => c.note);
  const contrib = Object.entries(bt.contributions).filter(([, v]) => Math.abs(v) >= 0.005);
  return (
    <div className="space-y-4">
      <dl className="grid grid-cols-2 gap-6 md:grid-cols-3">
        <Stat label="APY (USD)" value={<span className="text-positive">{pct(bt.apy)}</span>} note={`${bt.days}d window`} />
        <Stat label="Max drawdown" value={pct(bt.max_drawdown)} />
        <Stat label="Final" value={fmtUsd(bt.final_usd)} note={`from ${fmtUsd(bt.initial_usd)}`} />
      </dl>
      <div className="h-64">
        <ResponsiveContainer width="100%" height="100%" initialDimension={{ width: 1, height: 1 }}>
          <LineChart data={bt.curve} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
            <CartesianGrid vertical={false} strokeDasharray="3 4" stroke="var(--border)" />
            <XAxis dataKey="day" tick={{ fill: "var(--muted-foreground)", fontSize: 10 }} axisLine={false} tickLine={false} minTickGap={48} />
            <YAxis
              orientation="right"
              domain={["auto", "auto"]}
              tickFormatter={(v: number) => fmtUsd(v)}
              tick={{ fill: "var(--muted-foreground)", fontSize: 10 }}
              axisLine={false}
              tickLine={false}
              width={72}
            />
            <Tooltip
              contentStyle={{ background: "var(--popover)", border: "1px solid var(--border)", borderRadius: 12, fontSize: 12 }}
              formatter={(v) => [fmtUsd(Number(v)), "Basket"]}
            />
            <Line type="linear" dataKey="usd" stroke="var(--primary)" strokeWidth={1.75} dot={false} isAnimationActive={false} />
          </LineChart>
        </ResponsiveContainer>
      </div>
      <p className="text-xs text-muted-foreground">
        {bt.from} → {bt.to} · {bt.rebalances} rebalances · costs {fmtUsd(bt.costs_usd)}
      </p>
      {contrib.length > 0 && (
        <div className="flex flex-wrap gap-2 text-xs">
          {contrib.map(([k, v]) => (
            <span key={k} className="tnum inline-flex items-center gap-1.5 rounded-full border border-border bg-card px-2.5 py-1">
              {k === "price" ? "Price move" : k === "costs" ? "Costs" : seriesLabel(k)}
              <span className={v >= 0 ? "text-positive" : "text-destructive"}>
                {v >= 0 ? "+" : ""}
                {fmtUsd(v)}
              </span>
            </span>
          ))}
        </div>
      )}
      {clamped.length > 0 && (
        <div className="space-y-2 rounded-xl border border-border bg-secondary/30 p-4 text-xs">
          <p className="text-muted-foreground">Window shortened to the data every source has</p>
          <div className="flex flex-wrap gap-2">
            {clamped.map((c) => (
              <span key={c.series} className="tnum inline-flex items-center gap-1.5 rounded-full border border-border bg-card px-2.5 py-1">
                {seriesLabel(c.series)}
                <span className="text-muted-foreground">
                  {c.first} → {c.last}
                </span>
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// one slice per leg; stacked = donut above a full-width legend (narrow cards)
export function AllocationDonut({ def, stacked = false }: { def: BasketDef; stacked?: boolean }) {
  const data = def.legs.map((l) => ({ name: legLabel(l), value: l.weight_bps }));
  return (
    <div className={cn("flex items-center gap-6", stacked ? "flex-col" : "flex-wrap")}>
      <div className={stacked ? "size-44" : "size-40"}>
        <ResponsiveContainer width="100%" height="100%" initialDimension={{ width: 1, height: 1 }}>
          <PieChart>
            <Pie data={data} dataKey="value" innerRadius="62%" outerRadius="100%" stroke="var(--card)" strokeWidth={2} isAnimationActive={false}>
              {data.map((_, i) => (
                <Cell key={i} fill={COLORS[i % COLORS.length]} />
              ))}
            </Pie>
          </PieChart>
        </ResponsiveContainer>
      </div>
      <ul className={cn("space-y-2 text-sm", stacked ? "w-full" : "min-w-40 flex-1")}>
        {data.map((d, i) => (
          <li key={d.name} className="flex items-center gap-2">
            <span className="size-2.5 shrink-0 rounded-full" style={{ background: COLORS[i % COLORS.length] }} />
            <span className="truncate">{d.name}</span>
            <span className="tnum ml-auto text-muted-foreground">{+(d.value / 100).toFixed(2)}%</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
