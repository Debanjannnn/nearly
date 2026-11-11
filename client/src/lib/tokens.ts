import { DEPLOYMENTS } from "./config";

// id = NEP-141 contract ("near" for the native token)
export type Token = { id: string; symbol: string; name: string; decimals: number };

const META: Record<string, Omit<Token, "id">> = {
  usdc: { symbol: "USDC", name: "USD Coin", decimals: 6 },
  wnear: { symbol: "wNEAR", name: "Wrapped NEAR", decimals: 24 },
};

export const NEAR: Token = { id: "near", symbol: "NEAR", name: "NEAR", decimals: 24 };

export const TOKENS: Token[] = [
  NEAR,
  ...Object.entries(DEPLOYMENTS.tokens ?? {}).flatMap(([key, id]) => (id && META[key] ? [{ id, ...META[key] }] : [])),
];

export const tokenById = (id: string) => TOKENS.find((t) => t.id === id);
export const tokenBySymbol = (symbol: string) => TOKENS.find((t) => t.symbol === symbol);

// deployments token key ("usdc" | "wnear") helpers; baskets and holdings use keys
export const KEYS = Object.keys(META);
export const tokenIdOf = (key: string) => DEPLOYMENTS.tokens?.[key as "usdc" | "wnear"] ?? "";
export const symbolOf = (key: string) => META[key]?.symbol ?? key.toUpperCase();
export const decimalsOf = (key: string) => META[key]?.decimals ?? 24;
// atoms → float units, for USD display math only
export const toUnits = (raw: string | bigint, decimals: number) => Number(BigInt(raw)) / 10 ** decimals;

// bigint-safe: rounds down to maxFrac digits, groups thousands
export function fmtUnits(raw: bigint | string, decimals: number, maxFrac = 4): string {
  const v = BigInt(raw);
  const neg = v < 0n;
  const s = (neg ? -v : v).toString().padStart(decimals + 1, "0");
  const int = s.slice(0, s.length - decimals);
  const f = s.slice(s.length - decimals).slice(0, maxFrac).replace(/0+$/, "");
  return `${neg ? "-" : ""}${int.replace(/\B(?=(\d{3})+(?!\d))/g, ",")}${f ? `.${f}` : ""}`;
}

// user input → raw; null when not a valid non-negative amount
export function parseAmount(input: string, decimals: number): bigint | null {
  const s = input.trim();
  if (!/^\d*\.?\d*$/.test(s) || s === "" || s === ".") return null;
  const [int = "", frac = ""] = s.split(".");
  if (frac.length > decimals) return null;
  return BigInt((int || "0") + frac.padEnd(decimals, "0"));
}
