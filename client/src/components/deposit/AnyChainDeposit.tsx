"use client";

import { useEffect, useState } from "react";
import { AnimatePresence } from "motion/react";
import { Globe } from "lucide-react";
import { NETWORK } from "@/lib/config";
import { useAsync, useCopy, useDebounced } from "@/lib/hooks";
import {
  chainName,
  depositQuote,
  eta,
  fmtUsd,
  getIntentTokens,
  isDone,
  loadPending,
  savePending,
  sortChains,
  tokensOn,
  USDC_ASSET,
  validAddress,
  type IntentQuote,
  type IntentToken,
} from "@/lib/intents";
import { useSession } from "@/lib/session";
import { parseAmount, type Token } from "@/lib/tokens";
import { cn } from "@/lib/utils";
import { Button, ErrorBox, Row, Skeleton } from "@/components/ui";
import { Chip, CopyField, fc, Field, inputCls, IntentDialog, IntentIcon, More, register, TestnetForm, TGAS, Trail, useIntent } from "./parts";

type Pending = { q: IntentQuote; token: IntentToken; owner: string };
const KIND = "deposit";

// owner wallet -> sub-account ledger (ft_on_transfer); wNEAR wraps native NEAR first
const depositTxs = (t: Token, raw: bigint, owner: string, sub: string) => [
  {
    receiverId: t.address,
    actions: [
      ...(t.key === "wnear" ? [register(owner), fc("near_deposit", {}, raw.toString(), `10${TGAS}`)] : []),
      fc("ft_transfer_call", { receiver_id: sub, amount: raw.toString(), msg: "" }, "1", `100${TGAS}`),
    ],
  },
];

