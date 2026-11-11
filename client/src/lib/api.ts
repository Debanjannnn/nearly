import { API_URL } from "./config";

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

export type AuthInput = { account_id: string; public_key: string; signature: string; nonce: string };
// account_id = the user's sub-account created by the factory
export type UserAccount = { account_id: string };

export const postAuth = (b: AuthInput) =>
  request<{ token: string }>(`${API_URL}/auth`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(b),
  });
export const createAccount = () => authed<UserAccount>("/me/account", "POST");

// keyed by deployments token key (usdc, wnear); balance is raw
export type Holdings = { account_id: string; tokens: Record<string, { token: string; balance: string }> };
export const getHoldings = () => authed<Holdings>("/me/holdings");

export const shortId = (s: string) => (s.length > 20 ? `${s.slice(0, 8)}…${s.slice(-8)}` : s);

export const errMsg = (e: unknown): string => (e instanceof Error ? e.message : String(e));

const COPY: Record<string, string> = {
  unauthorized: "Session expired, sign in again.",
  invalid_signature: "Signature check failed, sign in again.",
  not_found: "Not found.",
};

// user-facing text: known code → copy (+ detail), else raw message
export const errText = (e: unknown): string => {
  const m = errMsg(e);
  const d = e instanceof ApiError && e.detail ? ` (${e.detail})` : "";
  return (COPY[m] ?? m) + d;
};
