#!/bin/sh
# Applies migrations/*.sql once each, in name order; libpq PG* env vars pick the db.
set -eu
DIR=${1:-/migrations}
q() { psql -v ON_ERROR_STOP=1 -qtAX "$@"; }
q -c "create table if not exists schema_migrations (name text primary key, applied_at timestamptz not null default now())"
for f in "$DIR"/*.sql; do
  n=$(basename "$f")
  [ -n "$(q -c "select 1 from schema_migrations where name = '$n'")" ] && continue
  echo "apply $n"
  q --single-transaction -f "$f" -c "insert into schema_migrations (name) values ('$n')"
done
