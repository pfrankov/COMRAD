#!/bin/sh
set -eu

BASE_URL="${COMRAD_BASE_URL:-http://127.0.0.1:1922}"
ADMIN_TOKEN="${COMRAD_ADMIN_TOKEN:-}"

curl -fsS "$BASE_URL/health" >/dev/null
curl -fsS "$BASE_URL/ready" >/dev/null

if [ -n "$ADMIN_TOKEN" ]; then
  curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/state" >/dev/null
fi

printf 'smoke ok: %s\n' "$BASE_URL"
