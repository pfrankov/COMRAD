#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="${1:-$ROOT/dist/bundle-darwin-arm64/bin}"

if [ "$(uname -s)" != "Darwin" ]; then
  echo "Skipping macOS ad-hoc signing on non-Darwin host."
  exit 0
fi

if ! command -v codesign >/dev/null 2>&1; then
  echo "Skipping macOS ad-hoc signing: codesign not found."
  exit 0
fi

find "$BIN_DIR" -type f \( -perm -111 -o -name "*.dylib" -o -name "*.so" \) \
  -exec codesign --force --sign - {} +
echo "Ad-hoc signed macOS bundle binaries in $BIN_DIR"
