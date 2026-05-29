#!/bin/sh
set -eu

BASE_URL="${COMRAD_BASE_URL:-http://127.0.0.1:1922}"

case "$BASE_URL" in
  http://127.0.0.1:*|http://localhost:*|http://[::1]:*)
    ;;
  *)
    printf 'refusing non-local Manager URL without explicit approval: %s\n' "$BASE_URL" >&2
    printf 'set COMRAD_ALLOW_NONLOCAL_NETWORK_CHECK=1 to perform a read-only check.\n' >&2
    if [ "${COMRAD_ALLOW_NONLOCAL_NETWORK_CHECK:-0}" != "1" ]; then
      exit 1
    fi
    ;;
esac

curl -fsS "$BASE_URL/health" >/dev/null
curl -fsS "$BASE_URL/ready" >/dev/null
printf 'network check ok: %s\n' "$BASE_URL"
