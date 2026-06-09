import { useState } from "react"

import { ActivityIcon, HardDriveIcon, MemoryStickIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Separator } from "@/components/ui/separator"
import { ConditionList, conditionSummary } from "@/components/comrad/conditions"
import { KeyValues, PageTitle } from "@/components/comrad/dashboard-primitives"
import { StatusBadge } from "@/components/comrad/status-badge"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import { fmtBytes, human, short, timeAgo } from "@/lib/comrad"
import {
  deleteNode,
  drainNode,
  enableNode,
  setCacheArtifactAction,
  unbanNode,
  unbanSlot,
  type Actions,
} from "@/comrad/actions"
import type {
  ArtifactEvictionRecord,
  Assignment,
  Node,
  Profile,
  Slot,
  StateResponse,
} from "@/types"

export function NodesPage({
  state,
  actions,
}: {
  state: StateResponse
  actions: Actions
}) {
  const { t } = useI18n()
  const nodes = state.nodes ?? []
  const [detailNodeId, setDetailNodeId] = useState("")
  const detailNode = nodes.find((node) => node.nodeId === detailNodeId)
  return (
    <>
      <PageTitle
        eyebrow={t("nav.group.serve", undefined, "Resources")}
        title={t("nodes.title", undefined, "Workers")}
        description={t(
          "nodes.description",
          undefined,
          "Worker machines, ready slots, resources, and the reason a worker is or is not used."
        )}
      />
      {!nodes.length ? (
        <Card>
          <CardContent className="py-8 text-sm text-muted-foreground">
            {t("nodes.empty", undefined, "No workers are connected yet.")}
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          {nodes.map((node) => (
            <NodeCard
              key={node.nodeId}
              node={node}
              slots={(state.slots ?? []).filter(
                (slot) => slot.nodeId === node.nodeId
              )}
              actions={actions}
              resources={plannedNodeResources(node, state)}
              openDetails={() => setDetailNodeId(node.nodeId)}
              t={t}
            />
          ))}
        </div>
      )}
      <NodeTechnicalDetailsDialog
        node={detailNode}
        evictionRecords={(state.artifactEvictions ?? []).filter(
          (record) => record.nodeId === detailNodeId
        )}
        actions={actions}
        open={detailNodeId !== ""}
        setOpen={(open) => !open && setDetailNodeId("")}
        t={t}
      />
    </>
  )
}

