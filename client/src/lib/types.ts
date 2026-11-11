// Wire shapes of api, market-data and history (token amounts = u128 atoms), plus the view shapes lib/api maps them into.

export type LegSource = "lending" | "staking" | "hold";
// token = deployments token key ("usdc" | "wnear"); a non-usdc leg is bought on pool_id
export type Leg = { token: string; source: LegSource; weight_bps: number; pool_id?: number };
export type BasketDef = { legs: Leg[]; drift_bps?: number };

export type Basket = { id: string; creator: string; name: string; definition: BasketDef; public: boolean; created_at: string };

export type KeyStatus = "none" | "pending" | "active" | "revoked";
export type Me = { account_id: string; sub_account: string | null; key_status: KeyStatus };

// tx is a near-connect signAndSendTransaction body (factory create_account)
export type TxBody = { receiverId: string; actions: { type: "FunctionCall"; params: { methodName: string; args: object; gas: string; deposit: string } }[] };
export type AccountReply = { account_id: string | null; key_status: KeyStatus; tx?: TxBody };

export type HoldingsWire = { account_id: string; tokens: Record<string, { token: string; balance: string }> };
// view: one row per token, amount in display units
export type Holdings = { wallet: string; tokens: { symbol: string; address: string; raw: string; amount: string }[] };

export type Step = {
  kind: "supply" | "withdraw_lend" | "swap" | "stake" | "unstake" | "fee" | "to_owner";
  token?: string;
  amount?: string;
  pool_id?: number;
  token_in?: string;
  token_out?: string;
  amount_in?: string;
  min_out?: string;
};

export type PositionState = "opening" | "open" | "withdrawing" | "closed";
// held[i] = atoms leg i holds, same order as the basket legs
export type Position = {
  id: string;
  basket_id: string;
  basket_name: string;
  state: PositionState;
  deposit: string;
  held: string[];
  created_at: string;
  updated_at: string;
};

export type ExecStatus = "pending" | "running" | "succeeded" | "failed";
export type ExecParams = { amount?: string; fee?: string; held?: string[]; bps?: number; held_after?: string[]; resume_of?: string; schedule_id?: string };
export type ExecStep = { index: number; step: Step; status: string; tx_hash: string | null; error: string | null };
export type Execution = {
  id: string;
  position_id: string | null;
  kind: string;
  status: ExecStatus;
  params: ExecParams;
  steps_done: number;
  error: string | null;
  created_at: string;
  updated_at: string;
  steps?: ExecStep[];
};

// 202 body of deploy / withdraw
export type Accepted = { execution_id?: string; position_id: string; fee?: string; resumed?: boolean; steps: Step[] };

export type ScheduleKind = "dca" | "recurring_buy" | "rebalance";
export type ScheduleInput = {
  kind: ScheduleKind;
  basket_id?: string;
  position_id?: string;
  params?: { amount?: string; drift_bps?: number };
  every_seconds: number;
};
export type Schedule = {
  id: string;
  basket_id: string | null;
  position_id: string | null;
  kind: ScheduleKind;
  params: { amount?: string; drift_bps?: number };
  every_seconds: number;
  next_run_at: string;
  active: boolean;
  created_at: string;
};
// basket = recurring buy / DCA, position = rebalance, market = token page (no backend schedule yet)
export type Target = { type: "market" | "position" | "strategy"; id: string };

// market-data; token = contract id, apy fraction (0.05 = 5%)
export type Rate = { venue: "lending" | "staking"; contract: string; token: string; apy: number; ts: number };
export type Rates = { rates: Rate[]; stale: boolean };
export type Price = { token: string; symbol: string; usd: number; ts: number };
export type Prices = { prices: Price[]; stale: boolean };
export type Quote = {
  pool_id: number;
  token_in: string;
  token_out: string;
  amount_in: string;
  amount_out: string;
  min_out: string;
  max_safe_in: string;
  impact_bps: number;
  ts: number;
};

// history POST /backtest; contributions keyed "<source>:<token>", "price", "costs"
export type BacktestWire = {
  from: string;
  to: string;
  days: number;
  coverage: { series: string; first: string; last: string; days: number; note?: string }[];
  initial_usd: number;
  final_usd: number;
  apy: number;
  max_drawdown: number;
  rebalances: number;
  costs_usd: number;
  contributions: Record<string, number>;
  curve: { day: string; usd: number }[];
};
export type Backtest = {
  from: string;
  to: string;
  days: number;
  coverage: { series: string; first: string; last: string; days: number; note?: string }[];
  initialUsd: number;
  finalUsd: number;
  apyUsd: number;
  maxDrawdownUsd: number;
  rebalances: number;
  costsUsd: number;
  curve: { day: string; usd: number }[];
};
export type PricePoint = { day: string; usd: string };

// view of a basket: one sleeve per token, sources split that sleeve (weights in bps of the sleeve)
export type SourceKind = "hold" | "lending" | "staking" | "lp";
export type Source = { kind: SourceKind; venue: string; weightBps: number };
export type Sleeve = { asset: string; weightBps: number; sources: Source[] };
export type Rules = { driftBps?: number };

export type Strategy = {
  id: string;
  creator: string;
  name: string;
  sleeves: Sleeve[];
  rules: Rules;
  public: boolean;
  createdAt: string;
  definition: BasketDef;
};

export type StrategyInput = { name: string; sleeves: Sleeve[]; rules: Rules; public: boolean };
