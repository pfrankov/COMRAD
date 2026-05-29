# KServe-Inspired Improvement Plan

This document records the parts of KServe that are useful for COMRAD and turns
them into a practical improvement plan. The goal is not to copy KServe or adopt
Kubernetes. COMRAD should remain a small control plane for known machines, with
Manager-owned state, outbound Worker connections, explicit artifacts, and local
runtime processes.

## Scope

COMRAD should adopt KServe's clearer control-plane vocabulary where it helps:

- declarative workload objects with `metadata`, `spec`, and status;
- explicit runtime definitions separate from model profiles;
- visible desired vs actual state for cache, warm slots, placement, and cleanup;
- readable global runtime configuration grouped by operator concern;
- status conditions with stable reasons that explain blocked or degraded state.

COMRAD should not adopt:

- Kubernetes CRDs, namespaces, admission webhooks, controllers, or ConfigMaps;
- Knative, Istio, Gateway API, HPA, KEDA, or pod-level scaling semantics;
- arbitrary container specs in model profiles;
- annotation-driven configuration as the primary operator interface;
- cluster-wide generic orchestration beyond COMRAD's Worker model.

## Useful KServe Concepts

| KServe concept | COMRAD adaptation | Usefulness |
| --- | --- | --- |
| `InferenceService` | Borrow desired-vs-actual workload vocabulary; defer a new profile YAML shape. | Medium |
| `ServingRuntime` / `ClusterServingRuntime` | A lightweight `RuntimeSummary`; defer a full catalog until multiple real runtime adapters exist. | Medium |
| `LocalModelCache` | First-class cache intent and cache status for Worker-local model artifacts. | High |
| `status.conditions` | Stable readiness and blocked reasons for profiles, capacity, Workers, slots, and evictions. | High |
| global deployment configuration | A grouped read-only runtime config view for Manager, storage, auth, scheduler, Workers, and observability. | Medium |
| KServe node groups | Existing COMRAD Worker tags, not a new grouping system. | Low |
| `InferenceGraph` | Not planned until there is a concrete routing or fallback feature. | Low |
| transformer / explainer / logger hooks | Not useful for the current local LLM contract. | Low |
| LLM-specific scheduler features | Not useful until a runtime exposes reliable KV/prefix-cache metrics. | Low |

## Current COMRAD Shape

COMRAD already has several KServe-like pieces:

- Manager owns placement, queueing, attempts, reports, quarantine, and policy.
- Workers report capabilities, cache, slot state, tokens, telemetry, and reports.
- Model profiles are YAML-backed workload descriptions.
- Capacity policy expresses desired cached and warm copy counts.
- Worker cache cleanup removes unused artifacts after model deletion or capacity
  reduction.
- Settings shows a sanitized, read-only runtime YAML config.
- Dashboard live state arrives over WebSocket and exposes connection health.

The main gap is naming and structure. Current YAML is operationally useful but
still reads as a flat process/config dump in some places, while KServe's
`metadata/spec/status` pattern makes desired state, actual state, and controller
responsibility easier to reason about.

The second gap is actionability. A useful KServe-inspired change should help an
operator answer a concrete question: why a model is not ready, where a model is
cached, why an old artifact was not removed, or which runtime can execute a
profile. Changes that only make YAML look more like Kubernetes should be avoided.

## Usefulness Audit

| Proposal | Decision | Reason |
| --- | --- | --- |
| Cache plan and cache status | Do soon | Directly addresses stale models, hot models, and per-Worker cleanup visibility. |
| Conditions with stable reasons | Do soon | Reduces ad hoc UI logic and explains blocked/degraded state consistently. |
| Read-only runtime config grouping | Do soon | Improves Settings without changing deployment or runtime behavior. |
| Runtime catalog | Do only as a lightweight summary now | Useful for validation/debugging, but a full catalog is not worth it while COMRAD has one primary runtime. |
| ModelProfile v2 YAML | Design only, do not migrate yet | The current profile YAML is already simple; changing shape now is mostly churn unless runtime/catalog work needs it. |
| CapacityPolicy v2 YAML | Design only, do not migrate yet | Existing `cachedCount`/`warmCount` already matches the operator workflow. Improve status first. |
| Defaults/effective-values layer | Do not build as a standalone phase | COMRAD does not have enough inherited defaults to justify another config layer yet. |
| Worker groups | Do not add | Existing Worker tags and placement constraints cover this need. |
| InferenceGraph-style routing | Not planned | No current requirement for ensemble, A/B, fallback, or graph routing. |
| Transformer/logger/explainer hooks | Not planned | Adds a pipeline abstraction before there is a concrete COMRAD workflow. |
| KV-cache/prefix-aware scheduling | Not planned | Needs reliable runtime metrics and multi-runtime pressure before it can pay off. |

