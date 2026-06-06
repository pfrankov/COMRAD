# Operations

## Validation And Build

Run the primary fast gate before handoff or deployment:

```sh
make validate
```

Run focused test/build targets when needed:

```sh
make test
make build
```

`make validate` rebuilds the shadcn/Vite dashboard, then runs guardrails and Go tests. It intentionally does not run live llama.cpp/GGUF checks.

Build outputs are written to `dist/` and include Manager and Worker binaries, a local macOS bundle, and sha256 manifests. `make build` downloads the pinned llama.cpp macOS arm64 archive into `.tools/llama.cpp` when needed, copies `llama-server` and neighboring runtime files into the macOS bundle, and ad-hoc signs the macOS bundle when building on macOS.

## Local Manager And Worker

Start the container stack with Manager, PostgreSQL, and Prometheus:

The install user must have Docker daemon access before running Compose. Verify this first:

```sh
docker ps
```

If this fails with a permission error, fix Docker access through the host's approved path before removing an existing Manager. Typical options are adding the deploy user to the `docker` group and starting a new login session, using an approved sudo path, or configuring rootless Docker prerequisites.

```sh
COMRAD_ADMIN_TOKEN='<admin-token>' \
COMRAD_CLIENT_API_KEY='<client-api-key>' \
COMRAD_WORKER_TOKEN='<worker-token>' \
COMRAD_EXTERNAL_URL='http://<manager-host>:1922' \
COMRAD_POSTGRES_PASSWORD='<postgres-password>' \
docker compose up --build
```

The Compose Manager uses `COMRAD_STORAGE_MODE=auto`. It stores state in PostgreSQL when `COMRAD_DATABASE_URL` is configured and reachable, and falls back to SQLite at `COMRAD_SQLITE_PATH` when PostgreSQL is not configured or unavailable. Set `COMRAD_STORAGE_MODE=postgres` to make PostgreSQL mandatory. Prometheus scrapes `/metrics` using `deploy/prometheus/prometheus.yml`.

Compose mounts `./imports` read-only at `/var/lib/comrad/imports` inside the Manager container for Admin API path imports. Browser uploads through **Models** are stored in `COMRAD_ARTIFACT_DIR`.

`COMRAD_EXTERNAL_URL` should be the URL Workers and operators use from outside the container host. It is used for authenticated HTTP fallback artifact URLs and generated Worker join commands. `COMRAD_CLIENT_API_KEY` is bootstrapped into a default API client for initial verification. Set `COMRAD_ENFORCE_BALANCE=true` when positive-cost profiles should require enough client balance. `COMRAD_WORKER_HEARTBEAT_TIMEOUT_SECONDS` controls when a silent Worker connection is marked offline; the default is `30`. Worker flap detection defaults to `COMRAD_WORKER_FLAP_THRESHOLD=4`, `COMRAD_WORKER_FLAP_WINDOW_SECONDS=300`, and `COMRAD_WORKER_FLAP_COOLDOWN_SECONDS=300`.

Start a local Manager from the bundle:

```sh
cd dist/bundle-darwin-arm64
COMRAD_ADMIN_TOKEN='<admin-token>' \
COMRAD_CLIENT_API_KEY='<client-api-key>' \
COMRAD_WORKER_TOKEN='<worker-token>' \
scripts/run-local-manager.sh
```

Start a foreground local Worker:

```sh
cd dist/bundle-darwin-arm64
COMRAD_MANAGER_URL='http://127.0.0.1:1922' \
COMRAD_WORKER_TOKEN='<worker-token>' \
COMRAD_WORKER_MAX_CONCURRENT_DOWNLOADS=1 \
COMRAD_WORKER_P2P_PORT=6881 \
COMRAD_WORKER_P2P_MAX_UPLOADS=8 \
COMRAD_WORKER_P2P_DOWNLOAD_TIMEOUT_SECONDS=120 \
scripts/run-local-worker.sh
```

Install the macOS Worker as a user LaunchAgent:

```sh
cd dist/bundle-darwin-arm64
COMRAD_MANAGER_URL='http://<manager-host>:1922' \
COMRAD_WORKER_TOKEN='<worker-token>' \
COMRAD_WORKER_MAX_CONCURRENT_DOWNLOADS=1 \
COMRAD_WORKER_P2P_PORT=6881 \
COMRAD_WORKER_P2P_MAX_UPLOADS=8 \
COMRAD_WORKER_P2P_DOWNLOAD_TIMEOUT_SECONDS=120 \
COMRAD_WORKER_UNIFIED_BYTES=17179869184 \
COMRAD_WORKER_DISK_BYTES=21474836480 \
scripts/install-worker-macos.sh
```

