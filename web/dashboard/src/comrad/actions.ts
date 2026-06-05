import { toast } from "sonner"

import { translate } from "@/i18n/i18n-provider"
import type { Artifact, CacheArtifactAction } from "@/types"

export type UploadProgress = {
  fileName: string
  loadedBytes: number
  totalBytes: number
  percent: number
  bytesPerSecond: number
}

export type ConfirmAction = {
  title: string
  body: string
  confirmLabel?: string
  variant?: "default" | "destructive"
  run: () => Promise<void>
}

export type Actions = {
  api: (path: string, init?: RequestInit) => Promise<void>
  fetchJSON: <T>(path: string, init?: RequestInit) => Promise<T>
  fetchText: (path: string, init?: RequestInit) => Promise<string>
  uploadArtifact: (
    file: File,
    kind: string,
    onUploadProgress?: (progress: UploadProgress) => void
  ) => Promise<Artifact>
  show: (id: string) => void
  setConfirm: (value: ConfirmAction | null) => void
}

export type ModelEditorForm = {
  profileId?: string
  alias: string
  contextTokens: string
  ram: string
  disk: string
  computeCost: string
  llamaArgs: string
  modelArtifactIds?: string[]
  cachedCount: string
  warmCount: string
  autoBalance: boolean
  minCachedCount: string
  maxCachedCount: string
  minWarmCount: string
  maxWarmCount: string
  maxCachedProfilesPerNode: string
  maxWarmProfilesPerNode: string
}

export function splitList(value: string) {
  return value
    .split(/[,\n]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

export async function savePolicy(
  actions: Actions,
  profileId: string,
  cachedCount: string,
  warmCount: string,
  tags: string,
  preferred: string,
  denied: string,
  pins: string,
  autoBalance = false,
  minCachedCount = "",
  maxCachedCount = "",
  minWarmCount = "",
  maxWarmCount = "",
  maxCachedProfilesPerNode = "",
  maxWarmProfilesPerNode = ""
) {
  await actions.api("/api/admin/policies", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      profileId,
      cachedCount: Number(cachedCount || 0),
      warmCount: Number(warmCount || 0),
      autoBalance,
      minCachedCount: Number(minCachedCount || 0),
      maxCachedCount: Number(maxCachedCount || 0),
      minWarmCount: Number(minWarmCount || 0),
      maxWarmCount: Number(maxWarmCount || 0),
      maxCachedProfilesPerNode: Number(maxCachedProfilesPerNode || 0),
      maxWarmProfilesPerNode: Number(maxWarmProfilesPerNode || 0),
      constraints: {
        requireTags: splitList(tags),
        preferNodes: splitList(preferred),
        denyNodes: splitList(denied),
      },
      hardPinnedSlots: splitList(pins),
    }),
  })
  toast.success(
    translate("placement.toast.policySaved", undefined, "Policy saved")
  )
}

export async function registerModel(
  actions: Actions,
  form: {
    alias: string
    contextTokens: string
    ram: string
    disk: string
    computeCost: string
    llamaArgs: string
  }
) {
  await saveModel(actions, {
    ...form,
    cachedCount: "1",
    warmCount: "1",
    autoBalance: false,
    minCachedCount: "",
    maxCachedCount: "",
    minWarmCount: "",
    maxWarmCount: "",
    maxCachedProfilesPerNode: "",
    maxWarmProfilesPerNode: "",
  })
}

