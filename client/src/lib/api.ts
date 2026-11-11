import { API_URL, HISTORY_URL, MARKET_DATA_URL } from "./config";
import type {
  AccountReply,
  Accepted,
  Backtest,
  Basket,
  BasketDef,
  Execution,
  Holdings,
  Me,
  Position,
  PricePoint,
  Prices,
  Quote,
  Rates,
  Schedule,
  ScheduleInput,
} from "./types";

// status 0 = network error; message = backend snake_code when it sent one
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

// session bearer; set by SessionProvider
let token: string | null = null;
export const setToken = (t: string | null) => {
  token = t;
};

async function authed<T>(path: string, method = "GET", body?: unknown): Promise<T> {
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

const json = (body: unknown): RequestInit => ({
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

const qs = (p: Record<string, string | number | undefined>) =>
  new URLSearchParams(Object.entries(p).flatMap(([k, v]) => (v === undefined || v === "" ? [] : [[k, String(v)]]))).toString();

export type AuthInput = { account_id: string; public_key: string; signature: string; nonce: string };
export const postAuth = (b: AuthInput) => request<{ token: string }>(`${API_URL}/auth`, json(b));

// api, public; the bearer lets a creator read their private basket
export const getBaskets = () => request<Basket[]>(`${API_URL}/baskets`);
export const getBasket = (id: string) =>
  request<Basket>(`${API_URL}/baskets/${id}`, token ? { headers: { Authorization: `Bearer ${token}` } } : undefined);

// api, /me
export const getMe = () => authed<Me>("/me");
export const createAccount = () => authed<AccountReply>("/me/account", "POST");
export const postRevoke = () => authed<{ key_status: "revoked"; schedules_deactivated: number }>("/me/revoke", "POST");
export const getHoldings = () => authed<Holdings>("/me/holdings");
export const getMyBaskets = () => authed<Basket[]>("/me/baskets");
export const createBasket = (name: string, definition: BasketDef, isPublic: boolean) =>
  authed<Basket>("/me/baskets", "POST", { name, definition, public: isPublic });
// amount = usdc atoms
export const deployBasket = (id: string, amount: string) => authed<Accepted>(`/me/baskets/${id}/deploy`, "POST", { amount });
export const getPositions = () => authed<Position[]>("/me/positions");
// bps 1..10000; on a stalled sell the api resumes it instead
export const withdrawPosition = (id: string, bps = 10_000) => authed<Accepted>(`/me/positions/${id}/withdraw`, "POST", { bps });
export const getExecutions = () => authed<Execution[]>("/me/executions");
export const getExecution = (id: string) => authed<Execution>(`/me/executions/${id}`);
export const getSchedules = () => authed<Schedule[]>("/me/schedules");
export const createSchedule = (s: ScheduleInput) => authed<Schedule>("/me/schedules", "POST", s);
export const patchSchedule = (id: string, b: { every_seconds?: number; active?: boolean }) =>
  authed<Schedule>(`/me/schedules/${id}`, "PATCH", b);
export const deleteSchedule = (id: string) => authed<{ deleted: boolean }>(`/me/schedules/${id}`, "DELETE");

// market-data; 503 "unavailable" / "stale" when the feed is down
export const getRates = () => request<Rates>(`${MARKET_DATA_URL}/rates`);
export const getPrices = () => request<Prices>(`${MARKET_DATA_URL}/prices`);
export const getQuote = (tokenIn: string, tokenOut: string, amountIn: string, poolId?: number) =>
  request<Quote>(`${MARKET_DATA_URL}/quote?${qs({ token_in: tokenIn, token_out: tokenOut, amount_in: amountIn, pool_id: poolId })}`);

// history; amount = USD decimal string
export const runBacktest = (basket: BasketDef, amount = "1000", from?: string) =>
  request<Backtest>(`${HISTORY_URL}/backtest`, json({ basket, amount, from }));
export const getPriceSeries = (token: string, from?: string) =>
  request<{ token: string; data: PricePoint[] }>(`${HISTORY_URL}/series/prices?${qs({ token, from })}`).then((r) => r.data);

export const shortId = (s: string) => (s.length > 20 ? `${s.slice(0, 8)}…${s.slice(-8)}` : s);

export const errMsg = (e: unknown): string => (e instanceof Error ? e.message : String(e));

const COPY: Record<string, string> = {
  unauthorized: "Session expired, sign in again.",
  invalid_signature: "Signature check failed, sign in again.",
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
  quote_unavailable: "Pool quote unavailable, try again shortly.",
  insufficient_liquidity: "Pool can't fill this size right now.",
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

// display formatters (numbers already in display units)
export const fmtUsd = (n: number) =>
  n !== 0 && Math.abs(n) < 0.01
    ? n.toLocaleString("en-US", { style: "currency", currency: "USD", maximumSignificantDigits: 2 })
    : n.toLocaleString("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 2 });
export const fmtUsdCompact = (n: number) =>
  n.toLocaleString("en-US", { style: "currency", currency: "USD", notation: "compact", maximumFractionDigits: 1 });
export const fmtNum = (n: number, maxFrac = 4) => n.toLocaleString("en-US", { maximumFractionDigits: maxFrac });
export const pct = (v: number | string | null | undefined, d = 2) =>
  v === null || v === undefined || v === "" ? "-" : `${(Number(v) * 100).toFixed(d)}%`;
export const fmtTime = (iso: string | number) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? String(iso) : d.toLocaleString();
};
