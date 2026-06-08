#!/bin/bash
set -euo pipefail

# Build and install COMRAD.app to /Applications.
# Unloads any existing launchd worker job before launching the tray app.

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$REPO_ROOT/dist"

# Build Go worker bundle if not present
if [ ! -d "$DIST/bundle-darwin-arm64/bin" ]; then
  echo "==> Running 'make build' to produce worker bundle..."
  make -C "$REPO_ROOT" build
fi

# Build the tray app
bash "$REPO_ROOT/scripts/build-tray-macos.sh"

# Stop any launchd-managed worker
launchctl unload ~/Library/LaunchAgents/com.comrad.worker.plist 2>/dev/null || true

# Install
echo "==> Installing COMRAD.app to /Applications..."
rm -rf /Applications/COMRAD.app
cp -r "$DIST/COMRAD.app" /Applications/

echo "==> Launching COMRAD.app..."
pkill -x ComradTray 2>/dev/null || true
sleep 1
open /Applications/COMRAD.app

echo ""
echo "COMRAD tray app installed. The menu-bar icon should appear shortly."
echo "The app now manages the comrad-worker process — no launchd job needed."
