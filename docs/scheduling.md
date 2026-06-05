# Scheduling, Retry, Quarantine, And Updates

## Queueing

The Manager has a bounded queue. If no compatible ready slot exists, compatible requests wait in the queue until a Worker slot becomes ready. If the queue is full, the client receives `no_capacity`.

Queue state is visible through:

```sh
curl -fsS -H "Authorization: Bearer <admin-token>" \
  http://<manager-host>:1922/api/admin/state
```

The dashboard shows queue depth on **Overview** and task lifecycle on **Tasks**.

## Capacity / Placement

The dashboard calls this **Capacity** because operators usually decide how many model copies should be downloaded and ready. The API and internal data model still call it placement policy. The policy controls desired cached and warm counts, optional auto-balance limits, tags, preferred nodes, denied nodes, and hard-pinned slots. The Manager builds profile-by-slot fit results and assigns profiles only to compatible slots.

Workers apply node-local download pressure before fetching model artifacts. The default is one concurrent model artifact download per Worker, with additional cached or warm assignments queued on that Worker. Warm slot assignments report `download_queued` before `downloading_artifact`; cached-only assignments do not reserve a runtime slot while they wait.

Manual policies use `cachedCount` and `warmCount` directly. Auto-balanced
policies derive effective desired counts from queued requests, running
requests, and smoothed recent demand from the last 10 minutes. Scale-up is
immediate. Scale-down is conservative: the Manager keeps prior effective
ready/downloaded demand during the auto-balance cooldown window
(`COMRAD_AUTO_BALANCE_SCALE_DOWN_COOLDOWN_SECONDS`, default `300`) before
removing desired copies. The derived count is clamped by `minWarmCount` /
`maxWarmCount`, and cached copies are kept at least as high as warm copies while
respecting cached min/max limits.

Placement is global across policies. The planner satisfies minimum capacity
first, gives scarce and larger profiles priority, and accounts for aggregate
Worker RAM and disk budgets so smaller models do not fill machines needed by
larger models. Cached-only copies are node-level downloads and do not reserve a
warm execution slot. Warm copies still require a compatible slot and one local
runtime process.

When capacity drops to zero, or a profile is deleted, the Manager also removes no-longer-desired Worker cache entries. Eviction is guarded: active, warming, draining, or still-assigned artifacts remain in place, while stale idle warm slots stop accepting new tasks and are cleared before the Worker deletes the local cached file.

Inspect placement:

```sh
curl -fsS -H "Authorization: Bearer <admin-token>" \
  http://<manager-host>:1922/api/admin/placement
```

Explain the same dry-run plan with selected and rejected node/slot candidates:

```sh
curl -fsS -H "Authorization: Bearer <admin-token>" \
  http://<manager-host>:1922/api/admin/placement/explain
```

Apply placement:

```sh
curl -fsS -X POST -H "Authorization: Bearer <admin-token>" \
  http://<manager-host>:1922/api/admin/placement/apply
```

## Retry

If a Worker disappears before first output, the attempt fails and the task can return to the queue. The Manager retries only on another compatible slot and records failed slots to avoid repeating the same failed candidate. Workers send heartbeats; if a connection becomes half-open and heartbeats stop, the Manager marks the Worker offline, marks its idle slots unavailable, and replans capacity. Repeated disconnect, reconnect, or heartbeat-expiry events within the flap window temporarily suppress new warm placement on that Worker; the node remains visible in admin state with recent flap events, `warmPlacementSuppressionReason`, and `warmPlacementSuppressionUntil`.

If failure happens after first output, the stream fails and is not retried.

## Quarantine

Repeated execution failures quarantine a slot. Quarantined slots are excluded from placement and scheduling until expiration or admin unban plus fresh readiness/admission checks.

Unban a node or slot:

```sh
curl -sS -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"slotId":"node-id/metal0"}' \
  http://<manager-host>:1922/api/admin/quarantine/unban
```

Use the dashboard **Nodes** page to review reason, counters, last failure, expiration, and affected profiles before unbanning.

## Updates

Updates are for Worker software packages only. Model edits do not use this flow; saving a model profile increments its profile version and the Manager refreshes affected warm slots automatically.

Register the Worker update artifact first, then create a Worker update:

```sh
curl -sS -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"kind":"worker","version":"v1.2.3","artifactId":"sha256:<artifact>","sha256":"sha256:<artifact>","targetNodes":["node-id"]}' \
  http://<manager-host>:1922/api/admin/updates/workers/apply
```

The dashboard **Updates** page explains the update purpose, shows pending records, previews impact, and lists failures.
