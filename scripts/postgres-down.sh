#!/bin/sh
# Stop the repo-local Postgres dev cluster.
set -e
DATA_DIR="$(pwd)/.local/postgres"
if pg_ctl -D "$DATA_DIR" status >/dev/null 2>&1; then
  pg_ctl -D "$DATA_DIR" stop
  echo "Postgres stopped."
else
  echo "Postgres not running."
fi
