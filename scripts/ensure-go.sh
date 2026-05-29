#!/bin/sh
set -eu

DEFAULT_VERSION="1.26.3"
VERSION="${COMRAD_GO_VERSION:-}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TOOLS="$ROOT/.tools"
GOROOT="$TOOLS/go"

if command -v go >/dev/null 2>&1; then
  exit 0
fi

if [ -z "$VERSION" ]; then
  latest="$(curl -fsSL 'https://go.dev/VERSION?m=text' 2>/dev/null | sed -n '1p' | sed 's/^go//' || true)"
  VERSION="${latest:-$DEFAULT_VERSION}"
fi

if [ -x "$GOROOT/bin/go" ] && "$GOROOT/bin/go" version | grep -q "go${VERSION} "; then
  exit 0
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64) arch="amd64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
case "$os" in
  darwin|linux) ;;
  *) echo "unsupported OS for bootstrap Go: $os" >&2; exit 1 ;;
esac

archive="go${VERSION}.${os}-${arch}.tar.gz"
url="https://go.dev/dl/${archive}"
fallback_url="https://dl.google.com/go/${archive}"
mkdir -p "$TOOLS"
tmp="$TOOLS/${archive}"
echo "Downloading Go ${VERSION} from ${url}" >&2
if ! curl -fsSL "$url" -o "$tmp"; then
  echo "Retrying Go download from ${fallback_url}" >&2
  curl -fsSL "$fallback_url" -o "$tmp"
fi
rm -rf "$GOROOT"
tar -C "$TOOLS" -xzf "$tmp"
"$GOROOT/bin/go" version
