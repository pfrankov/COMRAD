# Dashboard

The Manager serves the admin dashboard at `/`. For normal use, open **Settings** and save the admin token there before loading protected state.

```text
http://<manager-host>:1922/
```

The dashboard is implemented as a shadcn/Vite React app in `web/dashboard`. `make validate` and `make build` rebuild the static bundle and embed it into the Manager binary from `internal/comrad/dashboard_static`.

Dashboard HTML and assets are served with `Cache-Control: no-cache` so operators pick up a redeployed Manager dashboard without stale JavaScript.

After the admin token is saved, the dashboard posts to `/api/admin/state/ws-ticket` with `Authorization: Bearer <admin-token>` and receives a short-lived, one-use WebSocket ticket. The dashboard then opens `/api/admin/state/ws?ticket=<ticket>` and receives live state snapshots whenever Manager state changes. The long-lived admin token must not be sent in the WebSocket URL. `/api/admin/state` remains the bounded snapshot endpoint for scripts and API checks, but the dashboard must not poll it on an interval.

The top bar shows the Manager state stream status. **Live** means snapshots are flowing, **Connecting** is the first WebSocket attempt, **Reconnecting** means the last snapshot may be stale while the dashboard is opening a fresh ticket and socket, and **Disconnected** means no state has been received yet.

Dashboard pages render from the live WebSocket state by default. The **Tasks** page uses the live recent task window for its first history page, user summary, and timelines so new requests and reports appear without a manual refresh. Server-side `/api/admin/tasks` reads are still used for filters and deeper pagination, and those reads refresh when the live task state changes.

The Manager exposes dashboard stream health in Prometheus metrics. Watch `comrad_admin_state_ws_clients`, `comrad_admin_state_ws_connects_total`, `comrad_admin_state_ws_broadcasts_total`, `comrad_admin_state_ws_dropped_updates_total`, `comrad_admin_state_ws_write_failures_total`, and the last snapshot size/subscriber gauges when diagnosing stale UI state.

## Sections

- **Overview**: readiness, blockers, queue pressure, the COMRAD operating model, and common workflows.
- **Tasks**: live recent task history, paginated/filterable historical reads, requesting-user breakdown, and recent timelines without prompt or response content.
- **Models**: upload model files with percentage and speed feedback, edit or delete client-facing models, inspect linked files in a modal, tune llama.cpp server args, compute cost, and ready/downloaded copy counts.
- **Capacity**: choose downloaded and ready copies per model; advanced routing exposes tags, preferred nodes, denied nodes, and hard-pinned slots. The cache plan shows desired, actual, stale, and pending cleanup state per Worker.
- **Nodes**: worker machines, ready slots, resources, cache entries, cache cleanup queue/results, and Manager-provided conditions explaining whether a node or slot is connected, ready, compatible, assigned, serving, or quarantined.
- **Storage**: artifact kind, sha256, size, verification state, cached nodes, model usage, and guarded deletion for unused artifacts. P2P delivery is not implemented; current delivery is Manager-to-Worker over authenticated artifact URLs.
- **API clients**: client search, raw key lookup, and modal client details with API key status, compute balance, consumed/produced compute, owned nodes, top-ups, and ledger history.
- **Updates**: Worker software updates, pending rollout records, impact preview, and failures. Model edits do not use updates.
- **Settings**: admin token storage reachable before protected state loads, component purpose map, safety posture, links to operational settings, the built-in API reference, a read-only runtime adapter summary, and a highlighted runtime YAML config from `/api/admin/config.yaml`.

The sidebar is grouped by operator intent: **Operate** for readiness and request review, **Serve** for model/capacity/node/storage work, and **Govern** for API clients, updates, settings, and other control-plane responsibilities. On narrow screens the same sections collapse into a horizontal navigation strip so the working page starts near the top of the viewport.

## Theme and Language

The dashboard defaults to system theme and system language. Operators can pin **Light**, **Dark**, or **System** theme in **Settings**; the choice is stored in the browser. The language selector also defaults to the browser language and can be fixed per browser. The initial system language is resolved from the primary browser language (`navigator.languages[0]` or `navigator.language`) first, then the browser `Intl` locale, then the dashboard request `Accept-Language` as a server-provided fallback. COMRAD does not scan secondary browser languages when the primary browser language is supported, so `en-US, en, ru-RU, ru` resolves to English unless the operator pins Russian. Supported dashboard locales are English, Chinese, Spanish, French, Russian, German, Japanese, and Portuguese. Missing runtime values fall back to English.

The dashboard build runs `npm run i18n:validate` before TypeScript and Vite. The validator scans used translation keys, checks every supported locale file, and fails the build when any locale is missing a value, contains an empty value, or still carries an unused stale key.

## API Reference

The Manager serves an admin-only OpenAPI 3.1 document and a compact built-in reference:

```text
http://<manager-host>:1922/api/admin/docs
```

