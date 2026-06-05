import { useEffect, useMemo, useState } from "react"

import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { Toggle } from "@/components/ui/toggle"
import { ConditionList } from "@/components/comrad/conditions"
import { DataTable } from "@/components/comrad/data-table"
import { DryRun, PageTitle } from "@/components/comrad/dashboard-primitives"
import { StatusBadge } from "@/components/comrad/status-badge"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import {
  assignmentCounts,
  human,
  profileLabel,
  short,
  timeAgo,
} from "@/lib/comrad"
import {
  savePolicy,
  setCacheArtifactAction,
  type Actions,
} from "@/comrad/actions"
import type {
  Assignment,
  CachePlan,
  CacheWorkerStatus,
  FitResult,
  PlacementCandidateExplanation,
  PlacementExplainResponse,
  PlacementMissingExplanation,
  Policy,
  StateResponse,
} from "@/types"

export function PlacementPage({
  state,
  actions,
}: {
  state: StateResponse
  actions: Actions
}) {
  const { t } = useI18n()
  const firstProfile = state.profiles?.[0]?.profileId || ""
  const [profileId, setProfileId] = useState(firstProfile)
  const [cachedCount, setCachedCount] = useState("1")
  const [warmCount, setWarmCount] = useState("1")
  const [tags, setTags] = useState("")
  const [preferred, setPreferred] = useState("")
  const [denied, setDenied] = useState("")
  const [pins, setPins] = useState("")
  const [autoBalance, setAutoBalance] = useState(false)
  const [minCachedCount, setMinCachedCount] = useState("")
  const [maxCachedCount, setMaxCachedCount] = useState("")
  const [minWarmCount, setMinWarmCount] = useState("")
  const [maxWarmCount, setMaxWarmCount] = useState("")
  const [maxCachedProfilesPerNode, setMaxCachedProfilesPerNode] = useState("")
  const [maxWarmProfilesPerNode, setMaxWarmProfilesPerNode] = useState("")
  const [advanced, setAdvanced] = useState(false)
  const [explain, setExplain] = useState<PlacementExplainResponse | null>(null)
  const [explainLoading, setExplainLoading] = useState(false)
  const [explainError, setExplainError] = useState("")

  const assignments = useMemo(
    () => state.assignments ?? [],
    [state.assignments]
  )
  const selectedAssignments = useMemo(
    () => assignments.filter((item) => item.profileId === profileId),
    [assignments, profileId]
  )
  const selectedCachePlan = useMemo(
    () =>
      (state.cachePlans ?? []).find((item) => item.profileRef === profileId),
    [state.cachePlans, profileId]
  )
  const selectedPolicy = useMemo(
    () => (state.policies ?? []).find((item) => item.profileId === profileId),
    [state.policies, profileId]
  )
  useEffect(() => {
    if (!profileId && firstProfile) setProfileId(firstProfile)
  }, [firstProfile, profileId])
  useEffect(() => {
    const counts = assignmentCounts(profileId, assignments)
    setCachedCount(
      String(selectedPolicy?.cachedCount ?? (counts.desiredCached || 1))
    )
    setWarmCount(String(selectedPolicy?.warmCount ?? (counts.desiredWarm || 1)))
    setAutoBalance(Boolean(selectedPolicy?.autoBalance))
    setMinCachedCount(countField(selectedPolicy?.minCachedCount))
    setMaxCachedCount(countField(selectedPolicy?.maxCachedCount))
    setMinWarmCount(countField(selectedPolicy?.minWarmCount))
    setMaxWarmCount(countField(selectedPolicy?.maxWarmCount))
    setMaxCachedProfilesPerNode(
      countField(selectedPolicy?.maxCachedProfilesPerNode)
    )
    setMaxWarmProfilesPerNode(
      countField(selectedPolicy?.maxWarmProfilesPerNode)
    )
  }, [assignments, profileId, selectedPolicy])

  return (
    <>
      <PageTitle
        eyebrow={t("nav.group.serve", undefined, "Resources")}
        title={t("placement.title", undefined, "Capacity")}
        description={t(
          "placement.description",
          undefined,
          "Decide how many copies of each model COMRAD should download and keep ready."
        )}
      />
      <div className="grid gap-4 xl:grid-cols-[0.9fr_1.1fr]">
        <Card>
          <CardHeader>
            <CardTitle>
              {t("placement.planner.title", undefined, "Capacity planner")}
            </CardTitle>
            <CardDescription>
              {t(
                "placement.planner.description",
                undefined,
                "Pick a model, then choose desired capacity."
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <FieldGroup>
              <Field>
                <FieldLabel>
                  {t("placement.field.model", undefined, "Model")}
                </FieldLabel>
                <Select value={profileId} onValueChange={setProfileId}>
                  <SelectTrigger>
                    <SelectValue
                      placeholder={t(
                        "placement.chooseModel",
                        undefined,
                        "Choose model"
                      )}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {(state.profiles ?? []).map((profile) => (
                        <SelectItem
                          key={profile.profileId}
                          value={profile.profileId}
                        >
                          {profileLabel(profile)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FieldDescription>
                  {t(
                    "placement.model.description",
                    undefined,
                    "Use Models to add or edit a model. Use Capacity to decide where it should be ready."
                  )}
                </FieldDescription>
              </Field>
              <div className="grid gap-4 md:grid-cols-3">
                <Field>
                  <FieldLabel>
                    {t(
                      "placement.field.downloadedCopies",
                      undefined,
                      "Downloaded copies"
                    )}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={cachedCount}
                    onChange={(event) => setCachedCount(event.target.value)}
                  />
                  <FieldDescription>
                    {t(
                      "placement.downloadedCopies.description",
                      undefined,
                      "How many workers should keep the artifact on disk."
                    )}
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel>
                    {t(
                      "placement.field.autoBalance",
                      undefined,
                      "Auto balance"
                    )}
                  </FieldLabel>
                  <Toggle
                    variant="outline"
                    pressed={autoBalance}
                    onPressedChange={setAutoBalance}
                  >
                    {autoBalance
                      ? t("common.on", undefined, "On")
                      : t("common.off", undefined, "Off")}
                  </Toggle>
                  <FieldDescription>
                    {t(
                      "placement.autoBalance.description",
                      undefined,
                      "Raise or lower effective ready copies from recent demand."
                    )}
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel>
                    {t(
                      "placement.field.readyCopies",
                      undefined,
                      "Ready copies"
                    )}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={warmCount}
                    onChange={(event) => setWarmCount(event.target.value)}
                  />
                  <FieldDescription>
                    {t(
                      "placement.readyCopies.description",
                      undefined,
                      "How many slots should keep the model loaded."
                    )}
                  </FieldDescription>
                </Field>
              </div>
              {autoBalance ? (
                <div className="grid gap-4 md:grid-cols-4">
                  <Field>
                    <FieldLabel>
                      {t("placement.field.minReady", undefined, "Min ready")}
                    </FieldLabel>
                    <Input
                      type="number"
                      value={minWarmCount}
                      onChange={(event) => setMinWarmCount(event.target.value)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel>
                      {t("placement.field.maxReady", undefined, "Max ready")}
                    </FieldLabel>
                    <Input
                      type="number"
                      value={maxWarmCount}
                      onChange={(event) => setMaxWarmCount(event.target.value)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel>
                      {t(
                        "placement.field.minDownloaded",
                        undefined,
                        "Min downloaded"
                      )}
                    </FieldLabel>
                    <Input
                      type="number"
                      value={minCachedCount}
                      onChange={(event) =>
                        setMinCachedCount(event.target.value)
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel>
                      {t(
                        "placement.field.maxDownloaded",
                        undefined,
                        "Max downloaded"
                      )}
                    </FieldLabel>
                    <Input
                      type="number"
                      value={maxCachedCount}
                      onChange={(event) =>
                        setMaxCachedCount(event.target.value)
                      }
                    />
                  </Field>
                </div>
              ) : null}
              <Button variant="outline" onClick={() => setAdvanced(!advanced)}>
                {advanced
                  ? t(
                      "placement.advanced.hide",
                      undefined,
                      "Hide advanced routing"
                    )
                  : t(
                      "placement.advanced.show",
                      undefined,
                      "Show advanced routing"
                    )}
              </Button>
              {advanced ? (
                <div className="grid gap-4 rounded-lg border bg-muted/50 p-4">
                  <Field>
                    <FieldLabel>
                      {t(
                        "placement.field.requiredTags",
                        undefined,
                        "Required tags"
                      )}
                    </FieldLabel>
                    <Input
                      value={tags}
                      onChange={(event) => setTags(event.target.value)}
                      placeholder="local, trusted"
                    />
                  </Field>
                  <Field>
                    <FieldLabel>
                      {t(
                        "placement.field.preferredWorkers",
                        undefined,
                        "Preferred workers"
                      )}
                    </FieldLabel>
                    <Input
                      value={preferred}
                      onChange={(event) => setPreferred(event.target.value)}
                      placeholder="node-a, node-b"
                    />
                  </Field>
                  <Field>
                    <FieldLabel>
                      {t(
                        "placement.field.deniedWorkers",
                        undefined,
                        "Denied workers"
                      )}
                    </FieldLabel>
                    <Input
                      value={denied}
                      onChange={(event) => setDenied(event.target.value)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel>
                      {t(
                        "placement.field.pinnedSlots",
                        undefined,
                        "Hard-pinned slots"
                      )}
                    </FieldLabel>
                    <Textarea
                      value={pins}
                      onChange={(event) => setPins(event.target.value)}
                      placeholder="node-id/metal0"
                    />
                  </Field>
                  <div className="grid gap-4 md:grid-cols-2">
                    <Field>
                      <FieldLabel>
                        {t(
                          "placement.field.maxWarmPerWorker",
                          undefined,
                          "Max warm models per worker"
                        )}
                      </FieldLabel>
                      <Input
                        type="number"
                        value={maxWarmProfilesPerNode}
                        onChange={(event) =>
                          setMaxWarmProfilesPerNode(event.target.value)
                        }
                      />
                    </Field>
                    <Field>
                      <FieldLabel>
                        {t(
                          "placement.field.maxCachedPerWorker",
                          undefined,
                          "Max downloaded models per worker"
                        )}
                      </FieldLabel>
                      <Input
                        type="number"
                        value={maxCachedProfilesPerNode}
                        onChange={(event) =>
                          setMaxCachedProfilesPerNode(event.target.value)
                        }
                      />
                    </Field>
                  </div>
                </div>
              ) : null}
            </FieldGroup>
            <div className="flex flex-wrap gap-2">
              <Button
                onClick={() =>
                  confirmPolicy(
                    actions,
                    t,
                    profileId,
                    cachedCount,
                    warmCount,
                    tags,
                    preferred,
                    denied,
                    pins,
                    autoBalance,
                    minCachedCount,
                    maxCachedCount,
                    minWarmCount,
                    maxWarmCount,
                    maxCachedProfilesPerNode,
                    maxWarmProfilesPerNode
                  )
                }
              >
                {t("placement.action.applyNow", undefined, "Apply now")}
              </Button>
              <Button
                variant="secondary"
                onClick={() => confirmPlanner(actions, t)}
              >
                {t("placement.action.runPlanner", undefined, "Run planner")}
              </Button>
              <Button
                variant="outline"
                disabled={explainLoading}
                onClick={() =>
                  loadPlacementExplain(
                    actions,
                    setExplain,
                    setExplainLoading,
                    setExplainError
                  )
                }
              >
                {explainLoading
                  ? t("placement.explain.loading", undefined, "Explaining")
                  : t(
                      "placement.action.explainPlacement",
                      undefined,
                      "Explain placement"
                    )}
              </Button>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>
              {t("placement.preview.title", undefined, "What will happen")}
            </CardTitle>
            <CardDescription>
              {t(
                "placement.preview.description",
                undefined,
                "Preview current gaps before applying the desired capacity."
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 md:grid-cols-2">
            <DryRun
              title={t(
                "placement.preview.effectiveDesired",
                undefined,
                "Effective desired"
              )}
              items={[
                effectiveDesiredText(selectedPolicy, cachedCount, warmCount, t),
              ]}
            />
            <DryRun
              title={t(
                "placement.preview.demandSignal",
                undefined,
                "Demand signal"
              )}
              items={[demandSignalText(selectedPolicy, t)]}
            />
            <DryRun
              title={t(
                "placement.preview.downloadsNeeded",
                undefined,
                "Downloads needed"
              )}
              items={selectedAssignments
                .filter((a) => a.desiredCached && !a.actualCached)
                .map((a) => a.modelArtifactId || a.profileId)}
            />
            <DryRun
              title={t(
                "placement.preview.warmupsNeeded",
                undefined,
                "Warmups needed"
              )}
              items={selectedAssignments
                .filter((a) => a.desiredWarm && !a.actualWarm)
                .map((a) => a.logicalModel || a.profileId)}
            />
            <DryRun
              title={t(
                "placement.preview.workersUsed",
                undefined,
                "Workers used"
              )}
              items={
                [
                  ...new Set(
                    selectedAssignments.map((a) => a.nodeId).filter(Boolean)
                  ),
                ] as string[]
              }
            />
            <DryRun
              title={t(
                "placement.preview.blockers",
                undefined,
                "Cannot fully apply because"
              )}
              items={selectedAssignments
                .filter((a) => a.mismatchReason)
                .map((a) => human(a.mismatchReason, t))}
            />
          </CardContent>
        </Card>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>
            {t("placement.current.title", undefined, "Current capacity state")}
          </CardTitle>
          <CardDescription>
            {t(
              "placement.current.description",
              undefined,
              "Desired vs actual copies for the selected model."
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            items={selectedAssignments}
            empty={t(
              "placement.current.empty",
              undefined,
              "No capacity plan for this model"
            )}
            columns={[
              {
                header: t("placement.column.model", undefined, "Model"),
                cell: (item) => item.logicalModel || "-",
              },
              {
                header: t(
                  "placement.column.workerSlot",
                  undefined,
                  "Worker slot"
                ),
                cell: (item) => <code>{short(item.slotId)}</code>,
              },
              {
                header: t("placement.column.desired", undefined, "Desired"),
                cell: (item) =>
                  t(
                    "placement.copies.summary",
                    { downloaded: item.desiredCached, ready: item.desiredWarm },
                    `downloaded=${item.desiredCached} ready=${item.desiredWarm}`
                  ),
              },
              {
                header: t("placement.column.actual", undefined, "Actual"),
                cell: (item) =>
                  t(
                    "placement.copies.summary",
                    { downloaded: item.actualCached, ready: item.actualWarm },
                    `downloaded=${item.actualCached} ready=${item.actualWarm}`
                  ),
              },
              {
                header: t("placement.column.state", undefined, "State"),
                cell: (item) => (
                  <StatusBadge
                    value={
                      item.ready ? "ready" : item.mismatchReason || "waiting"
                    }
                  />
                ),
              },
              {
                header: t("placement.column.reason", undefined, "Reason"),
                cell: (item) => human(item.mismatchReason, t),
              },
            ]}
          />
        </CardContent>
      </Card>
      <CachePlanPanel
        actions={actions}
        assignments={selectedAssignments}
        cachePlan={selectedCachePlan}
        profileId={profileId}
        t={t}
      />
      <PlacementExplainPanel
        explain={explain}
        error={explainError}
        loading={explainLoading}
        profileId={profileId}
        refresh={() =>
          loadPlacementExplain(
            actions,
            setExplain,
            setExplainLoading,
            setExplainError
          )
        }
        t={t}
      />
      <Card>
        <CardHeader>
          <CardTitle>
            {t("placement.fit.title", undefined, "Technical fit checks")}
          </CardTitle>
          <CardDescription>
            {t(
              "placement.fit.description",
              undefined,
              "Why a model can or cannot run on each slot."
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            items={(state.fitMatrix ?? []).filter(
              (item) => !profileId || item.profileId === profileId
            )}
            empty={t("placement.fit.empty", undefined, "No fit results")}
            columns={[
              {
                header: t("placement.column.model", undefined, "Model"),
                cell: (item: FitResult) => item.logicalModel || "-",
              },
              {
                header: t("placement.column.slot", undefined, "Slot"),
                cell: (item) => <code>{short(item.slotId)}</code>,
              },
              {
                header: t("placement.column.fits", undefined, "Fits"),
                cell: (item) => <StatusBadge value={String(item.fits)} />,
              },
              {
                header: t("placement.column.reason", undefined, "Reason"),
                cell: (item) => human((item.reasons || []).join(", "), t),
              },
            ]}
          />
        </CardContent>
      </Card>
    </>
  )
}

function CachePlanPanel({
  actions,
  assignments,
  cachePlan,
  profileId,
  t,
}: {
  actions: Actions
  assignments: Assignment[]
  cachePlan?: CachePlan
  profileId: string
  t: TFunction
}) {
  const desiredNodes = desiredCachedNodes(assignments, profileId)
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t("placement.cachePlan.title", undefined, "Cache plan")}
        </CardTitle>
        <CardDescription>
          {t(
            "placement.cachePlan.description",
            undefined,
            "Desired, actual, stale, and cleanup state for Worker-local model files."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {!cachePlan ? (
          <p className="text-sm text-muted-foreground">
            {t(
              "placement.cachePlan.empty",
              undefined,
              "No cache plan for the selected model."
            )}
          </p>
        ) : (
          <>
            <div className="grid gap-3 sm:grid-cols-4">
              <CacheMetric
                label={t("placement.cachePlan.desired", undefined, "Desired")}
                value={String(cachePlan.desiredCopies ?? 0)}
              />
              <CacheMetric
                label={t("placement.cachePlan.actual", undefined, "Actual")}
                value={String(cachePlan.actualCopies ?? 0)}
              />
              <CacheMetric
                label={t("placement.cachePlan.stale", undefined, "Stale")}
                value={String(cachePlan.staleCopies ?? 0)}
              />
              <CacheMetric
                label={t("placement.cachePlan.evictions", undefined, "Cleanup")}
                value={String(cachePlan.evictionsPending ?? 0)}
              />
            </div>
            <DataTable
              items={cachePlan.workers ?? []}
              empty={t(
                "placement.cachePlan.workers.empty",
                undefined,
                "No Worker cache state for this model."
              )}
              columns={[
                {
                  header: t("placement.column.worker", undefined, "Worker"),
                  cell: (item: CacheWorkerStatus) => (
                    <code>{short(item.nodeId)}</code>
                  ),
                },
                {
                  header: t("placement.cachePlan.cached", undefined, "Cached"),
                  cell: (item) => <StatusBadge value={String(!!item.cached)} />,
                },
                {
                  header: t("placement.cachePlan.warm", undefined, "Warm"),
                  cell: (item) => <StatusBadge value={String(!!item.warm)} />,
                },
                {
                  header: t("placement.cachePlan.active", undefined, "Active"),
                  cell: (item) => <StatusBadge value={String(!!item.active)} />,
                },
                {
                  header: t(
                    "placement.cachePlan.eviction",
                    undefined,
                    "Cleanup"
                  ),
                  cell: (item) => (
                    <StatusBadge value={item.eviction?.status || "none"} />
                  ),
                },
                {
                  header: t("placement.cachePlan.intent", undefined, "Intent"),
                  cell: (item) => (
                    <StatusBadge value={item.intent?.action || "none"} />
                  ),
                },
                {
                  header: t("placement.column.reason", undefined, "Reason"),
                  cell: (item) =>
                    human(item.eviction?.failure || item.eviction?.reason, t),
                },
                {
                  header: t("placement.cachePlan.action", undefined, "Action"),
                  cell: (item) => (
                    <CacheWorkerActions
                      actions={actions}
                      artifacts={cachePlan.artifacts ?? []}
                      isStale={isStaleCacheWorker(item, desiredNodes)}
                      nodeId={item.nodeId}
                      t={t}
                    />
                  ),
                },
              ]}
            />
            <ConditionList
              conditions={cachePlan.conditions}
              empty={t(
                "placement.cachePlan.conditions.empty",
                undefined,
                "No cache plan conditions reported."
              )}
              t={t}
            />
          </>
        )}
      </CardContent>
    </Card>
  )
}

function CacheMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border bg-muted/50 p-3">
      <div className="font-mono text-xs text-muted-foreground uppercase">
        {label}
      </div>
      <div className="mt-1 text-lg font-semibold">{value}</div>
    </div>
  )
}

function CacheWorkerActions({
  actions,
  artifacts,
  isStale,
  nodeId,
  t,
}: {
  actions: Actions
  artifacts: string[]
  isStale: boolean
  nodeId: string
  t: TFunction
}) {
  if (!isStale) return <span>{t("common.none", undefined, "None")}</span>
  const disabled = artifacts.length === 0
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={() =>
          setCacheArtifactAction(nodeId, artifacts, "keep", actions)
        }
      >
        {t("nodes.action.cacheKeep", undefined, "Keep")}
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={() =>
          setCacheArtifactAction(nodeId, artifacts, "evict_when_idle", actions)
        }
      >
        {t("nodes.action.cacheEvictWhenIdle", undefined, "Evict when idle")}
      </Button>
      <Button
        size="sm"
        variant="destructive"
        disabled={disabled}
        onClick={() =>
          setCacheArtifactAction(nodeId, artifacts, "evict", actions)
        }
      >
        {t("nodes.action.evictArtifact", undefined, "Remove from worker")}
      </Button>
    </div>
  )
}

function desiredCachedNodes(assignments: Assignment[], profileId: string) {
  return new Set(
    assignments
      .filter(
        (assignment) =>
          assignment.profileId === profileId &&
          assignment.nodeId &&
          assignment.desiredCached
      )
      .map((assignment) => assignment.nodeId as string)
  )
}

function isStaleCacheWorker(
  worker: CacheWorkerStatus,
  desiredNodes: Set<string>
) {
  return Boolean(
    worker.cached && !worker.active && !desiredNodes.has(worker.nodeId)
  )
}

function PlacementExplainPanel({
  explain,
  error,
  loading,
  profileId,
  refresh,
  t,
}: {
  explain: PlacementExplainResponse | null
  error: string
  loading: boolean
  profileId: string
  refresh: () => void
  t: TFunction
}) {
  const profile = explain?.profiles?.find(
    (item) => item.profileId === profileId
  )
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div>
            <CardTitle>
              {t("placement.explain.title", undefined, "Placement explain")}
            </CardTitle>
            <CardDescription>
              {t(
                "placement.explain.description",
                undefined,
                "Dry-run candidate reasons from the Manager planner."
              )}
            </CardDescription>
          </div>
          <Button variant="outline" disabled={loading} onClick={refresh}>
            {loading
              ? t("placement.explain.loading", undefined, "Explaining")
              : t(
                  "placement.action.explainPlacement",
                  undefined,
                  "Explain placement"
                )}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {error ? (
          <p className="text-sm text-destructive">
            {t(
              "placement.explain.error",
              { error },
              `Explain failed: ${error}`
            )}
          </p>
        ) : null}
        {!explain ? (
          <p className="text-sm text-muted-foreground">
            {t(
              "placement.explain.empty",
              undefined,
              "Run explain to inspect selected, rejected, and missing placement candidates."
            )}
          </p>
        ) : !profile ? (
          <p className="text-sm text-muted-foreground">
            {t(
              "placement.explain.noProfile",
              undefined,
              "No explain data for the selected model."
            )}
          </p>
        ) : (
          <>
            <div className="text-xs text-muted-foreground">
              {t(
                "placement.explain.generated",
                { time: timeAgo(explain.generatedAt, t) },
                `Generated ${timeAgo(explain.generatedAt, t)}`
              )}
            </div>
            <div className="grid gap-3 md:grid-cols-3">
              <DryRun
                title={t("placement.explain.selected", undefined, "Selected")}
                items={candidateItems(profile.selected, t)}
              />
              <DryRun
                title={t("placement.explain.rejected", undefined, "Rejected")}
                items={candidateItems(profile.rejected, t)}
              />
              <DryRun
                title={t("placement.explain.missing", undefined, "Missing")}
                items={missingItems(profile.missing, t)}
              />
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function candidateItems(
  candidates: PlacementCandidateExplanation[] | undefined,
  t: TFunction
) {
  return (candidates ?? []).map((candidate) => {
    const target =
      candidate.slotId || candidate.nodeId || candidate.modelArtifactId
    const reasons = reasonList(candidate.reasons, t)
    return `${human(candidate.phase, t)} ${short(target)}: ${reasons}`
  })
}

function missingItems(
  missing: PlacementMissingExplanation[] | undefined,
  t: TFunction
) {
  return (missing ?? []).map(
    (item) => `${human(item.phase, t)}: ${reasonList(item.reasons, t)}`
  )
}

function reasonList(reasons: string[] | undefined, t: TFunction) {
  return reasons?.length
    ? reasons.map((reason) => human(reason, t)).join(", ")
    : t("common.none", undefined, "None")
}

function effectiveDesiredText(
  policy: Policy | undefined,
  cachedCount: string,
  warmCount: string,
  t: TFunction
) {
  const cached = policy?.effectiveCachedCount ?? Number(cachedCount || 0)
  const warm = policy?.effectiveWarmCount ?? Number(warmCount || 0)
  return t(
    "placement.preview.effectiveDesired.value",
    { downloaded: cached, ready: warm },
    `downloaded=${cached} ready=${warm}`
  )
}

function demandSignalText(policy: Policy | undefined, t: TFunction) {
  if (!policy?.autoBalance) {
    return t(
      "placement.preview.demandSignal.manual",
      undefined,
      "manual capacity"
    )
  }
  const queued = policy.demandQueued ?? 0
  const running = policy.demandRunning ?? 0
  const recent = policy.demandRecent ?? 0
  const smoothed = policy.demandSmoothed ?? 0
  return t(
    "placement.preview.demandSignal.value",
    { queued, running, recent, smoothed },
    `queued=${queued} running=${running} recent10m=${recent} smoothed=${smoothed}`
  )
}

async function loadPlacementExplain(
  actions: Actions,
  setExplain: (value: PlacementExplainResponse | null) => void,
  setLoading: (value: boolean) => void,
  setError: (value: string) => void
) {
  setLoading(true)
  setError("")
  try {
    setExplain(
      await actions.fetchJSON<PlacementExplainResponse>(
        "/api/admin/placement/explain"
      )
    )
  } catch (error) {
    setError(error instanceof Error ? error.message : String(error))
  } finally {
    setLoading(false)
  }
}

function confirmPolicy(
  actions: Actions,
  t: TFunction,
  profileId: string,
  cachedCount: string,
  warmCount: string,
  tags: string,
  preferred: string,
  denied: string,
  pins: string,
  autoBalance: boolean,
  minCachedCount: string,
  maxCachedCount: string,
  minWarmCount: string,
  maxWarmCount: string,
  maxCachedProfilesPerNode: string,
  maxWarmProfilesPerNode: string
) {
  actions.setConfirm({
    title: t(
      "placement.confirm.apply.title",
      undefined,
      "Apply capacity policy"
    ),
    body: t(
      "placement.confirm.apply.body",
      {
        model:
          profileId ||
          t("placement.confirm.selectedModel", undefined, "the selected model"),
        downloaded: cachedCount || 0,
        ready: warmCount || 0,
      },
      `This changes desired capacity for ${profileId || "the selected model"} to ${cachedCount || 0} downloaded and ${warmCount || 0} ready copies.`
    ),
    confirmLabel: t("placement.confirm.apply.label", undefined, "Apply policy"),
    variant: "default",
    run: () =>
      savePolicy(
        actions,
        profileId,
        cachedCount,
        warmCount,
        tags,
        preferred,
        denied,
        pins,
        autoBalance,
        minCachedCount,
        maxCachedCount,
        minWarmCount,
        maxWarmCount,
        maxCachedProfilesPerNode,
        maxWarmProfilesPerNode
      ),
  })
}

function confirmPlanner(actions: Actions, t: TFunction) {
  actions.setConfirm({
    title: t(
      "placement.confirm.planner.title",
      undefined,
      "Run capacity planner"
    ),
    body: t(
      "placement.confirm.planner.body",
      undefined,
      "The Manager will reconcile desired downloaded and ready copies against current Worker slots."
    ),
    confirmLabel: t(
      "placement.confirm.planner.label",
      undefined,
      "Run planner"
    ),
    variant: "default",
    run: async () => {
      await actions.api("/api/admin/placement/apply", { method: "POST" })
      toast.success(
        t("placement.toast.plannerRan", undefined, "Capacity planner ran")
      )
    },
  })
}

function countField(value?: number) {
  return value ? String(value) : ""
}
