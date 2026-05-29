#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="${COMRAD_MANAGER_TARGET_HOST:?set COMRAD_MANAGER_TARGET_HOST, for example user@manager-host}"
REMOTE_DIR="${COMRAD_MANAGER_REMOTE_DIR:-/home/pavel/comrad-manager}"
PORT="${COMRAD_MANAGER_PORT:-1922}"
ADDR="${COMRAD_MANAGER_ADDR:-0.0.0.0:$PORT}"
PUBLIC_URL="${COMRAD_MANAGER_PUBLIC_URL:-http://${TARGET#*@}:$PORT}"
ARCH="${COMRAD_MANAGER_TARGET_ARCH:-amd64}"
VERSION="${VERSION:-$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || date +%Y%m%d%H%M%S)}"
BINARY="$ROOT/dist/comrad-manager-linux-$ARCH"
ADMIN_TOKEN="${COMRAD_ADMIN_TOKEN:?set COMRAD_ADMIN_TOKEN}"
CLIENT_KEY="${COMRAD_CLIENT_API_KEY:?set COMRAD_CLIENT_API_KEY}"
WORKER_TOKEN="${COMRAD_WORKER_TOKEN:?set COMRAD_WORKER_TOKEN}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

sha256_file() {
  shasum -a 256 "$1" | awk '{print $1}'
}

remote() {
  ssh "$TARGET" "$@"
}

healthcheck() {
  for _ in $(seq 1 80); do
    if curl -fsS "$PUBLIC_URL/health" >/dev/null 2>&1 &&
       curl -fsS "$PUBLIC_URL/ready" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

rollback() {
  remote "set -eu
    if [ -L '$REMOTE_DIR/previous' ]; then
      [ -f '$REMOTE_DIR/shared/manager.pid' ] && kill \$(cat '$REMOTE_DIR/shared/manager.pid') >/dev/null 2>&1 || true
      ln -sfn \"\$(readlink '$REMOTE_DIR/previous')\" '$REMOTE_DIR/current'
      cd '$REMOTE_DIR'
      set -a; . '$REMOTE_DIR/shared/manager.env'; set +a
      nohup '$REMOTE_DIR/current/bin/comrad-manager' >'$REMOTE_DIR/shared/logs/manager.log' 2>'$REMOTE_DIR/shared/logs/manager.err.log' &
      echo \$! > '$REMOTE_DIR/shared/manager.pid'
    fi"
}

need curl
need scp
need ssh
need shasum

cd "$ROOT"
make validate
make build VERSION="$VERSION"

if [ ! -x "$BINARY" ]; then
  echo "missing build artifact: $BINARY" >&2
  exit 1
fi

sha="$(sha256_file "$BINARY")"
release_dir="$REMOTE_DIR/releases/$VERSION"
remote "mkdir -p '$release_dir/bin' '$REMOTE_DIR/shared/data' '$REMOTE_DIR/shared/artifacts' '$REMOTE_DIR/shared/logs'"
scp "$BINARY" "$TARGET:$release_dir/bin/comrad-manager.tmp"
remote "set -eu
  mv '$release_dir/bin/comrad-manager.tmp' '$release_dir/bin/comrad-manager'
  chmod +x '$release_dir/bin/comrad-manager'
  got=\$(sha256sum '$release_dir/bin/comrad-manager' | awk '{print \$1}')
  [ \"\$got\" = '$sha' ]"

remote "set -eu
  umask 077
  cat > '$REMOTE_DIR/shared/manager.env' <<EOF
COMRAD_MANAGER_ADDR='$ADDR'
COMRAD_SQLITE_PATH='$REMOTE_DIR/shared/data/comrad.sqlite'
COMRAD_ARTIFACT_DIR='$REMOTE_DIR/shared/artifacts'
COMRAD_ADMIN_TOKEN='$ADMIN_TOKEN'
COMRAD_CLIENT_API_KEY='$CLIENT_KEY'
COMRAD_WORKER_TOKEN='$WORKER_TOKEN'
EOF
  [ -L '$REMOTE_DIR/current' ] && ln -sfn \"\$(readlink '$REMOTE_DIR/current')\" '$REMOTE_DIR/previous' || true
  [ -f '$REMOTE_DIR/shared/manager.pid' ] && kill \$(cat '$REMOTE_DIR/shared/manager.pid') >/dev/null 2>&1 || true
  ln -sfn '$release_dir' '$REMOTE_DIR/current'
  cd '$REMOTE_DIR'
  set -a; . '$REMOTE_DIR/shared/manager.env'; set +a
  nohup '$REMOTE_DIR/current/bin/comrad-manager' >'$REMOTE_DIR/shared/logs/manager.log' 2>'$REMOTE_DIR/shared/logs/manager.err.log' &
  echo \$! > '$REMOTE_DIR/shared/manager.pid'"

if ! healthcheck; then
  echo "healthcheck failed, attempting rollback" >&2
  rollback
  healthcheck
fi

echo "Manager deployed: $PUBLIC_URL"
echo "sha256:$sha"
