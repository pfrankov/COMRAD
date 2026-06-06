# Model Management

COMRAD separates the client-facing model name from the concrete artifacts a Worker executes.

Clients request the logical model or alias. The Manager selects a compatible ready slot and records the exact artifact, sha256, runtime adapter, context, and runtime parameters used.

## Register Through The Dashboard

Open **Models** and use **Add a model**. The dialog uploads or registers model files, shows percent and speed while long uploads are in flight, creates a YAML-backed LLM workload profile, and sets downloaded/ready copy counts from the same screen.

Use **Edit model** from the existing model list to update the client model name, llama.cpp server args, resource budgets, compute cost, ready/downloaded copy counts, or replacement model files. Leave uploads empty when editing to keep the current linked files.

Use **Delete model** when a model should disappear from COMRAD. The Manager removes the profile, its capacity policy, and profile assignments, then asks online Workers to evict cached files that are no longer desired and not serving active work.

The macOS bundle includes `llama-server`, and the Worker installer copies it into the Worker install directory. Operators can override it with `COMRAD_LLAMA_CPP_URL` and `COMRAD_LLAMA_CPP_SHA256`. `llama-server` is not stored as a model file and is not delivered through model edits. The optional **llama.cpp server args** field is passed when the Worker starts the local server for that profile, but COMRAD rejects managed flags such as host, port, model path, mmproj, context size, API key, and TLS files. **Compute cost** is a manual per-profile value; missing cost defaults to `0`.

Saving an edited profile increments `profileVersion`. The Manager treats already warm slots with an older profile version as stale and dispatches the updated profile to affected Workers. The Worker stops the old local server, starts a new one with the updated model/settings, waits for `/health`, and only then marks the slot ready.

## Register Artifacts Through The Admin API

Register a GGUF artifact:

```sh
curl -sS -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/yaml" \
  --data-binary @model-artifact.yaml \
  http://<manager-host>:1922/api/admin/artifacts
```

Docker Compose exposes `/var/lib/comrad/imports` for Admin API path imports when an artifact already exists on the Manager host.

`model-artifact.yaml`:

```yaml
path: /absolute/path/to/model.gguf
kind: model_gguf
name: model.gguf
```

Upload a model artifact from your browser or local shell:

```sh
curl -sS -H "Authorization: Bearer <admin-token>" \
  -F kind=model_gguf \
  -F file=@model.gguf \
  http://<manager-host>:1922/api/admin/artifacts/upload
```

Delete an unused artifact:

```sh
curl -fsS -X DELETE -H "Authorization: Bearer <admin-token>" \
  http://<manager-host>:1922/api/admin/artifacts/sha256:<artifact>
```

Deletion is blocked while an artifact is referenced by a profile or update. Uploaded files stored in the Manager artifact directory are removed from disk; path imports registered through the Admin API are unregistered but the source file is left in place.

Each immutable artifact is also a public-distribution artifact. On upload or import, the Manager generates one stable torrent identity for that exact artifact and reuses it everywhere COMRAD assigns the artifact later. Workers try public DHT and magnet-based BitTorrent delivery first when their torrent runtime is available, then fall back to authenticated Manager HTTP download when torrent networking is unavailable, unproductive, or fails verification. SHA-256 remains the final correctness check before an artifact counts as cached.

## Worker Cache Cleanup

Model deletion and capacity changes clean Worker caches automatically. When a profile is deleted, or when a policy lowers downloaded/ready copies so an artifact is no longer desired on a Worker, the Manager sends an eviction command to that Worker. The Manager does not evict artifacts that are still assigned, warming, or serving active work. The Worker stops affected warm runtimes, clears stale slot state, deletes the cached file, and reports `evicted`.

The Admin state includes a `cachePlans` view for each capacity policy. It reports the profile, required artifacts, required tags, desired cached copies, actual cached copies, stale copies, pending evictions, and per-Worker cached/warm/active/eviction/intent state. The dashboard shows this under **Capacity** so operators can answer where a model is cached and why stale files have not been removed yet.

