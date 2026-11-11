import { NETWORK, TOTAL_BPS } from "@/lib/config";
import { keyOf, tokenIdOf } from "@/lib/tokens";
import type { Rates, Sleeve, Source, SourceKind, Strategy } from "@/lib/types";

// basket tokens the planner accepts (deployments tokens)
export const ASSETS = ["USDC", "wNEAR"] as const;
export const MAX_TOKENS = 2;

export const parts = (s: Pick<Strategy, "sleeves">): Sleeve[] => s.sleeves;

// venue id → display name + protocol (ProtocolIcon key)
const VENUES: Record<string, { name: string; protocol: string }> = {
  "rhea-lend": { name: "Rhea Lending", protocol: "rhea-lending" },
  linear: { name: "LiNEAR", protocol: "linear" },
  "rhea-dex": { name: "Rhea DEX pool", protocol: "rhea-dex" },
  wallet: { name: "Your account", protocol: "wallet" },
};
export const protocolOf = (venue: string) => VENUES[venue]?.protocol ?? venue;
export const venueName = (venue: string) => VENUES[venue]?.name ?? venue;

export const lendingVenues = (): { venue: string; name: string }[] => [{ venue: "rhea-lend", name: VENUES["rhea-lend"].name }];

export const KIND_LABEL: Record<SourceKind, string> = { hold: "Hold", lending: "Lending", staking: "Staking", lp: "Liquidity" };

// planner: lending + hold for every token, staking (LiNEAR) for wNEAR; liquidity has no backend venue yet
export function kindsFor(asset: string): SourceKind[] {
  return asset === "wNEAR" ? ["hold", "lending", "staking"] : ["hold", "lending"];
}
export const SOON: SourceKind[] = ["lp"];

export const defaultVenue = (kind: SourceKind) => (kind === "hold" ? "wallet" : kind === "lending" ? "rhea-lend" : kind === "staking" ? "linear" : "rhea-dex");

// live APY per source; null = no rate
export function sourceApy(s: Pick<Source, "kind">, asset: string, rates: Rates | null | undefined): number | null {
  if (s.kind === "hold") return 0;
  if (!rates || s.kind === "lp") return null;
  const id = tokenIdOf(keyOf(asset));
  const r = rates.rates.find((x) => x.venue === s.kind && (s.kind === "staking" || x.token === id));
  return r ? r.apy : null;
}

// Σ weight × apy; null = no apy anywhere
const weighted = <T extends { weightBps: number }>(xs: T[], apy: (x: T) => number | null) => {
  const ps = xs.map(apy);
  return ps.every((p) => p === null) ? null : ps.reduce<number>((a, p, i) => a + (p ?? 0) * (xs[i].weightBps / TOTAL_BPS), 0);
};

// Σ sleeve weight × Σ source weight × live rate
export const blendedApy = (sleeves: Sleeve[], rates: Rates | null | undefined) =>
  weighted(sleeves, (sl) => weighted(sl.sources, (s) => sourceApy(s, sl.asset, rates)));

export const liveApy = (s: Strategy, rates: Rates | null | undefined) => blendedApy(parts(s), rates);

// LiNEAR exit: mainnet swaps back to wNEAR on Rhea; testnet has no LST pool, so it unstakes and NEAR comes later via claim_unstaked
export const STAKING_EXIT =
  NETWORK === "mainnet"
    ? "LiNEAR legs exit by swapping back to wNEAR on Rhea DEX."
    : "LiNEAR legs unstake on sell: NEAR arrives after ~4 epochs (2-3 days) via claim_unstaked.";
