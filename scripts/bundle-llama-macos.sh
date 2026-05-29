#!/bin/sh
set -eu

fail() {
  echo "$1" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
. "$ROOT/scripts/llama-runtime.env"

DEST_BIN="${1:-$ROOT/dist/bundle-darwin-arm64/bin}"
CACHE_DIR="${COMRAD_LLAMA_CPP_CACHE_DIR:-$ROOT/.tools/llama.cpp}"
if [ -n "${COMRAD_LLAMA_CPP_URL:-}" ]; then
  LLAMA_CPP_URL="$COMRAD_LLAMA_CPP_URL"
  LLAMA_CPP_SHA256="${COMRAD_LLAMA_CPP_SHA256:-}"
else
  LLAMA_CPP_URL="$DEFAULT_LLAMA_CPP_URL"
  LLAMA_CPP_SHA256="${COMRAD_LLAMA_CPP_SHA256:-$DEFAULT_LLAMA_CPP_SHA256}"
fi

sha256_file() {
  shasum -a 256 "$1" | awk '{print "sha256:" $1}'
}

archive_name() {
  name="${LLAMA_CPP_URL##*/}"
  if [ -n "$name" ] && [ "$name" != "$LLAMA_CPP_URL" ]; then
    printf '%s\n' "$name"
  else
    printf '%s\n' "llama.cpp-macos-arm64.tar.gz"
  fi
}

download_archive() {
  archive="$1"
  if [ -f "$archive" ] && [ "$(sha256_file "$archive")" = "$LLAMA_CPP_SHA256" ]; then
    return 0
  fi
  tmp="$archive.part"
  rm -f "$tmp"
  echo "Downloading llama.cpp Worker runtime: $LLAMA_CPP_URL"
  curl -fL --retry 3 -o "$tmp" "$LLAMA_CPP_URL" || fail "failed to download llama.cpp runtime"
  got="$(sha256_file "$tmp")"
  if [ "$got" != "$LLAMA_CPP_SHA256" ]; then
    rm -f "$tmp"
    fail "llama.cpp archive sha256 mismatch: got $got, want $LLAMA_CPP_SHA256"
  fi
  mv "$tmp" "$archive"
}

copy_runtime_neighbors() {
  src_dir="$(dirname "$1")"
  for file in "$src_dir"/*.dylib "$src_dir"/*.so "$src_dir"/*.metal "$src_dir"/*.metallib; do
    if [ -e "$file" ]; then
      cp "$file" "$DEST_BIN/"
    fi
  done
}

require_cmd curl
require_cmd tar
require_cmd shasum
if [ -z "$LLAMA_CPP_SHA256" ]; then
  fail "COMRAD_LLAMA_CPP_SHA256 is required when bundling a custom llama.cpp runtime"
fi

mkdir -p "$CACHE_DIR" "$DEST_BIN"
archive="$CACHE_DIR/$(archive_name)"
extract_dir="$CACHE_DIR/extracted"
download_archive "$archive"
rm -rf "$extract_dir"
mkdir -p "$extract_dir"
tar -xzf "$archive" -C "$extract_dir" || fail "failed to extract llama.cpp runtime"

llama_server="$(find "$extract_dir" -type f -name llama-server | head -n 1)"
if [ -z "$llama_server" ]; then
  fail "llama-server not found in llama.cpp release archive"
fi

cp "$llama_server" "$DEST_BIN/llama-server"
copy_runtime_neighbors "$llama_server"
chmod +x "$DEST_BIN/llama-server"
echo "Bundled llama-server into $DEST_BIN"