Delete a model profile:

```sh
curl -fsS -X DELETE -H "Authorization: Bearer <admin-token>" \
  "http://<manager-host>:1922/api/admin/profiles?profileId=llm.chat/local/context-4096"
```

Remove a stale cached artifact from one selected Worker:

```sh
curl -fsS -X DELETE -H "Authorization: Bearer <admin-token>" \
  http://<manager-host>:1922/api/admin/nodes/<node-id>/artifacts/sha256:<artifact>
```

Set an explicit stale-cache action for one selected Worker:

```sh
curl -fsS -X POST -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"action":"keep"}' \
  http://<manager-host>:1922/api/admin/nodes/<node-id>/artifacts/sha256:<artifact>
```

Actions are `keep`/`pin`, `evict`, and `evict_when_idle`. `keep` persists operator intent and excludes that stale cache entry from automatic eviction planning. `evict` queues immediate eviction and is blocked when the Worker is offline, the artifact is not cached there, or the artifact is still assigned, warming, or active on that Worker. `evict_when_idle` persists intent and queues eviction once the artifact is no longer assigned, warming, or active. Blocked, queued, evicted, and failed eviction records include bounded `conditions` with stable reasons such as `WorkerOffline`. To delete the Manager's stored artifact after all profiles and updates stop referencing it, use `DELETE /api/admin/artifacts/sha256:<artifact>` separately.

## Profile YAML

Profile config is YAML. It contains only fields required for execution:

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

Create the profile:

```sh
curl -sS -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/yaml" \
  --data-binary @profile.yaml \
  http://<manager-host>:1922/api/admin/profiles
```

Notes:

- `modelArtifacts` is the ordered artifact set for the model. Put the primary GGUF first.
- `llama-server` is installed on Workers; it is not uploaded as a model artifact.
- `contextTokens` is explicit because it is required for scheduling and the `--ctx-size` llama.cpp server argument.
- `llamaCpp.args` is for tuning flags such as GPU layers or thread counts. Do not include COMRAD-managed server flags such as `--host`, `--port`, `--model`, `--mmproj`, `--ctx-size`, API key, or TLS flags.
- Quantization, tokenizer hash, config hash, and model sha are not profile config fields.
- `runtime.adapter` is not repeated under requirements.
- `computeCost` is explicit and defaults to `0`; it is copied to tasks, attempts, reports, and ledger entries when the profile is used.

## Capacity Policy YAML

The dashboard calls this **Capacity**. The API keeps the existing placement-policy endpoint name.

```yaml
profileId: llm.chat/local/context-4096
cachedCount: 1
warmCount: 1
```

Manual policies keep exactly those downloaded and ready counts, subject to
compatible Worker capacity. Auto-balance is opt-in per model:

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

`0` for the per-node limits means "limited only by Worker memory, disk, and
slots." In auto mode, the Manager derives effective desired ready copies from
queued, running, and smoothed recent requests, then keeps downloaded copies at
least as high as ready copies. Scale-up happens immediately; scale-down waits
for `COMRAD_AUTO_BALANCE_SCALE_DOWN_COOLDOWN_SECONDS` (default `300`) before
dropping desired ready/downloaded copies.

Apply capacity intent:

```sh
curl -sS -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/yaml" \
  --data-binary @policy.yaml \
  http://<manager-host>:1922/api/admin/policies
```

Profiles without requirements are unschedulable.

Setting both `cachedCount` and `warmCount` to `0` stops keeping that profile hot for manual policies. For auto policies, set the min and max fields to `0` as well. If the artifact is not desired by another profile or update on a Worker, the Manager queues Worker cache eviction so old model files do not accumulate.

Profiles and capacity policies also expose bounded `conditions` in Admin state. Profiles report `Ready`, `Schedulable`, and `ArtifactsAvailable`. Capacity policies report `Cached`, `Warm`, and `PlacementSatisfied`. Nodes report `WarmPlacementSuppressed` when recent flapping makes them temporarily ineligible for new warm placement. These fields are derived from Manager state and are not editable configuration.