Workers default to one concurrent model artifact download per node. Increase `COMRAD_WORKER_MAX_CONCURRENT_DOWNLOADS` only when the Worker storage, network, and Manager artifact serving path can tolerate parallel model transfers. Warm assignments queued behind download pressure report `download_queued` on the assigned slot before the Worker begins the artifact transfer.
Workers also attempt public BitTorrent delivery for immutable artifacts whenever their torrent runtime starts successfully. There is no separate enable switch. `COMRAD_WORKER_P2P_PORT`, `COMRAD_WORKER_P2P_MAX_UPLOADS`, and `COMRAD_WORKER_P2P_DOWNLOAD_TIMEOUT_SECONDS` tune only the Worker networking behavior; when torrent startup, peer discovery, timeout, or final digest verification fail, COMRAD falls back to the authenticated Manager HTTP artifact path.

The installer copies the bundled `bin/llama-server` by default, ad-hoc signs installed macOS binaries, and verifies startup. Set both `COMRAD_LLAMA_CPP_URL` and `COMRAD_LLAMA_CPP_SHA256` to install a different llama.cpp archive. If a custom bundle is missing `bin/llama-server`, the installer downloads COMRAD's pinned archive as a fallback.

Generate a Worker join command from a running Manager:

```sh
curl -fsS -H "Authorization: Bearer <admin-token>" \
  http://<manager-host>:1922/api/admin/worker-join
```

This endpoint is admin-only and returns the Worker enrollment token because the token is needed to connect. Do not paste the response into logs or public tickets.

Inspect the sanitized runtime YAML config from a running Manager:

```sh
curl -fsS -H "Authorization: Bearer <admin-token>" \
  http://<manager-host>:1922/api/admin/config.yaml
```

The same read-only config is shown with highlighting in dashboard **Settings**. Tokens and database URLs are redacted; use this for effective process configuration, not for secret recovery. The YAML is grouped by operator concern: `manager`, `storage`, `auth`, `scheduler`, `workers`, and `observability`. The `scheduler` group includes `autoBalanceScaleDownCooldownSeconds`, which comes from `COMRAD_AUTO_BALANCE_SCALE_DOWN_COOLDOWN_SECONDS` and defaults to `300`, plus `workerFlap` thresholds for warm-placement suppression. The `workers.p2p` section summarizes public BitTorrent mode, runtime availability, and the effective Worker port, upload, and timeout values reported by connected Workers.

Dashboard **Settings** also shows a read-only runtime summary derived from profiles, Workers, and slots. It lists runtime adapters, supported model formats, task kinds, Worker-installed runtime command, COMRAD-managed runtime flags, available Workers, and ready slots.

## Debian Manager Deployment

Deploy a user-owned Manager process to Debian:

```sh
COMRAD_MANAGER_TARGET_HOST='<user@debian-host>' \
COMRAD_MANAGER_PUBLIC_URL='http://<manager-host>:1922' \
COMRAD_ADMIN_TOKEN='<admin-token>' \
COMRAD_CLIENT_API_KEY='<client-api-key>' \
COMRAD_WORKER_TOKEN='<worker-token>' \
make deploy-production-manager
```

The deploy script validates, builds a specific version, uploads the Linux Manager, verifies sha256 remotely, restarts the user-owned process, checks `/health` and `/ready`, and rolls back to the previous release if health checks fail.

It does not install or modify system daemon configuration.

## Smoke And Network Checks

Smoke an already running Manager:

```sh
COMRAD_BASE_URL='http://<manager-host>:1922' \
COMRAD_ADMIN_TOKEN='<admin-token>' \
make smoke
```

Run a read-only network check:

```sh
COMRAD_BASE_URL='http://<manager-host>:1922' make check-network
```

Inspect Prometheus metrics:

```sh
curl -fsS http://<manager-host>:1922/metrics
```

Metrics include dashboard state WebSocket gauges and counters for active clients, connects, broadcasts, dropped updates, write failures, last snapshot bytes, and last broadcast subscribers.
Capacity gauges are emitted per bounded `model` and `profile` label:
`comrad_capacity_desired_cached`, `comrad_capacity_actual_cached`,
`comrad_capacity_desired_warm`, `comrad_capacity_actual_warm`,
`comrad_capacity_warming`, `comrad_capacity_failed`, and
`comrad_capacity_blocked`.

Run the expensive real runtime path explicitly:

```sh
make e2e-real
```

`make e2e-real` may download the pinned llama.cpp archive and a GGUF model. Keep it outside fast local validation. The normal Worker runtime path keeps one local `llama-server` process per ready slot and proxies streaming chat chunks from that server; unexpected server exits are restarted with a bounded retry limit, and failures after the first chunk are reported to the client instead of retried. The Manager persists the first-output transition and final compute report, but individual token chunks are streamed without durable state writes. Compute reports use runtime token and generation timing fields from `llama-server` when available; older or incomplete streams fall back to local prompt/output estimates.

If a client disconnects while a chat request is queued or running, the Manager marks that task `cancelled`. Running attempts receive a Worker cancel message; queued tasks are not assigned later.

## Rollback

Rollback a local deployment from an explicit bundle:

```sh
COMRAD_ROLLBACK_BUNDLE=/path/comrad-local-darwin-arm64.tar.gz \
COMRAD_DEPLOY_DIR=/path/to/deploy \
make rollback-local
```
