import { API_URL, DEPLOYMENTS } from "./config";
import { authed, request } from "./api";

// NEAR Intents 1Click via the nearly api; the api fills recipient (deposit) and refund (withdraw) itself.
export type IntentToken = { assetId: string; decimals: number; blockchain: string; symbol: string; price: number; contractAddress?: string };

export type IntentQuote = {
  quote: {
    timeEstimate: number;
    amountIn: string;
    amountInFormatted: string;
    amountInUsd: string;
    amountOut: string;
    amountOutFormatted: string;
    amountOutUsd: string;
    minAmountOut: string;
    depositAddress?: string;
    depositMemo?: string;
    deadline?: string;
  };
};

export type IntentStatus = "PENDING_DEPOSIT" | "KNOWN_DEPOSIT_TX" | "INCOMPLETE_DEPOSIT" | "PROCESSING" | "SUCCESS" | "REFUNDED" | "FAILED";
type TxRef = string | { hash: string; explorerUrl?: string };
export type Intent = {
  deposit_address: string;
  deposit_memo?: string;
  kind: "deposit" | "withdraw";
  status: IntentStatus;
  deposit_tx: string | null;
  swap_details: { amountOutFormatted?: string; refundReason?: string; destinationChainTxHashes?: TxRef[] } | null;
};

export const USDC_ASSET = `nep141:${DEPLOYMENTS.tokens?.usdc ?? ""}`;

export const getIntentTokens = () => request<IntentToken[]>(`${API_URL}/intents/tokens`);

type QuoteIn = { origin_asset: string; amount: string; swap_type: "EXACT_INPUT" | "EXACT_OUTPUT"; dry: boolean };
export const depositQuote = (b: QuoteIn & { refund_to: string }) => authed<IntentQuote>("/me/intents/deposit-quote", "POST", b);
export const withdrawQuote = (b: QuoteIn & { destination_asset: string; recipient: string }) =>
  authed<IntentQuote>("/me/intents/withdraw-quote", "POST", b);

export const submitIntent = (deposit_address: string, deposit_memo: string | undefined, tx_hash: string) =>
  authed<Intent>("/me/intents/submit", "POST", { deposit_address, deposit_memo: deposit_memo ?? "", tx_hash });

// api refreshes non-terminal swaps from 1Click on each call
export const intentStatus = (addr: string) =>
  authed<Intent[]>(`/me/intents/status?deposit_address=${encodeURIComponent(addr)}`).then((l) => l[0] ?? null);

export const isDone = (s: IntentStatus | undefined) => s === "SUCCESS" || s === "REFUNDED" || s === "FAILED";
export const txHash = (t: TxRef) => (typeof t === "string" ? t : t.hash);
export const txLink = (t: TxRef) => (typeof t === "string" ? null : (t.explorerUrl ?? null));

// one open swap per kind per wallet, so it survives reload until it settles or expires
const key = (kind: string) => `nearly.intent.${kind}`;
export function savePending<T extends { owner: string }>(kind: string, d: T | null) {
  try {
    if (d) localStorage.setItem(key(kind), JSON.stringify(d));
    else localStorage.removeItem(key(kind));
  } catch {}
}
export function loadPending<T extends { owner: string; q: IntentQuote; sent?: boolean }>(kind: string, owner: string | null): T | null {
  try {
    const d = JSON.parse(localStorage.getItem(key(kind)) ?? "null") as T | null;
    const expired = !d?.sent && !!d?.q.quote.deadline && new Date(d.q.quote.deadline).getTime() < Date.now();
    return d && d.owner === owner && !expired ? d : null;
  } catch {
    return null;
  }
}

// chains where a 0x address is valid
export const EVM = new Set(["eth", "base", "arb", "op", "pol", "bsc", "avax", "gnosis", "bera", "scroll", "xlayer", "plasma", "abs", "hood", "adi"]);
export const validAddress = (chain: string, a: string) => (EVM.has(chain) ? /^0x[0-9a-fA-F]{40}$/.test(a) : a.length >= 16 && !/\s/.test(a));