export async function saveModel(
  actions: Actions,
  form: ModelEditorForm,
  uploadFiles: File[] = [],
  onUploadProgress?: (progress: UploadProgress) => void
) {
  const isEdit = Boolean(form.profileId)
  const modelName = form.alias.trim()
  if (!modelName) {
    toast.error(
      translate(
        "profiles.toast.modelNameRequired",
        undefined,
        "Model name is required"
      )
    )
    return false
  }
  const modelArtifacts = await resolveModelArtifacts(
    actions,
    form,
    uploadFiles,
    isEdit,
    onUploadProgress
  )
  if (!modelArtifacts.length) return false
  const context = Number(form.contextTokens || 4096)
  const profileId =
    form.profileId || `llm.chat/${slug(modelName)}/context-${context}`
  const gib = 1073741824
  const yaml = `profileId: ${yamlString(profileId)}
model: ${yamlString(modelName)}
kind: llm.chat
computeCost: ${Number(form.computeCost || 0)}
runtime:
  adapter: llama.cpp-metal
  modelArtifacts:
${modelArtifacts.map((artifact) => `    - ${yamlString(artifact.artifactId)}`).join("\n")}
  contextTokens: ${context}
  llamaCpp:
    args: ${JSON.stringify(splitArgs(form.llamaArgs))}
requirements:
  target: darwin-arm64-metal
  unifiedMemoryBytes: ${Number(form.ram || 6) * gib}
  diskBytes: ${Number(form.disk || 8) * gib}
warmable: true
`
  await actions.api("/api/admin/profiles", {
    method: "POST",
    headers: { "Content-Type": "application/yaml" },
    body: yaml,
  })
  await actions.api("/api/admin/policies", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      profileId,
      cachedCount: Number(form.cachedCount || 0),
      warmCount: Number(form.warmCount || 0),
      autoBalance: form.autoBalance,
      minCachedCount: Number(form.minCachedCount || 0),
      maxCachedCount: Number(form.maxCachedCount || 0),
      minWarmCount: Number(form.minWarmCount || 0),
      maxWarmCount: Number(form.maxWarmCount || 0),
      maxCachedProfilesPerNode: Number(form.maxCachedProfilesPerNode || 0),
      maxWarmProfilesPerNode: Number(form.maxWarmProfilesPerNode || 0),
    }),
  })
  toast.success(
    isEdit
      ? translate("profiles.toast.updated", undefined, "Model updated")
      : translate("profiles.toast.added", undefined, "Model added")
  )
  return true
}

export function deleteModel(
  profileId: string,
  label: string,
  actions: Actions
) {
  actions.setConfirm({
    title: translate(
      "profiles.confirm.delete.title",
      undefined,
      "Delete model"
    ),
    body: translate(
      "profiles.confirm.delete.body",
      { model: label },
      `This removes ${label} from COMRAD, clears desired capacity, and evicts no-longer-used cached files from Workers.`
    ),
    confirmLabel: translate("profiles.deleteModel", undefined, "Delete model"),
    variant: "destructive",
    run: async () => {
      await actions.api(
        `/api/admin/profiles?profileId=${encodeURIComponent(profileId)}`,
        { method: "DELETE" }
      )
      toast.success(
        translate("profiles.toast.deleted", undefined, "Model deleted")
      )
    },
  })
}

export function drainNode(nodeId: string, actions: Actions) {
  actions.setConfirm({
    title: translate(
      "nodes.confirm.drain.title",
      undefined,
      "Drain / disable worker"
    ),
    body: translate(
      "nodes.confirm.drain.body",
      { worker: nodeId },
      `This disables scheduling for ${nodeId}. Active work is not killed by this UI action.`
    ),
    confirmLabel: translate("nodes.action.drain", undefined, "Drain worker"),
    variant: "destructive",
    run: () =>
      actions.api("/api/admin/nodes", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ nodeId, mode: "disabled", state: "disabled" }),
      }),
  })
}

export function enableNode(nodeId: string, actions: Actions) {
  actions.setConfirm({
    title: translate("nodes.confirm.enable.title", undefined, "Enable worker"),
    body: translate(
      "nodes.confirm.enable.body",
      undefined,
      "Scheduling still requires fit and Worker readiness."
    ),
    confirmLabel: translate("nodes.action.enable", undefined, "Enable worker"),
    variant: "default",
    run: () =>
      actions.api("/api/admin/nodes", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          nodeId,
          mode: "reserved_always",
          state: "online",
        }),
      }),
  })
}

export function unbanNode(nodeId: string, actions: Actions) {
  actions.setConfirm({
    title: translate(
      "nodes.confirm.unbanWorker.title",
      undefined,
      "Unban worker after healthcheck"
    ),
    body: translate(
      "nodes.confirm.unbanWorker.body",
      undefined,
      "The worker becomes schedulable only after health and admission checks pass."
    ),
    confirmLabel: translate("nodes.action.unban", undefined, "Unban"),
    variant: "default",
    run: () =>
      actions.api("/api/admin/quarantine/unban", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ nodeId }),
      }),
  })
}

