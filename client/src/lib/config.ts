import mainnet from "../../../contracts/deployments/mainnet.json";
import testnet from "../../../contracts/deployments/testnet.json";

export type Network = "testnet" | "mainnet";

// shared networks file; fields stay optional until contracts are deployed
export type Deployments = {
  root?: string;
  factory?: string;
  treasury?: string;
  tokens?: { usdc?: string; wnear?: string };
  venues?: { lending?: string; dex?: string; staking?: string };
};

// NEXT_PUBLIC_* is baked in at build time; restart dev server after changing.
export const NETWORK: Network = process.env.NEXT_PUBLIC_NETWORK === "mainnet" ? "mainnet" : "testnet";
export const RPC_URL = process.env.NEXT_PUBLIC_RPC_URL ?? `https://rpc.${NETWORK}.near.org`;
export const API_URL = `${process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080"}/api/v1`;
// live rates/prices topics come from market-data
export const WS_URL = process.env.NEXT_PUBLIC_WS_URL ?? "ws://localhost:8082/api/v1/ws";
export const MARKET_DATA_URL = `${process.env.NEXT_PUBLIC_MARKET_DATA_URL ?? "http://localhost:8082"}/api/v1`;
export const HISTORY_URL = `${process.env.NEXT_PUBLIC_HISTORY_URL ?? "http://localhost:8087"}/api/v1`;

export const DEPLOYMENTS: Deployments = NETWORK === "mainnet" ? mainnet : testnet;

// Rhea USDC/wNEAR pool (docs/STACK.md); a wnear leg is bought on it
export const WNEAR_POOL = NETWORK === "mainnet" ? 4512 : 1900;
export const FEE_BPS = 40; // open fee to treasury (planner FeeBps)
export const TOTAL_BPS = 10_000;

const EXPLORER = NETWORK === "mainnet" ? "https://nearblocks.io" : "https://testnet.nearblocks.io";
export const txUrl = (hash: string) => `${EXPLORER}/txns/${hash}`;
export const accountUrl = (id: string) => `${EXPLORER}/address/${id}`;
