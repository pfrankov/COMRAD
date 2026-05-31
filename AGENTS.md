# COMRAD Agent Contract

COMRAD means Compute Orchestrator for Model Routing, Allocation and Dispatch.

This file is the repo-local operating contract for Codex and other coding agents. A task is complete only after implementation, review, validation, documentation, and a clear handoff.

## Canonical Sources

- Quickstart docs and copy-paste getting-started path: `README.md`.
- Detailed operator docs and API examples: `docs/`.
- Visual design language, tokens, typography, spacing, component primitives, and design do/don't rules: `DESIGN.md`.
- Current product requirements and implementation status live in the root product requirements file. It is not a historical design backlog; keep future work separate from implemented support.
- Durable project memory: `MEMORY.md`.
- Repo-local operations skill: `skills/comrad/SKILL.md`.
- Build, validation, packaging, and local operation commands: `Makefile` and `scripts/`.
- Manager, API, scheduler, dashboard serving: `internal/comrad/manager*.go`, `api.go`, `fit.go`, `store.go`.
- Dashboard frontend source: `web/dashboard`; built assets are embedded from `internal/comrad/dashboard_static`.
- Worker, runtime adapter, cache, update flow: `internal/comrad/worker.go`, `protocol.go`.
- Entity schema and public JSON fields: `internal/comrad/types.go`.
- Tests are the behavior source of truth for queueing, retry, placement, auth, artifact integrity, quarantine, and e2e hooks.

Do not duplicate source-of-truth lists of hosts, services, models, roles, endpoints, or artifacts in multiple places. Keep public aliases and APIs separate from backend tags, runtime variant ids, internal hosts, and artifact paths.

## Architecture Invariants

- Clients talk only to the Manager public API: `/v1/models`, `/v1/chat/completions`, `/v1/jobs/*`, `/health`, `/ready`.
- Admin and dashboard state are Manager-only surfaces under `/api/admin/*` and require the admin token. The dashboard consumes live Manager state over `/api/admin/state/ws`; `/api/admin/state` remains the bounded snapshot endpoint.
- The built-in OpenAPI spec and API reference are admin-only surfaces under `/api/admin/openapi.json` and `/api/admin/docs`; they must not embed configured token values.
- The read-only runtime YAML config is an admin-only surface under `/api/admin/config.yaml`; it must redact secrets and database URLs before dashboard or API exposure.
- The dashboard is an admin-only operator control center. It must explain degraded/blocked states with human-readable reasons, hide prompts/responses by default, and guard mutating actions with confirmation.
- Dashboard admin token entry belongs in **Settings** and must remain reachable before state is loaded.
- Dashboard UI work must follow `DESIGN.md` while staying within the shadcn/Vite implementation. Map visual choices to the documented ink/canvas palette, Geist typography, hairline borders, restrained elevation, and form/table/card primitives; do not introduce a parallel theme or decorative system.
- Workers connect outbound to the Manager over one WebSocket at `/api/worker/ws`; the Manager never opens inbound connections to Workers.
- Workers must heartbeat over the Manager WebSocket; missed heartbeats make the Manager mark the Worker offline, mark idle slots unavailable, and replan capacity.
- Workers download model/update artifacts only through authenticated Manager artifact URLs and only when assigned.
- Workers evict cached model artifacts only when the Manager requests it; eviction is allowed only for artifacts that are no longer desired or active on that Worker.
- Runtime executables such as `llama-server` are Worker-installed capabilities, not model artifacts. The macOS Worker bundle must include the pinned `llama-server` runtime by default; explicit archive overrides remain operator-controlled. Runtime processes stay local to the Worker and must not be exposed to the LAN.
- Admin artifact deletion must be guarded: artifacts still referenced by profiles or updates are not deletable.
- One slot runs at most one active task.
- The Manager owns placement, task state, attempts, reports, queue state, and quarantine state.
- Placement must account for aggregate Worker RAM and disk budgets across all desired model copies; auto-balanced policies may change effective desired counts only within configured per-model min/max limits.
- The Manager owns users, API key hashes, compute pricing, balances, and the append-only compute ledger; the ledger is the source of truth for balances.
- Workers report capabilities, cache, slot state, tokens, telemetry, and reports; Workers do not decide global placement.
- Logical model names are client-facing; runtime variant ids, exact model artifact sets, context, runtime parameters, and runtime adapters are execution details that must be preserved in Manager state and reports.
- Client API keys identify exactly one user. Tasks, attempts, reports, and positive-cost ledger entries must preserve the requesting user and effective profile compute cost.
- Unknown, unauthenticated, disabled, unapproved, incompatible, or quarantined Workers/slots must not receive tasks.

