---
name: comrad
description: Operate COMRAD as an admin or API user. Use for Manager/Worker setup, dashboard workflows, API calls, model registration, queue/placement/debugging, verification, recovery, and live macOS Worker flows.
---

# COMRAD Operations Skill

Use this skill when operating a COMRAD installation or checking how an admin/user workflow should be performed. For repository contribution rules and hard limits, read `AGENTS.md`. For medium-term project context, read `MEMORY.md`.

## Quick Variables

```sh
BASE_URL='http://127.0.0.1:1922'
ADMIN_TOKEN='<admin-token>'
CLIENT_API_KEY='<client-api-key>'
WORKER_TOKEN='<worker-token>'
```

`CLIENT_API_KEY` must belong to exactly one COMRAD user. The configured `COMRAD_CLIENT_API_KEY` is bootstrapped into a default API user for first verification; issue per-user keys through the dashboard or admin API for real users.

## Admin Workflows

Build and validate:

```sh
make validate
make build
```

These commands rebuild the shadcn/Vite dashboard from `web/dashboard` before Go tests/builds embed it into the Manager.
On macOS, `make build` and the Worker installer ad-hoc sign the macOS binaries and llama.cpp runtime files before verification.

Run the container stack:

```sh
docker ps
```

`docker ps` must work for the install user. If Docker daemon access fails, fix that host prerequisite before removing a working Manager.

```sh
COMRAD_ADMIN_TOKEN="$ADMIN_TOKEN" \
COMRAD_CLIENT_API_KEY="$CLIENT_API_KEY" \
COMRAD_WORKER_TOKEN="$WORKER_TOKEN" \
COMRAD_POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
docker compose up --build
```

Compose starts Manager, PostgreSQL, and Prometheus. The Manager uses `COMRAD_STORAGE_MODE=auto`: PostgreSQL is used when `COMRAD_DATABASE_URL` is configured and reachable; otherwise state falls back to SQLite at `COMRAD_SQLITE_PATH`. Prometheus scrapes `/metrics` and stores time series in its Docker volume.

Compose mounts `./imports` read-only at `/var/lib/comrad/imports` inside the Manager container. Put Manager-local model imports there, and register them with `/var/lib/comrad/imports/<file>` paths. Browser uploads are stored in the Manager artifact directory.

Run a local Manager:

```sh
cd dist/bundle-darwin-arm64
COMRAD_ADMIN_TOKEN="$ADMIN_TOKEN" \
COMRAD_CLIENT_API_KEY="$CLIENT_API_KEY" \
COMRAD_WORKER_TOKEN="$WORKER_TOKEN" \
scripts/run-local-manager.sh
```

Deploy a Debian Manager:

```sh
COMRAD_MANAGER_TARGET_HOST='<user@debian-host>' \
COMRAD_MANAGER_PUBLIC_URL='http://<manager-host>:1922' \
COMRAD_ADMIN_TOKEN="$ADMIN_TOKEN" \
COMRAD_CLIENT_API_KEY="$CLIENT_API_KEY" \
COMRAD_WORKER_TOKEN="$WORKER_TOKEN" \
make deploy-production-manager
```

Install and launch the macOS tray app (replaces launchd Worker):

```sh
make install-tray-macos
```

Or install a macOS Worker LaunchAgent (headless, no tray app):

```sh
cd dist/bundle-darwin-arm64
COMRAD_MANAGER_URL="$BASE_URL" \
COMRAD_WORKER_TOKEN="$WORKER_TOKEN" \
COMRAD_WORKER_MAX_CONCURRENT_DOWNLOADS=1 \
COMRAD_WORKER_P2P_PORT=6881 \
COMRAD_WORKER_P2P_MAX_UPLOADS=8 \
COMRAD_WORKER_P2P_DOWNLOAD_TIMEOUT_SECONDS=120 \
COMRAD_WORKER_UNIFIED_BYTES=17179869184 \
COMRAD_WORKER_DISK_BYTES=21474836480 \
scripts/install-worker-macos.sh
```