## Phase 1: Normalize Read-Only Runtime Config

Priority: High

Improve `/api/admin/config.yaml` so it reads like an operator-facing runtime
configuration instead of a flat Manager struct dump.

Target shape:

```yaml
version: v...
manager:
  listen: 127.0.0.1:1922
  publicUrl: http://...
storage:
  mode: auto
  backend: sqlite
  artifactDir: /...
  sqlitePath: /...
  databaseUrl: <redacted: configured>
auth:
  adminToken: <redacted: configured>
  clientBootstrapKey: <redacted: configured>
  workerEnrollmentToken: <redacted: configured>
  allowDevDefaults: false
scheduler:
  queueLimit: 32
  streamWaitSeconds: 15
  quarantine:
    threshold: 3
    seconds: 300
workers:
  connection: outboundWebSocket
  autoApprove: true
observability:
  dashboardStateStream: websocket
```

Implementation notes:

- Keep the endpoint read-only and admin-only.
- Keep all secrets and database URLs redacted.
- Keep env vars as the runtime source of truth for now.
- Update Settings display only if the changed grouping affects labels or tests.
- Add tests that assert the new grouping and secret redaction.

Acceptance criteria:

- The config is easier to scan by operator concern.
- No configured secret value can appear in the YAML.
- Existing deployments do not need config file migration.
- Operators can tell which part of the system owns each setting.

## Phase 2: Add Lightweight Runtime Summary

Priority: Medium

Introduce a small COMRAD-native runtime summary inspired by KServe
`ServingRuntime`.

This should describe installed or supported runtime adapters without turning
profiles into container definitions. It should stay read-only until COMRAD has
more than one real runtime adapter or operators need runtime defaults.

Example shape:

```yaml
apiVersion: comrad.local/v1
kind: RuntimeSummary
items:
  - metadata:
      name: llama.cpp-metal
    spec:
      adapter: llama.cpp-metal
      modelFormats:
        - gguf
      taskKinds:
        - llm.chat
      runtimeBinary:
        source: worker-installed
        command: llama-server
      managedArgs:
        - --host
        - --port
        - --model
        - --mmproj
        - --ctx-size
      status:
        availableWorkers: 2
        readySlots: 1
```

Implementation notes:

- Start as a Manager-derived read-only view.
- Do not allow operators to submit arbitrary executable paths from the dashboard.
- Make managed runtime flags visible, because they are currently validated but
  not easy to discover.
- Include supported artifact kinds, task kinds, and minimum Worker/runtime
  expectations.
- Do not build a full runtime registry until there are at least two meaningful
  runtime adapters or runtime-specific defaults.

Acceptance criteria:

- Operators can answer: "Which runtimes does this COMRAD deployment support?"
- Model profile validation has a clear future destination if adapter-specific
  rules grow.
- Future runtime adapters have one obvious place to appear in the UI/API.

## Phase 3: Design ModelProfile v2, But Do Not Migrate Yet

Priority: Low

Keep a KServe-like object shape as a design target, but do not migrate profile
YAML until it solves a concrete implementation problem. Current COMRAD profile
YAML is compact and already matches the local LLM workflow.

Target shape:

```yaml
apiVersion: comrad.local/v1
kind: ModelProfile
metadata:
  name: assistant-default
spec:
  task: llm.chat
  cost:
    compute: 0
  predictor:
    runtime:
      ref: llama.cpp-metal
      version: worker-installed
      protocol: openai-chat
    model:
      logicalName: assistant-default
      format: gguf
      artifacts:
        - sha256:<model>
        - sha256:<mmproj>
      contextTokens: 4096
      llamaCpp:
        args: ["-ngl", "99", "--threads", "6"]
    resources:
      target: darwin-arm64-metal
      unifiedMemoryBytes: 6442450944
      diskBytes: 8589934592
  warmable: true
```

