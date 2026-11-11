"use client";

import Link from "next/link";
import { ChevronDown } from "lucide-react";
import { fmtUsd } from "@/lib/api";
import type { PriceOf } from "@/lib/hooks";
import { decimalsOf, fmtUnits, symbolOf, toUnits } from "@/lib/tokens";
import type { Basket, Holdings as HoldingsT } from "@/lib/types";
import { Panel, Skeleton, useDismissable } from "@/components/ui";
import { TokenIcon } from "@/components/TokenIcon";

export const holdingsUsd = (h: HoldingsT | null, priceOf: PriceOf) =>
  Object.entries(h?.tokens ?? {}).reduce((s, [k, t]) => s + toUnits(t.balance, decimalsOf(k)) * (priceOf(k) ?? 0), 0);

// free balances in the sub-account (not in any venue)
export function Holdings({ h, loading, priceOf, baskets }: { h: HoldingsT | null; loading: boolean; priceOf: PriceOf; baskets: Basket[] | null }) {
  const rows = Object.entries(h?.tokens ?? {});
  return (
    <Panel className="p-5 sm:p-6">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="mr-auto text-lg font-medium">Account balances</h2>
        <InvestMenu baskets={baskets} />
      </div>
      {h && <p className="tnum mt-1 text-xs break-all text-muted-foreground">{h.account_id}</p>}
      <div className="mt-4 overflow-x-auto">
        <table className="w-full min-w-[480px] text-sm">
          <thead className="text-left text-xs text-muted-foreground">
            <tr className="border-b border-border">
              <th className="py-3 pl-3 font-normal">Token</th>
              <th className="px-3 py-3 font-normal">Price ($)</th>
              <th className="px-3 py-3 font-normal">Balance</th>
              <th className="py-3 pr-3 font-normal">Value ($)</th>
            </tr>
          </thead>
          <tbody className="tnum divide-y divide-border">
            {loading && !h ? (
              <tr>
                <td className="py-4 pl-3" colSpan={4}>
                  <Skeleton className="h-8 w-full" />
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={4} className="py-10 text-center text-sm text-muted-foreground">
                  No balances yet.
                </td>
              </tr>
            ) : (
              rows.map(([k, t]) => {
                const px = priceOf(k);
                const dec = decimalsOf(k);
                return (
                  <tr key={k} className="transition-colors hover:bg-secondary/30">
                    <td className="py-3 pl-3">
                      <div className="flex items-center gap-3">
                        <TokenIcon token={k} size={28} />
                        <div className="min-w-0">
                          <div className="font-medium">{symbolOf(k)}</div>
                          <div className="truncate text-xs text-muted-foreground">{t.token}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-3 py-3">{px === null ? "-" : fmtUsd(px)}</td>
                    <td className="px-3 py-3">{fmtUnits(t.balance, dec, 4)}</td>
                    <td className="py-3 pr-3">{px === null ? "-" : fmtUsd(toUnits(t.balance, dec) * px)}</td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </Panel>
  );
}

function InvestMenu({ baskets }: { baskets: Basket[] | null }) {
  const { open, setOpen, ref } = useDismissable();
  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
      >
        Invest <ChevronDown className="size-4" />
      </button>
      {open && (
        <ul className="absolute right-0 z-30 mt-2 max-h-72 w-60 overflow-y-auto rounded-xl border border-border bg-popover p-1 shadow-xl">
          {[{ href: "/baskets", name: "All baskets" }, ...(baskets ?? []).map((b) => ({ href: `/baskets/${b.id}`, name: b.name }))].map((b) => (
            <li key={b.href}>
              <Link href={b.href} onClick={() => setOpen(false)} className="block truncate rounded-lg px-3 py-2 text-sm transition-colors hover:bg-secondary/70">
                {b.name}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