function NodeCard({
  node,
  slots,
  actions,
  resources,
  openDetails,
  t,
}: {
  node: Node
  slots: Slot[]
  actions: Actions
  resources: NodeResourcePlan
  openDetails: () => void
  t: TFunction
}) {
  const reason = reasonForNode(node, slots, t)
  const readySlots = slots.filter(
    (slot) =>
      slot.state === "ready" && slot.acceptsNewTasks && !slot.quarantined
  ).length
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div className="min-w-0">
            <CardTitle className="truncate">
              {node.name || short(node.nodeId)}
            </CardTitle>
            <CardDescription>
              {node.os || "-"} / {node.arch || "-"} / {node.target || "-"}
            </CardDescription>
            <div className="mt-1 text-xs text-muted-foreground">
              {t(
                "nodes.lastSeenInline",
                { lastSeen: timeAgo(node.lastSeen, t) },
                `Last seen ${timeAgo(node.lastSeen, t)}`
              )}
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <StatusBadge
              value={node.quarantined ? "quarantined" : node.state}
            />
            <StatusBadge value={node.mode} />
          </div>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="rounded-lg border bg-muted/50 p-3">
          <div className="text-sm font-medium">
            {t(
              "nodes.canServe.title",
              undefined,
              "Can this worker serve work?"
            )}
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{reason}</p>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <ResourceStat
            icon={ActivityIcon}
            label={t("nodes.metric.readySlots", undefined, "Ready slots")}
            value={`${readySlots}/${slots.length}`}
          />
          <ResourceStat
            icon={MemoryStickIcon}
            label={t(
              "nodes.metric.memoryRemaining",
              undefined,
              "Memory remaining"
            )}
            value={fmtBytes(resources.memoryRemaining)}
          />
          <ResourceStat
            icon={HardDriveIcon}
            label={t("nodes.metric.diskRemaining", undefined, "Disk remaining")}
            value={fmtBytes(resources.diskRemaining)}
          />
          <ResourceStat
            icon={ActivityIcon}
            label={t(
              "nodes.metric.downloadPressure",
              undefined,
              "Download pressure"
            )}
            value={formatDownloadPressure(node, t)}
          />
          <ResourceStat
            icon={ActivityIcon}
            label={t("nodes.metric.p2p", undefined, "P2P runtime")}
            value={formatP2PRuntime(node, t)}
          />
        </div>
        {node.warmPlacementSuppressed ? (
          <div className="rounded-lg border bg-muted/50 p-3">
            <div className="text-sm font-medium">
              {t(
                "nodes.warmPlacementSuppressed.title",
                undefined,
                "Warm placement suppressed"
              )}
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              {warmSuppressionText(node, t)}
            </p>
          </div>
        ) : null}
        <Separator />
        <SlotList slots={slots} actions={actions} t={t} />
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={openDetails}>
            {t("nodes.technicalDetails", undefined, "Technical details")}
          </Button>
          {node.mode === "disabled" ? (
            <Button
              size="sm"
              variant="outline"
              onClick={() => enableNode(node.nodeId, actions)}
            >
              {t("nodes.action.enable", undefined, "Enable worker")}
            </Button>
          ) : (
            <Button
              size="sm"
              variant="outline"
              onClick={() => drainNode(node.nodeId, actions)}
            >
              {t("nodes.action.drain", undefined, "Drain worker")}
            </Button>
          )}
          {node.quarantined ? (
            <Button
              size="sm"
              variant="destructive"
              onClick={() => unbanNode(node.nodeId, actions)}
            >
              {t(
                "nodes.action.unbanAfterHealthcheck",
                undefined,
                "Unban after healthcheck"
              )}
            </Button>
          ) : null}
          {node.state !== "online" ? (
            <Button
              size="sm"
              variant="destructive"
              onClick={() => deleteNode(node.nodeId, actions)}
            >
              {t("nodes.action.delete", undefined, "Delete worker")}
            </Button>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}

function NodeTechnicalDetailsDialog({
  node,
  evictionRecords,
  actions,
  open,
  setOpen,
  t,
}: {
  node?: Node
  evictionRecords: ArtifactEvictionRecord[]
  actions: Actions
  open: boolean
  setOpen: (open: boolean) => void
  t: TFunction
}) {
  if (!node) return null
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="sm:max-w-[720px]">
        <DialogHeader>
          <DialogTitle>{node.name || short(node.nodeId)}</DialogTitle>
          <DialogDescription>
            {t(
              "nodes.technicalDetails.description",
              undefined,
              "Worker identity, adapters, cache, warm models, and failure context."
            )}
          </DialogDescription>
        </DialogHeader>
        <KeyValues
          values={[
            [t("nodes.field.workerId", undefined, "Worker id"), node.nodeId],
            [
              t("nodes.field.adapters", undefined, "Adapters"),
              node.runtimeAdapters?.join(", ") || "-",
            ],
            [
              t("nodes.field.cachedArtifacts", undefined, "Cached artifacts"),
              String(node.cachedArtifacts?.length || 0),
            ],
            [
              t("nodes.field.readyModels", undefined, "Ready models"),
              node.warmProfiles?.map(short).join(", ") || "-",
            ],
            [
              t("nodes.field.lastSeen", undefined, "Last seen"),
              timeAgo(node.lastSeen, t),
            ],
            [
              t("nodes.field.downloadPressure", undefined, "Download pressure"),
              formatDownloadPressure(node, t),
            ],
            [t("nodes.field.p2p", undefined, "P2P runtime"), formatP2PRuntime(node, t)],
            [
              t("nodes.field.p2pSeeders", undefined, "P2P seeding"),
              String(node.p2p?.seedingCount ?? 0),
            ],
            [
              t("nodes.field.p2pDownloads", undefined, "P2P downloading"),
              String(node.p2p?.downloadingCount ?? 0),
            ],
            [
              t("nodes.field.p2pPeers", undefined, "P2P peers"),
              String(node.p2p?.peers ?? 0),
            ],
            [
              t("nodes.field.p2pFallbacks", undefined, "P2P fallbacks"),
              String(node.p2p?.fallbackCount ?? 0),
            ],
            [
              t("nodes.field.p2pLastFailure", undefined, "P2P last failure"),
              node.p2p?.lastFailure || "-",
            ],
            [
              t(
                "nodes.field.warmPlacementSuppression",
                undefined,
                "Warm placement suppression"
              ),
              node.warmPlacementSuppressed
                ? human(node.warmPlacementSuppressionReason, t)
                : "-",
            ],
            [
              t(
                "nodes.field.warmPlacementSuppressionUntil",
                undefined,
                "Warm placement suppression until"
              ),
              node.warmPlacementSuppressionUntil || "-",
            ],
            [
              t("nodes.field.lastFailure", undefined, "Last failure"),
              node.lastFailure || "-",
            ],
            [
              t("nodes.field.quarantine", undefined, "Quarantine"),
              node.quarantineReason || "-",
            ],
            [
              t(
                "nodes.field.quarantineUntil",
                undefined,
                "Quarantine expiration"
              ),
              node.quarantineUntil || "-",
            ],
          ]}
        />
        <div className="grid gap-4">
          <div>
            <div className="mb-2 text-sm font-medium">
              {t("nodes.conditions.title", undefined, "Worker conditions")}
            </div>
            <ConditionList
              conditions={node.conditions}
              empty={t(
                "nodes.conditions.empty",
                undefined,
                "No Worker conditions reported."
              )}
              t={t}
            />
          </div>
          <CachedArtifactList
            node={node}
            evictionRecords={evictionRecords}
            actions={actions}
            t={t}
          />
          <CacheCleanupList records={evictionRecords} t={t} />
        </div>
      </DialogContent>
    </Dialog>
  )
}

function CachedArtifactList({
  node,
  evictionRecords,
  actions,
  t,
}: {
  node: Node
  evictionRecords: ArtifactEvictionRecord[]
  actions: Actions
  t: TFunction
}) {
  const artifacts = node.cachedArtifacts ?? []
  if (!artifacts.length) {
    return (
      <p className="text-sm text-muted-foreground">
        {t("nodes.cache.empty", undefined, "No cached artifacts reported.")}
      </p>
    )
  }
  const canEvict = node.state === "online"
  return (
    <div className="flex flex-col gap-2">
      <div className="text-sm font-medium">
        {t("nodes.cache.title", undefined, "Cached artifacts")}
      </div>
      {artifacts.map((artifactId) => {
        const latestEviction = latestArtifactEviction(
          evictionRecords,
          artifactId
        )
        return (
          <div
            key={artifactId}
            className="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
          >
            <div className="min-w-0">
              <code className="block truncate">{short(artifactId)}</code>
              <ArtifactEvictionInline record={latestEviction} t={t} />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  setCacheArtifactAction(
                    node.nodeId,
                    [artifactId],
                    "keep",
                    actions
                  )
                }
              >
                {t("nodes.action.cacheKeep", undefined, "Keep")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  setCacheArtifactAction(
                    node.nodeId,
                    [artifactId],
                    "evict_when_idle",
                    actions
                  )
                }
              >
                {t(
                  "nodes.action.cacheEvictWhenIdle",
                  undefined,
                  "Evict when idle"
                )}
              </Button>
              <Button
                size="sm"
                variant="destructive"
                disabled={!canEvict}
                title={
                  canEvict
                    ? t(
                        "nodes.action.evictArtifact",
                        undefined,
                        "Remove from worker"
                      )
                    : t(
                        "nodes.cache.workerOffline",
                        undefined,
                        "Worker must be online to remove cached files"
                      )
                }
                onClick={() =>
                  setCacheArtifactAction(
                    node.nodeId,
                    [artifactId],
                    "evict",
                    actions
                  )
                }
              >
                {t(
                  "nodes.action.evictArtifact",
                  undefined,
                  "Remove from worker"
                )}
              </Button>
            </div>
          </div>
        )
      })}
    </div>
  )
}

