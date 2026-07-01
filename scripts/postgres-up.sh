#!/bin/sh
# Start a repo-local Postgres dev cluster on 127.0.0.1:5433 for Antenne.
set -e

DATA_DIR="$(pwd)/.local/postgres"
PORT=5433
DB=antenne
USER=antenne

if [ ! -d "$DATA_DIR" ]; then
  echo "Initialising Postgres cluster in $DATA_DIR"
  initdb -D "$DATA_DIR" -U "$USER" --auth=trust >/dev/null
fi

if pg_ctl -D "$DATA_DIR" status >/dev/null 2>&1; then
  echo "Postgres already running."
else
  pg_ctl -D "$DATA_DIR" -o "-p $PORT -k /tmp" -l "$DATA_DIR/postgres.log" start
fi

# Wait for readiness, then ensure role + database exist.
until pg_isready -h 127.0.0.1 -p "$PORT" >/dev/null 2>&1; do sleep 0.3; done
psql -h 127.0.0.1 -p "$PORT" -U "$USER" -d postgres -tc \
  "SELECT 1 FROM pg_database WHERE datname='$DB'" | grep -q 1 || \
  createdb -h 127.0.0.1 -p "$PORT" -U "$USER" "$DB"

echo "Postgres ready on 127.0.0.1:$PORT (db=$DB user=$USER)"