export function unbanSlot(slotId: string, actions: Actions) {
  actions.setConfirm({
    title: translate(
      "nodes.confirm.unbanSlot.title",
      undefined,
      "Unban slot after healthcheck"
    ),
    body: translate(
      "nodes.confirm.unbanSlot.body",
      undefined,
      "The slot becomes schedulable only after health and admission checks pass."
    ),
    confirmLabel: translate(
      "nodes.confirm.unbanSlot.label",
      undefined,
      "Unban slot"
    ),
    variant: "default",
    run: () =>
      actions.api("/api/admin/quarantine/unban", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ slotId }),
      }),
  })
}

export function evictWorkerArtifact(
  nodeId: string,
  artifactId: string,
  actions: Actions
) {
  setCacheArtifactAction(nodeId, [artifactId], "evict", actions)
}

export function setCacheArtifactAction(
  nodeId: string,
  artifactIds: string[],
  action: CacheArtifactAction,
  actions: Actions
) {
  const targets = artifactIds.filter(Boolean)
  const label = cacheActionLabel(action)
  actions.setConfirm({
    title:
      action === "evict"
        ? translate(
            "nodes.confirm.evictArtifact.title",
            undefined,
            "Remove cached artifact"
          )
        : translate(
            "nodes.confirm.cacheArtifactAction.title",
            undefined,
            label
          ),
    body:
      action === "evict" && targets.length === 1
        ? translate(
            "nodes.confirm.evictArtifact.body",
            { artifact: targets[0], worker: nodeId },
            `This asks ${nodeId} to delete ${targets[0]} from its local cache. If the artifact is still assigned or active, the Manager blocks the request.`
          )
        : translate(
            "nodes.confirm.cacheArtifactAction.body",
            {
              action: label.toLowerCase(),
              worker: nodeId,
              count: targets.length,
            },
            `This asks ${nodeId} to ${label.toLowerCase()} ${targets.length} cached artifact(s).`
          ),
    confirmLabel: label,
    variant: action === "evict" ? "destructive" : "default",
    run: async () => {
      await Promise.all(
        targets.map((artifactId) =>
          actions.api(
            `/api/admin/nodes/${encodeURIComponent(nodeId)}/artifacts/${encodeURIComponent(artifactId)}`,
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ action }),
            }
          )
        )
      )
      toast.success(
        action === "evict"
          ? translate(
              "nodes.toast.evictQueued",
              undefined,
              "Artifact removal queued"
            )
          : translate(
              "nodes.toast.cacheActionUpdated",
              undefined,
              "Cached artifact action updated"
            )
      )
    },
  })
}

function cacheActionLabel(action: CacheArtifactAction) {
  if (action === "keep" || action === "pin") {
    return translate("nodes.action.cacheKeep", undefined, "Keep")
  }
  if (action === "evict_when_idle") {
    return translate(
      "nodes.action.cacheEvictWhenIdle",
      undefined,
      "Evict when idle"
    )
  }
  return translate(
    "nodes.action.evictArtifact",
    undefined,
    "Remove from worker"
  )
}

export async function triggerUpdate(
  actions: Actions,
  version: string,
  artifactId: string,
  sha256: string,
  nodes: string
) {
  await actions.api("/api/admin/updates/workers/apply", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      kind: "worker",
      version,
      artifactId,
      sha256,
      targetNodes: splitList(nodes),
    }),
  })
  toast.success(translate("updates.toast.created", undefined, "Update created"))
}

export function deleteArtifact(artifactId: string, actions: Actions) {
  actions.setConfirm({
    title: translate(
      "artifacts.confirm.delete.title",
      undefined,
      "Delete artifact"
    ),
    body: translate(
      "artifacts.confirm.delete.body",
      undefined,
      "This removes an unused artifact from Manager storage. Artifacts still used by models or updates are blocked."
    ),
    confirmLabel: translate("artifacts.delete", undefined, "Delete artifact"),
    variant: "destructive",
    run: async () => {
      await actions.api(
        `/api/admin/artifacts/${encodeURIComponent(artifactId)}`,
        { method: "DELETE" }
      )
      toast.success(
        translate("artifacts.toast.deleted", undefined, "Artifact deleted")
      )
    },
  })
}