Design decisions:

- Keep sha256 artifacts, not `storageUri`, as COMRAD's execution source of truth.
- Keep `runtime.ref` simple; it should reference the runtime catalog, not a
  Kubernetes object.
- Preserve model format, serving protocol, and runtime version as explicit
  execution details when they affect scheduling, reports, or debugging.
- Keep `resources` as COMRAD scheduling requirements, not pod resources.
- Keep `logicalName` separate from runtime variant and artifact identity.

Migration approach:

- First document the v2 shape and render it in the dashboard as a preview.
- Do not add parser support until runtime summary, conditions, or profile
  validation need this structure.
- If implementation starts later, prefer a deliberate hard switch or a short,
  explicit transition with tests. Do not maintain two long-lived profile formats.

Acceptance criteria:

- The profile YAML clearly separates model identity, runtime, artifacts,
  resources, and cost.
- New operators can understand a profile without reading Go structs.
- Existing placement, reports, ledger entries, and artifact deletion guards keep
  the same behavior.
- No code is changed just to make YAML look more like KServe.

## Phase 4: Improve Capacity Status Before Capacity YAML

Priority: Medium

Keep capacity separate from model profile. KServe often bundles
deployment/scaling intent into one resource; COMRAD should not do that because
local operators need to edit model identity separately from downloaded/ready
copy intent.

The useful work is not a new YAML format. The useful work is clearer status:
desired cached copies, actual cached copies, desired warm copies, actual warm
copies, blocked placements, and pending evictions.

Possible future shape, only if the current API becomes hard to explain:

```yaml
apiVersion: comrad.local/v1
kind: CapacityPolicy
metadata:
  name: assistant-default
spec:
  profileRef: llm.chat/local/context-4096
  cache:
    copies: 1
  warm:
    copies: 1
  placement:
    requireTags: []
    preferWorkers: []
    denyWorkers: []
    hardPinnedSlots: []
```

Implementation notes:

- Keep the user-facing dashboard term "Capacity".
- Preserve existing placement constraints and hard-pinned slots.
- Use existing Worker tags for machine-class targeting.
- Keep deletion and capacity reduction tied to guarded Worker cache eviction.
- Do not add `workerGroups` unless Worker tags become insufficient.

Acceptance criteria:

- Operators can distinguish "this model exists" from "this model should be
  cached or hot".
- Reducing capacity to zero still stops hot slots and removes no-longer-desired
  cached artifacts.
- Placement preview can show desired and actual state using the same vocabulary.
- The current simple capacity API is preserved unless a real workflow needs a
  richer object.

## Phase 5: Add Conditions Everywhere Operators Need Reasons

Priority: High

Adopt KServe-style conditions without copying Kubernetes condition types.

Recommended condition shape:

```yaml
conditions:
  - type: Ready
    status: "False"
    reason: NoCompatibleWorker
    message: No approved Worker has enough unified memory for this profile.
    lastTransitionTime: "2026-05-28T12:00:00Z"
  - type: Cached
    status: "True"
    reason: DesiredCopiesAvailable
  - type: Warm
    status: "False"
    reason: RuntimeStarting
```

Initial condition targets:

- `ModelProfile`: `Ready`, `Schedulable`, `ArtifactsAvailable`.
- `CapacityPolicy`: `Cached`, `Warm`, `PlacementSatisfied`.
- `Worker`: `Connected`, `Approved`, `Compatible`, `Quarantined`.
- `Slot`: `Ready`, `Assigned`, `Serving`, `Quarantined`.
- `ArtifactEviction`: `Queued`, `Blocked`, `Evicted`, `Failed`.

Implementation notes:

- Use stable machine-readable `reason` values.
- Keep human-readable `message` safe for the dashboard and logs.
- Do not include prompts, responses, raw tokens, secret values, or local private
  paths in condition messages.

Acceptance criteria:

- The dashboard can show why a model is not ready without recomputing ad hoc
  explanations in React.
- API clients can inspect blocked state programmatically.
- Conditions stay bounded and do not grow unbounded history.

