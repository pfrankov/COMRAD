#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export COMRAD_MANAGER_ADDR="${COMRAD_MANAGER_ADDR:-127.0.0.1:1922}"
export COMRAD_SQLITE_PATH="${COMRAD_SQLITE_PATH:-${COMRAD_DB_PATH:-$ROOT/data/comrad.sqlite}}"
export COMRAD_ARTIFACT_DIR="${COMRAD_ARTIFACT_DIR:-$ROOT/data/artifacts}"
export COMRAD_ADMIN_TOKEN="${COMRAD_ADMIN_TOKEN:-dev-admin-token}"
export COMRAD_CLIENT_API_KEY="${COMRAD_CLIENT_API_KEY:-dev-client-key}"
export COMRAD_WORKER_TOKEN="${COMRAD_WORKER_TOKEN:-dev-worker-token}"
export COMRAD_ALLOW_DEV_DEFAULTS="${COMRAD_ALLOW_DEV_DEFAULTS:-true}"

exec "$ROOT/bin/comrad-manager"
