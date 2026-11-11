-- 0001 init. Token amounts are raw u128 atoms: numeric(39,0).

create table users (
  account_id  text primary key,                -- owner wallet, e.g. alice.testnet
  sub_account text unique,                     -- alice.<root>, set once the factory created it
  key_status  text not null default 'none'
              check (key_status in ('none', 'pending', 'active', 'revoked')),
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);

create table baskets (
  id         uuid primary key default gen_random_uuid(),
  creator    text not null references users (account_id),
  name       text not null,
  definition jsonb not null,                   -- planner.Basket
  public     boolean not null default false,
  created_at timestamptz not null default now()
);
create index baskets_public_idx on baskets (created_at desc) where public;

create table positions (
  id         uuid primary key default gen_random_uuid(),
  account_id text not null references users (account_id),
  basket_id  uuid not null references baskets (id),
  state      text not null default 'opening'
             check (state in ('opening', 'open', 'withdrawing', 'closed')),
  deposit    numeric(39,0) not null,           -- base token atoms put in
  held       jsonb not null default '[]',      -- per basket leg, token atoms
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create index positions_account_idx on positions (account_id, state);

create table executions (
  id          uuid primary key default gen_random_uuid(),
  account_id  text not null references users (account_id),
  position_id uuid references positions (id),
  kind        text not null check (kind in ('deploy', 'withdraw', 'rebalance', 'dca')),
  status      text not null default 'pending'
              check (status in ('pending', 'running', 'succeeded', 'failed')),
  params      jsonb not null default '{}',     -- request input, e.g. {"bps":5000}
  steps_done  int not null default 0,          -- resume point
  error       text,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);
create index executions_position_idx on executions (position_id, created_at desc);

create table execution_steps (
  execution_id uuid not null references executions (id) on delete cascade,
  idx          int not null,
  step         jsonb not null,                 -- shared Step JSON
  status       text not null default 'pending' check (status in ('pending', 'ok', 'failed')),
  tx_hash      text,
  error        text,
  updated_at   timestamptz not null default now(),
  primary key (execution_id, idx)
);

create table schedules (
  id          uuid primary key default gen_random_uuid(),
  account_id  text not null references users (account_id),
  basket_id   uuid references baskets (id),
  position_id uuid references positions (id),
  kind        text not null check (kind in ('dca', 'recurring_buy', 'rebalance')),
  params      jsonb not null default '{}',
  every       interval not null,
  next_run_at timestamptz not null,
  active      boolean not null default true,
  created_at  timestamptz not null default now()
);
create index schedules_due_idx on schedules (next_run_at) where active;

create table fee_ledger (
  id            bigserial primary key,
  execution_id  uuid not null references executions (id),
  position_id   uuid references positions (id),
  token         text not null,
  amount        numeric(39,0) not null,        -- open fee sent to treasury
  creator       text references users (account_id),
  creator_share numeric(39,0) not null default 0,
  created_at    timestamptz not null default now(),
  unique (execution_id, token)
);
