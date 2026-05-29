#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
. "$ROOT/scripts/llama-runtime.env"
WORK="${COMRAD_E2E_DIR:-$ROOT/.e2e}"
DOWNLOADS="$WORK/downloads"
RUN="$WORK/run"
LLAMA_DIR="$WORK/llama"
PORT="${COMRAD_E2E_PORT:-1922}"
BASE_URL="http://127.0.0.1:$PORT"
ADMIN_TOKEN="${COMRAD_ADMIN_TOKEN:-e2e-admin-token}"
CLIENT_KEY="${COMRAD_CLIENT_API_KEY:-e2e-client-key}"
WORKER_TOKEN="${COMRAD_WORKER_TOKEN:-e2e-worker-token}"
HF_REPO="${COMRAD_E2E_HF_REPO:-sphaela/gemma-4-E2B-it-AutoRound-GGUF}"
HF_FILE="${COMRAD_E2E_HF_FILE:-gemma-4-E2B-it-Q2_K_S.gguf}"
MODEL_URL="${COMRAD_E2E_MODEL_URL:-https://huggingface.co/$HF_REPO/resolve/main/$HF_FILE}"
PROFILE_ID="${COMRAD_E2E_PROFILE_ID:-llm.chat/gemma-4-e2b/context-512}"
MODEL_ALIAS="${COMRAD_E2E_MODEL_ALIAS:-gemma4-e2b}"
MAX_TOKENS="${COMRAD_E2E_MAX_TOKENS:-4}"
CONTEXT_TOKENS="${COMRAD_E2E_CONTEXT_TOKENS:-512}"
LLAMA_CPP_URL="${COMRAD_E2E_LLAMA_CPP_URL:-$DEFAULT_LLAMA_CPP_URL}"
LLAMA_CPP_SHA256="${COMRAD_E2E_LLAMA_CPP_SHA256:-$DEFAULT_LLAMA_CPP_SHA256}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

sha256_file() {
  shasum -a 256 "$1" | awk '{print "sha256:" $1}'
}

download() {
  url="$1"
  out="$2"
  expected="${3:-}"
  mkdir -p "$(dirname "$out")"
  if [ -f "$out" ] && [ -n "$expected" ] && [ "$(sha256_file "$out")" = "$expected" ]; then
    return 0
  fi
  if [ -f "$out" ] && [ -z "$expected" ]; then
    return 0
  fi
  tmp="$out.part"
  curl -fL --continue-at - -o "$tmp" "$url"
  mv "$tmp" "$out"
  if [ -n "$expected" ]; then
    got="$(sha256_file "$out")"
    if [ "$got" != "$expected" ]; then
      echo "sha256 mismatch for $out: expected $expected got $got" >&2
      exit 1
    fi
  fi
}

yaml_post() {
  path="$1"
  file="$2"
  curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/yaml" \
    --data-binary "@$file" \
    "$BASE_URL$path"
}

