"use client";

import { useEffect, useRef, useState } from "react";
import { motion, useIsPresent } from "motion/react";
import { Check, Copy, ExternalLink, X } from "lucide-react";
import { txUrl } from "@/lib/config";
import { intentStatus, isDone, tokenLogos, txHash, txLink, type Intent, type IntentToken } from "@/lib/intents";
import { cn } from "@/lib/utils";
import { ErrorBox } from "@/components/ui";
import { EASE } from "@/components/motion";

// modal shell; Esc/backdrop unmount via state so the exit animation plays
export function IntentDialog({ title, subtitle, onClose, children }: { title: string; subtitle: string; onClose: () => void; children: React.ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  const present = useIsPresent();
  useEffect(() => ref.current?.showModal(), []);
  return (
    <motion.dialog
      ref={ref}
      aria-label={title}
      onClose={onClose}
      onCancel={(e) => (e.preventDefault(), onClose())}
      onClick={(e) => e.target === e.currentTarget && onClose()}
      initial={{ opacity: 0, scale: 0.98, y: 8 }}
      animate={{ opacity: 1, scale: 1, y: 0 }}
      exit={{ opacity: 0, scale: 0.98, y: 8 }}
      transition={{ duration: 0.18, ease: EASE }}
      className={cn(
        "mx-auto mt-[8vh] w-[calc(100%-2rem)] max-w-md space-y-5 rounded-3xl border border-border bg-popover p-5 text-foreground shadow-2xl sm:p-6 backdrop:bg-black/60 backdrop:backdrop-blur-sm backdrop:transition-opacity backdrop:duration-200 starting:backdrop:opacity-0",
        !present && "pointer-events-none backdrop:opacity-0",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-medium">{title}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{subtitle}</p>
        </div>
        <button type="button" aria-label="Close" onClick={onClose} className="rounded-full p-1.5 text-muted-foreground hover:bg-secondary hover:text-foreground">
          <X className="size-4" />
        </button>
      </div>
      {children}
      <p className="text-center text-[11px] text-muted-foreground">Routed by NEAR Intents 1Click</p>
    </motion.dialog>
  );
}

// polls the api every 5s until the swap settles
export function useIntent(addr: string | null) {
  const [s, setS] = useState<Intent | null>(null);
  const done = isDone(s?.status);
  useEffect(() => {
    if (!addr || done) return;
    let live = true;
    const tick = () => intentStatus(addr).then((r) => live && r && setS(r)).catch(() => {});
    void tick();
    const t = setInterval(tick, 5000);
    return () => {
      live = false;
      clearInterval(t);
    };
  }, [addr, done]);
  return s;
}

const stepOf = (s: Intent["status"] | undefined) => (s === "SUCCESS" ? 3 : s === "PROCESSING" || s === "KNOWN_DEPOSIT_TX" ? 1 : 0);

// status trail: labels[0] waits for funds, [1] swapping, [2] arrived
export function Trail({ intent, labels }: { intent: Intent | null; labels: [string, string, string] }) {
  const s = intent?.status;
  const step = stepOf(s);
  const done = isDone(s);
  const dest = intent?.swap_details?.destinationChainTxHashes?.[0];
  const link = dest && (txLink(dest) ?? (intent?.kind === "deposit" ? txUrl(txHash(dest)) : null));
  return (
    <div className="space-y-4">
      <ol className="space-y-2.5">
        {labels.map((label, i) => (
          <li key={label} className="flex items-center gap-3 text-sm">
            <span
              className={cn(
                "grid size-6 shrink-0 place-items-center rounded-full text-[11px]",
                i < step ? "bg-positive/15 text-positive" : i === step ? "bg-primary/15 text-primary" : "bg-secondary text-muted-foreground",
              )}
            >
              {i < step ? <Check className="size-3.5" /> : i + 1}
            </span>
            <span className={i <= step ? "text-foreground" : "text-muted-foreground"}>{label}</span>
            {i === step && !done && <span className="ml-auto size-1.5 animate-pulse rounded-full bg-primary" />}
          </li>
        ))}
      </ol>
      {s === "SUCCESS" && link && (
        <a href={link} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs text-positive hover:underline">
          View arrival tx <ExternalLink className="size-3" />
        </a>
      )}
      {(s === "REFUNDED" || s === "FAILED") && (
        <ErrorBox message={`${s === "REFUNDED" ? "Refunded" : "Failed"}${intent?.swap_details?.refundReason ? `: ${intent.swap_details.refundReason}` : ""}`} />
      )}
      {s === "INCOMPLETE_DEPOSIT" && <p className="text-xs text-warning">Deposit below the quoted amount. Send the rest, or it is refunded after the deadline.</p>}
    </div>
  );
}

// selectable value + copy button (buttons can't be text-selected)
export function CopyField({ label, value, suffix, mono, copied, copy }: { label: string; value: string; suffix?: string; mono?: boolean; copied: string | null; copy: (v: string) => void }) {
  return (
    <div className="flex items-center gap-3 rounded-2xl border border-border bg-card px-4 py-3">
      <div className="min-w-0 flex-1">
        <p className="text-[11px] text-muted-foreground">{label}</p>
        <p className={cn("select-all break-all", mono ? "font-mono text-xs leading-5" : "tnum text-base font-medium")}>
          {value}
          {suffix && <span className="select-none text-muted-foreground"> {suffix}</span>}
        </p>
      </div>
      <button
        type="button"
        aria-label={`Copy ${label.toLowerCase()}`}
        onClick={() => copy(value)}
        className="shrink-0 rounded-lg p-2 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
      >
        {copied === value ? <Check className="size-4 text-positive" /> : <Copy className="size-4" />}
      </button>
    </div>
  );
}

export function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      {children}
    </div>
  );
}

// first `keep` chips (plus the selected one), rest behind "+N more"; `search` adds a filter box
export function More<T>({
  items,
  keep,
  selected,
  search,
  placeholder,
  children,
}: {
  items: T[];
  keep: number;
  selected: T | undefined;
  search?: (x: T) => string;
  placeholder?: string;
  children: (x: T) => React.ReactNode;
}) {
  const [all, setAll] = useState(false);
  const [q, setQ] = useState("");
  const needle = q.trim().toLowerCase();
  const shown = needle && search ? items.filter((x) => search(x).toLowerCase().includes(needle)) : all ? items : items.filter((x, i) => i < keep || x === selected);
  return (
    <div className="space-y-2">
      {search && (
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={placeholder}
          aria-label={placeholder}
          className="w-full rounded-full border border-border bg-transparent px-3 py-1.5 text-xs outline-none placeholder:text-muted-foreground/60 focus:border-primary/50"
        />
      )}
      <div className="flex flex-wrap gap-1.5">
        {shown.map(children)}
        {needle && shown.length === 0 && <span className="px-1 py-1 text-xs text-muted-foreground">No match</span>}
        {!needle && items.length > shown.length && (
          <button type="button" onClick={() => setAll(true)} className="rounded-full px-2 py-1 text-xs text-muted-foreground hover:text-foreground">
            +{items.length - shown.length} more
          </button>
        )}
      </div>
    </div>
  );
}

export function Chip({ on, onClick, children }: { on: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs transition-colors",
        on ? "border-primary/50 bg-primary/10 text-foreground" : "border-border text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

// each remote logo in turn (onError -> next), else nothing
export function IntentIcon({ token, size }: { token: IntentToken; size: number }) {
  const [i, setI] = useState(0);
  const urls = tokenLogos(token);
  if (i >= urls.length) return null;
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img src={urls[i]} alt={token.symbol} width={size} height={size} onError={() => setI(i + 1)} className="shrink-0 rounded-full" style={{ width: size, height: size }} />
  );
}

export const inputCls = "w-full rounded-xl border border-border bg-card px-3 py-2.5 font-mono text-xs outline-none focus:border-primary/50";
