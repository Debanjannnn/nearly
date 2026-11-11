"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { Plus } from "lucide-react";
import { getBaskets, getMyBaskets, pct, shortId } from "@/lib/api";
import { useAsync, useRates } from "@/lib/hooks";
import { useSession } from "@/lib/session";
import { KEYS, symbolOf } from "@/lib/tokens";
import type { Basket } from "@/lib/types";
import { cn } from "@/lib/utils";
import { ErrorBox, Notice, Skeleton } from "@/components/ui";
import { IconStack } from "@/components/TokenIcon";
import { TabPill } from "@/components/motion";
import { blendedApy, legApy, legLabel, tokensOf } from "./legs";

export function BasketList() {
  const router = useRouter();
  const params = useSearchParams();
  const { authenticated } = useSession();
  const list = useAsync("baskets", getBaskets);
  // live APY: rates refresh by WS push, so the column moves without a reload
  const { rates } = useRates();
  const [tab, setTab] = useState(params.get("token") ?? "all");
  // mine includes private baskets, so it comes from /me
  const mine = useAsync(tab === "mine" && authenticated ? "my-baskets" : null, getMyBaskets);
  const shown = tab === "mine" ? mine : list;
  const rows = (shown.data ?? []).filter((b) => tab === "all" || tab === "mine" || b.definition.legs.some((l) => l.token === tab));
  const apy = (b: Basket) => blendedApy(b.definition, rates);
  const best = (list.data ?? []).reduce<Basket | null>((m, b) => ((apy(b) ?? -1) > (m ? (apy(m) ?? -1) : -1) ? b : m), null);
  const stable = (list.data ?? []).filter((b) => b.definition.legs.every((l) => l.token === "usdc")).reduce<Basket | null>((m, b) => ((apy(b) ?? -1) > (m ? (apy(m) ?? -1) : -1) ? b : m), null);

  return (
    <main className="w-full px-4 pb-24 md:px-6">
      <section className="pt-20 pb-16 text-center md:pt-28 md:pb-24">
        <h1 className="text-[40px] leading-tight font-medium tracking-tight md:text-[60px]">Baskets on NEAR</h1>
        <p className="mx-auto mt-6 max-w-lg leading-7 text-pretty text-muted-foreground">
          Baskets split USDC across Rhea lending, LiNEAR staking and plain holds, and run from your own account contract.
        </p>
        <Link
          href="/baskets/new"
          className="mt-10 inline-flex items-center gap-2 rounded-full bg-primary px-6 py-3 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
        >
          <Plus className="size-4" /> Build a basket
        </Link>
      </section>

      {shown.error && (
        <div className="mx-auto mb-4 max-w-7xl">
          <ErrorBox message={`Baskets unavailable: ${shown.error}`} />
        </div>
      )}
      {rates?.stale && (
        <div className="mx-auto mb-4 max-w-7xl">
          <Notice>Venue rates are stale; APYs may lag.</Notice>
        </div>
      )}

      <section className="mx-auto grid max-w-7xl gap-4 pb-14 md:grid-cols-2">
        {[
          { title: "Highest live APY", b: best },
          { title: "Best all-USDC basket", b: stable },
        ].map(({ title, b }) => (
          <article key={title} className="rounded-[22px] bg-secondary/40">
            <p className="px-6 pt-2.5 text-[15px] text-muted-foreground">{title}</p>
            <div className="mt-3 rounded-[22px] border border-white/10 bg-card p-6">
              {b ? (
                <Link href={`/baskets/${b.id}`} className="flex items-center justify-between gap-3">
                  <span className="flex min-w-0 items-center gap-2.5">
                    <IconStack tokens={tokensOf(b.definition)} size={26} ring="ring-card" />
                    <span className="truncate text-[22px] leading-none">{b.name}</span>
                  </span>
                  <span className="tnum text-[22px] font-medium text-positive">{pct(apy(b))}</span>
                </Link>
              ) : list.loading ? (
                <Skeleton className="h-7 w-full" />
              ) : (
                <p className="text-sm text-muted-foreground">No baskets yet.</p>
              )}
            </div>
          </article>
        ))}
      </section>

      <section className="mx-auto max-w-7xl space-y-4">
        <div role="tablist" className="flex flex-wrap items-center gap-1">
          {["all", ...(authenticated ? ["mine"] : []), ...KEYS].map((t) => (
            <button
              key={t}
              role="tab"
              aria-selected={tab === t}
              onClick={() => setTab(t)}
              className={cn("relative rounded-full px-3 py-1 text-[22px] transition-colors", tab === t ? "text-foreground" : "text-muted-foreground hover:text-foreground")}
            >
              {tab === t && <TabPill id="baskets-tab" className="rounded-full bg-secondary" />}
              <span className="relative">{t === "all" ? "All" : t === "mine" ? "Mine" : symbolOf(t)}</span>
            </button>
          ))}
        </div>
        <div className="overflow-hidden rounded-3xl border border-white/10 bg-card shadow-[0_8px_40px_rgba(0,0,0,0.25)]">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[680px] border-collapse text-sm">
              <thead>
                <tr className="border-b border-border bg-secondary/40 text-[13px] text-muted-foreground [&>th]:px-4 [&>th]:py-3 [&>th]:font-medium">
                  <th className="pl-6 text-left">Basket</th>
                  <th className="text-left">Legs</th>
                  <th className="pr-6 text-right">Live APY</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {shown.loading && !shown.data ? (
                  [0, 1, 2].map((i) => (
                    <tr key={i} className="h-16">
                      <td colSpan={3} className="px-6">
                        <Skeleton className="h-8 w-full" />
                      </td>
                    </tr>
                  ))
                ) : rows.length === 0 ? (
                  <tr>
                    <td colSpan={3} className="py-10 text-center text-muted-foreground">
                      No baskets here yet.{" "}
                      <Link href="/baskets/new" className="text-foreground underline underline-offset-4">
                        Build one
                      </Link>
                      .
                    </td>
                  </tr>
                ) : (
                  rows.map((b) => (
                    <tr
                      key={b.id}
                      tabIndex={0}
                      onClick={() => router.push(`/baskets/${b.id}`)}
                      onKeyDown={(e) => e.key === "Enter" && router.push(`/baskets/${b.id}`)}
                      className="h-16 cursor-pointer transition-colors hover:bg-secondary/40 [&>td]:px-4"
                    >
                      <td className="pl-6">
                        <div className="flex items-center gap-3">
                          <IconStack tokens={tokensOf(b.definition)} size={26} ring="ring-card" />
                          <div className="min-w-0">
                            <div className="truncate text-[15px] font-medium">
                              {b.name}
                              {!b.public && <span className="ml-2 rounded-full bg-secondary px-2 py-0.5 text-[11px] text-muted-foreground">Private</span>}
                            </div>
                            <div className="text-xs text-muted-foreground">by {shortId(b.creator)}</div>
                          </div>
                        </div>
                      </td>
                      <td>
                        <div className="flex flex-wrap gap-1.5">
                          {b.definition.legs.map((l) => {
                            const a = legApy(l, rates);
                            return (
                              <span key={l.token + l.source} className="tnum inline-flex items-center gap-1 rounded-full bg-secondary px-2 py-0.5 text-[11px]">
                                {legLabel(l)} {l.weight_bps / 100}%{a ? <span className="text-positive">{pct(a, 1)}</span> : null}
                              </span>
                            );
                          })}
                        </div>
                      </td>
                      <td className="tnum pr-6 text-right font-medium text-positive">{pct(apy(b))}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </section>
    </main>
  );
}
