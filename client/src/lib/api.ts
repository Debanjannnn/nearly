import { API_URL, HISTORY_URL, MARKET_DATA_URL, RPC_URL, TOTAL_BPS, WNEAR_POOL } from "./config";
import { decimalsOf, formatUnits, keyOf, NEAR, parseAmount, symbolOf, USDC } from "./tokens";
import type {
  AccountReply,
  Accepted,
  Backtest,
  BacktestWire,
  Basket,
  BasketDef,
  Execution,
  Holdings,
  HoldingsWire,
  Leg,
  LegSource,
  Me,
  Position,
  PricePoint,
  Prices,
  Quote,
  Rates,
  Rules,
  Schedule,
  ScheduleInput,
  Sleeve,
  Strategy,
  StrategyInput,
} from "./types";

// status 0 = network error; message = backend snake_code when it sent one; detail = planner/history detail.
export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly detail?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export async function request<T>(url: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, init);
  } catch (e) {
    throw new ApiError(0, `cannot reach ${url} (${e instanceof Error ? e.message : "network error"})`);
  }
  const text = await res.text();
  let body: unknown = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  if (!res.ok) {
    const { error: code, detail } = (body ?? {}) as { error?: unknown; detail?: unknown };
    throw new ApiError(
      res.status,
      typeof code === "string" ? code : `request failed (${res.status})`,
      typeof detail === "string" ? detail : undefined,
    );
  }
  return body as T;
}

// session bearer (NEP-413 login); set by SessionProvider
let token: string | null = null;
export const setToken = (t: string | null) => {
  token = t;
};

