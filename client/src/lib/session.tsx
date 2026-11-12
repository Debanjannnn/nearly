"use client";

import type { NearConnector, SignAndSendTransactionsParams } from "@hot-labs/near-connect";
import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { ApiError, createAccount, errMsg, errText, getAuthNonce, getMe, postAuth, postRevoke, setToken } from "./api";
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
  login: () => Promise<boolean>;
  // runs login when the account isn't ready yet; actions await it, then continue
  ensureReady: () => Promise<boolean>;
  logout: () => Promise<void>;
  revoke: () => Promise<void>;
  // shared shape with the page components: address = sub-account, mainAddress = wallet, delegated = executor key on
  address: string | undefined;
  mainAddress: string | undefined;
  delegated: boolean;
  delegate: () => Promise<boolean>;
  delegating: boolean;
  delegationError: string | null;
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

const unb64 = (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));

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

  // the one connect chain: wallet → NEP-413 sign-in → sub-account; skips steps already done, true once ready
  const login = useCallback(async (): Promise<boolean> => {
    const c = connector.current;
    if (!c) return false;
    setPending(true);
    setError(null);
    let cur = s;
    try {
      const wallet = await c.wallet().catch(() => c.connect());
      if (!cur?.token) {
        // server-issued, single-use, short ttl: an old signed login cannot be replayed
        const { nonce } = await getAuthNonce();
        const signed = await wallet.signMessage({ ...LOGIN, nonce: unb64(nonce) });
        const { token } = await postAuth({
          account_id: signed.accountId,
          public_key: signed.publicKey,
          signature: signed.signature,
          nonce,
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
      return true;
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && cur) save({ ...cur, token: null });
      setError(errText(e));
      return false;
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
        ensureReady: () => (s?.token && s.account ? Promise.resolve(true) : login()),
        logout,
        revoke,
        send,
        address: s?.account ?? undefined,
        mainAddress: s?.accountId ?? undefined,
        delegated: keyStatus === "active",
        delegate: login,
        delegating: pending,
        delegationError: error,
      }}
    >
      {children}
    </Ctx.Provider>
  );
}

// key null = idle; changing key refetches; reload() refetches keeping current data; pollMs = reload on an interval.
export function useAsync<T>(key: string | null, fn: () => Promise<T>, pollMs?: number) {
  const [nonce, setNonce] = useState(0);
  const [st, setSt] = useState<{ key: string | null; nonce: number; data: T | null; error: string | null }>({
    key: null,
    nonce: -1,
    data: null,
    error: null,
  });

  useEffect(() => {
    if (key === null) return;
    let live = true;
    fn()
      .then((data) => live && setSt({ key, nonce, data, error: null }))
      .catch((e) => live && setSt((p) => ({ key, nonce, data: p.key === key ? p.data : null, error: errMsg(e) })));
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, nonce]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  useEffect(() => {
    if (key === null || !pollMs) return;
    const t = setInterval(reload, pollMs);
    return () => clearInterval(t);
  }, [key, pollMs, reload]);
  const same = key !== null && st.key === key;
  return {
    data: same ? st.data : null,
    error: same ? st.error : null,
    loading: key !== null && !(same && st.nonce === nonce),
    reload,
  };
}
