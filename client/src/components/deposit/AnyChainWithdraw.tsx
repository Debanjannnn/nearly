"use client";

import { useState } from "react";
import { AnimatePresence } from "motion/react";
import { Send } from "lucide-react";
import { getLedgerBalance } from "@/lib/api";
import { NETWORK } from "@/lib/config";
import { useAsync, useDebounced, useHoldings } from "@/lib/hooks";
import {
  chainName,
  eta,
  fmtUsd,
  getIntentTokens,
  isDone,
  loadPending,
  savePending,
  sortChains,
  submitIntent,
  tokensOn,
  validAddress,
  withdrawQuote,
  type IntentQuote,
  type IntentToken,
} from "@/lib/intents";
import { useSession } from "@/lib/session";
import { fmtUnits, parseAmount, TOKENS, type Token } from "@/lib/tokens";
import { cn, decimalInput } from "@/lib/utils";
import { Button, ErrorBox, Row, Skeleton } from "@/components/ui";
import { Chip, fc, Field, inputCls, IntentDialog, IntentIcon, More, register, TestnetForm, TGAS, Trail, useIntent } from "./parts";

type Pending = { q: IntentQuote; from: Token; amount: string; dest: IntentToken; recipient: string; owner: string; sub: string; withdrawn?: boolean; sent?: boolean };
const KIND = "withdraw";
const FTS = TOKENS.filter((t) => !!t.key);

// account.withdraw pays the owner, then the owner sends it to the 1Click deposit address
const withdrawTxs = (p: Pending) => [
  { receiverId: p.from.address, actions: [register(p.owner), register(p.q.quote.depositAddress!)] },
  { receiverId: p.sub, actions: [fc("withdraw", { token: p.from.address, amount: p.amount }, "1", `100${TGAS}`)] },
];
// testnet: account.withdraw straight to the owner wallet
const testnetTxs = (t: Token, raw: bigint, owner: string, sub: string) => [
  { receiverId: t.address, actions: [register(owner)] },
  { receiverId: sub, actions: [fc("withdraw", { token: t.address, amount: raw.toString() }, "1", `100${TGAS}`)] },
];
const transferTx = (p: Pending) => ({
  receiverId: p.from.address,
  actions: [fc("ft_transfer", { receiver_id: p.q.quote.depositAddress!, amount: p.amount, memo: p.q.quote.depositMemo ?? null }, "1")],
});

