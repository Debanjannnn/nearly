"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { errText } from "./api";

// copied = last copied value, cleared after ms
export function useCopy(ms = 1200) {
  const [copied, setCopied] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);
  const copy = useCallback(
    (v: string) => {
      void navigator.clipboard?.writeText(v).then(() => {
        setCopied(v);
        if (timer.current) clearTimeout(timer.current);
        timer.current = setTimeout(() => setCopied(null), ms);
      });
    },
    [ms],
  );
  return { copied, copy };
}

// key null = idle; changing key refetches; pollMs = refetch on an interval
export function useAsync<T>(key: string | null, fn: () => Promise<T>, pollMs?: number) {
  const [nonce, setNonce] = useState(0);
  const [s, setS] = useState<{ key: string | null; nonce: number; data: T | null; error: string | null }>({
    key: null,
    nonce: -1,
    data: null,
    error: null,
  });

  useEffect(() => {
    if (key === null) return;
    let live = true;
    fn()
      .then((data) => live && setS({ key, nonce, data, error: null }))
      .catch((e) => live && setS({ key, nonce, data: null, error: errText(e) }));
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
  const same = key !== null && s.key === key;
  return {
    data: same ? s.data : null,
    error: same ? s.error : null,
    loading: key !== null && !(same && s.nonce === nonce),
    reload,
  };
}

export function useDebounced<T>(v: T, ms = 400) {
  const [d, setD] = useState(v);
  useEffect(() => {
    const t = setTimeout(() => setD(v), ms);
    return () => clearTimeout(t);
  }, [v, ms]);
  return d;
}