Open it from **Settings** after saving the admin token. The dashboard requests a short-lived ticket for the docs page instead of putting the long-lived admin token into the URL. The raw spec is available at `/api/admin/openapi.json` with the admin bearer token. It describes client, admin, admin state WebSocket ticket issuance, health, metrics, Worker WebSocket, Worker join, and artifact delivery endpoints without embedding configured tokens.

Use `Cmd+K` to search nodes, models, recent tasks, storage, and common actions. Full task history is intentionally not loaded into global dashboard state.

## Common Admin Workflows

Connect a Worker:

1. Set `COMRAD_EXTERNAL_URL` on the Manager when the Worker is not on the same host or reaches the Manager through a proxy.
2. Fetch `/api/admin/worker-join` with the admin bearer token.
3. Run the returned command on a macOS Apple Silicon Worker from the unpacked bundle directory.
4. Confirm the Worker appears under **Nodes** with `llama.cpp-metal` in its reported runtime adapters.
5. Assign node ownership later only when produced-compute credits should accrue to an API client.

Register a model:

1. Go to **Models**.
2. Click **Add a model**.
3. Upload the model files and watch percent/speed progress for long uploads.
4. Set client model name, context, memory and disk budgets, compute cost, ready/downloaded copy counts, and optional llama.cpp server args.
5. Click **Add model** and confirm the mutation.
6. Confirm the model warms under **Models**, **Capacity**, and **Nodes**.

Edit a model:

1. Go to **Models**.
2. Click **Edit model** on the existing model row.
3. Review linked files, then update client model name, llama.cpp server args, budgets, cost, ready/downloaded copy counts, or model files.
4. Leave uploads empty to keep the existing model artifacts.
5. Click **Save changes** and confirm the mutation.

The dashboard writes minimal YAML profile config. It does not ask for a runtime archive, model sha, tokenizer, config hash, or quantization because `llama-server` belongs to Worker installation and model identity comes from artifact-level properties. Saving a model edit increments the profile version, so Workers restart affected local servers before serving with the new settings. COMRAD owns host, port, model path, mmproj, context size, API key, and TLS flags for `llama-server`; profile args are only for safe runtime tuning.

Delete a model or stale Worker cache:

1. Go to **Models** and click **Delete model** to remove a model from COMRAD.
2. Confirm the destructive action. The Manager removes the profile and capacity policy, then queues eviction for Worker cache entries that are no longer desired or active.
3. For one machine only, open **Nodes**, click **Technical details**, and use **Remove from worker** on a cached artifact.
4. Check **Worker conditions** and **Cache cleanup** in the same details dialog for queued, blocked, evicted, or failed cleanup records, including artifacts that have already disappeared from the current cache list.
5. COMRAD blocks removal while the artifact is still assigned, active, or the Worker is offline.

Register an API client and key:

1. Open **API clients**.
2. Find the client by name, id, key id, key label, or raw key lookup, or create one from the page action.
3. Open **View** for that client; details open in a modal.
4. Edit the client name/status when needed, or issue an API key.
5. Use that key as the client `Authorization` bearer token.

The dashboard shows only key status after issuance. The raw key is returned once; raw key lookup hashes the pasted key server-side and returns the matching client without exposing stored hashes.

Review compute:

1. Open **API clients**.
2. Search for the client, then click **View** to open the detail modal.
3. Check balance, consumed compute, produced compute, API keys, and owned nodes.
4. Inspect that client's ledger entries for task, attempt, report, profile version, amount, direction, and reason.
5. Use **Top up balance** only for admin top-ups or corrections, then confirm the ledger mutation.

Adjust capacity:

1. Open **Capacity**.
2. Select the model.
3. Set downloaded copies and ready copies, or enable **Auto balance** and set min/max limits.
4. Open advanced routing only when tags, preferred nodes, denied nodes, hard-pinned slots, or per-Worker model limits are needed.
5. Review the effective desired counts, demand signal, current capacity state, and cache plan.
6. Apply now or run the planner, then confirm the mutation.

**Nodes** shows remaining planned memory and disk for each Worker. Offline
Workers show the last-seen time, and offline Workers are excluded from placement
until they reconnect and report fresh state.

Handle quarantine:

1. Open **Overview** or **Nodes**.
2. Review the quarantine reason, counters, last failure, and affected slot.
3. Use **Unban slot** only after the node has been investigated.
4. The slot becomes schedulable only if readiness and admission checks pass.

Review a request:

1. Open **Tasks**.
2. Filter by task id, status, user id, or profile id.
3. Inspect the timeline from queued through report storage.
4. Compare first-token latency, generation time, and token rate in the report.
5. For retries, check failed slot, failure reason, and excluded retry candidates.

LLM reports prefer runtime token and generation timing values from `llama-server` when available. `totalAttemptMs` remains the full Worker attempt time, while `generationMs` and `tokensPerSecond` describe model output generation.

The Overview state contains aggregate task counts and only a bounded recent task window. Use `/api/admin/tasks?limit=50&offset=0` or the **Tasks** page for paginated history when the Manager has a large task table.

Prompt and response content is hidden by default.