// "Deposit from any chain" trigger; need = USDC (display units) prefilled as the amount to receive
export function AnyChainButton({ need, onArrived, className, children }: { need?: number; onArrived?: () => void; className?: string; children?: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const { ensureReady } = useSession();
  return (
    <>
      <button type="button" onClick={() => void ensureReady().then((ok) => ok && setOpen(true))} className={cn("inline-flex items-center gap-1.5 text-xs text-primary hover:underline", className)}>
        {children ?? (
          <>
            <Globe className="size-3.5" />
            {need ? `Add ${fmtUsd(need)} from any chain` : "Deposit from any chain"}
          </>
        )}
      </button>
      <AnimatePresence>{open && <AnyChainDeposit need={need} onClose={() => setOpen(false)} onArrived={onArrived} />}</AnimatePresence>
    </>
  );
}

export function AnyChainDeposit({ need, onClose, onArrived }: { need?: number; onClose: () => void; onArrived?: () => void }) {
  const { accountId, account, authenticated } = useSession();
  const live = authenticated && !!account && !!accountId;
  const tokens = useAsync(live && NETWORK === "mainnet" ? "intent-tokens" : null, getIntentTokens);
  const [chain, setChain] = useState("base");
  const [assetId, setAssetId] = useState<string | null>(null);
  const [amount, setAmount] = useState(need ? String(Math.ceil(need * 100) / 100) : "");
  const [refund, setRefund] = useState("");
  const [deposit, setDeposit] = useState<Pending | null>(() => loadPending<Pending>(KIND, accountId));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const all = tokens.data ?? [];
  const dest = all.find((t) => t.assetId === USDC_ASSET);
  const chains = sortChains(all);
  const onChain = tokensOn(all, chain);
  const token = onChain.find((t) => t.assetId === assetId) ?? onChain[0];
  const refundTo = refund.trim();
  const out = dest ? parseAmount(amount, dest.decimals) : null;
  const ready = !!(dest && token && out && out > 0n && validAddress(chain, refundTo));
  const body = ready ? { origin_asset: token!.assetId, amount: out!.toString(), swap_type: "EXACT_OUTPUT" as const, refund_to: refundTo } : null;
  const key = body ? JSON.stringify(body) : null;

  // live dry quote (no deposit address yet), debounced
  const settled = useDebounced(key);
  const pq = useAsync(key && key === settled && !deposit ? `deposit:${key}` : null, () => depositQuote({ ...body!, dry: true }));

  async function start() {
    setBusy(true);
    setErr(null);
    try {
      const q = await depositQuote({ ...body!, dry: false });
      if (!q.quote.depositAddress) throw new Error("No deposit address returned");
      const d = { q, token: token!, owner: accountId! };
      savePending(KIND, d);
      setDeposit(d);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <IntentDialog title="Deposit from any chain" subtitle={NETWORK === "mainnet" ? "Arrives as USDC in your nearly account" : "From your NEAR wallet into your nearly account"} onClose={onClose}>
      {!live ? (
        <p className="text-sm text-muted-foreground">Connect your wallet first so funds have somewhere to land.</p>
      ) : NETWORK !== "mainnet" ? (
        <TestnetForm verb="Deposit" note="Your wallet signs one transaction. wNEAR is wrapped from your NEAR first." txs={(t, raw) => depositTxs(t, raw, accountId!, account!)} onDone={onArrived} />
      ) : tokens.error ? (
        <ErrorBox message={`NEAR Intents unavailable: ${tokens.error}`} />
      ) : !tokens.data ? (
        <Skeleton className="h-64 w-full rounded-2xl" />
      ) : deposit ? (
        <Progress
          deposit={deposit}
          onArrived={onArrived}
          onReset={() => {
            savePending(KIND, null);
            setDeposit(null);
          }}
        />
      ) : (
        <>
          <Field label="From">
            <More items={chains} keep={10} selected={chain} search={chainName} placeholder="Search chains">
              {(c) => (
                <Chip key={c} on={c === chain} onClick={() => (setChain(c), setAssetId(null))}>
                  {chainName(c)}
                </Chip>
              )}
            </More>
          </Field>
          <Field label="Pay with">
            <More key={chain} items={onChain} keep={5} selected={token}>
              {(t) => (
                <Chip key={t.assetId} on={t.assetId === token?.assetId} onClick={() => setAssetId(t.assetId)}>
                  <IntentIcon token={t} size={16} />
                  {t.symbol}
                </Chip>
              )}
            </More>
          </Field>
          <Field label="You receive">
            <div className="flex items-center gap-3 rounded-2xl border border-border bg-card px-4 py-3">
              <input
                autoFocus
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                inputMode="decimal"
                placeholder="0"
                aria-label="USDC to receive"
                className="tnum w-full min-w-0 bg-transparent text-3xl font-medium outline-none placeholder:text-muted-foreground/40"
              />
              <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-secondary/70 py-1.5 pl-2 pr-3 text-sm font-medium">
                {dest && <IntentIcon token={dest} size={18} />} USDC
              </span>
            </div>
          </Field>
          <Field label={`Your ${chainName(chain)} address, for refunds`}>
            <input value={refund} onChange={(e) => setRefund(e.target.value)} placeholder="Refund address" aria-label="Refund address" className={inputCls} />
          </Field>

          <dl className="tnum space-y-1.5 rounded-2xl bg-secondary/40 px-4 py-3 text-sm">
            <Row label="You send">
              {pq.data ? `${pq.data.quote.amountInFormatted} ${token?.symbol}` : ready && !pq.error ? <Skeleton className="h-4 w-24" /> : "-"}
            </Row>
            <Row label="Value">{pq.data ? fmtUsd(Number(pq.data.quote.amountInUsd)) : "-"}</Row>
            <Row label="Time">{pq.data ? `~${eta(pq.data.quote.timeEstimate)}` : "-"}</Row>
          </dl>
          {pq.error && <ErrorBox message={pq.error} />}
          {err && <ErrorBox message={err} />}
          <Button className="w-full py-3" pending={busy} disabled={!pq.data || busy} onClick={() => void start()}>
            Get deposit address
          </Button>
        </>
      )}
    </IntentDialog>
  );
}

// deposit address + live status until the USDC lands
function Progress({ deposit, onArrived, onReset }: { deposit: Pending; onArrived?: () => void; onReset: () => void }) {
  const { q, token } = deposit;
  const s = useIntent(q.quote.depositAddress!);
  const { copied, copy } = useCopy(1500);
  const done = isDone(s?.status);
  useEffect(() => {
    if (s?.status === "SUCCESS") onArrived?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [s?.status]);

  return (
    <div className="space-y-5">
      {!done && (
        <div className="space-y-3">
          <p className="text-sm">Send exactly this amount on {chainName(token.blockchain)}, from any wallet:</p>
          <CopyField label="Amount" value={q.quote.amountInFormatted} suffix={token.symbol} copied={copied} copy={copy} />
          <CopyField label="To address" value={q.quote.depositAddress!} mono copied={copied} copy={copy} />
          {q.quote.depositMemo && (
            <p className="text-xs text-warning">
              Memo required: <span className="font-mono">{q.quote.depositMemo}</span>
            </p>
          )}
          {q.quote.deadline && (
            <p className="text-[11px] text-muted-foreground">Address valid until {new Date(q.quote.deadline).toLocaleString()}. Unused funds are refunded.</p>
          )}
        </div>
      )}
      <Trail intent={s} labels={["Waiting for deposit", "Swapping to USDC", "Arrived in your account"]} />
      {s?.status === "SUCCESS" && (
        <p className="rounded-2xl bg-positive/10 px-4 py-3 text-sm font-medium text-positive">
          {s.swap_details?.amountOutFormatted ?? q.quote.amountOutFormatted} USDC arrived in your account
        </p>
      )}
      {(done || !s || s.status === "PENDING_DEPOSIT") && (
        <button type="button" onClick={onReset} className="w-full text-center text-xs text-muted-foreground hover:text-foreground">
          {done ? "New deposit" : "Cancel and start over"}
        </button>
      )}
    </div>
  );
}