Workers default to one concurrent model artifact download per node; raise `COMRAD_WORKER_MAX_CONCURRENT_DOWNLOADS` only after confirming the node and Manager artifact path can absorb parallel model transfers. Workers also try public BitTorrent delivery for immutable artifacts whenever torrent networking starts successfully. There is no operator enable switch; tune only `COMRAD_WORKER_P2P_PORT`, `COMRAD_WORKER_P2P_MAX_UPLOADS`, and `COMRAD_WORKER_P2P_DOWNLOAD_TIMEOUT_SECONDS`, and expect authenticated Manager HTTP fallback plus final SHA-256 verification when torrent delivery is unavailable or fails.

The macOS bundle includes `bin/llama-server`. The installer copies it by default and verifies startup. Use `COMRAD_LLAMA_CPP_URL` and `COMRAD_LLAMA_CPP_SHA256` to install a different llama.cpp archive; if a custom bundle is missing `bin/llama-server`, the installer downloads the pinned archive as a fallback.

Open the dashboard:

```text
http://127.0.0.1:1922/
```

Go to **Settings** and save the admin token there. The dashboard uses the admin bearer token to get short-lived tickets for browser-only surfaces, then streams state over `/api/admin/state/ws`; use `/api/admin/state` for one-off API snapshots. The top bar shows whether that state stream is live, connecting, reconnecting, or disconnected. Dashboard pages should render from live WebSocket state by default; the **Tasks** page uses `/api/admin/tasks` only for filters and deeper pagination. Settings also shows the sanitized runtime YAML config from `/api/admin/config.yaml` as read-only highlighted text. Nodes can show warm-placement suppression when recent Worker flapping temporarily blocks new warm runtimes.

Register a model in the dashboard:

1. Ensure each Worker reports `llama.cpp-metal`; the installer copies bundled `llama-server` or an explicitly overridden archive.
2. Open **Models**.
3. Upload the main GGUF plus optional support files such as mmproj, or use Manager-local paths from `/var/lib/comrad/imports/<file>`. Browser uploads show percent and speed while the Manager receives the file.
4. Click **Add a model**, including explicit compute cost and ready/downloaded copy counts or auto-balance min/max limits, then click **Add model**.
5. Use **Edit model** later to change client model name, llama.cpp server args, budgets, cost, ready/downloaded copies, auto-balance settings, or replacement artifact paths.
6. Use **Delete model** to remove a profile and let Workers evict no-longer-desired cached files.
7. Use **Models**, **Capacity**, and **Nodes** to confirm the profile becomes warm and the slot becomes ready, or that stale cache is removed. Nodes technical details show cache cleanup records as queued, blocked, evicted, or failed.

Register API clients and keys in the dashboard:

1. Open **API clients**.
2. Find or create an API client.
3. Open **View** to inspect the client detail modal and issue an API key; the raw key appears once.
4. Use the key in client `Authorization` headers.
5. Review balances, owned nodes, consumed/produced compute, and ledger history in the same section.

## Admin API

All admin API calls use:

```sh
-H "Authorization: Bearer $ADMIN_TOKEN"
```

Interactive API reference:

```sh
open "$BASE_URL/"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/openapi.json"
```

Open **Settings** in the dashboard to launch the built-in API reference without putting the admin token into the URL.

Worker join command:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/worker-join"
```

The response contains the Worker enrollment token. Treat it like a secret and do not paste it into logs.

State:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/state"
curl -fsS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/state/ws-ticket"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/config.yaml"
```

Nodes and slots:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/nodes"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/slots"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"nodeId":"node-id","mode":"disabled","state":"disabled"}' \
  "$BASE_URL/api/admin/nodes"
```

Artifacts:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/artifacts"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @model-artifact.yaml \
  "$BASE_URL/api/admin/artifacts"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F kind=model_gguf \
  -F file=@model.gguf \
  "$BASE_URL/api/admin/artifacts/upload"
```

`model-artifact.yaml` contains `path`, `kind`, and `name`. Uploads use multipart form data and are copied into Manager artifact storage.

Delete an unused artifact:

```sh
curl -fsS -X DELETE -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/api/admin/artifacts/sha256:<artifact>"
```

