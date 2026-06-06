# COMRAD Memory

This file is medium-term agent memory for future COMRAD work sessions.

It stores important project context that is easy to lose between sessions: non-obvious decisions, unexpected findings, where problems occurred, temporary concerns, and items that should be reviewed during later reflection. It must not duplicate `AGENTS.md`, become a changelog, act as a task list, or restate rules already enforced by guardrails.

## Durable Context

- The acceptance path is macOS Apple Silicon Worker execution with a small GGUF model. Debian Manager deployment exists, but Windows/Linux/NVIDIA Worker support is not required for acceptance.
- Manager state owns placement, queueing, attempts, reports, and quarantine. Workers report capabilities and execute assigned artifacts.
- Logical model identity is client-facing. Runtime variants preserve exact artifact, sha256, context, runtime parameters, and runtime adapter in state and reports.
- Client API keys now resolve to users. Compute balance is cached, but the append-only compute ledger is the source of truth for consumed/produced/admin-adjusted compute.
- The dashboard source is a Vite React/shadcn app under `web/dashboard`; `make validate` and `make build` rebuild static assets into `internal/comrad/dashboard_static` for Go embedding.
- Admin `/api/admin/state` must stay bounded for large histories: it exposes aggregate `taskSummary` plus a recent task window. Use `/api/admin/tasks` for paginated/filterable task history with page-related attempts/reports.
- Dashboard live state is pushed through `/api/admin/state/ws` after the dashboard gets a short-lived one-use ticket from `/api/admin/state/ws-ticket`; avoid reintroducing interval polling or sending the long-lived admin token in the WS URL.
- Dashboard WebSocket reconnect state is operator-visible in the top bar: connecting, live, reconnecting, disconnected, and the last received state time in the tooltip.
- The default **Tasks** view must render from live WebSocket state so request history, summaries, and timelines update immediately; `/api/admin/tasks` is for filters and deeper pagination and should refresh when task state changes.
- Dashboard WebSocket health and capacity plan drift are exported in Prometheus metrics. Capacity gauges use bounded logical model/profile labels for desired and actual cached/warm copies, warming, failed, and blocked counts.
- Dashboard HTML and assets intentionally use `Cache-Control: no-cache`; stale JS was observed after redeploy when the in-app browser reused a same-named Vite asset.
- Dashboard model operations are centered on **Models**. **Add a model** uploads or imports model artifact sets and creates profile/policy; **Edit model** updates the client model name, args, budgets, cost, ready/downloaded copy counts, or replacement artifacts. **Capacity** is the user-facing name for placement policy.
- Capacity policies can opt into auto-balance with min/max downloaded and ready counts. The planner derives effective demand from queued, running, and smoothed recent requests, scales up immediately, waits for the scale-down cooldown before removing desired copies, and plans globally across Worker RAM/disk so scarce large models are placed before smaller extras.
- Worker flapping is tracked from disconnect, reconnect, and heartbeat-expiry events. Flapping Workers remain visible but are temporarily suppressed for new warm placement until the configured cooldown expires.
- Workers default to one concurrent model artifact download per node. Extra cached-only or warm assignments queue on the Worker; warm slots report `download_queued` before artifact transfer begins. When available, Workers first try public BitTorrent delivery for immutable artifacts and fall back to Manager HTTP when torrent delivery is unavailable, slow, or fails verification.
- Deleting a model removes its profile, capacity policy, and assignments, then queues guarded Worker cache eviction for artifacts that are no longer desired, warming, or active. Admin cache actions on a selected Worker can keep/pin stale cache, evict it now, or persist evict-when-idle intent.
- Worker cache cleanup records are persisted as `artifactEvictions` and shown in Nodes technical details so operators can see queued, blocked, evicted, and failed cleanup even after an artifact leaves the current cache list.
- Storage deletion is intentionally guarded: artifacts referenced by profiles or updates cannot be deleted; uploaded Manager-owned files are removed from disk, while external Manager-local imports are only unregistered.
- The Manager serves an admin-only OpenAPI 3.1 spec at `/api/admin/openapi.json` and a built-in API reference at `/api/admin/docs`; neither should embed configured token values.
- The Manager serves an admin-only sanitized runtime YAML config at `/api/admin/config.yaml`; Settings renders it read-only with highlighting, and secrets/database URLs must stay redacted.
- Dashboard navigation is scenario-oriented: **Operate** covers readiness/tasks, **Serve** covers models/capacity/nodes/storage, and **Govern** covers users/updates/settings. Overview and Settings should explain component purpose, not just expose raw state.
- Admin token entry lives in **Settings**, which must remain accessible before a valid token has loaded protected Manager state.
- Dashboard UI and design-token changes should treat `DESIGN.md` as the visual source of truth while keeping the implementation in shadcn/Vite.
- Worker LLM reports prefer `llama-server` runtime token/timing fields when available, so `tokensPerSecond` describes output generation rather than full request wall-clock time.
- Token streaming is intentionally not persisted per chunk. The Manager records first output once, streams later chunks directly to the active request, and persists the final compute report to keep long generations from writing the whole state on every token.
- New Manager runtime storage is SQL-backed: `COMRAD_STORAGE_MODE=auto` uses PostgreSQL from `COMRAD_DATABASE_URL` when reachable, otherwise SQLite at `COMRAD_SQLITE_PATH`. Direct `.json` store paths exist only for legacy compatibility tests.
- The user-facing default Manager port is `1922`; Docker internals still listen on `8080` inside the compose network.
- README is intentionally a short getting-started path. Detailed operations, model, scheduling, dashboard, and accounting material belongs under `docs/`.
- The root product requirements file should describe real implementation status and current requirements, not treat future Windows/Linux/NVIDIA/payment work as already supported.