cleanup() {
  if [ "${COMRAD_E2E_KEEP_RUNNING:-0}" = "1" ]; then
    return 0
  fi
  [ -f "$RUN/worker.pid" ] && kill "$(cat "$RUN/worker.pid")" >/dev/null 2>&1 || true
  [ -f "$RUN/manager.pid" ] && kill "$(cat "$RUN/manager.pid")" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

need curl
need jq
need tar
need shasum

mkdir -p "$DOWNLOADS" "$RUN" "$LLAMA_DIR"

llama_name="${LLAMA_CPP_URL##*/}"
llama_archive="$DOWNLOADS/$llama_name"

echo "Downloading llama.cpp Worker runtime: $llama_name"
download "$LLAMA_CPP_URL" "$llama_archive" "$LLAMA_CPP_SHA256"
rm -rf "$LLAMA_DIR/extracted"
mkdir -p "$LLAMA_DIR/extracted"
tar -xzf "$llama_archive" -C "$LLAMA_DIR/extracted"
llama_server="$(find "$LLAMA_DIR/extracted" -type f -name llama-server | head -n 1)"
if [ -z "$llama_server" ]; then
  echo "llama-server not found in llama.cpp release archive" >&2
  exit 1
fi
chmod +x "$llama_server"
"$llama_server" --help >/dev/null

model_path="$DOWNLOADS/$HF_FILE"
echo "Downloading GGUF model artifact: $HF_REPO/$HF_FILE"
download "$MODEL_URL" "$model_path"
model_sha="$(sha256_file "$model_path")"

rm -rf "$RUN/data" "$RUN/cache" "$RUN/bin"
mkdir -p "$RUN/data" "$RUN/cache" "$RUN/bin"
cp "$ROOT/dist/comrad-worker-darwin-arm64" "$RUN/bin/comrad-worker"
cp "$llama_server" "$RUN/bin/llama-server"
runtime_dir="$(dirname "$llama_server")"
for file in "$runtime_dir"/*.dylib "$runtime_dir"/*.so "$runtime_dir"/*.metal "$runtime_dir"/*.metallib; do
  if [ -e "$file" ]; then
    cp "$file" "$RUN/bin/"
  fi
done
chmod +x "$RUN/bin/comrad-worker" "$RUN/bin/llama-server"

echo "Starting Manager on $BASE_URL"
if [ "${COMRAD_E2E_KEEP_RUNNING:-0}" = "1" ]; then
  nohup env COMRAD_MANAGER_ADDR="127.0.0.1:$PORT" \
    COMRAD_SQLITE_PATH="$RUN/data/comrad.sqlite" \
    COMRAD_ARTIFACT_DIR="$RUN/data/artifacts" \
    COMRAD_ADMIN_TOKEN="$ADMIN_TOKEN" \
    COMRAD_CLIENT_API_KEY="$CLIENT_KEY" \
    COMRAD_WORKER_TOKEN="$WORKER_TOKEN" \
    "$ROOT/dist/comrad-manager-darwin-arm64" >"$RUN/manager.log" 2>&1 &
else
  COMRAD_MANAGER_ADDR="127.0.0.1:$PORT" \
  COMRAD_SQLITE_PATH="$RUN/data/comrad.sqlite" \
  COMRAD_ARTIFACT_DIR="$RUN/data/artifacts" \
  COMRAD_ADMIN_TOKEN="$ADMIN_TOKEN" \
  COMRAD_CLIENT_API_KEY="$CLIENT_KEY" \
  COMRAD_WORKER_TOKEN="$WORKER_TOKEN" \
  "$ROOT/dist/comrad-manager-darwin-arm64" >"$RUN/manager.log" 2>&1 &
fi
echo $! > "$RUN/manager.pid"

for _ in $(seq 1 100); do
  if curl -fsS "$BASE_URL/ready" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
curl -fsS "$BASE_URL/health" | tee "$RUN/health.txt"
curl -fsS "$BASE_URL/ready" | tee "$RUN/ready.txt"

echo "Starting Worker on the same node"
if [ "${COMRAD_E2E_KEEP_RUNNING:-0}" = "1" ]; then
  nohup env COMRAD_MANAGER_URL="$BASE_URL" \
    COMRAD_WORKER_TOKEN="$WORKER_TOKEN" \
    COMRAD_WORKER_STATE_PATH="$RUN/data/worker-state.json" \
    COMRAD_WORKER_CACHE_DIR="$RUN/cache" \
    COMRAD_WORKER_UNIFIED_BYTES="${COMRAD_WORKER_UNIFIED_BYTES:-17179869184}" \
    COMRAD_WORKER_DISK_BYTES="${COMRAD_WORKER_DISK_BYTES:-21474836480}" \
    "$RUN/bin/comrad-worker" >"$RUN/worker.log" 2>&1 &
else
  COMRAD_MANAGER_URL="$BASE_URL" \
  COMRAD_WORKER_TOKEN="$WORKER_TOKEN" \
  COMRAD_WORKER_STATE_PATH="$RUN/data/worker-state.json" \
  COMRAD_WORKER_CACHE_DIR="$RUN/cache" \
  COMRAD_WORKER_UNIFIED_BYTES="${COMRAD_WORKER_UNIFIED_BYTES:-17179869184}" \
  COMRAD_WORKER_DISK_BYTES="${COMRAD_WORKER_DISK_BYTES:-21474836480}" \
  "$RUN/bin/comrad-worker" >"$RUN/worker.log" 2>&1 &
fi
echo $! > "$RUN/worker.pid"

for _ in $(seq 1 200); do
  if curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/state" | jq -e '.nodes | length > 0' >/dev/null; then
    break
  fi
  sleep 0.2
done

cat > "$RUN/model-artifact.yaml" <<YAML
path: "$model_path"
kind: model_gguf
name: "$HF_FILE"
sha256: "$model_sha"
YAML
model_artifact="$(yaml_post /api/admin/artifacts "$RUN/model-artifact.yaml")"
model_id="$(printf '%s' "$model_artifact" | jq -r '.artifactId')"

cat > "$RUN/profile.yaml" <<YAML
profileId: "$PROFILE_ID"
model: "$MODEL_ALIAS"
kind: llm.chat
runtime:
  adapter: llama.cpp-metal
  modelArtifacts:
    - "$model_id"
  contextTokens: $CONTEXT_TOKENS
  llamaCpp:
    args: ["-ngl", "99"]
requirements:
  target: darwin-arm64-metal
  unifiedMemoryBytes: 6442450944
  diskBytes: 8589934592
warmable: true
YAML
yaml_post /api/admin/profiles "$RUN/profile.yaml" > "$RUN/profile-response.json"

cat > "$RUN/policy.yaml" <<YAML
profileId: "$PROFILE_ID"
cachedCount: 1
warmCount: 1
YAML
yaml_post /api/admin/policies "$RUN/policy.yaml" > "$RUN/policy-response.json"

echo "Waiting for assigned Worker slot to become ready..."
for _ in $(seq 1 600); do
  curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/state" > "$RUN/state.json"
  if jq -e --arg p "$PROFILE_ID" '.slots[]? | select(.profileId == $p and .state == "ready")' "$RUN/state.json" >/dev/null; then
    break
  fi
  sleep 1
done
jq -e --arg p "$PROFILE_ID" '.slots[]? | select(.profileId == $p and .state == "ready")' "$RUN/state.json" >/dev/null

echo "Checking model list through curl..."
curl -fsS -H "Authorization: Bearer $CLIENT_KEY" "$BASE_URL/v1/models" | tee "$RUN/models.json" >/dev/null
jq -e --arg p "$PROFILE_ID" '.data[] | select(.id == $p)' "$RUN/models.json" >/dev/null

cat > "$RUN/chat-request.json" <<JSON
{
  "model":"$MODEL_ALIAS",
  "stream":true,
  "max_tokens":$MAX_TOKENS,
  "temperature":0.1,
  "messages":[{"role":"user","content":"Answer with one short sentence: what is 2 plus 2?"}]
}
JSON

echo "Calling /v1/chat/completions through curl..."
curl -fsS -N -H "Authorization: Bearer $CLIENT_KEY" \
  -H "Content-Type: application/json" \
  -d "@$RUN/chat-request.json" \
  "$BASE_URL/v1/chat/completions" | tee "$RUN/chat.sse" >/dev/null

grep -Fq 'data: [DONE]' "$RUN/chat.sse"

for _ in $(seq 1 100); do
  curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/state" > "$RUN/state-final.json"
  if jq -e '.reports[]? | select(.status == "completed")' "$RUN/state-final.json" >/dev/null; then
    break
  fi
  sleep 0.2
done
jq -e '.reports[]? | select(.status == "completed")' "$RUN/state-final.json" >/dev/null

cat > "$RUN/summary.json" <<JSON
{
  "baseUrl":"$BASE_URL",
  "dashboardUrl":"$BASE_URL/",
  "managerPid":$(cat "$RUN/manager.pid"),
  "workerPid":$(cat "$RUN/worker.pid"),
  "profileId":"$PROFILE_ID",
  "modelArtifact":"$model_id",
  "runtimePath":"$RUN/bin/llama-server",
  "modelSha256":"$model_sha",
  "stateFile":"$RUN/state-final.json",
  "chatFile":"$RUN/chat.sse"
}
JSON

echo "E2E passed. Summary:"
cat "$RUN/summary.json"
if [ "${COMRAD_E2E_KEEP_RUNNING:-0}" = "1" ]; then
  echo "Manager and Worker left running for dashboard inspection."
fi
