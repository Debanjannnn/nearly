"use client";

import Link from "next/link";
import { errText, getMyStrategies, getPositions, getStrategies } from "@/lib/api";
import { useAsync, useSession } from "@/lib/session";
import { ErrorBox, Panel, Skeleton } from "@/components/ui";
import { Exposures, VenueRisk } from "@/components/risk/VenueRisk";

// Venue risk: the NEAR venues baskets route into (protocol view) above the viewer's legs (account view)
export default function RiskPage() {
  const { ready, authenticated, address } = useSession();
  const on = ready && authenticated && !!address;
  const positions = useAsync(on ? "positions" : null, getPositions, 30_000);
  const baskets = useAsync("strategies", () => getStrategies());
  const mine = useAsync(on ? "my-strategies" : null, getMyStrategies);
  const strats = new Map([...(baskets.data ?? []), ...(mine.data ?? [])].map((b) => [b.id, b]));
  const open = (positions.data ?? []).filter((p) => p.state !== "closed");

  return (
    <main className="w-full px-4 pb-24 pt-8 md:px-6">
      <div className="mx-auto w-full space-y-6">
        <header className="space-y-1 px-1">
          <h1 className="text-2xl font-medium">Risk</h1>
          <p className="text-sm text-muted-foreground">
            Baskets route into Rhea Lending, LiNEAR staking and the Rhea DEX pool. The venues that set your yield and exit cost, and every leg you hold.
          </p>
        </header>

        <VenueRisk />

        <section className="space-y-3">
          <h2 className="px-1 text-lg font-medium">Your exposures</h2>
          {!ready ? (
            <Skeleton className="h-40 w-full rounded-3xl" />
          ) : !on ? (
            <Panel className="p-5 text-sm text-muted-foreground">Connect to see where each of your basket legs sits and how it exits.</Panel>
          ) : positions.error ? (
            <ErrorBox message={errText(new Error(positions.error))} />
          ) : !positions.data ? (
            <Skeleton className="h-40 w-full rounded-3xl" />
          ) : open.length === 0 ? (
            <Panel className="p-5 text-sm text-muted-foreground">
              No open baskets yet.{" "}
              <Link href="/baskets" className="text-foreground underline underline-offset-4 hover:no-underline">
                Explore baskets
              </Link>{" "}
              and their legs show up here.
            </Panel>
          ) : (
            <Panel className="p-5 sm:p-6">
              <Exposures positions={open} strategyOf={(id) => strats.get(id)} />
            </Panel>
          )}
        </section>
      </div>
    </main>
  );
}