## Phase 6: Promote Cache Plan And Cache Status

Priority: High

Make Worker cache management as explicit as KServe's LocalModelCache idea, but
keep it COMRAD-native.

Recommended state:

```yaml
cachePlan:
  profileRef: llm.chat/local/context-4096
  artifacts:
    - sha256:<model>
  requireTags:
    - local-metal
  desiredCopies: 1
  actualCopies: 1
  staleCopies: 0
  evictionsPending: 0
workers:
  - nodeId: mac-mini-a
    cached: true
    warm: true
    active: false
    eviction:
      status: none
```

Implementation notes:

- Keep profile deletion and capacity reduction as the trigger for cleanup.
- Keep manual per-Worker artifact eviction for selected machines.
- Use existing Worker tags when operators need cache placement by machine class.
- Show blocked evictions clearly when a Worker is offline or the artifact is
  assigned/active.

Acceptance criteria:

- Operators can answer: "Where is this model cached?"
- Operators can answer: "Why has this old model not been deleted yet?"
- Hot/cached state cannot silently drift from capacity policy without a visible
  reason.

## Phase 7: Explicitly Defer Advanced KServe Ideas

Priority: Low

Do not implement these now. Mentioning them here is useful only to prevent
accidental scope creep:

- `InferenceGraph`-like routing for future aliases, fallback, A/B routing, or
  ensemble chains.
- Transformer-like preprocessing hooks only if COMRAD needs controlled,
  auditable request shaping before runtime execution.
- Logger-like output hooks only for bounded metadata or operator-selected audit
  targets; prompts and responses must remain excluded by default.
- Explainer-like hooks are out of scope for local LLM serving unless COMRAD later
  supports model classes where explanations are a first-class feature.
- LLM-specific prefix/KV-cache-aware scheduling if COMRAD later supports a
  runtime that exposes reliable cache locality metrics.
- Separate prefill/decode scheduling only if COMRAD grows into multi-GPU serving
  where that complexity pays for itself.

Acceptance criteria:

- No current implementation work depends on these features.
- Future docs can revisit this section only when there is a concrete user
  workflow, not because KServe has the feature.

## Suggested Implementation Order

1. Regroup read-only runtime config.
2. Add bounded conditions to profiles, policies, Workers, slots, and evictions.
3. Improve cache plan/cache status API and dashboard display.
4. Add a lightweight runtime summary if it helps validation or operator
   debugging.
5. Design `ModelProfile` v2 and `CapacityPolicy` v2 only as documentation until
   the current YAML becomes limiting.
6. Revisit graph/routing and LLM-specific scheduling only after runtime metrics
   justify it.

## Implementation Status

Implemented now:

- `/api/admin/config.yaml` is grouped by operator concern and remains
  admin-only, read-only, and secret-redacted.
- Admin state includes bounded `conditions` for profiles, capacity policies,
  Workers, slots, and artifact evictions.
- Admin state includes `cachePlans` with desired, actual, stale, pending
  eviction, and per-Worker cache status.
- Admin state includes a lightweight `runtimeSummary` derived from profiles,
  Workers, and slots.
- The dashboard shows profile readiness conditions, capacity cache plans, Worker
  conditions, the runtime summary, and the full highlighted read-only YAML
  config.

Still intentionally deferred:

- `ModelProfile` v2 YAML parser support.
- `CapacityPolicy` v2 YAML parser support.
- Worker groups, InferenceGraph routing, transformer/logger/explainer hooks,
  and KV-cache-aware scheduling.

## Validation Plan

Each implementation phase should include:

- focused Go tests for API shape, redaction, validation, and condition reasons;
- dashboard tests or browser verification when operator views change;
- `make validate` before handoff;
- documentation updates in `docs/` and any affected README/API examples;
- a self-review for secrets, local paths, oversized files, and compatibility
  with Manager-owned state and outbound Worker WebSocket architecture.

## Non-Goals

- Do not require Kubernetes.
- Do not introduce a second scheduler outside the Manager.
- Do not move Worker runtime ownership into Manager-side containers.
- Do not store prompts or responses as telemetry.
- Do not make model artifacts mutable or path-based after registration.
- Do not let stale cached models remain hot without a visible condition.
