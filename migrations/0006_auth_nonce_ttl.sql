-- 0006 server-issued login nonces: account_id null = issued, unused; set on consume.

alter table auth_nonces alter column account_id drop not null;
alter table auth_nonces add column expires_at timestamptz not null default now() + interval '5 minutes';
create index auth_nonces_unused on auth_nonces (expires_at) where account_id is null;
