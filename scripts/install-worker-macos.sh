#!/bin/sh
set -eu

fail() {
  echo "$1" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

if [ "$(uname -s)" != "Darwin" ] || [ "$(uname -m)" != "arm64" ]; then
  fail "comrad macOS worker package requires darwin arm64"
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
. "$ROOT/scripts/llama-runtime.env"
INSTALL_DIR="${COMRAD_INSTALL_DIR:-$HOME/Library/Application Support/COMRAD}"
PLIST="$HOME/Library/LaunchAgents/com.comrad.worker.plist"
MANAGER_URL="${COMRAD_MANAGER_URL:-http://127.0.0.1:1922}"
WORKER_TOKEN="${COMRAD_WORKER_TOKEN:-dev-worker-token}"
MAX_CONCURRENT_DOWNLOADS="${COMRAD_WORKER_MAX_CONCURRENT_DOWNLOADS:-1}"
UNIFIED_BYTES="${COMRAD_WORKER_UNIFIED_BYTES:-8589934592}"
DISK_BYTES="${COMRAD_WORKER_DISK_BYTES:-21474836480}"
if [ -n "${COMRAD_LLAMA_CPP_URL:-}" ]; then
  LLAMA_CPP_URL="$COMRAD_LLAMA_CPP_URL"
  LLAMA_CPP_SHA256="${COMRAD_LLAMA_CPP_SHA256:-}"
else
  LLAMA_CPP_URL="$DEFAULT_LLAMA_CPP_URL"
  LLAMA_CPP_SHA256="${COMRAD_LLAMA_CPP_SHA256:-$DEFAULT_LLAMA_CPP_SHA256}"
fi
LLAMA_SERVER_PATH="$INSTALL_DIR/bin/llama-server"
COMRAD_TMPDIR=""

cleanup_tmp() {
  if [ -n "$COMRAD_TMPDIR" ] && [ -d "$COMRAD_TMPDIR" ]; then
    rm -rf "$COMRAD_TMPDIR"
  fi
}
trap cleanup_tmp EXIT

copy_runtime_neighbors() {
  src_dir="$(dirname "$1")"
  for file in "$src_dir"/*.dylib "$src_dir"/*.so "$src_dir"/*.metal "$src_dir"/*.metallib; do
    if [ -e "$file" ]; then
      cp "$file" "$INSTALL_DIR/bin/"
    fi
  done
}

install_llama_server_from_path() {
  src="$1"
  if [ ! -x "$src" ]; then
    return 1
  fi
  if [ "$src" != "$LLAMA_SERVER_PATH" ]; then
    cp "$src" "$LLAMA_SERVER_PATH"
  fi
  copy_runtime_neighbors "$src"
  chmod +x "$LLAMA_SERVER_PATH"
  return 0
}

verify_archive_sha256() {
  file="$1"
  if [ -z "$LLAMA_CPP_SHA256" ]; then
    fail "COMRAD_LLAMA_CPP_SHA256 is required when downloading llama.cpp runtime"
  fi
  require_cmd shasum
  want="$(printf '%s' "$LLAMA_CPP_SHA256" | sed 's/^sha256://')"
  got="$(shasum -a 256 "$file" | awk '{print $1}')"
  if [ "$got" != "$want" ]; then
    fail "llama.cpp archive sha256 mismatch: got $got, want $want"
  fi
}

download_llama_cpp_server() {
  require_cmd curl
  require_cmd tar
  COMRAD_TMPDIR="$(mktemp -d)"
  archive="$COMRAD_TMPDIR/llama.cpp-macos-arm64.tar.gz"
  extract_dir="$COMRAD_TMPDIR/extract"
  mkdir -p "$extract_dir"
  url="$LLAMA_CPP_URL"
  echo "Downloading llama.cpp server runtime: $url"
  curl -fL --retry 3 -o "$archive" "$url" || fail "failed to download llama.cpp runtime"
  verify_archive_sha256 "$archive"
  tar -xzf "$archive" -C "$extract_dir" || fail "failed to extract llama.cpp runtime"
  server_path="$(find "$extract_dir" -type f -name llama-server | head -n 1)"
  if [ -z "$server_path" ]; then
    fail "llama-server not found in llama.cpp release archive"
  fi
  chmod +x "$server_path"
  install_llama_server_from_path "$server_path" || fail "failed to install llama-server"
}

verify_llama_server() {
  if [ ! -x "$LLAMA_SERVER_PATH" ]; then
    fail "llama-server is not installed at $LLAMA_SERVER_PATH"
  fi
  DYLD_LIBRARY_PATH="$INSTALL_DIR/bin${DYLD_LIBRARY_PATH:+:$DYLD_LIBRARY_PATH}" "$LLAMA_SERVER_PATH" --help >/dev/null 2>&1 || fail "installed llama-server failed to start"
}

sign_installed_binaries() {
  if ! command -v codesign >/dev/null 2>&1; then
    return 0
  fi
  find "$INSTALL_DIR/bin" -type f \( -perm -111 -o -name "*.dylib" -o -name "*.so" \) \
    -exec codesign --force --sign - {} + >/dev/null 2>&1 || fail "failed to sign installed macOS binaries"
}

mkdir -p "$INSTALL_DIR/bin" "$INSTALL_DIR/data"
cp "$ROOT/bin/comrad-worker" "$INSTALL_DIR/bin/comrad-worker"
chmod +x "$INSTALL_DIR/bin/comrad-worker"

if [ -n "${COMRAD_LLAMA_CPP_URL:-}" ]; then
  download_llama_cpp_server
elif [ -x "$ROOT/bin/llama-server" ]; then
  install_llama_server_from_path "$ROOT/bin/llama-server" || fail "failed to install bundled llama-server"
else
  download_llama_cpp_server
fi
sign_installed_binaries
verify_llama_server

cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.comrad.worker</string>
  <key>ProgramArguments</key>
  <array><string>$INSTALL_DIR/bin/comrad-worker</string></array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>COMRAD_MANAGER_URL</key><string>$MANAGER_URL</string>
    <key>COMRAD_WORKER_TOKEN</key><string>$WORKER_TOKEN</string>
    <key>COMRAD_WORKER_STATE_PATH</key><string>$INSTALL_DIR/data/worker-state.json</string>
    <key>COMRAD_WORKER_CACHE_DIR</key><string>$INSTALL_DIR/data/cache</string>
    <key>COMRAD_WORKER_MAX_CONCURRENT_DOWNLOADS</key><string>$MAX_CONCURRENT_DOWNLOADS</string>
    <key>COMRAD_WORKER_UNIFIED_BYTES</key><string>$UNIFIED_BYTES</string>
    <key>COMRAD_WORKER_DISK_BYTES</key><string>$DISK_BYTES</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$INSTALL_DIR/data/worker.log</string>
  <key>StandardErrorPath</key><string>$INSTALL_DIR/data/worker.err.log</string>
</dict>
</plist>
EOF

launchctl unload "$PLIST" >/dev/null 2>&1 || true
launchctl load "$PLIST"
echo "COMRAD worker installed and started."