function CacheCleanupList({
  records,
  t,
}: {
  records: ArtifactEvictionRecord[]
  t: TFunction
}) {
  const visible = [...records]
    .sort((a, b) => timeSortValue(b.updatedAt) - timeSortValue(a.updatedAt))
    .slice(0, 8)
  return (
    <div className="flex flex-col gap-2">
      <div className="text-sm font-medium">
        {t("nodes.cacheCleanup.title", undefined, "Cache cleanup")}
      </div>
      {!visible.length ? (
        <p className="text-sm text-muted-foreground">
          {t(
            "nodes.cacheCleanup.empty",
            undefined,
            "No cache cleanup requests reported yet."
          )}
        </p>
      ) : (
        visible.map((record) => (
          <div
            key={record.evictionId}
            className="grid gap-2 rounded-lg border p-3 sm:grid-cols-[1fr_auto] sm:items-center"
          >
            <div className="min-w-0">
              <code className="block truncate">{short(record.artifactId)}</code>
              <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                <span>{human(record.reason, t)}</span>
                {record.failure ? (
                  <span>{human(record.failure, t)}</span>
                ) : null}
                <span>{timeAgo(record.updatedAt, t)}</span>
              </div>
            </div>
            <StatusBadge value={record.status} />
          </div>
        ))
      )}
    </div>
  )
}