const CHAIN: Record<string, string> = {
  eth: "Ethereum",
  base: "Base",
  arb: "Arbitrum",
  op: "Optimism",
  pol: "Polygon",
  bsc: "BNB Chain",
  avax: "Avalanche",
  sol: "Solana",
  btc: "Bitcoin",
  tron: "Tron",
  near: "NEAR",
  sui: "Sui",
  ton: "TON",
  aptos: "Aptos",
  gnosis: "Gnosis",
  bera: "Berachain",
  scroll: "Scroll",
  xlayer: "X Layer",
  plasma: "Plasma",
  abs: "Abstract",
  hood: "Robinhood",
  stellar: "Stellar",
  xrp: "XRP Ledger",
  doge: "Dogecoin",
  ltc: "Litecoin",
  bch: "Bitcoin Cash",
  zec: "Zcash",
  cardano: "Cardano",
  starknet: "Starknet",
  hypercore: "Hyperliquid",
};
export const chainName = (c: string) => CHAIN[c] ?? c.toUpperCase();
// popular first, the rest alphabetical
const ORDER = ["eth", "base", "arb", "sol", "btc", "op", "pol", "bsc", "avax", "tron"];
export const chainRank = (c: string) => (ORDER.includes(c) ? ORDER.indexOf(c) : ORDER.length);
export const sortChains = (tokens: IntentToken[]) =>
  [...new Set(tokens.map((t) => t.blockchain))]
    .filter((c) => c !== "near")
    .sort((a, b) => chainRank(a) - chainRank(b) || chainName(a).localeCompare(chainName(b)));

// USDC, USDT, native coin, other dollars, majors, the rest
const tokenRank = (t: IntentToken) =>
  t.symbol === "USDC" ? 0 : /^USDT/.test(t.symbol) ? 1 : !t.contractAddress ? 2 : /^USD|DAI$/.test(t.symbol) ? 3 : /^W?(ETH|BTC)$|^cbBTC$/.test(t.symbol) ? 4 : 5;
export const tokensOn = (tokens: IntentToken[], chain: string) =>
  tokens.filter((t) => t.blockchain === chain).sort((a, b) => tokenRank(a) - tokenRank(b) || a.symbol.localeCompare(b.symbol));

// 1Click tokens carry no logo: Trust Wallet per chain, then coincap by symbol
const TW: Record<string, string> = {
  eth: "ethereum", base: "base", arb: "arbitrum", op: "optimism", pol: "polygon", bsc: "smartchain", avax: "avalanchec", gnosis: "xdai",
  sol: "solana", tron: "tron", btc: "bitcoin", doge: "doge", xrp: "xrp", sui: "sui", cardano: "cardano", ltc: "litecoin",
  bch: "bitcoincash", zec: "zcash", stellar: "stellar", aptos: "aptos", near: "near", ton: "ton", starknet: "starknet",
};
export function tokenLogos(t: IntentToken): string[] {
  const out: string[] = [];
  const chain = TW[t.blockchain];
  const a = t.contractAddress;
  // shortcut: Trust Wallet wants checksummed 0x paths; EVM contracts fall back to coincap
  if (chain && !a) out.push(`https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/${chain}/info/logo.png`);
  else if (chain && a && !a.startsWith("0x")) out.push(`https://raw.githubusercontent.com/trustwallet/assets/master/blockchains/${chain}/assets/${a}/logo.png`);
  for (const sym of new Set([t.symbol, t.symbol.replace(/^(w|st)(?=[A-Z])/, "")]))
    out.push(`https://assets.coincap.io/assets/icons/${sym.toLowerCase().replace(/[^a-z0-9]/g, "")}@2x.png`);
  return out;
}

export const eta = (s: number) => (s < 90 ? `${Math.round(s)}s` : `${Math.round(s / 60)} min`);
export const fmtUsd = (v: number) => v.toLocaleString("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 2 });