## Operational Findings

- `make validate` is the cheap confidence gate. Live llama.cpp/GGUF checks belong in explicit e2e or smoke workflows, not the fast gate.
- Debian Manager deployment uses `scripts/deploy-manager-debian.sh`; it starts a user-owned process and does not install systemd units.
- macOS Worker LaunchAgent install carries Manager URL, Worker token, and resource budgets; llama.cpp support is not env-gated.
- `make build` bundles the pinned macOS arm64 `llama-server` runtime into `dist/bundle-darwin-arm64/bin`; installer downloads only for explicit overrides or malformed custom bundles.
- macOS bundle and install flows ad-hoc sign `comrad-worker`, `llama-server`, and neighboring runtime libraries before verification; unsigned fresh copies were killed by Gatekeeper before Worker startup.
- Real llama.cpp execution uses a Worker-installed `llama-server` plus assigned model artifacts. Runtime archives are not part of profile/model registration, and ready slots own local server processes.
- Model artifact paths in dashboard/API registration are Manager-local paths, but the dashboard can also upload model files into Manager artifact storage.
- In embedded Browser runs, direct navigation to the remote Tailscale Manager rendered blank while `curl` succeeded and Manager logs showed `0.0.0.0:18080`. A local SSH tunnel to the same Manager allowed dashboard verification.
- This workspace was not initialized as a Git repo during publish-prep work; prepare files for GitHub without assuming `git status` is available.
- One Debian target had Docker CLI/Compose installed but the deploy user could not access the Docker daemon, and rootless Docker prerequisites were missing. Confirm `docker ps` works before removing a working Manager for Compose reinstall.
- Containerized Manager imports are container-local: Compose mounts host `./imports` read-only as `/var/lib/comrad/imports`, and path-based model registration must use the container path.

## Review Later

- Keep dashboard implementation in the React source tree rather than reintroducing a large inline Go template.
- Revisit whether the dashboard model registration form should support richer runtime variants beyond the macOS Metal quick path.
- If task history grows toward millions of rows, replace the current snapshot-style Store with row-normalized task/attempt/report tables. The dashboard/API are bounded now, but persistence still loads Manager state as a snapshot.
- Review production process management if COMRAD needs a system service; current deploy intentionally avoids daemon configuration changes.
