import type { Assignment, Profile, StateResponse } from "@/types"
import type { TFunction } from "@/i18n/i18n-provider"

export function short(value?: string) {
  const text = value ?? ""
  return text.length > 24 ? `${text.slice(0, 12)}...${text.slice(-8)}` : text
}

export function human(value?: string, t?: TFunction) {
  const raw = value || "none"
  const text = raw.replaceAll("_", " ")
  return t
    ? t(
        `value.${raw.toLowerCase().replace(/[^a-z0-9]+/g, ".")}`,
        undefined,
        text
      )
    : text
}

export function fmtBytes(value?: number) {
  let bytes = Number(value || 0)
  if (!bytes) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  let index = 0
  while (bytes >= 1024 && index < units.length - 1) {
    bytes /= 1024
    index += 1
  }
  return `${bytes >= 10 ? bytes.toFixed(0) : bytes.toFixed(1)} ${units[index]}`
}

export function timeAgo(value?: string, t?: TFunction) {
  if (!value || value.startsWith("0001-")) return "-"
  const ms = Date.now() - new Date(value).getTime()
  if (ms < 60_000) {
    return t
      ? t("time.secondsAgo", { value: Math.max(0, Math.round(ms / 1000)) })
      : `${Math.max(0, Math.round(ms / 1000))}s ago`
  }
  if (ms < 3_600_000) {
    return t
      ? t("time.minutesAgo", { value: Math.round(ms / 60_000) })
      : `${Math.round(ms / 60_000)}m ago`
  }
  if (ms < 86_400_000) {
    return t
      ? t("time.hoursAgo", { value: Math.round(ms / 3_600_000) })
      : `${Math.round(ms / 3_600_000)}h ago`
  }
  return t
    ? t("time.daysAgo", { value: Math.round(ms / 86_400_000) })
    : `${Math.round(ms / 86_400_000)}d ago`
}

export function statusTone(status?: string) {
  const value = (status || "").toLowerCase()
  if (/ready|online|completed|healthy|true|ok|succeeded|evicted/.test(value))
    return "ready"
  if (/queued|waiting|warming|cached|downloading|loading|expected/.test(value))
    return "waiting"
  if (/fail|error|quarantine|mismatch|required|false|blocked/.test(value))
    return "failed"
  if (/running|serving|assigned/.test(value)) return "running"
  if (/update/.test(value)) return "updating"
  return "offline"
}

export function profileLabel(profile: Profile) {
  return (
    profile.logicalModel || profile.alias || profile.name || profile.profileId
  )
}

export function profileVariants(profile: Profile) {
  if (profile.runtimeVariants?.length) return profile.runtimeVariants
  return [
    {
      variantId: profile.runtimeVariantId || profile.profileId,
      target: profile.requirements?.target,
      runtimeAdapter: profile.runtimeAdapter,
      artifacts: profile.artifacts,
      requirements: profile.requirements,
      llm: profile.llm,
      runtime: profile.runtime,
    },
  ]
}

export function summary(state: StateResponse) {
  const slots = state.slots ?? []
  const nodes = state.nodes ?? []
  const tasks = state.taskSummary
  const assignments = state.assignments ?? []
  const ready = slots.filter(
    (slot) =>
      slot.state === "ready" && slot.acceptsNewTasks && !slot.quarantined
  ).length
  const warmDesired = assignments.filter((item) => item.desiredWarm).length
  const warmActual = assignments.filter((item) => item.actualWarm).length
  const active = tasks?.running ?? 0
  const queued = tasks?.queued ?? 0
  const failHour = tasks?.failuresLastHour ?? 0
  const nodesAction = nodes.filter(
    (node) =>
      node.state !== "online" ||
      !node.approved ||
      node.quarantined ||
      node.updateRequired
  ).length
  const updates = nodes.filter((node) => node.updateRequired).length
  const healthy =
    ready > 0 &&
    queued === 0 &&
    nodesAction === 0 &&
    failHour === 0 &&
    assignments.every((a) => !a.desiredWarm || a.actualWarm)
  return {
    ready,
    warmDesired,
    warmActual,
    active,
    queued,
    failHour,
    nodesAction,
    updates,
    healthy,
  }
}

