#!/usr/bin/env sh
# Run Epoch School straight from the source code for testing, with no binary build.
# The UI is served from web/, so HTML/CSS/JS edits show up on a browser refresh;
# Go changes need a restart (Ctrl+C, then run this again).
#
# Testing uses its own PostgreSQL database (epoch_dev) and data folder (.dev-data/),
# so it never touches the real school data. It starts with the demo school.
# Use another database with DEV_DATABASE_URL=postgres:///something ./dev.sh
set -e
cd "$(dirname "$0")"
export WEB_DIR=web
export EPOCH_DATA_DIR="${EPOCH_DATA_DIR:-$PWD/.dev-data}"
export DATABASE_URL="${DEV_DATABASE_URL:-postgres:///epoch_dev}"
export DB_PATH="" # don't import the real school's old SQLite data
export SEED_DEMO=true
if [ -z "$DEV_DATABASE_URL" ] && command -v psql >/dev/null; then
  psql -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='epoch_dev'" | grep -q 1 || createdb epoch_dev
fi
exec go run . "$@"
