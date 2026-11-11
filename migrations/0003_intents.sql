-- 0003 NEAR Intents 1Click swaps (cross-chain deposit / withdraw), polled until terminal.

create table intents (
  deposit_address text not null,
  deposit_memo    text not null default '',     -- '' when the chain needs none
  account_id      text not null references users (account_id),
  kind            text not null check (kind in ('deposit', 'withdraw')),
  status          text not null default 'PENDING_DEPOSIT',  -- 1Click status
  deposit_tx      text,
  quote           jsonb not null,               -- signed 1Click quote, kept for disputes
  swap_details    jsonb,                        -- last 1Click swapDetails
  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now(),
  primary key (deposit_address, deposit_memo)
);
create index intents_account_idx on intents (account_id, created_at desc);