Deletion is blocked while the artifact is referenced by a profile or update. Uploaded files are removed from Manager artifact storage; Manager-local imports outside that storage are only unregistered.

Profiles and policies:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/profiles"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/policies"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @profile.yaml \
  "$BASE_URL/api/admin/profiles"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @policy.yaml \
  "$BASE_URL/api/admin/policies"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"profileId":"llm.chat/local/context-4096","computeCost":5}' \
  "$BASE_URL/api/admin/profiles/compute-cost"
curl -fsS -X DELETE -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/api/admin/profiles?profileId=llm.chat/local/context-4096"
curl -fsS -X DELETE -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/api/admin/nodes/<node-id>/artifacts/sha256:<artifact>"
curl -fsS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"action":"keep"}' \
  "$BASE_URL/api/admin/nodes/<node-id>/artifacts/sha256:<artifact>"
```

Minimal `profile.yaml`:

```yaml
profileId: llm.chat/local/context-4096
model: assistant-default
kind: llm.chat
computeCost: 0
runtime:
  adapter: llama.cpp-metal
  modelArtifacts:
    - sha256:<model>
    - sha256:<mmproj>
  contextTokens: 4096
  llamaCpp:
    args: ["-ngl", "99", "--threads", "6"]
requirements:
  target: darwin-arm64-metal
  unifiedMemoryBytes: 6442450944
  diskBytes: 8589934592
warmable: true
```

Do not put model sha, tokenizer, config hash, or quantization in profile config; model identity comes from `modelArtifacts`. `llama-server` belongs to Worker installation, not model registration.

Deleting a profile removes its capacity policy and assignments, then queues eviction for Worker cache files that are no longer desired, warming, or active. Setting `cachedCount` and `warmCount` to `0` stops keeping a manual policy hot; for auto-balance policies also set min/max counts to `0`. Manual node artifact eviction is for one selected Worker and is blocked when the Worker is offline or the artifact is assigned, warming, or active there. POST the same node artifact URL with `{"action":"keep"}`, `{"action":"evict"}`, or `{"action":"evict_when_idle"}` to persist stale-cache intent or request guarded cleanup. Cache cleanup records are exposed in `/api/admin/state` as `artifactEvictions`.

API clients, keys, and compute:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/users"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"operator"}' \
  "$BASE_URL/api/admin/users"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"userId":"<user-id>","name":"client"}' \
  "$BASE_URL/api/admin/api-keys"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"userId":"<user-id>","amount":10,"reason":"top-up"}' \
  "$BASE_URL/api/admin/users/adjust-balance"
```

The compute ledger is visible in `/api/admin/state`. Positive-cost completed tasks debit the requester and credit the executing node owner when the node has `ownerUserId`. Failed attempts do not charge by default. Set `COMRAD_ENFORCE_BALANCE=true` to block positive-cost requests without enough balance; zero-cost profiles remain usable.

Capacity / placement API:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/placement"
curl -fsS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/placement/apply"
```

Auto-balance policy fields are optional and per model:

```yaml
profileId: llm.chat/local/context-4096
autoBalance: true
minCachedCount: 1
maxCachedCount: 4
minWarmCount: 1
maxWarmCount: 3
maxCachedProfilesPerNode: 0
maxWarmProfilesPerNode: 0
```

Auto-balance scales up immediately from queued, running, and smoothed recent demand. Scale-down waits for `COMRAD_AUTO_BALANCE_SCALE_DOWN_COOLDOWN_SECONDS` (default `300`) before removing desired ready/downloaded copies.

Tasks, attempts, and reports:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/api/admin/tasks?limit=50&offset=0"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/api/admin/tasks?status=queued&userId=<user-id>&limit=50"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/attempts"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/reports"
```

For LLM slowness triage, compare `timing.timeToFirstTokenMs`, `timing.generationMs`, `timing.totalAttemptMs`, and `llm.tokensPerSecond`. Worker reports prefer `llama-server` token/timing fields when they are present, so token rate reflects output generation rather than full request wall-clock time.

`/api/admin/state` returns aggregate `taskSummary` plus a bounded recent task window for dashboard responsiveness. Use `/api/admin/tasks` for full paginated history; it returns `items`, page-related `attempts`/`reports`, `total`, `limit`, `offset`, `hasMore`, and filtered `summary`.

