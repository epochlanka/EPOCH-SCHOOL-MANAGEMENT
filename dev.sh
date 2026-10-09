#!/usr/bin/env sh
# Run Epoch School straight from the source code for testing, with no binary build.
# The UI is served from web/, so HTML/CSS/JS edits show up on a browser refresh;
# Go changes need a restart (Ctrl+C, then run this again).
#
# Testing uses its own data folder (.dev-data/) so it never touches the real
# school database. Point it elsewhere with EPOCH_DATA_DIR=/some/folder ./dev.sh
set -e
cd "$(dirname "$0")"
export WEB_DIR=web
export EPOCH_DATA_DIR="${EPOCH_DATA_DIR:-$PWD/.dev-data}"
exec go run . "$@"
