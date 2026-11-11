-- 0003 daily history for backtests. apy is a fraction (0.05 = 5%).

create table series_rate (
  source text not null check (source in ('lending', 'staking')),
  token  text not null,                        -- token symbol, e.g. usdc, wnear
  day    date not null,
  apy    numeric not null,
  primary key (source, token, day)
);

create table series_price (
  token text not null,                         -- token symbol; usdc is not stored (= 1)
  day   date not null,
  usd   numeric not null,
  primary key (token, day)
);

-- LST share price (NEAR per LST unit) sampled once a day; staking apy derives from it
create table series_lst (
  day    date primary key,
  ratio  numeric not null,
  height bigint not null
);

create table history_cursor (
  name text primary key,
  day  date not null                           -- last day fully sampled
);
