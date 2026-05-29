# COMRAD Product Requirements And Status

COMRAD means Compute Orchestrator for Model Routing, Allocation and Dispatch.

## Product Summary

COMRAD is a self-hosted compute orchestrator for local model workloads. A central Manager owns API access, placement, task state, user identity, accounting, artifacts, and the dashboard. Workers connect outbound, report capabilities, cache verified artifacts, warm assigned profiles, and execute tasks on local runtime slots.

The core execution path is:

```text
Client request -> User API key -> Task -> Workload Profile -> Runtime Variant -> Slot -> Attempt -> Compute Report -> Ledger
```

The current implementation is optimized for one practical acceptance path: a Manager plus a macOS Apple Silicon Worker running a small GGUF model through llama.cpp Metal.

## Current Supported Path

- Manager: Go server, dashboard, OpenAI-compatible chat API, admin API, Worker WebSocket API.
- Storage: SQL store with PostgreSQL when configured and reachable, SQLite fallback in `auto` mode.
- Metrics: Prometheus-compatible `/metrics`; Prometheus is provided by Docker Compose.
- Dashboard: React/shadcn operator UI embedded into the Manager binary.
- Worker: native Go macOS arm64 Worker installed through launchd.
- Runtime target: `darwin-arm64-metal`.
- LLM runtime: Worker-installed llama.cpp Metal through `llama-server`; `make build` bundles the pinned macOS arm64 server, and the macOS installer installs and verifies it by default.
- Model artifacts: one or more model files registered or uploaded by sha256, such as a primary GGUF plus optional mmproj.
- Client API: `/v1/models`, `/v1/chat/completions`, `/v1/jobs/*`.
- Auth: admin token for admin APIs; client API key for user requests; Worker enrollment token for Workers.

## Explicit Non-Goals For The Current Release

- Windows Workers.
- Linux GPU Workers.
- NVIDIA/CUDA runtime execution.
- ComfyUI or arbitrary Docker jobs.
- Distributed execution of one task across multiple Workers.
- Multiple active tasks on one slot.
- Public marketplace, paid checkout, or payment provider integration.
- High-availability Manager clustering.
- P2P artifact delivery as a required path.

The data model and terminology should keep these future directions possible, but the implementation must not pretend they are already supported.

## Product Requirements

### Manager

The Manager must:

- start locally and in the Docker Compose server stack;
- initialize database schema automatically;
- expose `/health` and `/ready`;
- serve the dashboard;
- expose an admin-only Worker join command using the configured external Manager URL and Worker enrollment token;
- expose an admin-only sanitized runtime YAML config with secrets redacted;
- authenticate admin, client, and Worker requests;
- persist state across restart;
- store Nodes, Slots, Artifacts, Profiles, Policies, Tasks, Attempts, Reports, Users, API keys, and compute ledger entries;
- bound task queues and return `no_capacity` on overflow;
- schedule only compatible ready slots;
- preserve exact runtime variant, artifact, context, runtime params, and profile version in attempts and reports.

### Worker

The Worker must:

- connect outbound to `/api/worker/ws`;
- authenticate with the Worker token;
- report node id, OS, arch, version, budgets, and `darwin-arm64-metal`;
- report only runtime adapters that are actually installed and executable;
- send full state after reconnect;
- verify assigned artifact sha256 before use;
- use only assigned artifacts;
- keep the runtime local to the Worker host;
- keep a local `llama-server` process alive for each ready slot and selected profile version;
- run at most one active task per slot;
- reconnect after Manager restart.

Unknown, unauthenticated, disabled, incompatible, or quarantined Workers must not receive tasks.

### Admin State Stream

The Manager must expose admin dashboard state as both a bounded snapshot and a live stream:

- `/api/admin/state` returns the current bounded snapshot for scripts and API checks;
- `/api/admin/state/ws-ticket` issues a short-lived one-use ticket using the admin bearer token, so the long-lived admin token is not sent in the WebSocket URL;
- `/api/admin/state/ws` accepts that ticket, sends an initial snapshot after authentication, and pushes a fresh snapshot whenever Manager state changes;
- the dashboard must use the WebSocket stream instead of interval polling;
- Prometheus metrics must expose dashboard WebSocket clients, connects, broadcasts, dropped updates, write failures, and snapshot size.

### Workload Profiles And Runtime Variants

A Workload Profile describes a client-visible workload. For LLM chat, it must support:

- logical model identity and aliases;
- runtime-specific variants;
- exact model artifact ids and sha256 values;
- selected Worker runtime adapter and runtime parameters;
- context tokens;
- runtime adapter;
- explicit resource requirements;
- warmable flag;
- optional llama.cpp runtime parameters;
- manually configured `computeCost`, defaulting to `0`.

The configuration format is YAML. Fields that can be derived from the selected artifact, such as model sha and concrete quantization, must not be duplicated in profile config.

When a client requests a logical model, the Manager must select a compatible runtime variant for the selected slot while keeping the concrete artifact and runtime details auditable.

### Artifact Integrity

Artifacts are immutable and content-addressed by sha256. The Manager uploads or registers model/update artifacts from Manager-local paths. Workers must reject corrupted artifacts and must not run an artifact whose digest does not match Manager metadata.

When a profile is deleted or capacity no longer desires an artifact on a Worker, the Manager must queue guarded Worker cache eviction. Workers must stop stale warm runtimes before deleting the cached file and must refuse eviction while an artifact is serving active work. Cleanup records must remain visible as queued, blocked, evicted, or failed.

### Scheduling, Queueing, Retry, And Quarantine