Quarantine:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"slotId":"node-id/metal0"}' \
  "$BASE_URL/api/admin/quarantine/unban"
```

Updates:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/updates"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kind":"worker","version":"v1.2.3","artifactId":"sha256:<artifact>","sha256":"sha256:<artifact>","targetNodes":["node-id"]}' \
  "$BASE_URL/api/admin/updates/workers/apply"
```

Metrics:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/metrics"
```

Metrics include dashboard state WebSocket clients, connects, broadcasts, dropped updates, write failures, last snapshot bytes, and last broadcast subscribers. Capacity gauges use bounded `model` and `profile` labels: `comrad_capacity_desired_cached`, `comrad_capacity_actual_cached`, `comrad_capacity_desired_warm`, `comrad_capacity_actual_warm`, `comrad_capacity_warming`, `comrad_capacity_failed`, and `comrad_capacity_blocked`.

## User API

All client calls use:

```sh
-H "Authorization: Bearer $CLIENT_API_KEY"
```

List models:

```sh
curl -fsS -H "Authorization: Bearer $CLIENT_API_KEY" "$BASE_URL/v1/models"
```

Streaming chat:

```sh
curl -N -H "Authorization: Bearer $CLIENT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"assistant-default","stream":true,"messages":[{"role":"user","content":"hello"}]}' \
  "$BASE_URL/v1/chat/completions"
```

Non-streaming chat:

```sh
curl -fsS -H "Authorization: Bearer $CLIENT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"assistant-default","messages":[{"role":"user","content":"hello"}]}' \
  "$BASE_URL/v1/chat/completions"
```

Inspect or cancel a job:

```sh
curl -fsS -H "Authorization: Bearer $CLIENT_API_KEY" "$BASE_URL/v1/jobs/<task-id>"
curl -fsS -X POST -H "Authorization: Bearer $CLIENT_API_KEY" "$BASE_URL/v1/jobs/<task-id>/cancel"
```

Client disconnects cancel queued/running chat tasks. Queued tasks are not assigned later; running attempts receive a Worker cancel message.

## Verification

Health and readiness:

```sh
curl -fsS "$BASE_URL/health"
curl -fsS "$BASE_URL/ready"
```

Manager state should show at least one node, one ready slot, one profile, and queue capacity:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE_URL/api/admin/state"
```

Use the dashboard **Overview** to confirm ready slots and warm profiles. Use **Tasks** after a chat call to confirm the timeline reaches `report stored`.

Remote Manager plus local Mac Worker e2e:

1. Deploy the Manager from repository artifacts, not by editing the remote host directly.
2. Confirm `curl -fsS "$BASE_URL/health"` and `curl -fsS "$BASE_URL/ready"`.
3. Install or restart the macOS Worker LaunchAgent with the remote Manager URL.
4. Upload model artifacts or register paths that exist on the Manager host.
5. Open the dashboard and check **Nodes**, **Models**, **Capacity**, and **Tasks**.
6. Send a streaming chat request with a user API key and verify a report plus ledger state.

If direct embedded Browser navigation to a remote Tailscale address is blank while shell `curl` succeeds, first confirm the Manager is listening on `0.0.0.0`, then verify with a temporary local SSH tunnel to the same Manager service and record the reason.

## Debugging And Recovery

- `no_capacity`: inspect queue depth and ready slots in `/api/admin/state`; lower load or add a compatible ready Worker.
- `unknown_requirements`: inspect the profile requirements and runtime variants; profiles need requirements or variants to schedule.
- `artifact_digest_mismatch`: re-register the artifact from the Manager-local path and compare sha256.
- `worker_disconnected`: check Worker logs and Manager state; retry only happens before first output and avoids failed slots.
- `quarantined`: inspect counters and last failure on **Nodes**, fix the Worker/runtime cause, then unban.
- Direct remote dashboard navigation can fail in some embedded browser contexts even when `curl` works and the Manager listens on `0.0.0.0`; verify through a local SSH tunnel to the deployed Manager if needed.