export async function authed<T>(path: string, method = "GET", body?: unknown): Promise<T> {
  if (!token) throw new ApiError(401, "unauthorized");
  return request<T>(API_URL + path, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

const q = (params: Record<string, string | number | undefined>) => {
  const s = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "") s.set(k, String(v));
  const out = s.toString();
  return out ? `?${out}` : "";
};

const json = (body: unknown): RequestInit => ({
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

// basket legs ↔ view sleeves; venue = ProtocolIcon / sources.ts key
const VENUE: Record<LegSource, string> = { lending: "rhea-lend", staking: "linear", hold: "wallet" };

export function toStrategy(b: Basket): Strategy {
  const sleeves: Sleeve[] = [];
  for (const l of b.definition.legs) {
    const asset = symbolOf(l.token);
    let sl = sleeves.find((x) => x.asset === asset);
    if (!sl) sleeves.push((sl = { asset, weightBps: 0, sources: [] }));
    sl.weightBps += l.weight_bps;
    sl.sources.push({ kind: l.source, venue: VENUE[l.source], weightBps: l.weight_bps });
  }
  // source weights: of the whole basket → of their sleeve
  for (const sl of sleeves) for (const s of sl.sources) s.weightBps = Math.round((s.weightBps * TOTAL_BPS) / sl.weightBps);
  return { id: b.id, creator: b.creator, name: b.name, sleeves, rules: { driftBps: b.definition.drift_bps }, public: b.public, createdAt: b.created_at, definition: b.definition };
}

// sleeves → planner legs (bps of the whole basket, exact sum); lp has no backend venue and is dropped
export function toDef(sleeves: Sleeve[], rules: Rules): BasketDef {
  const legs: Leg[] = sleeves.flatMap((sl) =>
    sl.sources
      .filter((s): s is typeof s & { kind: LegSource } => s.kind !== "lp")
      .map((s) => {
        const token = keyOf(sl.asset);
        return { token, source: s.kind, weight_bps: Math.round((sl.weightBps * s.weightBps) / TOTAL_BPS), ...(token === "usdc" ? {} : { pool_id: WNEAR_POOL }) };
      }),
  );
  const sum = legs.reduce((a, l) => a + l.weight_bps, 0);
  if (legs.length && sum !== TOTAL_BPS) legs.reduce((m, l) => (l.weight_bps > m.weight_bps ? l : m)).weight_bps += TOTAL_BPS - sum;
  return { legs, ...(rules.driftBps ? { drift_bps: rules.driftBps } : {}) };
}

// api — auth (NEP-413 signed message → bearer)
export type AuthInput = { account_id: string; public_key: string; signature: string; nonce: string };
export const getAuthNonce = () => request<{ nonce: string }>(`${API_URL}/auth/nonce`);
export const postAuth = (b: AuthInput) => request<{ token: string }>(`${API_URL}/auth`, json(b));

// api — public; the bearer lets a creator read their private basket
export const getStrategies = () => request<Basket[]>(`${API_URL}/baskets`).then((l) => l.map(toStrategy));
export const getStrategy = (id: string) =>
  request<Basket>(`${API_URL}/baskets/${id}`, token ? { headers: { Authorization: `Bearer ${token}` } } : undefined).then(toStrategy);

// api — /me
export const getMe = () => authed<Me>("/me");
export const createAccount = () => authed<AccountReply>("/me/account", "POST");
export const postRevoke = () => authed<{ key_status: "revoked"; schedules_deactivated: number }>("/me/revoke", "POST");
export const getMyStrategies = () => authed<Basket[]>("/me/baskets").then((l) => l.map(toStrategy));
export const createStrategy = (s: StrategyInput) =>
  authed<Basket>("/me/baskets", "POST", { name: s.name, definition: toDef(s.sleeves, s.rules), public: s.public }).then(toStrategy);
// amountUsdc = display decimal string
export const deployStrategy = (id: string, amountUsdc: string) => {
  const raw = parseAmount(amountUsdc, USDC.decimals);
  if (!raw) return Promise.reject(new ApiError(400, "invalid_amount"));
  return authed<Accepted>(`/me/baskets/${id}/deploy`, "POST", { amount: raw.toString() });
};
export const getPositions = () => authed<Position[]>("/me/positions");
// bps 1..10000; on a stalled sell the api resumes it instead
export const withdrawPosition = (id: string, bps = TOTAL_BPS) => authed<Accepted>(`/me/positions/${id}/withdraw`, "POST", { bps });
export const getExecutions = () => authed<Execution[]>("/me/executions");
export const getExecution = (id: string) => authed<Execution>(`/me/executions/${id}`);
export const getSchedules = () => authed<Schedule[]>("/me/schedules");
export const createSchedule = (s: ScheduleInput) => authed<Schedule>("/me/schedules", "POST", s);
export const patchSchedule = (id: string, b: { every_seconds?: number; active?: boolean }) => authed<Schedule>(`/me/schedules/${id}`, "PATCH", b);
export const deleteSchedule = (id: string) => authed<{ deleted: boolean }>(`/me/schedules/${id}`, "DELETE");

// sub-account NEAR (gas) straight from RPC; the api lists FTs only
async function nearBalance(account: string): Promise<string> {
  const r = await request<{ result?: { amount: string } }>(
    RPC_URL,
    json({ jsonrpc: "2.0", id: "1", method: "query", params: { request_type: "view_account", finality: "final", account_id: account } }),
  );
  return r.result?.amount ?? "0";
}

// account contract ledger: what `withdraw` can actually pay out (U128 string)
export async function getLedgerBalance(account: string, token: string): Promise<string> {
  const args = btoa(JSON.stringify({ token }));
  const r = await request<{ result?: { result: number[] }; error?: unknown }>(
    RPC_URL,
    json({ jsonrpc: "2.0", id: "1", method: "query", params: { request_type: "call_function", finality: "final", account_id: account, method_name: "get_balance", args_base64: args } }),
  );
  if (!r.result) throw new Error("get_balance failed");
  return JSON.parse(new TextDecoder().decode(new Uint8Array(r.result.result))) as string;
}

export async function getHoldings(): Promise<Holdings> {
  const h = await authed<HoldingsWire>("/me/holdings");
  const near = await nearBalance(h.account_id).catch(() => null);
  const tokens = Object.entries(h.tokens).map(([k, t]) => ({ symbol: symbolOf(k), address: t.token, raw: t.balance, amount: formatUnits(t.balance, decimalsOf(k)) }));
  if (near !== null) tokens.unshift({ symbol: NEAR.symbol, address: NEAR.address, raw: near, amount: formatUnits(near, NEAR.decimals) });
  return { wallet: h.account_id, tokens };
}

// market-data; 503 "unavailable" / "stale" when the feed is down
export const getRates = () => request<Rates>(`${MARKET_DATA_URL}/rates`);
export const getPrices = () => request<Prices>(`${MARKET_DATA_URL}/prices`);
// Rhea DEX pool quote; amountIn = atoms
export const getQuote = (tokenIn: string, tokenOut: string, amountIn: bigint, slippageBps = 50, poolId?: number) =>
  request<Quote>(`${MARKET_DATA_URL}/quote${q({ token_in: tokenIn, token_out: tokenOut, amount_in: amountIn.toString(), slippage_bps: slippageBps, pool_id: poolId })}`);

// history; daily venue APY (fraction) per source + token key
export const getRateSeries = (source: string, token: string, from?: string) =>
  request<{ data: { day: string; apy: string }[] }>(`${HISTORY_URL}/series/rates${q({ source, token, from })}`).then((r) => r.data);

// history; daily USD closes per token key ("wnear") or contract id
export const getPriceSeries = (tokenKey: string, from?: string) =>
  request<{ token: string; data: PricePoint[] }>(`${HISTORY_URL}/series/prices${q({ token: tokenKey, from })}`).then((r) => r.data);

// amount = USD decimal string
export const runBacktest = (basket: BasketDef, amount = "1000", from?: string) =>
  request<BacktestWire>(`${HISTORY_URL}/backtest`, json({ basket, amount, from })).then(
    (b): Backtest => ({
      from: b.from,
      to: b.to,
      days: b.days,
      coverage: b.coverage,
      initialUsd: b.initial_usd,
      finalUsd: b.final_usd,
      apyUsd: b.apy,
      returnUsd: b.total_return,
      maxDrawdownUsd: b.max_drawdown,
      rebalances: b.rebalances,
      costsUsd: b.costs_usd,
      curve: b.curve,
    }),
  );

// display formatters (numbers already in display units)
export const fmtUsdCompact = (n: number) =>
  n.toLocaleString("en-US", { style: "currency", currency: "USD", notation: "compact", maximumFractionDigits: 1 });

// sub-cent values keep 2 significant digits instead of rounding to $0.00
export const fmtUsd = (n: number) =>
  n !== 0 && Math.abs(n) < 0.01
    ? n.toLocaleString("en-US", { style: "currency", currency: "USD", maximumSignificantDigits: 2 })
    : n.toLocaleString("en-US", { style: "currency", currency: "USD", maximumFractionDigits: Math.abs(n) < 1 ? 4 : 2 });

export const fmtNum = (n: number, maxFrac = 4) => n.toLocaleString("en-US", { maximumFractionDigits: maxFrac });

export const fmtBps = (bps: number) => `${(bps / 100).toFixed(2)}%`;

export const shortHash = (h: string) => (h.length > 20 ? `${h.slice(0, 8)}…${h.slice(-6)}` : h);

export const fmtTime = (iso: string | number) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? String(iso) : d.toLocaleString();
};

export const errMsg = (e: unknown): string => (e instanceof Error ? e.message : String(e));

const COPY: Record<string, string> = {
  unauthorized: "Session expired, sign in again.",
  invalid_signature: "Signature check failed, sign in again.",
  invalid_amount: "Invalid amount.",
  not_found: "Not found.",
  account_not_ready: "Create your account first.",
  executor_revoked: "Automation key revoked. Buys, sells and schedules are off.",
  executor_key_active: "Executor key still on chain; the revoke tx has not landed yet.",
  position_not_open: "Position isn't open.",
  execution_running: "Another run on this position is still going.",
  rpc_unavailable: "NEAR RPC unavailable, try again shortly.",
  factory_not_deployed: "Account factory is not deployed on this network.",
  invalid_schedule: "Schedule is invalid (interval 1 min to 1 year, amount above 0).",
  invalid_basket: "Basket is invalid.",
  unknown_pool: "No Rhea pool for this pair.",
  quote_unavailable: "Pool quote unavailable, try again shortly.",
  insufficient_liquidity: "Pool can't fill this size right now.",
  amount_too_small: "Amount too small: the staking leg needs at least 0.01 NEAR.",
  nothing_to_withdraw: "Nothing left to withdraw.",
  executor_unavailable: "Executor unreachable; nothing was sent.",
  stale: "Market data is stale, try again shortly.",
  unavailable: "Market data unavailable.",
  no_data: "Not enough history for this basket.",
};

// user-facing text: known code → copy (+ detail), else raw message
export const errText = (e: unknown): string => {
  const m = errMsg(e);
  const d = e instanceof ApiError && e.detail ? ` (${e.detail})` : "";
  return (COPY[m] ?? m) + d;
};