## Engineering Limits

Guardrails are enforced by `scripts/check-guardrails.sh` and must stay green:

- Go source files: max 500 lines.
- Go test files: max 1000 lines.
- `AGENTS.md` and `MEMORY.md`: max 200 lines each.
- `skills/comrad/SKILL.md`: max 1000 lines.
- Shell scripts: max 350 lines.
- Go functions/tests: max 140 lines.
- Go function cyclomatic complexity budget: max 10 branch tokens, counted from `if`, `for`, `switch`, `select`, and `case`.

Split files by responsibility, not arbitrary size. Reusable helpers belong in the narrowest package/file that serves at least two real call sites or removes meaningful complexity from a single high-risk path. Do not add single-use abstraction layers, registries, or frameworks. Keep public contracts boring and explicit.

## Repository Workflow

All implementation work must start from tests: add or update a failing test that describes the expected behavior, implement the minimal fix, then run `make validate`. Do not patch deployed machines manually. Fixes for client or server behavior must go through this repository, pass validation, and then be redeployed from built artifacts.

For GitHub publishing, keep generated/local directories ignored (`.agents/`, `.tools/`, `.e2e/`, `dist/`, `data/`, dashboard `node_modules/`). CI must run the same fast gate as local development: `make validate`.

## Validation Gates

Primary fast gate:

```sh
make validate
```

`make validate` builds the shadcn/Vite dashboard, then runs static guardrails and Go tests. It must not download models, run llama.cpp, run load tests, or require a live Manager/Worker.

Other gates:

```sh
make test
make build
make e2e-real
```

`make e2e-real` is intentionally expensive. It downloads/caches llama.cpp and a GGUF model, starts local Manager and Worker processes, and exercises the real streaming path. Do not add it to `make validate`.

## Operations

Local package/deploy artifact:

```sh
make deploy-local
```

Local live smoke against an already running Manager:

```sh
COMRAD_BASE_URL=http://127.0.0.1:1922 COMRAD_ADMIN_TOKEN='<admin-token>' make smoke
```

Read-only network/health check:

```sh
COMRAD_BASE_URL=http://127.0.0.1:1922 make check-network
```

Local rollback from an explicit bundle:

```sh
COMRAD_ROLLBACK_BUNDLE=/path/comrad-local-darwin-arm64.tar.gz \
COMRAD_DEPLOY_DIR=/path/to/deploy \
make rollback-local
```

Production Debian Manager deploy:

```sh
COMRAD_MANAGER_TARGET_HOST='<user@debian-host>' \
COMRAD_MANAGER_PUBLIC_URL='http://<manager-host>:1922' \
COMRAD_ADMIN_TOKEN='<admin-token>' \
COMRAD_CLIENT_API_KEY='<client-api-key>' \
COMRAD_WORKER_TOKEN='<worker-token>' \
make deploy-production-manager
```

The deploy path must validate, deploy a specific version, verify sha256 on the target, health-check the Manager, and roll back on failed health checks.

Separate fast validation, local deploy, real e2e, smoke, and load tests. Do not change system settings, network, storage layout, daemon config, power, or reboot behavior without explicit approval, reason, risk, and rollback plan.

## Security Boundaries

- Never store secrets, tokens, users, emails, prompts, responses, auth headers, sudo credentials, or private hostnames in code, logs, metrics labels, dashboard labels, docs, tests, or commits.
- Prompts and model responses are not telemetry by default.
- Metrics labels must be bounded and must not include user text, prompts, response text, artifact paths, tokens, or unbounded ids.
- Public surface is limited to client APIs and health/readiness. Admin APIs, Worker APIs, artifacts, updates, placement, attempts, reports, and dashboard state are internal/admin-only.

## Documentation Lookup

When a task asks about a library, framework, SDK, API, CLI tool, or cloud service, use `ctx7` first:

```sh
npx ctx7@latest library <name> "<question>"
npx ctx7@latest docs /org/project "<question>"
```

Do not use this for ordinary refactors, business logic debugging, code review, or general Go concepts.

## Documentation Sync

Any behavior, architecture, command, asset, design token, security boundary, or public contract change must update all affected tests, `README.md`, `docs/`, `DESIGN.md`, this `AGENTS.md`, `MEMORY.md`, `skills/comrad/SKILL.md`, scripts, and examples in the same change. Copy-paste examples must stay synchronized. Do not leave important operational knowledge only in chat.

## Self-Review Before Handoff

Before responding, review the diff for stale docs, secrets, local paths, oversized files, complexity, single-use helpers, performance regressions, validation gaps, and live-process leaks. State what changed, how it was validated, and remaining risks.