export function attentionItems(state: StateResponse, t?: TFunction) {
  const tx = (
    key: string,
    fallback: string,
    values?: Record<string, string | number>
  ) => (t ? t(key, values, fallback) : fallback)
  const out: Array<{
    problem: string
    reason: string
    impact: string
    action: string
    target: string
    severity: string
  }> = []
  for (const assignment of state.assignments ?? []) {
    if (
      assignment.mismatchReason ||
      (assignment.desiredWarm && !assignment.actualWarm) ||
      (assignment.desiredCached && !assignment.actualCached)
    ) {
      out.push({
        problem: tx(
          "overview.attention.warmProblem",
          `${assignment.logicalModel || assignment.profileId} ready copies are below target`,
          { model: assignment.logicalModel || assignment.profileId }
        ),
        reason:
          assignment.mismatchReason ||
          tx(
            "overview.attention.warmReason",
            "Desired placement has not reached actual state"
          ),
        impact: tx(
          "overview.attention.warmImpact",
          "Reduced ready capacity or slower first request"
        ),
        action: tx("overview.attention.setReadyCopies", "Set ready copies"),
        target: "placement",
        severity: assignment.mismatchReason ? "failed" : "waiting",
      })
    }
  }
  for (const task of (state.tasks ?? []).filter(
    (item) => item.status === "queued"
  )) {
    out.push({
      problem: tx("overview.attention.taskQueued", "Request is queued"),
      reason:
        task.failureReason ||
        tx(
          "overview.attention.noCompatibleSlot",
          "No compatible ready slot is currently available"
        ),
      impact: tx(
        "overview.attention.clientWaiting",
        "Client is waiting for capacity"
      ),
      action: tx("overview.attention.openTask", "Open request"),
      target: "tasks",
      severity: "waiting",
    })
  }
  const visibleQueued = (state.tasks ?? []).filter(
    (item) => item.status === "queued"
  ).length
  const hiddenQueued = Math.max(
    0,
    (state.taskSummary?.queued ?? 0) - visibleQueued
  )
  if (hiddenQueued > 0) {
    out.push({
      problem: tx(
        "overview.attention.hiddenQueuedProblem",
        `${hiddenQueued} queued requests are outside the recent state window`,
        { count: hiddenQueued }
      ),
      reason: tx(
        "overview.attention.historyPaginated",
        "Request history is paginated to keep the dashboard responsive"
      ),
      impact: tx(
        "overview.attention.useTasksFilters",
        "Use the Requests page filters to inspect the full queue"
      ),
      action: tx("overview.attention.openTasks", "Open requests"),
      target: "tasks",
      severity: "waiting",
    })
  }
  for (const slot of (state.slots ?? []).filter((item) => item.quarantined)) {
    out.push({
      problem: tx(
        "overview.attention.slotQuarantined",
        "Worker slot is quarantined"
      ),
      reason:
        slot.quarantineReason ||
        slot.lastFailure ||
        tx(
          "overview.attention.repeatedFailures",
          "Repeated execution failures"
        ),
      impact: tx(
        "overview.attention.slotExcluded",
        "Slot is excluded from scheduling"
      ),
      action: tx("overview.attention.reviewUnban", "Review / unban"),
      target: "nodes",
      severity: "failed",
    })
  }
  for (const node of (state.nodes ?? []).filter(
    (item) =>
      item.state !== "online" ||
      !item.approved ||
      item.updateRequired ||
      item.quarantined
  )) {
    out.push({
      problem: tx(
        "overview.attention.workerRequiresAction",
        "Worker requires action"
      ),
      reason: node.quarantined
        ? node.quarantineReason ||
          tx("overview.attention.workerQuarantined", "Worker quarantined")
        : !node.approved
          ? tx("overview.attention.workerNotApproved", "Worker is not approved")
          : node.updateRequired
            ? tx(
                "overview.attention.workerUpdateRequired",
                "Worker update required"
              )
            : tx(
                "overview.attention.workerState",
                `Worker state is ${node.state}`,
                { state: node.state || "-" }
              ),
      impact: tx(
        "overview.attention.capacityUnavailable",
        "Capacity may be unavailable"
      ),
      action: tx("overview.attention.openWorker", "Open worker"),
      target: "nodes",
      severity: node.updateRequired ? "updating" : "failed",
    })
  }
  return out.length
    ? out
    : [
        {
          problem: tx("overview.attention.noAction", "No action required"),
          reason: tx(
            "overview.attention.allReady",
            "All visible placement is ready"
          ),
          impact: tx(
            "overview.attention.clusterReady",
            "Cluster can serve compatible requests"
          ),
          action: tx("overview.attention.reviewOverview", "Review overview"),
          target: "overview",
          severity: "ready",
        },
      ]
}

export function assignmentCounts(profileId: string, assignments: Assignment[]) {
  const items = assignments.filter((item) => item.profileId === profileId)
  return {
    desiredCached: items.filter((item) => item.desiredCached).length,
    desiredWarm: items.filter((item) => item.desiredWarm).length,
    actualCached: items.filter((item) => item.actualCached).length,
    actualWarm: items.filter((item) => item.actualWarm).length,
  }
}
