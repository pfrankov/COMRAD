import {
  ArrowRightIcon,
  BoxesIcon,
  CpuIcon,
  DatabaseIcon,
  KeyRoundIcon,
  ListChecksIcon,
  NetworkIcon,
  SlidersHorizontalIcon,
  UploadCloudIcon,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { DataTable } from "@/components/comrad/data-table"
import { MetricCard } from "@/components/comrad/metric-card"
import { PageTitle } from "@/components/comrad/dashboard-primitives"
import { StatusBadge } from "@/components/comrad/status-badge"
import { useI18n } from "@/i18n/i18n-provider"
import { attentionItems, human, summary } from "@/lib/comrad"
import type { Actions } from "@/comrad/actions"
import type { StateResponse } from "@/types"

export function OverviewPage({
  state,
  actions,
}: {
  state: StateResponse
  actions: Actions
}) {
  const { t } = useI18n()
  const s = summary(state)
  const items = attentionItems(state, t)
  return (
    <>
      <PageTitle
        eyebrow={t("nav.group.operate", undefined, "Operate")}
        title={t("overview.title", undefined, "Overview")}
        description={t(
          "overview.description",
          undefined,
          "A command center for serving models, operating worker capacity, and understanding what needs attention."
        )}
      />
      <Card className="overflow-hidden">
        <CardHeader>
          <Badge variant="secondary">
            {t("overview.operatingModel", undefined, "COMRAD operating model")}
          </Badge>
          <CardTitle className="mt-4 text-3xl leading-tight">
            {s.healthy
              ? t("overview.readyTitle", undefined, "Ready to serve requests")
              : t(
                  "overview.attentionTitle",
                  undefined,
                  "Attention needed before serving reliably"
                )}
          </CardTitle>
          <CardDescription className="max-w-3xl text-base">
            {t(
              "overview.heroDescription",
              undefined,
              "The Manager accepts client requests, chooses a compatible ready Worker slot, streams output back, and records reports plus compute accounting."
            )}
          </CardDescription>
          <CardAction>
            <StatusBadge value={s.healthy ? "ready" : "waiting"} />
          </CardAction>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-4">
          <MetricCard
            label={t("overview.metric.readySlots", undefined, "Ready slots")}
            value={s.ready}
            detail={t(
              "overview.metric.readySlots.detail",
              undefined,
              "Can receive work now"
            )}
          />
          <MetricCard
            label={t("overview.metric.modelsReady", undefined, "Models ready")}
            value={`${s.warmActual}/${s.warmDesired}`}
            detail={t(
              "overview.metric.modelsReady.detail",
              undefined,
              "Ready copies vs target"
            )}
          />
          <MetricCard
            label={t(
              "overview.metric.queuedRequests",
              undefined,
              "Queued requests"
            )}
            value={s.queued}
            detail={t(
              "overview.metric.queuedRequests.detail",
              undefined,
              "Waiting for capacity"
            )}
          />
          <MetricCard
            label={t(
              "overview.metric.failuresHour",
              undefined,
              "Failures last hour"
            )}
            value={s.failHour}
            detail={t(
              "overview.metric.failuresHour.detail",
              undefined,
              "Recent failed attempts"
            )}
          />
        </CardContent>
      </Card>
      <div className="grid gap-4 xl:grid-cols-[1.15fr_0.85fr]">
        <Card>
          <CardHeader>
            <CardTitle>
              {t("overview.how.title", undefined, "How COMRAD works")}
            </CardTitle>
            <CardDescription>
              {t(
                "overview.how.description",
                undefined,
                "The system is small: each part has one job and one owner."
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 md:grid-cols-3">
            <SystemPart
              icon={NetworkIcon}
              title={t(
                "overview.part.manager.title",
                undefined,
                "Manager coordinates"
              )}
              body={t(
                "overview.part.manager.body",
                undefined,
                "Owns auth, queueing, placement, reports, dashboard, and the public API."
              )}
            />
            <SystemPart
              icon={BoxesIcon}
              title={t(
                "overview.part.models.title",
                undefined,
                "Models define work"
              )}
              body={t(
                "overview.part.models.body",
                undefined,
                "A model is the client-facing name plus exact artifacts, context, cost, and llama.cpp server args."
              )}
            />
            <SystemPart
              icon={CpuIcon}
              title={t(
                "overview.part.workers.title",
                undefined,
                "Workers run requests"
              )}
              body={t(
                "overview.part.workers.body",
                undefined,
                "Workers connect outbound, report slots and budgets, then execute assigned artifacts only."
              )}
            />
            <SystemPart
              icon={SlidersHorizontalIcon}
              title={t(
                "overview.part.capacity.title",
                undefined,
                "Capacity keeps models ready"
              )}
              body={t(
                "overview.part.capacity.body",
                undefined,
                "Downloaded copies cache files; ready copies keep the model loaded on compatible slots."
              )}
            />
            <SystemPart
              icon={KeyRoundIcon}
              title={t(
                "overview.part.clients.title",
                undefined,
                "API clients spend compute"
              )}
              body={t(
                "overview.part.clients.body",
                undefined,
                "API keys identify clients. Completed paid work writes auditable ledger entries."
              )}
            />
            <SystemPart
              icon={DatabaseIcon}
              title={t(
                "overview.part.storage.title",
                undefined,
                "Storage verifies artifacts"
              )}
              body={t(
                "overview.part.storage.body",
                undefined,
                "Model files are addressed by sha256 so execution is exact and repeatable."
              )}
            />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>
              {t("overview.workflows.title", undefined, "Admin workflows")}
            </CardTitle>
            <CardDescription>
              {t(
                "overview.workflows.description",
                undefined,
                "Start from the thing you want to accomplish."
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2">
            <WorkflowAction
              icon={BoxesIcon}
              label={t("overview.workflow.models", undefined, "Serve models")}
              detail={t(
                "overview.workflow.models.detail",
                undefined,
                "Add, edit, tune, and price model profiles."
              )}
              onClick={() => actions.show("profiles")}
            />
            <WorkflowAction
              icon={SlidersHorizontalIcon}
              label={t(
                "overview.workflow.capacity",
                undefined,
                "Supply capacity"
              )}
              detail={t(
                "overview.workflow.capacity.detail",
                undefined,
                "Keep model copies downloaded and ready."
              )}
              onClick={() => actions.show("placement")}
            />
            <WorkflowAction
              icon={KeyRoundIcon}
              label={t("overview.workflow.access", undefined, "Control access")}
              detail={t(
                "overview.workflow.access.detail",
                undefined,
                "Find API clients, issue keys, edit status, and adjust balances."
              )}
              onClick={() => actions.show("users")}
            />
            <WorkflowAction
              icon={ListChecksIcon}
              label={t(
                "overview.workflow.requests",
                undefined,
                "Observe requests"
              )}
              detail={t(
                "overview.workflow.requests.detail",
                undefined,
                "Inspect queues, attempts, failures, and reports."
              )}
              onClick={() => actions.show("tasks")}
            />
            <WorkflowAction
              icon={UploadCloudIcon}
              label={t(
                "overview.workflow.updates",
                undefined,
                "Roll out updates"
              )}
              detail={t(
                "overview.workflow.updates.detail",
                undefined,
                "Update idle workers and retry failed updates."
              )}
              onClick={() => actions.show("updates")}
            />
          </CardContent>
        </Card>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>
            {t("overview.triage.title", undefined, "Readiness triage")}
          </CardTitle>
          <CardDescription>
            {t(
              "overview.triage.description",
              undefined,
              "Needs attention items include the reason, impact, and safest next action."
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            items={items}
            columns={[
              {
                header: t("overview.table.problem", undefined, "Problem"),
                cell: (item) => (
                  <div className="font-medium">{item.problem}</div>
                ),
              },
              {
                header: t("overview.table.reason", undefined, "Reason"),
                cell: (item) => human(item.reason, t),
              },
              {
                header: t("overview.table.impact", undefined, "Impact"),
                cell: (item) => item.impact,
              },
              {
                header: t("overview.table.action", undefined, "Action"),
                cell: (item) => (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => actions.show(item.target)}
                  >
                    {item.action}
                  </Button>
                ),
              },
            ]}
          />
        </CardContent>
      </Card>
    </>
  )
}

function SystemPart({
  icon: Icon,
  title,
  body,
}: {
  icon: typeof NetworkIcon
  title: string
  body: string
}) {
  return (
    <div className="rounded-lg border bg-muted/50 p-4">
      <div className="flex items-center gap-2 font-medium">
        <Icon data-icon="inline-start" />
        {title}
      </div>
      <Separator className="my-3" />
      <p className="text-sm leading-6 text-muted-foreground">{body}</p>
    </div>
  )
}

function WorkflowAction({
  icon: Icon,
  label,
  detail,
  onClick,
}: {
  icon: typeof BoxesIcon
  label: string
  detail: string
  onClick: () => void
}) {
  return (
    <Button
      variant="ghost"
      className="h-auto justify-start gap-3 px-3 py-3 text-left"
      onClick={onClick}
    >
      <Icon data-icon="inline-start" />
      <span className="min-w-0 flex-1">
        <span className="block font-medium">{label}</span>
        <span className="block text-xs leading-5 text-muted-foreground">
          {detail}
        </span>
      </span>
      <ArrowRightIcon data-icon="inline-end" />
    </Button>
  )
}
