import { DEPLOYMENTS, WNEAR_POOL } from "./config";

// address = NEP-141 contract ("near" for the native token); key = deployments token key the api uses
export type Token = { address: string; symbol: string; name: string; decimals: number; key?: string; icon?: string; color?: string };

const NEAR_ICON = "/tokens/near.svg";

export const NEAR: Token = { address: "near", symbol: "NEAR", name: "NEAR", decimals: 24, icon: NEAR_ICON };

const META: Record<string, Omit<Token, "address" | "key">> = {
  usdc: { symbol: "USDC", name: "USD Coin", decimals: 6, icon: "/tokens/usdc.svg" },
  wnear: { symbol: "wNEAR", name: "Wrapped NEAR", decimals: 24, icon: "/tokens/wnear.svg" },
};

// LiNEAR: the staking venue contract is also its FT
const LINEAR: Token = { address: DEPLOYMENTS.venues?.staking ?? "", symbol: "LiNEAR", name: "LiNEAR", decimals: 24, icon: "/tokens/linear.svg" };

const list: Token[] = [
  NEAR,
  ...Object.entries(DEPLOYMENTS.tokens ?? {}).flatMap(([key, id]) => (id && META[key] ? [{ address: id, key, ...META[key] }] : [])),
  LINEAR,
];

export const TOKENS = list;
export const tokenBySymbol = (symbol: string) => list.find((t) => t.symbol === symbol);
export const tokenById = (id: string) => list.find((t) => t.address === id);

// deployments token key helpers; baskets and holdings use keys
export const KEYS = Object.keys(META);
export const tokenIdOf = (key: string) => DEPLOYMENTS.tokens?.[key as "usdc" | "wnear"] ?? "";
export const symbolOf = (key: string) => META[key]?.symbol ?? key.toUpperCase();
export const keyOf = (symbol: string) => KEYS.find((k) => META[k].symbol === symbol) ?? symbol.toLowerCase();
export const decimalsOf = (key: string) => META[key]?.decimals ?? 24;
// atoms → float units, for USD display math only
export const toUnits = (raw: string | bigint, decimals: number) => Number(BigInt(raw)) / 10 ** decimals;

export const USDC: Token = tokenBySymbol("USDC") ?? { address: "", key: "usdc", ...META.usdc };
const WNEAR = tokenBySymbol("wNEAR") ?? { address: "", key: "wnear", ...META.wnear };

// tradable tokens on Rhea DEX against USDC. swap = FT the pool holds (NEAR trades as wNEAR);
// history = series key for daily closes (history only stores wnear)
export type MarketDef = { symbol: string; token: Token; quote: Token; swap: Token; history: string | null; pool?: number };
export const MARKETS: MarketDef[] = [
  { symbol: "NEAR", token: NEAR, quote: USDC, swap: WNEAR, history: "wnear", pool: WNEAR_POOL },
  { symbol: "wNEAR", token: WNEAR, quote: USDC, swap: WNEAR, history: "wnear", pool: WNEAR_POOL },
  { symbol: "LiNEAR", token: LINEAR, quote: USDC, swap: LINEAR, history: null },
];
export const marketOf = (symbol: string) => MARKETS.find((m) => m.symbol.toLowerCase() === symbol.toLowerCase());
export const pairsOf = (m: MarketDef) => MARKETS.filter((x) => x.token.symbol === m.token.symbol);
export const LISTED = MARKETS;

// bigint-safe: rounds down to maxFrac digits, groups thousands
export function fmtUnits(raw: bigint | string, decimals: number, maxFrac = 4): string {
  const v = BigInt(raw);
  const neg = v < 0n;
  const s = (neg ? -v : v).toString().padStart(decimals + 1, "0");
  const int = s.slice(0, s.length - decimals);
  const f = s.slice(s.length - decimals).slice(0, maxFrac).replace(/0+$/, "");
  return `${neg ? "-" : ""}${int.replace(/\B(?=(\d{3})+(?!\d))/g, ",")}${f ? `.${f}` : ""}`;
}

// atoms → plain decimal string (no grouping), for inputs
export const formatUnits = (raw: bigint | string, decimals: number) => fmtUnits(raw, decimals, decimals).replace(/,/g, "");

// user input → raw; null when not a valid non-negative amount
export function parseAmount(input: string, decimals: number): bigint | null {
  const s = input.trim();
  if (!/^\d*\.?\d*$/.test(s) || s === "" || s === ".") return null;
  const [int = "", frac = ""] = s.split(".");
  if (frac.length > decimals) return null;
  return BigInt((int || "0") + frac.padEnd(decimals, "0"));
}
