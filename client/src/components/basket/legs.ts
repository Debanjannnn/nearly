import { DEPLOYMENTS, TOTAL_BPS, WNEAR_POOL } from "@/lib/config";
import { symbolOf, tokenIdOf } from "@/lib/tokens";
import type { BasketDef, Leg, Rates, Source } from "@/lib/types";

export const SOURCES: Source[] = ["lending", "staking", "hold"];
export const SOURCE_LABEL: Record<Source, string> = { lending: "Lending", staking: "Staking", hold: "Hold" };
// venue names per source (contract ids from the networks file)
export const VENUE_NAME: Record<Source, string> = { lending: "Rhea Lending", staking: "LiNEAR", hold: "Your account" };
export const venueId = (s: Source) => (s === "hold" ? null : DEPLOYMENTS.venues?.[s] ?? null);

// planner: staking takes wnear only; lending and hold take any listed token
export const sourcesFor = (token: string): Source[] => (token === "wnear" ? SOURCES : ["lending", "hold"]);

export const legLabel = (l: Pick<Leg, "token" | "source">) => `${symbolOf(l.token)} · ${SOURCE_LABEL[l.source]}`;

// a non-usdc leg is bought on the Rhea pool
export const withPool = (l: Leg): Leg => (l.token === "usdc" ? { ...l, pool_id: undefined } : { ...l, pool_id: WNEAR_POOL });

// live APY of one leg; hold earns 0, null = no rate yet
export function legApy(l: Pick<Leg, "token" | "source">, rates: Rates | null | undefined): number | null {
  if (l.source === "hold") return 0;
  if (!rates) return null;
  const id = tokenIdOf(l.token);
  const r = rates.rates.find((x) => x.venue === l.source && (l.source === "staking" || x.token === id));
  return r ? r.apy : null;
}

// Σ weight × live APY; null when no leg has a rate
export function blendedApy(def: BasketDef, rates: Rates | null | undefined): number | null {
  const ps = def.legs.map((l) => legApy(l, rates));
  return ps.every((p) => p === null) ? null : ps.reduce<number>((a, p, i) => a + (p ?? 0) * (def.legs[i].weight_bps / TOTAL_BPS), 0);
}

export const tokensOf = (def: BasketDef) => [...new Set(def.legs.map((l) => l.token))];

// n equal bps weights summing to 10000; remainder goes to the first legs
export const evenSplit = (n: number): number[] =>
  Array.from({ length: n }, (_, i) => Math.floor(TOTAL_BPS / n) + (i < TOTAL_BPS % n ? 1 : 0));

// sets w[i] to bps, capped at what the other legs leave of 100%
export const setWeight = (w: number[], i: number, bps: number): number[] => {
  const others = w.reduce((a, b, j) => (j === i ? a : a + b), 0);
  return w.map((x, j) => (j === i ? Math.max(0, Math.min(bps, TOTAL_BPS - others)) : x));
};

export const weightsError = (w: number[]): string | null => {
  if (w.length === 0) return "Pick at least one leg";
  if (w.some((x) => x <= 0)) return "Every weight must be above 0%";
  const sum = w.reduce((a, b) => a + b, 0);
  return sum === TOTAL_BPS ? null : `Weights sum to ${(sum / 100).toFixed(2)}%, need 100%`;
};
