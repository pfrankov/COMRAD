#!/bin/bash
set -euo pipefail

# Build COMRAD.app — bundles the Swift tray app with comrad-worker and llama-server.
# Prerequisites: `make build` must have run to populate dist/bundle-darwin-arm64/bin/.

VERSION="${VERSION:-dev}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$REPO_ROOT/dist"
PKG="$REPO_ROOT/clients/macos"
APP="$DIST/COMRAD.app"
APP_MACOS="$APP/Contents/MacOS"
APP_RES="$APP/Contents/Resources"
APP_BIN="$APP_RES/bin"
BUNDLE_BIN="$DIST/bundle-darwin-arm64/bin"

# Require the Go bundle to exist
if [ ! -d "$BUNDLE_BIN" ]; then
  echo "error: dist/bundle-darwin-arm64/bin not found — run 'make build' first" >&2
  exit 1
fi

echo "==> Building Swift tray app (ComradTray)..."
DEVELOPER_DIR="${DEVELOPER_DIR:-}"
if [ -d "/Applications/Xcode.app/Contents/Developer" ] && [ -z "$DEVELOPER_DIR" ]; then
  export DEVELOPER_DIR="/Applications/Xcode.app/Contents/Developer"
fi
swift build -c release --package-path "$PKG"

echo "==> Assembling $APP..."
rm -rf "$APP"
mkdir -p "$APP_MACOS" "$APP_BIN"

# Copy Swift binary
cp "$PKG/.build/release/ComradTray" "$APP_MACOS/"

# Copy translation files
TRANSLATIONS_SRC="$PKG/Sources/ComradTray/Resources/translations"
TRANSLATIONS_DST="$APP_RES/translations"
if [ -d "$TRANSLATIONS_SRC" ]; then
  mkdir -p "$TRANSLATIONS_DST"
  cp "$TRANSLATIONS_SRC"/*.json "$TRANSLATIONS_DST/"
fi

# Fill in Info.plist version placeholder
sed "s/__VERSION__/$VERSION/g" "$PKG/Resources/Info.plist.in" > "$APP/Contents/Info.plist"

# App icon
cp "$PKG/Resources/COMRAD.icns" "$APP_RES/"

# Copy worker binary and runtime
cp "$BUNDLE_BIN/comrad-worker" "$APP_BIN/"
if [ -f "$BUNDLE_BIN/llama-server" ]; then
  cp "$BUNDLE_BIN/llama-server" "$APP_BIN/"
fi
# Copy any dylibs
find "$BUNDLE_BIN" -maxdepth 1 -name "*.dylib" -exec cp {} "$APP_BIN/" \;

# Sign nested binaries first (same pattern as scripts/sign-macos-bundle.sh)
find "$APP_BIN" -type f \( -perm -111 -o -name "*.dylib" \) \
  -exec codesign --force --sign - {} \;

codesign --force --deep --sign - "$APP"
codesign --verify --deep "$APP"

echo "==> Built: $APP"
