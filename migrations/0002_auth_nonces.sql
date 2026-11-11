-- 0002 NEP-413 login nonces, kept so a signed login cannot be replayed.

create table auth_nonces (
  nonce      bytea primary key,
  account_id text not null,
  created_at timestamptz not null default now()
);