function ArtifactEvictionInline({
  record,
  t,
}: {
  record?: ArtifactEvictionRecord
  t: TFunction
}) {
  if (!record) return null
  return (
    <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <span>{t("nodes.cacheCleanup.latest", undefined, "Last cleanup")}</span>
      <StatusBadge value={record.status} />
      <span>{human(record.reason, t)}</span>
      {record.failure ? <span>{human(record.failure, t)}</span> : null}
    </div>
  )
}

function latestArtifactEviction(
  records: ArtifactEvictionRecord[],
  artifactId: string
) {
  return records
    .filter((record) => record.artifactId === artifactId)
    .sort((a, b) => timeSortValue(b.updatedAt) - timeSortValue(a.updatedAt))[0]
}

function timeSortValue(value?: string) {
  if (!value) return 0
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? 0 : parsed
}

type NodeResourcePlan = {
  memoryRemaining?: number
  diskRemaining?: number
}

function plannedNodeResources(
  node: Node,
  state: StateResponse
): NodeResourcePlan {
  const profiles = state.profiles ?? []
  const assignments = (state.assignments ?? []).filter(
    (assignment) => assignment.nodeId === node.nodeId
  )
  const memoryUsed = assignments
    .filter((assignment) => assignment.desiredWarm)
    .reduce(
      (total, assignment) =>
        total + profileMemory(profileFor(assignment, profiles)),
      0
    )
  const cachedProfiles = new Set(
    assignments
      .filter((assignment) => assignment.desiredCached)
      .map((assignment) => assignment.profileId)
  )
  const diskUsed = [...cachedProfiles].reduce(
    (total, profileId) =>
      total + profileDisk(profileForId(profileId, profiles)),
    0
  )
  return {
    memoryRemaining: remainingBytes(
      node.budgets?.unifiedMemoryBytes || node.budgets?.ramBytes,
      memoryUsed
    ),
    diskRemaining: remainingBytes(node.budgets?.diskBytes, diskUsed),
  }
}

function profileFor(assignment: Assignment, profiles: Profile[]) {
  return profileForId(assignment.profileId, profiles)
}

function profileForId(profileId: string, profiles: Profile[]) {
  return profiles.find((profile) => profile.profileId === profileId)
}

function profileMemory(profile?: Profile) {
  return (
    profile?.requirements?.unifiedMemoryBytes ||
    profile?.requirements?.ramBytes ||
    0
  )
}

function profileDisk(profile?: Profile) {
  return profile?.requirements?.diskBytes || 0
}

function remainingBytes(total?: number, used = 0) {
  if (!total) return undefined
  return Math.max(0, total - used)
}

function formatDownloadPressure(node: Node, t: TFunction) {
  const pressure = node.downloadPressure
  if (!pressure) return "-"
  const active = pressure.active ?? 0
  const queued = pressure.queued ?? 0
  const max = pressure.maxConcurrent ?? 0
  return t(
    "nodes.metric.downloadPressure.value",
    { active, max, queued },
    `${active}/${max} active, ${queued} queued`
  )
}

