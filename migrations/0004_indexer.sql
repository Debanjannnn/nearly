-- 0004 indexer: NEP-297 "nearly" events from user sub-accounts + last read of each account's ledger.

create table chain_events (
  receipt_id   text not null,
  log_index    int not null,
  block_height bigint not null,
  block_time   timestamptz not null,
  tx_hash      text,
  account_id   text not null,                  -- sub-account that emitted the event
  event        text not null check (event in ('deposit', 'step', 'withdraw', 'revoke')),
  data         jsonb not null,
  primary key (receipt_id, log_index)
);
create index chain_events_account_idx on chain_events (account_id, block_height desc);

create table indexer_cursor (
  name   text primary key,
  height bigint not null                       -- last final block fully processed
);

-- account contract ledger (get_balances / get_positions) as last read
create table account_slots (
  account_id text not null,                    -- sub-account
  slot       text not null,                    -- "balance:<token>", "lending:<token>", "staking"
  amount     numeric(39,0) not null,
  idle       numeric(39,0) not null default 0, -- on chain but in no open position
  updated_at timestamptz not null default now(),
  primary key (account_id, slot)
);