export function AnyChainWithdrawButton({ className, children }: { className?: string; children?: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const { ensureReady } = useSession();
  return (
    <>
      <button type="button" onClick={() => void ensureReady().then((ok) => ok && setOpen(true))} className={cn("inline-flex items-center gap-1.5 text-xs text-primary hover:underline", className)}>
        {children ?? (
          <>
            <Send className="size-3.5" />
            Withdraw to any chain
          </>
        )}
      </button>
      <AnimatePresence>{open && <AnyChainWithdraw onClose={() => setOpen(false)} />}</AnimatePresence>
    </>
  );
}

export function AnyChainWithdraw({ onClose }: { onClose: () => void }) {
  const { accountId, account, authenticated, send } = useSession();
  const live = authenticated && !!account && !!accountId;
  const intents = live && NETWORK === "mainnet";
  const tokens = useAsync(intents ? "intent-tokens" : null, getIntentTokens);
  const holdings = useHoldings();

  const [fromId, setFromId] = useState(FTS[0]?.address);
  const [chain, setChain] = useState("base");
  const [assetId, setAssetId] = useState<string | null>(null);
  const [amount, setAmount] = useState("");
  const [recipient, setRecipient] = useState("");
  const [pending, setPending] = useState<Pending | null>(() => loadPending<Pending>(KIND, accountId));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const all = tokens.data ?? [];
  const from = FTS.find((t) => t.address === fromId);
  // Max = account ledger (get_balance), not ft_balance_of: withdraw only pays what the ledger holds
  const ledger = useAsync(intents && fromId ? `ledger:${account}:${fromId}` : null, () => getLedgerBalance(account!, fromId!));
  const bal = ledger.data ?? undefined;
  const chains = sortChains(all);
  const onChain = tokensOn(all, chain);
  const dest = onChain.find((t) => t.assetId === assetId) ?? onChain[0];
  const to = recipient.trim();
  const raw = from ? parseAmount(amount, from.decimals) : null;
  const over = raw !== null && bal !== undefined && raw > BigInt(bal);
  const ready = !!(from && dest && raw && raw > 0n && !over && validAddress(chain, to));
  const body = ready
    ? { origin_asset: `nep141:${from!.address}`, destination_asset: dest!.assetId, amount: raw!.toString(), swap_type: "EXACT_INPUT" as const, recipient: to }
    : null;
  const key = body ? JSON.stringify(body) : null;
  const settled = useDebounced(key);
  const pq = useAsync(key && key === settled && !pending ? `withdraw:${key}` : null, () => withdrawQuote({ ...body!, dry: true }));

  const save = (p: Pending | null) => {
    savePending(KIND, p);
    setPending(p);
  };

  async function sign(p: Pending) {
    setBusy(true);
    setErr(null);
    try {
      // two signs, each step saved, so a retry never re-runs a withdraw that landed
      if (!p.withdrawn) {
        await send(withdrawTxs(p));
        save((p = { ...p, withdrawn: true }));
      }
      const [hash] = await send([transferTx(p)]);
      save({ ...p, sent: true });
      await submitIntent(p.q.quote.depositAddress!, p.q.quote.depositMemo, hash).catch(() => {});
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function start() {
    setBusy(true);
    setErr(null);
    try {
      const q = await withdrawQuote({ ...body!, dry: false });
      if (!q.quote.depositAddress) throw new Error("No deposit address returned");
      const p = { q, from: from!, amount: body!.amount, dest: dest!, recipient: to, owner: accountId!, sub: account! };
      save(p);
      setBusy(false);
      await sign(p);
    } catch (e) {
      setErr((e as Error).message);
      setBusy(false);
    }
  }

  return (
    <IntentDialog title="Withdraw to any chain" subtitle={NETWORK === "mainnet" ? "From your nearly account to an address on another chain" : "From your nearly account back to your NEAR wallet"} onClose={onClose}>
      {!live ? (
        <p className="text-sm text-muted-foreground">Connect your wallet first.</p>
      ) : NETWORK !== "mainnet" ? (
        <TestnetForm
          verb="Withdraw"
          note="Your wallet signs twice: storage registration on the token, then the withdraw to itself."
          balanceOf={(t) => holdings.data?.tokens.find((x) => x.address === t.address)?.raw}
          txs={(t, raw) => testnetTxs(t, raw, accountId!, account!)}
          onDone={holdings.reload}
        />
      ) : tokens.error ? (
        <ErrorBox message={`NEAR Intents unavailable: ${tokens.error}`} />
      ) : !tokens.data ? (
        <Skeleton className="h-64 w-full rounded-2xl" />
      ) : pending ? (
        <Progress pending={pending} busy={busy} err={err} onSign={() => void sign(pending)} onReset={() => save(null)} />
      ) : (
        <>
          <Field label="Withdraw">
            <div className="flex items-center gap-3 rounded-2xl border border-border bg-card px-4 py-3">
              <input
                autoFocus
                value={amount}
                onChange={(e) => { const v = decimalInput(e.target.value); if (v !== null) setAmount(v); }}
                inputMode="decimal"
                placeholder="0"
                aria-label="Amount to withdraw"
                className="tnum w-full min-w-0 bg-transparent text-3xl font-medium outline-none placeholder:text-muted-foreground/40"
              />
              <div className="flex shrink-0 gap-1">
                {FTS.map((t) => (
                  <Chip key={t.address} on={t.address === fromId} onClick={() => setFromId(t.address)}>
                    {t.symbol}
                  </Chip>
                ))}
              </div>
            </div>
            {from && bal !== undefined && (
              <button type="button" onClick={() => setAmount(fmtUnits(bal, from.decimals, from.decimals).replace(/,/g, ""))} className="tnum text-xs text-muted-foreground hover:text-foreground">
                Balance {fmtUnits(bal, from.decimals)} {from.symbol} · Max
              </button>
            )}
            {over && <p className="text-xs text-destructive">More than your balance.</p>}
          </Field>
          <Field label="To chain">
            <More items={chains} keep={10} selected={chain} search={chainName} placeholder="Search chains">
              {(c) => (
                <Chip key={c} on={c === chain} onClick={() => (setChain(c), setAssetId(null))}>
                  {chainName(c)}
                </Chip>
              )}
            </More>
          </Field>
          <Field label="Receive">
            <More key={chain} items={onChain} keep={5} selected={dest}>
              {(t) => (
                <Chip key={t.assetId} on={t.assetId === dest?.assetId} onClick={() => setAssetId(t.assetId)}>
                  <IntentIcon token={t} size={16} />
                  {t.symbol}
                </Chip>
              )}
            </More>
          </Field>
          <Field label={`${chainName(chain)} address to receive`}>
            <input value={recipient} onChange={(e) => setRecipient(e.target.value)} placeholder="Recipient address" aria-label="Recipient address" className={inputCls} />
          </Field>

          <dl className="tnum space-y-1.5 rounded-2xl bg-secondary/40 px-4 py-3 text-sm">
            <Row label="You receive">
              {pq.data ? `${pq.data.quote.amountOutFormatted} ${dest?.symbol}` : ready && !pq.error ? <Skeleton className="h-4 w-24" /> : "-"}
            </Row>
            <Row label="Value">{pq.data ? fmtUsd(Number(pq.data.quote.amountOutUsd)) : "-"}</Row>
            <Row label="Time">{pq.data ? `~${eta(pq.data.quote.timeEstimate)}` : "-"}</Row>
          </dl>
          {pq.error && <ErrorBox message={pq.error} />}
          {err && <ErrorBox message={err} />}
          <Button className="w-full py-3" pending={busy} disabled={!pq.data || busy} onClick={() => void start()}>
            Withdraw to {chainName(chain)}
          </Button>
          <p className="text-[11px] text-muted-foreground">Your wallet signs twice: the withdraw to itself, then the transfer into NEAR Intents. Refunds return to your wallet.</p>
        </>
      )}
    </IntentDialog>
  );
}

function Progress({ pending: p, busy, err, onSign, onReset }: { pending: Pending; busy: boolean; err: string | null; onSign: () => void; onReset: () => void }) {
  const s = useIntent(p.sent ? p.q.quote.depositAddress! : null);
  const done = isDone(s?.status);
  return (
    <div className="space-y-5">
      <dl className="tnum space-y-1.5 rounded-2xl bg-secondary/40 px-4 py-3 text-sm">
        <Row label="Send">
          {fmtUnits(p.amount, p.from.decimals)} {p.from.symbol}
        </Row>
        <Row label="Receive">
          ~{p.q.quote.amountOutFormatted} {p.dest.symbol} on {chainName(p.dest.blockchain)}
        </Row>
        <Row label="To">
          <span className="font-mono text-xs" title={p.recipient}>
            {p.recipient}
          </span>
        </Row>
      </dl>
      {!p.sent ? (
        <>
          {err && <ErrorBox message={err} />}
          <Button className="w-full py-3" pending={busy} onClick={onSign}>
            {p.withdrawn ? "Send to NEAR Intents" : "Sign and send"}
          </Button>
        </>
      ) : (
        <Trail intent={s} labels={["Sent from NEAR", `Swapping to ${p.dest.symbol}`, `Arrived on ${chainName(p.dest.blockchain)}`]} />
      )}
      {(done || !p.sent) && (
        <button type="button" onClick={onReset} className="w-full text-center text-xs text-muted-foreground hover:text-foreground">
          {done ? "New withdrawal" : "Cancel"}
        </button>
      )}
    </div>
  );
}