function formatP2PRuntime(node: Node, t: TFunction) {
  if (!node.p2p) return "-"
  if (!node.p2p.available) {
    return t("nodes.metric.p2p.unavailable", undefined, "Unavailable")
  }
  return t(
    "nodes.metric.p2p.value",
    {
      seeders: node.p2p.seedingCount ?? 0,
      downloads: node.p2p.downloadingCount ?? 0,
      peers: node.p2p.peers ?? 0,
    },
    `${node.p2p.seedingCount ?? 0} seeding, ${node.p2p.downloadingCount ?? 0} downloading, ${node.p2p.peers ?? 0} peers`
  )
}

function warmSuppressionText(node: Node, t: TFunction) {
  const reason = human(node.warmPlacementSuppressionReason, t)
  const until = timeAgo(node.warmPlacementSuppressionUntil, t)
  return t(
    "nodes.reason.warmPlacementSuppressed",
    { reason, until },
    `New warm placement is temporarily blocked because ${reason}. Expires ${until}.`
  )
}

function ResourceStat({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof ActivityIcon
  label: string
  value: string
}) {
  return (
    <div className="rounded-lg border bg-card p-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <Icon data-icon="inline-start" />
        {label}
      </div>
      <div className="mt-2 truncate font-medium">{value}</div>
    </div>
  )
}

function SlotList({
  slots,
  actions,
  t,
}: {
  slots: Slot[]
  actions: Actions
  t: TFunction
}) {
  if (!slots.length)
    return (
      <p className="text-sm text-muted-foreground">
        {t("nodes.slots.empty", undefined, "No execution slots reported.")}
      </p>
    )
  return (
    <div className="flex flex-col gap-2">
      {slots.map((slot) => (
        <div
          key={slot.slotId}
          className="grid gap-3 rounded-lg border p-3 lg:grid-cols-[1fr_120px_1.2fr_auto] lg:items-center"
        >
          <div className="min-w-0">
            <div className="font-medium">
              {slot.logicalModel ||
                t("nodes.slots.noModel", undefined, "No model loaded")}
            </div>
            <div className="font-mono text-xs text-muted-foreground">
              {short(slot.slotId)}
            </div>
          </div>
          <StatusBadge value={slot.quarantined ? "quarantined" : slot.state} />
          <div className="text-sm text-muted-foreground">
            {conditionSummary(
              slot.conditions,
              "Ready",
              t,
              human(
                slot.mismatchReason ||
                  slot.quarantineReason ||
                  slot.lastFailure ||
                  (slot.acceptsNewTasks
                    ? "ready for new tasks"
                    : "not accepting new tasks"),
                t
              )
            )}
          </div>
          {slot.quarantined ? (
            <Button
              size="sm"
              variant="outline"
              onClick={() => unbanSlot(slot.slotId, actions)}
            >
              {t("nodes.action.unban", undefined, "Unban")}
            </Button>
          ) : null}
        </div>
      ))}
    </div>
  )
}

function reasonForNode(node: Node, slots: Slot[], t: TFunction) {
  if (node.state === "offline")
    return t(
      "nodes.reason.offline",
      { lastSeen: timeAgo(node.lastSeen, t) },
      `Worker is offline. Last seen ${timeAgo(node.lastSeen, t)}.`
    )
  if (!node.approved)
    return t(
      "nodes.reason.notApproved",
      undefined,
      "Worker is not approved, so COMRAD will not schedule requests here."
    )
  if (node.mode === "disabled" || node.state === "disabled")
    return t("nodes.reason.disabled", undefined, "Worker is disabled.")
  if (node.quarantined)
    return (
      node.quarantineReason ||
      t(
        "nodes.reason.quarantined",
        undefined,
        "Worker is quarantined after repeated failures."
      )
    )
  if (node.updateRequired)
    return t(
      "nodes.reason.updateRequired",
      undefined,
      "Worker has a pending software update."
    )
  if (
    !slots.some(
      (slot) =>
        slot.state === "ready" && slot.acceptsNewTasks && !slot.quarantined
    )
  )
    return t(
      "nodes.reason.noReadySlot",
      undefined,
      "No ready slot is currently available."
    )
  return t(
    "nodes.reason.ready",
    undefined,
    "Yes. At least one slot is ready and accepting new requests."
  )
}
