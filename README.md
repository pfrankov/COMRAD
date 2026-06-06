# COMRAD

COMRAD means Compute Orchestrator for Model Routing, Allocation and Dispatch.

<img width="1507" height="845" alt="image" src="https://github.com/user-attachments/assets/6a54243e-f67b-44d2-9080-72b49d507e45" />


COMRAD is a small control plane for running local LLM inference on shared machines. It gives clients an OpenAI-compatible API, gives operators a dashboard, and sends each request to a Worker that has the right model ready.

Use COMRAD when you want to:

- share local Mac or LAN compute between several API clients;
- keep model files and warm `llama-server` processes on Worker machines;
- route `/v1/chat/completions` through one stable Manager URL;
- see readiness, queueing, model storage, API keys, balances, and failures in one dashboard;
- add more Worker machines without changing client code.

COMRAD is not a hosted model provider. You bring the machines, the GGUF model files, and the deployment secrets. COMRAD coordinates them.

## The Main Idea

Clients never talk to Workers. They call the Manager. Workers keep one outbound WebSocket open to the Manager, download assigned model files, warm local slots, and stream generated tokens back through the Manager.

```text
                  +----------------------+
                  | operator dashboard   |
                  | admin API            |
                  +----------+-----------+
                             |
                             v
+----------------+     +-----+-------------------+
| API clients    | --> | Manager                 |
| OpenAI API     | <-- | auth, queue, scheduler  |
+----------------+     | state, artifacts        |
                       +-----+-------------------+
                             ^
              outbound WS    |    outbound WS
            +----------------+----------------+
            |                                 |
    +-------+--------+                +-------+--------+
    | Worker         |                | Worker         |
    | model cache    |                | model cache    |
    | llama-server   |                | llama-server   |
    +----------------+                +----------------+
```

For one inference request:

1. A client sends an OpenAI-compatible request to the Manager.
2. The Manager checks the client API key and requested model name.
3. The Manager chooses a ready Worker slot, or queues the request until one is ready.
4. The Worker runs the request against its local `llama-server`.
5. Generated text streams back Worker -> Manager -> client.
6. The Manager records task state, attempts, reports, and compute ledger entries.

## Inference Looks Like This

After the Manager is running, at least one Worker is connected, and a model is ready, clients use the normal OpenAI-style endpoint:

```sh
curl -N -H "Authorization: Bearer <client-api-key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"assistant-default","stream":true,"messages":[{"role":"user","content":"hello"}]}' \
  http://<manager-host>:1922/v1/chat/completions
```

List available models with the same client key:

```sh
curl -fsS -H "Authorization: Bearer <client-api-key>" \
  http://<manager-host>:1922/v1/models
```

Health checks do not need a token:

```sh
curl -fsS http://<manager-host>:1922/health
curl -fsS http://<manager-host>:1922/ready
```

## What Runs Where

| Part | Runs on | Purpose |
| --- | --- | --- |
| Manager | Server or workstation | Public API, admin API, dashboard, queue, placement, state, artifact URLs |
| Worker | Compute machine | Model cache, local `llama-server` processes, task execution |
| Dashboard | Served by Manager | Operator UI for models, capacity, nodes, API clients, tasks, storage, updates, settings |
| API client | Your app or script | Sends OpenAI-compatible requests with a client API key |
| PostgreSQL or SQLite | Manager side | Durable Manager state |
| Prometheus | Optional Compose service | Metrics storage for `/metrics` |

`/metrics` includes bounded per-model/profile capacity gauges for desired vs actual cached and warm copies, warming copies, failed copies, and blocked placement.

The first supported Worker path is macOS Apple Silicon with bundled `llama.cpp`. The Manager can run from Docker Compose or from built binaries.

## Quick Start

You need Docker, Docker Compose, a GGUF model file, and a macOS Apple Silicon machine for the first Worker. The install user needs Docker daemon access; check it before starting:

```sh
docker ps
```

Start the Manager:

```sh
cp .env.example .env
```

Fill every value in `.env`, especially:

```text
COMRAD_ADMIN_TOKEN=
COMRAD_CLIENT_API_KEY=
COMRAD_WORKER_TOKEN=
COMRAD_POSTGRES_PASSWORD=
```

Then run:

```sh
docker compose up --build
```

The Manager is published on `COMRAD_MANAGER_PORT` or `1922`. Compose also mounts `./imports` as `/var/lib/comrad/imports` inside the Manager container for Manager-local model imports.

Open the dashboard:

```text
http://127.0.0.1:1922/
```