The Manager must:

- build profile-by-slot fit results;
- explain why a profile does not fit a slot;
- schedule only `ready` slots;
- never assign two active tasks to one slot;
- queue requests when no compatible ready slot is available;
- assign queued work when compatible Workers become ready;
- expose queue state through admin state and dashboard APIs;
- retry only before first output;
- fail the stream after first output failure;
- exclude previously failed slots from automatic retry for the same task;
- quarantine Workers or slots that repeatedly report ready but fail execution;
- keep quarantined slots out of scheduling;
- expose quarantine reason, counters, last failure, and expiration;
- allow guarded manual unban followed by health/admission checks.

### Streaming LLM Flow

The required happy path is:

```text
Manager starts -> Worker connects -> capabilities reported -> profile created -> artifact verified -> profile warmed -> slot ready -> streaming chat works -> exact slot selected -> report stored -> ledger updated -> dashboard shows lifecycle
```

Tokens must stream local `llama-server` to Worker to Manager to Client. Cancellation must reach the Worker and produce a Compute Report. Runtime failures may be retried only before the first output chunk is sent.

Queued client requests must also be cancellable when the client disconnects or calls the job cancel endpoint, and cancelled queued work must not be assigned later.

### Users And Compute Accounting

Every client request must be associated with exactly one user through the `Authorization` API key. Missing or invalid client API keys are rejected.

The Manager must support:

- user registration;
- API key issue/revoke;
- optional node ownership by user;
- compute balance;
- append-only compute ledger;
- admin balance adjustment;
- future purchase entries without requiring payment integration now.

When a successful positive-cost task completes:

- debit the requesting user;
- credit the owner of the Worker node when ownership exists;
- store effective cost from the profile version used at execution time;
- link ledger entries to task, attempt, node, slot, profile, and report.

Failed attempts must not charge by default. Zero-cost profiles must still record requesting user identity, but do not need balance-changing ledger entries.

Balance enforcement is configurable. When enabled, positive-cost requests require sufficient balance; zero-cost requests remain allowed with zero balance.

### Dashboard

The dashboard is an admin-only control center. It must answer:

1. Is the cluster ready to serve tasks?
2. If not, what exactly is wrong?
3. What safe action can the admin take now?

It must include:

- Overview;
- Nodes;
- Workload Profiles;
- Placement;
- Tasks;
- Artifacts;
- Updates;
- Users;
- Settings;
- command palette/search.

Every blocked or degraded state must have a visible human-readable reason. Prompt and response content must not be shown by default.

The dashboard must expose:

- logical model identity and concrete runtime variant/artifact;
- upload progress for long model-file uploads, including percentage and speed;
- model deletion and selected-Worker stale cache removal;
- stale cache cleanup history and status per Worker;
- desired vs actual placement;
- queue depth and queued task reasons;
- task and attempt timelines;
- retry and excluded-slot behavior;
- quarantine state and manual unban action;
- update state;
- user balances, API key status, consumed/produced compute, ledger history, and node ownership through modal details rather than side panels.

Admin token entry must live in **Settings**, and **Settings** must be reachable before protected state has loaded.

Settings must show the full sanitized runtime YAML config as read-only highlighted text. Editing runtime config from the dashboard is not supported.

### Operations

The project must provide:

- `make validate` as the fast validation gate;
- `make build` for release artifacts, the bundled macOS `llama-server` runtime, and sha256 manifests;
- Dockerfile and Docker Compose server stack;
- macOS Worker bundle and install script;
- Debian Manager deployment script;
- local smoke, network check, rollback, and real llama.cpp e2e scripts.

`make validate` must remain cheap: it builds the dashboard, runs guardrails, and runs Go tests. It must not download models, start live runtimes, or require a live Manager/Worker.

### Security And Privacy

- Secrets must not be stored in code, docs, logs, metrics labels, dashboard labels, or commits.
- API keys are stored hashed.
- Admin APIs and dashboard state are admin-only.
- Worker APIs are Worker-token protected.
- Prompts and responses are excluded from telemetry by default.
- Metrics labels must be bounded and avoid user text, prompts, responses, artifact paths, auth headers, and unbounded ids.

## Acceptance Criteria

- `make validate` passes.
- `make build` produces Manager, macOS Worker, a bundle with `llama-server`, manifests, and sha256 metadata.
- Docker Compose starts Manager, PostgreSQL, and Prometheus.
- Manager falls back to SQLite only according to documented storage mode behavior.
- `/health`, `/ready`, `/metrics`, dashboard, admin API, client API, and Worker WebSocket are reachable in their intended surfaces.
- A macOS Apple Silicon Worker connects, reports `darwin-arm64-metal`, verifies artifacts, warms a profile, and reaches ready state.
- The Worker install path can set up the required macOS llama.cpp runtime, and Managers do not schedule llama.cpp profiles to Workers that did not report the adapter.
- A streaming chat request with a valid user API key succeeds end to end.
- Task, attempt, report, selected profile version, selected runtime variant, selected artifact, requesting user, and compute ledger behavior are auditable.
- Queueing, retry-before-first-output, failure-after-first-output, cancellation, and quarantine behavior are covered by tests.
- Dashboard shows readiness, degradation reasons, placement, tasks, artifacts, updates, users, balances, and ledger state.

## Future Work

- Linux and Windows Workers.
- NVIDIA/CUDA runtime adapters.
- Additional workload kinds.
- P2P artifact distribution.
- Paid compute purchase flow.
- Stronger production process supervision.
- High-availability Manager design.