async function resolveModelArtifacts(
  actions: Actions,
  form: ModelEditorForm,
  files: File[],
  isEdit: boolean,
  onUploadProgress?: (progress: UploadProgress) => void
) {
  if (files.length) {
    return orderedModelArtifacts(
      await uploadModelArtifacts(actions, files, onUploadProgress)
    )
  }
  if (form.modelArtifactIds?.length) {
    return form.modelArtifactIds.map((artifactId) => ({ artifactId }))
  }
  toast.error(
    isEdit
      ? translate(
          "profiles.toast.uploadOrKeepFiles",
          undefined,
          "Upload model files or keep the current files"
        )
      : translate("profiles.toast.uploadFile", undefined, "Upload a model file")
  )
  return []
}

async function uploadModelArtifacts(
  actions: Actions,
  files: File[],
  onUploadProgress?: (progress: UploadProgress) => void
) {
  const artifacts = []
  const totalBytes = files.reduce((sum, file) => sum + file.size, 0)
  const startedAt = performance.now()
  let completedBytes = 0
  for (const file of files) {
    const artifact = await actions.uploadArtifact(
      file,
      artifactKind(file.name),
      (progress) => {
        const loadedBytes = completedBytes + progress.loadedBytes
        const elapsedSeconds = Math.max(
          0.001,
          (performance.now() - startedAt) / 1000
        )
        onUploadProgress?.({
          fileName: progress.fileName,
          loadedBytes,
          totalBytes,
          percent: totalBytes ? (loadedBytes / totalBytes) * 100 : 0,
          bytesPerSecond: loadedBytes / elapsedSeconds,
        })
      }
    )
    artifacts.push(artifact)
    completedBytes += file.size
  }
  return artifacts
}

export function uploadArtifactRequest(
  token: string,
  file: File,
  kind: string,
  onUploadProgress?: (progress: UploadProgress) => void
) {
  return new Promise<Artifact>((resolve, reject) => {
    const body = new FormData()
    body.append("file", file)
    body.append("kind", kind)
    const xhr = new XMLHttpRequest()
    const startedAt = performance.now()
    xhr.open("POST", "/api/admin/artifacts/upload")
    xhr.setRequestHeader("Authorization", `Bearer ${token}`)
    xhr.upload.onprogress = (event) => {
      const totalBytes = event.lengthComputable ? event.total : file.size
      const elapsedSeconds = Math.max(
        0.001,
        (performance.now() - startedAt) / 1000
      )
      onUploadProgress?.({
        fileName: file.name,
        loadedBytes: event.loaded,
        totalBytes,
        percent: totalBytes ? (event.loaded / totalBytes) * 100 : 0,
        bytesPerSecond: event.loaded / elapsedSeconds,
      })
    }
    xhr.onload = () => {
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(new Error(xhr.responseText || `Upload failed: ${xhr.status}`))
        return
      }
      try {
        resolve(JSON.parse(xhr.responseText) as Artifact)
      } catch {
        reject(new Error("Upload returned unreadable JSON"))
      }
    }
    xhr.onerror = () => reject(new Error("Upload failed"))
    xhr.send(body)
  })
}

function orderedModelArtifacts(
  artifacts: Array<Pick<Artifact, "artifactId" | "kind" | "name">>
) {
  const primary =
    artifacts.find(
      (artifact) =>
        artifactKind(artifact.name || "").includes("model_gguf") ||
        artifact.kind === "model_gguf"
    ) || artifacts[0]
  return [primary, ...artifacts.filter((artifact) => artifact !== primary)]
}

function splitArgs(value: string) {
  return value.trim().split(/\s+/).filter(Boolean)
}

function slug(value: string) {
  return (
    (value || "model")
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9._-]+/g, "-")
      .replace(/^-|-$/g, "") || "model"
  )
}

function yamlString(value: string) {
  return JSON.stringify(value)
}

function artifactKind(value: string) {
  const lower = value.toLowerCase()
  if (lower.includes("mmproj")) return "model_mmproj"
  if (lower.endsWith(".gguf")) return "model_gguf"
  return "model_support"
}
