import { useEffect, useMemo, useState } from "react"

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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { PageTitle } from "@/components/comrad/dashboard-primitives"
import { DataTable } from "@/components/comrad/data-table"
import { StatusBadge } from "@/components/comrad/status-badge"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import { human, short, timeAgo } from "@/lib/comrad"
import type { Actions } from "@/comrad/actions"
import type {
  Attempt,
  Report,
  StateResponse,
  Task,
  TaskListResponse,
} from "@/types"

const pageLimit = 50

export function TasksPage({
  state,
  actions,
}: {
  state: StateResponse
  actions: Actions
}) {
  const { t } = useI18n()
  const [status, setStatus] = useState("all")
  const [userId, setUserId] = useState("")
  const [profileId, setProfileId] = useState("")
  const [taskId, setTaskId] = useState("")
  const [offset, setOffset] = useState(0)
  const [page, setPage] = useState<TaskListResponse | null>(null)
  const [error, setError] = useState("")

  const hasServerFilters = useMemo(
    () =>
      status !== "all" ||
      Boolean(userId.trim()) ||
      Boolean(profileId.trim()) ||
      Boolean(taskId.trim()),
    [profileId, status, taskId, userId]
  )
  const usesLiveState = !hasServerFilters && offset === 0
  const serverStateVersion = useMemo(
    () => taskStateVersion(state),
    [state.attempts, state.reports, state.taskSummary, state.tasks]
  )
  const serverRefreshKey = usesLiveState ? "" : serverStateVersion

  useEffect(() => setOffset(0), [status, userId, profileId, taskId])

  const query = useMemo(
    () => taskQuery(status, userId, profileId, taskId, offset),
    [status, userId, profileId, taskId, offset]
  )
  useEffect(() => {
    if (usesLiveState) {
      setPage(null)
      setError("")
      return
    }
    let cancelled = false
    actions
      .fetchJSON<TaskListResponse>(`/api/admin/tasks?${query}`)
      .then((next) => {
        if (!cancelled) {
          setPage(next)
          setError("")
        }
      })
      .catch((nextError) => {
        if (!cancelled) setError(nextError.message)
      })
    return () => {
      cancelled = true
    }
  }, [actions, query, serverRefreshKey, usesLiveState])

  const liveTasks = useMemo(
    () => sortTasksNewest(state.tasks ?? []),
    [state.tasks]
  )
  const tasks = useMemo(
    () => (usesLiveState ? liveTasks : (page?.items ?? [])),
    [liveTasks, page, usesLiveState]
  )
  const attempts = useMemo(
    () => (usesLiveState ? (state.attempts ?? []) : (page?.attempts ?? [])),
    [page, state.attempts, usesLiveState]
  )
  const reports = useMemo(
    () => (usesLiveState ? (state.reports ?? []) : (page?.reports ?? [])),
    [page, state.reports, usesLiveState]
  )
  const summary = usesLiveState ? state.taskSummary : page?.summary
  const latest = useMemo(() => latestTasks(tasks), [tasks])
  const attemptByTask = useMemo(() => latestAttemptByTask(attempts), [attempts])
  const reportByTask = useMemo(() => latestReportByTask(reports), [reports])
  const total = usesLiveState
    ? (summary?.total ?? tasks.length)
    : (page?.total ?? summary?.total ?? tasks.length)
  const start = total === 0 ? 0 : offset + 1
  const end = Math.min(offset + tasks.length, total)
  const hasMore = usesLiveState
    ? state.tasksTruncated || tasks.length < total
    : Boolean(page?.hasMore)

  return (
    <>
      <PageTitle
        eyebrow={t("nav.group.operate", undefined, "Operate")}
        title={t("tasks.title", undefined, "Requests")}
        description={t(
          "tasks.description",
          undefined,
          "Paginated request and attempt lifecycle without prompt or response content."
        )}
      />
      <TaskFilters
        t={t}
        status={status}
        userId={userId}
        profileId={profileId}
        taskId={taskId}
        setStatus={setStatus}
        setUserId={setUserId}
        setProfileId={setProfileId}
        setTaskId={setTaskId}
      />
      {state.tasksTruncated ? (
        <p className="text-sm text-muted-foreground">
          {t(
            "tasks.truncated",
            { limit: state.taskPageLimit ?? 0 },
            `Cluster state carries the latest ${state.taskPageLimit} requests only. Use this page to inspect the full request history.`
          )}
        </p>
      ) : null}
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      <Tabs defaultValue="history" className="gap-4">
        <TabsList
          variant="line"
          className="w-full justify-start overflow-x-auto"
        >
          <TabsTrigger value="history">
            {t("tasks.tab.history", undefined, "History")}
          </TabsTrigger>
          <TabsTrigger value="users">
            {t("tasks.tab.clients", undefined, "By client")}
          </TabsTrigger>
          <TabsTrigger value="timelines">
            {t("tasks.tab.timelines", undefined, "Timelines")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="history" className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>
                {t("tasks.history.title", undefined, "Request history")}
              </CardTitle>
              <CardDescription>
                {t(
                  "tasks.history.description",
                  undefined,
                  "Request status, selected model, assignment, first-token latency, token rate, and failure reason."
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <DataTable
                items={tasks}
                empty={t("tasks.empty", undefined, "No requests")}
                columns={[
                  {
                    header: t("tasks.column.task", undefined, "Request"),
                    cell: (task: Task) => <code>{short(task.taskId)}</code>,
                  },
                  {
                    header: t("tasks.column.user", undefined, "Client"),
                    cell: (task) => <code>{short(task.userId)}</code>,
                  },
                  {
                    header: t("tasks.column.kind", undefined, "Kind"),
                    cell: (task) => task.kind || "-",
                  },
                  {
                    header: t(
                      "tasks.column.selectedProfile",
                      undefined,
                      "Selected model"
                    ),
                    cell: (task) => <code>{short(task.profileId)}</code>,
                  },
                  {
                    header: t("tasks.column.cost", undefined, "Cost"),
                    cell: (task) => task.computeCost ?? 0,
                  },
                  {
                    header: t("tasks.column.status", undefined, "Status"),
                    cell: (task) => <StatusBadge value={task.status} />,
                  },
                  {
                    header: t("tasks.column.queueAge", undefined, "Queue age"),
                    cell: (task) => timeAgo(task.createdAt, t),
                  },
                  {
                    header: t(
                      "tasks.column.assignedSlot",
                      undefined,
                      "Assigned slot"
                    ),
                    cell: (task) => (
                      <code>
                        {short(attemptByTask.get(task.taskId)?.slotId)}
                      </code>
                    ),
                  },
                  {
                    header: t(
                      "tasks.column.timeToFirstToken",
                      undefined,
                      "Time to first token"
                    ),
                    cell: (task) =>
                      `${reportByTask.get(task.taskId)?.timing?.timeToFirstTokenMs ?? "-"} ms`,
                  },
                  {
                    header: t(
                      "tasks.column.tokensSec",
                      undefined,
                      "Tokens/sec"
                    ),
                    cell: (task) =>
                      (
                        reportByTask.get(task.taskId)?.llm?.tokensPerSecond ?? 0
                      ).toFixed(2),
                  },
                  {
                    header: t("tasks.column.failure", undefined, "Failure"),
                    cell: (task) => human(task.failureReason, t),
                  },
                ]}
              />
            </CardContent>
          </Card>
          <div className="flex items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">
              {t(
                "tasks.showing",
                { start, end, total },
                `Showing ${start}-${end} of ${total}`
              )}
            </p>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={offset === 0}
                onClick={() => setOffset(Math.max(0, offset - pageLimit))}
              >
                {t("tasks.previous", undefined, "Previous")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={!hasMore}
                onClick={() => setOffset(offset + pageLimit)}
              >
                {t("tasks.next", undefined, "Next")}
              </Button>
            </div>
          </div>
        </TabsContent>
        <TabsContent value="users">
          <Card>
            <CardHeader>
              <CardTitle>
                {t(
                  "tasks.byClient.title",
                  undefined,
                  "Requests grouped by API client"
                )}
              </CardTitle>
              <CardDescription>
                {t(
                  "tasks.byClient.description",
                  undefined,
                  "Use this when queue pressure or spend needs to be attributed to an API client."
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <DataTable
                items={summary?.byUser ?? []}
                empty={t(
                  "tasks.byClient.empty",
                  undefined,
                  "No requests by client"
                )}
                columns={[
                  {
                    header: t(
                      "tasks.byClient.requestingUser",
                      undefined,
                      "Requesting client"
                    ),
                    cell: (item) => <code>{short(item.userId)}</code>,
                  },
                  {
                    header: t("tasks.column.total", undefined, "Total"),
                    cell: (item) => item.total ?? 0,
                  },
                  {
                    header: t("tasks.column.queued", undefined, "Queued"),
                    cell: (item) => item.queued ?? 0,
                  },
                  {
                    header: t("tasks.column.running", undefined, "Running"),
                    cell: (item) => item.running ?? 0,
                  },
                  {
                    header: t("tasks.column.completed", undefined, "Completed"),
                    cell: (item) => item.completed ?? 0,
                  },
                  {
                    header: t("tasks.column.failed", undefined, "Failed"),
                    cell: (item) => item.failed ?? 0,
                  },
                  {
                    header: t(
                      "tasks.column.computeCost",
                      undefined,
                      "Compute cost"
                    ),
                    cell: (item) => item.computeCost ?? 0,
                  },
                ]}
              />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="timelines">
          <Card>
            <CardHeader>
              <CardTitle>
                {t(
                  "tasks.timelines.title",
                  undefined,
                  "Latest request timelines"
                )}
              </CardTitle>
              <CardDescription>
                {t(
                  "tasks.timelines.description",
                  undefined,
                  "Retry behavior and excluded candidates are visible without prompt content."
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 xl:grid-cols-2">
              {latest.map((task) => (
                <TaskTimelinePanel
                  key={task.taskId}
                  task={task}
                  attempts={attempts.filter(
                    (attempt) => attempt.taskId === task.taskId
                  )}
                  reports={reports.filter(
                    (report) => report.taskId === task.taskId
                  )}
                  t={t}
                />
              ))}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  )
}

function TaskFilters(props: {
  t: TFunction
  status: string
  userId: string
  profileId: string
  taskId: string
  setStatus: (value: string) => void
  setUserId: (value: string) => void
  setProfileId: (value: string) => void
  setTaskId: (value: string) => void
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {props.t("tasks.filters.title", undefined, "Request history")}
        </CardTitle>
        <CardDescription>
          {props.t(
            "tasks.filters.description",
            undefined,
            "Server-side filters keep the dashboard responsive with large request histories."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <FieldGroup className="grid gap-4 md:grid-cols-4">
          <Field>
            <FieldLabel>
              {props.t("tasks.filter.status", undefined, "Status")}
            </FieldLabel>
            <Select value={props.status} onValueChange={props.setStatus}>
              <SelectTrigger className="w-full">
                <SelectValue
                  placeholder={props.t(
                    "tasks.filter.status",
                    undefined,
                    "Status"
                  )}
                />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="all">
                    {props.t(
                      "tasks.filter.allStatuses",
                      undefined,
                      "All statuses"
                    )}
                  </SelectItem>
                  <SelectItem value="queued">
                    {props.t("value.queued", undefined, "Queued")}
                  </SelectItem>
                  <SelectItem value="running">
                    {props.t("value.running", undefined, "Running")}
                  </SelectItem>
                  <SelectItem value="completed">
                    {props.t("value.completed", undefined, "Completed")}
                  </SelectItem>
                  <SelectItem value="failed">
                    {props.t("value.failed", undefined, "Failed")}
                  </SelectItem>
                  <SelectItem value="cancelled">
                    {props.t("value.cancelled", undefined, "Cancelled")}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel>
              {props.t("tasks.filter.user", undefined, "Client")}
            </FieldLabel>
            <Input
              value={props.userId}
              placeholder="user-id"
              onChange={(event) => props.setUserId(event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>
              {props.t("tasks.filter.profile", undefined, "Model")}
            </FieldLabel>
            <Input
              value={props.profileId}
              placeholder="profile-id"
              onChange={(event) => props.setProfileId(event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>
              {props.t("tasks.filter.task", undefined, "Request")}
            </FieldLabel>
            <Input
              value={props.taskId}
              placeholder="task-id"
              onChange={(event) => props.setTaskId(event.target.value)}
            />
            <FieldDescription className="sr-only">
              {props.t(
                "tasks.filters.appliedServerSide",
                undefined,
                "Filters are applied on the server."
              )}
            </FieldDescription>
          </Field>
        </FieldGroup>
      </CardContent>
    </Card>
  )
}

function taskQuery(
  status: string,
  userId: string,
  profileId: string,
  taskId: string,
  offset: number
) {
  const params = new URLSearchParams({
    limit: String(pageLimit),
    offset: String(offset),
  })
  if (status !== "all") params.set("status", status)
  if (userId.trim()) params.set("userId", userId.trim())
  if (profileId.trim()) params.set("profileId", profileId.trim())
  if (taskId.trim()) params.set("taskId", taskId.trim())
  return params.toString()
}

function latestTasks(tasks: Task[]) {
  return sortTasksNewest(tasks).slice(0, 8)
}

function sortTasksNewest(tasks: Task[]) {
  return [...tasks].sort((a, b) => {
    const created =
      Date.parse(b.createdAt || "") - Date.parse(a.createdAt || "")
    if (created !== 0) return created
    return (b.taskId || "").localeCompare(a.taskId || "")
  })
}

function taskStateVersion(state: StateResponse) {
  return [
    summaryVersion(state.taskSummary),
    taskCollectionVersion(state.tasks),
    attemptCollectionVersion(state.attempts),
    reportCollectionVersion(state.reports),
  ].join(":")
}

function summaryVersion(summary: StateResponse["taskSummary"]) {
  return [
    summary?.total ?? 0,
    summary?.queued ?? 0,
    summary?.running ?? 0,
    summary?.completed ?? 0,
    summary?.failed ?? 0,
    summary?.cancelled ?? 0,
  ].join(",")
}

function taskCollectionVersion(tasks?: Task[]) {
  return (tasks ?? [])
    .map((task) =>
      [
        task.taskId,
        task.status,
        task.updatedAt,
        task.createdAt,
        task.failureReason,
      ].join(",")
    )
    .join(";")
}

function attemptCollectionVersion(attempts?: Attempt[]) {
  return (attempts ?? [])
    .map((attempt) =>
      [
        attempt.attemptId,
        attempt.taskId,
        attempt.status,
        attempt.phase,
        attempt.firstOutputSent ? "1" : "0",
        attempt.failureReason,
        attempt.startedAt,
      ].join(",")
    )
    .join(";")
}

function reportCollectionVersion(reports?: Report[]) {
  return (reports ?? [])
    .map((report) =>
      [
        report.reportId,
        report.taskId,
        report.status,
        report.phase,
        report.failureReason,
        report.createdAt,
        report.llm?.tokensPerSecond,
        report.timing?.timeToFirstTokenMs,
      ].join(",")
    )
    .join(";")
}

function latestAttemptByTask(attempts: Attempt[]) {
  const out = new Map<string, Attempt>()
  for (const attempt of attempts) {
    const previous = out.get(attempt.taskId)
    if (
      !previous ||
      Date.parse(attempt.startedAt || "") >=
        Date.parse(previous.startedAt || "")
    )
      out.set(attempt.taskId, attempt)
  }
  return out
}

function latestReportByTask(reports: Report[]) {
  const out = new Map<string, Report>()
  for (const report of reports) {
    const previous = out.get(report.taskId)
    if (
      !previous ||
      Date.parse(report.createdAt || "") >= Date.parse(previous.createdAt || "")
    )
      out.set(report.taskId, report)
  }
  return out
}

function TaskTimelinePanel({
  task,
  attempts,
  reports,
  t,
}: {
  task: Task
  attempts: Attempt[]
  reports: Report[]
  t: TFunction
}) {
  const steps = [
    t("value.queued", undefined, "queued"),
    ...attempts.flatMap((attempt) => [
      t(
        "tasks.timeline.assigned",
        { slot: attempt.slotId },
        `assigned ${attempt.slotId}`
      ),
      t("tasks.timeline.leaseStarted", undefined, "lease started"),
      attempt.phase === "running"
        ? t("tasks.timeline.accepted", undefined, "worker accepted")
        : t(
            "tasks.timeline.phase",
            { phase: attempt.phase || "assigned" },
            `phase ${attempt.phase || "assigned"}`
          ),
      attempt.firstOutputSent
        ? t("tasks.timeline.firstOutput", undefined, "first output")
        : t(
            "tasks.timeline.beforeFirstOutput",
            undefined,
            "before first output"
          ),
      attempt.status === "failed"
        ? t(
            "tasks.timeline.failed",
            { reason: human(attempt.failureReason, t) },
            `failed: ${attempt.failureReason}`
          )
        : t("tasks.timeline.attemptRunning", undefined, "attempt running"),
    ]),
    ...reports.map((report) =>
      t(
        "tasks.timeline.reportStored",
        {
          status:
            report.status === "completed"
              ? t("value.completed", undefined, "completed")
              : t("value.failed", undefined, "failed"),
        },
        `${report.status === "completed" ? "completed" : "failed"} -> report stored`
      )
    ),
  ]
  return (
    <div className="rounded-lg border bg-muted/50 p-4">
      <div className="font-mono text-sm font-medium">{task.taskId}</div>
      <div className="mt-4 flex flex-col gap-3">
        {steps.map((step, index) => (
          <div
            key={`${step}-${index}`}
            className="flex items-center gap-3 text-sm"
          >
            <span className="size-2 rounded-full bg-primary" />
            <span>{step}</span>
          </div>
        ))}
      </div>
      {task.failedSlots?.length ? (
        <p className="mt-4 text-sm text-muted-foreground">
          {t(
            "tasks.timeline.excludedCandidates",
            { slots: task.failedSlots.join(", ") },
            `Excluded retry candidates: ${task.failedSlots.join(", ")}`
          )}
        </p>
      ) : null}
    </div>
  )
}
