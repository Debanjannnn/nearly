"use client";

import type { NearConnector } from "@hot-labs/near-connect";
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { ApiError, createAccount, errText, postAuth, setToken } from "./api";
import { NETWORK, RPC_URL } from "./config";

// shared auth contract with the api (NEP-413)
const LOGIN = { message: "nearly login", recipient: "nearly" } as const;
const KEY = "nearly.session";

type Saved = { accountId: string; token: string | null; account: string | null };

export type Session = {
  ready: boolean;
  accountId: string | null; // connected wallet account
  account: string | null; // user's sub-account from the factory
  authenticated: boolean;
  pending: boolean;
  error: string | null;
  login: () => Promise<void>;
  logout: () => Promise<void>;
};

const Ctx = createContext<Session | null>(null);

export function useSession(): Session {
  const s = useContext(Ctx);
  if (!s) throw new Error("useSession outside SessionProvider");
  return s;
}

const load = (): Saved | null => {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? "null") as Saved | null;
  } catch {
    return null;
  }
};

const persist = (s: Saved | null) => {
  try {
    if (s) localStorage.setItem(KEY, JSON.stringify(s));
    else localStorage.removeItem(KEY);
  } catch {}
};

const b64 = (b: Uint8Array) => btoa(String.fromCharCode(...b));

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const connector = useRef<NearConnector | null>(null);
  const [ready, setReady] = useState(false);
  const [s, setS] = useState<Saved | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const save = useCallback((next: Saved | null) => {
    setToken(next?.token ?? null);
    persist(next);
    setS(next);
  }, []);

  // connector touches window/IndexedDB, so it is built client-side only
  useEffect(() => {
    let live = true;
    void import("@hot-labs/near-connect").then(async ({ NearConnector }) => {
      const c = new NearConnector({
        network: NETWORK,
        providers: { [NETWORK]: [RPC_URL] },
        features: { signMessage: true, [NETWORK]: true },
        footerBranding: null,
      });
      c.on("wallet:signOut", () => save(null));
      connector.current = c;
      const id = await c.getConnectedWallet().then(
        (r) => r.accounts[0]?.accountId ?? null,
        () => null,
      );
      if (!live) return;
      const saved = load();
      save(id ? (saved?.accountId === id ? saved : { accountId: id, token: null, account: null }) : null);
      setReady(true);
    });
    return () => {
      live = false;
    };
  }, [save]);

  // idempotent: connects, signs in, creates the sub-account; skips steps already done
  const login = useCallback(async () => {
    const c = connector.current;
    if (!c) return;
    setPending(true);
    setError(null);
    let cur = s;
    try {
      const wallet = await c.wallet().catch(() => c.connect());
      if (!cur?.token) {
        const nonce = crypto.getRandomValues(new Uint8Array(32));
        const signed = await wallet.signMessage({ ...LOGIN, nonce });
        const { token } = await postAuth({
          account_id: signed.accountId,
          public_key: signed.publicKey,
          signature: signed.signature,
          nonce: b64(nonce),
        });
        cur = { accountId: signed.accountId, token, account: null };
        save(cur);
      }
      if (!cur.account) {
        const { account_id } = await createAccount();
        save({ ...cur, account: account_id });
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && cur) save({ ...cur, token: null });
      setError(errText(e));
    } finally {
      setPending(false);
    }
  }, [s, save]);

  const logout = useCallback(async () => {
    await connector.current?.disconnect().catch(() => {});
    save(null);
  }, [save]);

  return (
    <Ctx.Provider
      value={{
        ready,
        accountId: s?.accountId ?? null,
        account: s?.account ?? null,
        authenticated: !!s?.token,
        pending,
        error,
        login,
        logout,
      }}
    >
      {children}
    </Ctx.Provider>
  );
}
