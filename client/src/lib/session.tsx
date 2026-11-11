"use client";

import type { NearConnector, SignAndSendTransactionsParams } from "@hot-labs/near-connect";
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { ApiError, createAccount, errText, getMe, postAuth, postRevoke, setToken } from "./api";
import { NETWORK, RPC_URL } from "./config";
import type { KeyStatus } from "./types";

// shared auth contract with the api (NEP-413)
const LOGIN = { message: "nearly login", recipient: "nearly" } as const;
const KEY = "nearly.session";

type Saved = { accountId: string; token: string | null; account: string | null };

export type Session = {
  ready: boolean;
  accountId: string | null; // connected wallet account
  account: string | null; // user's sub-account from the factory
  authenticated: boolean;
  keyStatus: KeyStatus | null; // executor function-call key on the sub-account
  pending: boolean;
  error: string | null;
  login: () => Promise<void>;
  logout: () => Promise<void>;
  revoke: () => Promise<void>;
  // signs with the connected wallet; resolves tx hashes, throws if any tx failed
  send: (txs: SignAndSendTransactionsParams["transactions"]) => Promise<string[]>;
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
  const [keyStatus, setKeyStatus] = useState<KeyStatus | null>(null);

  const save = useCallback((next: Saved | null) => {
    setToken(next?.token ?? null);
    persist(next);
    setS(next);
    if (!next?.token) setKeyStatus(null);
  }, []);

  // key status lives on the api (revoke can happen from another device)
  const token = s?.token ?? null;
  useEffect(() => {
    if (!token) return;
    let live = true;
    getMe().then(
      (me) => live && setKeyStatus(me.key_status),
      () => {},
    );
    return () => {
      live = false;
    };
  }, [token]);

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

  const send = useCallback(async (transactions: SignAndSendTransactionsParams["transactions"]) => {
    const c = connector.current;
    if (!c) throw new Error("wallet not ready");
    const out = await (await c.wallet()).signAndSendTransactions({ transactions });
    const bad = out.find((o) => typeof o.status === "object" && "Failure" in o.status);
    if (bad) throw new Error(`transaction ${bad.transaction_outcome.id} failed`);
    return out.map((o) => o.transaction_outcome.id);
  }, []);

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
        let r = await createAccount();
        // api has no relayer: the owner signs the factory create_account tx, then we poll until the key lands
        if (r.tx) await send([r.tx]);
        for (let i = 0; !r.account_id && i < 20; i++) {
          await new Promise((ok) => setTimeout(ok, 1500));
          r = await createAccount();
        }
        if (!r.account_id) throw new Error("Account still pending on chain, try again shortly.");
        save({ ...cur, account: r.account_id });
        setKeyStatus(r.key_status);
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && cur) save({ ...cur, token: null });
      setError(errText(e));
    } finally {
      setPending(false);
    }
  }, [s, save, send]);

  const logout = useCallback(async () => {
    await connector.current?.disconnect().catch(() => {});
    save(null);
  }, [save]);

  // owner signs revoke_executor (1 yocto, full-access key), then the api turns every schedule off
  const revoke = useCallback(async () => {
    const sub = s?.account;
    if (!sub) return;
    setPending(true);
    setError(null);
    try {
      // a failed tx may mean it was revoked on chain already; the api check below decides
      let txErr: unknown = null;
      await send([
        {
          receiverId: sub,
          actions: [{ type: "FunctionCall", params: { methodName: "revoke_executor", args: {}, gas: "30000000000000", deposit: "1" } }],
        },
      ]).catch((e) => (txErr = e));
      // key deletion is a receipt after the call; retry while the api still sees it
      for (let i = 0; ; i++) {
        try {
          await postRevoke();
          break;
        } catch (e) {
          if (!(e instanceof ApiError && e.message === "executor_key_active")) throw e;
          if (txErr || i >= 10) throw txErr ?? e;
          await new Promise((ok) => setTimeout(ok, 1500));
        }
      }
      setKeyStatus("revoked");
    } catch (e) {
      setError(errText(e));
    } finally {
      setPending(false);
    }
  }, [s, send]);

  return (
    <Ctx.Provider
      value={{
        ready,
        accountId: s?.accountId ?? null,
        account: s?.account ?? null,
        authenticated: !!s?.token,
        keyStatus,
        pending,
        error,
        login,
        logout,
        revoke,
        send,
      }}
    >
      {children}
    </Ctx.Provider>
  );
}
