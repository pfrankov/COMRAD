#!/bin/sh
set -eu

BUNDLE="${COMRAD_ROLLBACK_BUNDLE:-}"
DEPLOY_DIR="${COMRAD_DEPLOY_DIR:-}"

if [ -z "$BUNDLE" ] || [ -z "$DEPLOY_DIR" ]; then
  printf 'usage: COMRAD_ROLLBACK_BUNDLE=/path/comrad-local-darwin-arm64.tar.gz COMRAD_DEPLOY_DIR=/path/to/deploy scripts/rollback-local.sh\n' >&2
  exit 1
fi

case "$DEPLOY_DIR" in
  /|/System|/System/*|/Library|/Library/*|/usr|/usr/*|/bin|/bin/*|/sbin|/sbin/*)
    printf 'refusing unsafe deploy dir: %s\n' "$DEPLOY_DIR" >&2
    exit 1
    ;;
esac

test -f "$BUNDLE"
mkdir -p "$DEPLOY_DIR"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

tar -xzf "$BUNDLE" -C "$tmp"
test -x "$tmp/bundle-darwin-arm64/bin/comrad-manager"
test -x "$tmp/bundle-darwin-arm64/bin/comrad-worker"
test -x "$tmp/bundle-darwin-arm64/bin/llama-server"

backup="$DEPLOY_DIR.previous"
rm -rf "$backup"
if [ -d "$DEPLOY_DIR/current" ]; then
  mv "$DEPLOY_DIR/current" "$backup"
fi
mv "$tmp/bundle-darwin-arm64" "$DEPLOY_DIR/current"
printf 'rollback installed: %s/current\n' "$DEPLOY_DIR"
