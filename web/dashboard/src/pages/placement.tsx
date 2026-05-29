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
import { ConditionList } from "@/components/comrad/conditions"
import { DataTable } from "@/components/comrad/data-table"
import { DryRun, PageTitle } from "@/components/comrad/dashboard-primitives"
import { StatusBadge } from "@/components/comrad/status-badge"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import { assignmentCounts, human, profileLabel, short } from "@/lib/comrad"
import { savePolicy, type Actions } from "@/comrad/actions"
import type { CachePlan, CacheWorkerStatus, FitResult, StateResponse } from "@/types"

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
  const [advanced, setAdvanced] = useState(false)

  const assignments = useMemo(
    () => state.assignments ?? [],
    [state.assignments]
  )
  const selectedAssignments = useMemo(
    () => assignments.filter((item) => item.profileId === profileId),
    [assignments, profileId]
  )
  const selectedCachePlan = useMemo(
    () => (state.cachePlans ?? []).find((item) => item.profileRef === profileId),
    [state.cachePlans, profileId]
  )
  useEffect(() => {
    if (!profileId && firstProfile) setProfileId(firstProfile)
  }, [firstProfile, profileId])
  useEffect(() => {
    const counts = assignmentCounts(profileId, assignments)
    if (counts.desiredCached) setCachedCount(String(counts.desiredCached))
    if (counts.desiredWarm) setWarmCount(String(counts.desiredWarm))
  }, [assignments, profileId])

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
              <div className="grid gap-4 md:grid-cols-2">
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
                    pins
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
      <CachePlanPanel cachePlan={selectedCachePlan} t={t} />
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
  cachePlan,
  t,
}: {
  cachePlan?: CachePlan
  t: TFunction
}) {
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
                  header: t("placement.cachePlan.eviction", undefined, "Cleanup"),
                  cell: (item) => (
                    <StatusBadge value={item.eviction?.status || "none"} />
                  ),
                },
                {
                  header: t("placement.column.reason", undefined, "Reason"),
                  cell: (item) =>
                    human(item.eviction?.failure || item.eviction?.reason, t),
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

function confirmPolicy(
  actions: Actions,
  t: TFunction,
  profileId: string,
  cachedCount: string,
  warmCount: string,
  tags: string,
  preferred: string,
  denied: string,
  pins: string
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
        pins
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