Open **Settings** and save the admin token. From there you can add models, connect Workers, issue client keys, and open the built-in API reference.

Build the local Worker bundle:

```sh
make build
```

Install a macOS Worker from the bundle:

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

Use `COMRAD_MANAGER_URL=http://127.0.0.1:1922` when the Manager and Worker run on the same Mac.
Workers default to one concurrent model artifact download so multiple queued cached or warm assignments do not saturate one node.
Workers also try public BitTorrent delivery for immutable artifacts whenever torrent networking starts successfully. There is no operator enable switch: tune only `COMRAD_WORKER_P2P_PORT`, `COMRAD_WORKER_P2P_MAX_UPLOADS`, and `COMRAD_WORKER_P2P_DOWNLOAD_TIMEOUT_SECONDS`. If torrent startup, peers, or verification fail, the Worker falls back to Manager HTTP and still verifies SHA-256 before caching.

## Add The First Model

The dashboard path is the shortest path for normal use:

1. Open **Models**.
2. Click **Add a model**.
3. Upload the GGUF model file. Large uploads show percentage and transfer speed.
4. Choose the client-facing model name, for example `assistant-default`.
5. Set context tokens, memory and disk budgets, compute cost, ready copies, and downloaded copies.
6. Click **Add model**.
7. Watch **Models**, **Capacity**, and **Nodes** until the model is ready.

COMRAD separates the client model name from the concrete files Workers execute. A profile can point at one or more model artifacts, and Workers refresh ready slots when that profile changes.
Uploaded and imported artifacts are public-distribution artifacts: the Manager creates one stable torrent per immutable artifact, and Workers reuse that torrent identity for model files, support files, and Worker updates.

Deleting a model from **Models** removes its profile and capacity policy, then asks online Workers to evict cached files that are no longer desired, warming, or active. Admin APIs can also keep a stale cached artifact, evict it now, or mark it for eviction when it becomes idle on one selected Worker.

More detail: [docs/model-management.md](docs/model-management.md).

## Issue A Client Key

1. Open **API clients**.
2. Create or open a client.
3. Use **View** to open the client details modal.
4. Issue an API key.
5. Use that key as `Authorization: Bearer <client-api-key>`.

Each client key belongs to exactly one API client. The Manager stores key hashes, not raw keys. Completed positive-cost requests debit the requesting client and can credit the Worker node owner.

More detail: [docs/compute-accounting.md](docs/compute-accounting.md).

## Operator Workflow

Most day-to-day work happens in the dashboard:

- **Overview**: readiness, blockers, queue pressure, and triage.
- **Models**: upload files, create, edit, and delete profiles, adjust runtime settings, and inspect linked artifacts.
- **Capacity**: choose downloaded and ready copies per model, including auto-balance min/max limits.
- **Nodes**: inspect Workers, slots, resources, readiness reasons, warm-placement suppression, and remove stale cached artifacts from a selected Worker.
- **API clients**: manage clients, keys, balances, owned nodes, and ledgers.
- **Tasks**: review request history, retries, and timelines without showing prompts by default.
- **Storage**: inspect artifact hashes and delete unused files safely.
- **Settings**: save the admin token, theme, language, runtime summary, read-only config, and API reference access.

More detail: [docs/dashboard.md](docs/dashboard.md).

## Repository Work

Run the normal quality gate before handoff:

```sh
make validate
```

Build release artifacts:

```sh
make build
```

Build outputs are written to `dist/`. The macOS Worker bundle is `dist/bundle-darwin-arm64` and includes `bin/llama-server` plus neighboring runtime files. On macOS, the bundle and installed Worker binaries are ad-hoc signed so Gatekeeper does not block local execution.

The macOS Worker bundle includes `bin/llama-server` by default. For a copy-paste Worker install command from a running Manager, call `/api/admin/worker-join` with the admin bearer token.

The dashboard uses system detection for theme and language unless the operator pins a value. Supported dashboard locales are English, Chinese, Spanish, French, Russian, German, Japanese, and Portuguese.

## More Manuals

- [docs/operations.md](docs/operations.md): local runs, deploy, smoke checks, real e2e, rollback, storage, Prometheus, and runtime overrides.
- [docs/dashboard.md](docs/dashboard.md): dashboard sections and operator workflows.
- [docs/model-management.md](docs/model-management.md): model artifacts, profile YAML, browser uploads, and Admin API examples.
- [docs/compute-accounting.md](docs/compute-accounting.md): API clients, keys, balances, compute cost, and ledger accounting.
- [docs/scheduling.md](docs/scheduling.md): queueing, retry, placement, quarantine, and updates.
