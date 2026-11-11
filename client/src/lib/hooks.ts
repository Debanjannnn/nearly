"use client";

import { useCallback, useEffect, useRef, useState } from "react";

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
