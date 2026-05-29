import { useState } from "react"

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
import { Textarea } from "@/components/ui/textarea"
import { PageTitle } from "@/components/comrad/dashboard-primitives"
import { DataTable } from "@/components/comrad/data-table"
import { StatusBadge } from "@/components/comrad/status-badge"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import { human, short } from "@/lib/comrad"
import { triggerUpdate, type Actions } from "@/comrad/actions"
import type { StateResponse, UpdateRecord } from "@/types"

export function UpdatesPage({
  state,
  actions,
}: {
  state: StateResponse
  actions: Actions
}) {
  const { t } = useI18n()
  const [version, setVersion] = useState("")
  const [artifactId, setArtifactId] = useState("")
  const [sha256, setSha256] = useState("")
  const [nodes, setNodes] = useState("")
  const candidates = state.nodes ?? []
  return (
    <>
      <PageTitle
        eyebrow={t("nav.group.govern", undefined, "Admin")}
        title={t("updates.title", undefined, "Updates")}
        description={t(
          "updates.description",
          undefined,
          "Worker software updates, progress, failures, and impact preview."
        )}
      />
      <Card>
        <CardHeader>
          <CardTitle>
            {t("updates.explain.title", undefined, "Worker software updates")}
          </CardTitle>
          <CardDescription>
            {t(
              "updates.explain.description",
              undefined,
              "Use updates only when the installed Worker program itself must change. Model edits do not use updates: changing a model profile automatically sends the new profile version to the affected Workers."
            )}
          </CardDescription>
        </CardHeader>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>
            {t("updates.trigger.title", undefined, "Trigger Worker update")}
          </CardTitle>
          <CardDescription>
            {t(
              "updates.trigger.description",
              undefined,
              "Preview impact before applying a verified Worker package to selected workers."
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <FieldGroup>
            <FieldGroup className="grid gap-4 md:grid-cols-3">
              <Field>
                <FieldLabel>
                  {t(
                    "updates.field.version",
                    undefined,
                    "Target Worker version"
                  )}
                </FieldLabel>
                <Input
                  value={version}
                  onChange={(event) => setVersion(event.target.value)}
                  placeholder="v1.2.3"
                />
              </Field>
              <Field>
                <FieldLabel>
                  {t("updates.field.artifactId", undefined, "Artifact id")}
                </FieldLabel>
                <Input
                  value={artifactId}
                  onChange={(event) => setArtifactId(event.target.value)}
                  placeholder="sha256:..."
                />
              </Field>
              <Field>
                <FieldLabel>
                  {t("updates.field.sha256", undefined, "SHA256")}
                </FieldLabel>
                <Input
                  value={sha256}
                  onChange={(event) => setSha256(event.target.value)}
                  placeholder="sha256:..."
                />
              </Field>
            </FieldGroup>
            <Field>
              <FieldLabel>
                {t("updates.field.targetWorkers", undefined, "Target workers")}
              </FieldLabel>
              <Textarea
                value={nodes}
                onChange={(event) => setNodes(event.target.value)}
                placeholder="node-id, node-id-2"
              />
              <FieldDescription>
                {t(
                  "updates.field.targetWorkers.description",
                  undefined,
                  "Leave this to the idle-worker action when the update should avoid busy or quarantined workers."
                )}
              </FieldDescription>
            </Field>
          </FieldGroup>
          <div className="flex flex-wrap gap-2">
            <Button
              onClick={() =>
                confirmUpdate(actions, t, version, artifactId, sha256, nodes)
              }
            >
              {t("updates.action.updateSelected", undefined, "Update selected")}
            </Button>
            <Button
              variant="outline"
              onClick={() =>
                confirmUpdateIdle(
                  actions,
                  t,
                  candidates,
                  version,
                  artifactId,
                  sha256
                )
              }
            >
              {t(
                "updates.action.updateIdle",
                undefined,
                "Update all idle workers"
              )}
            </Button>
            <Button
              variant="secondary"
              onClick={() =>
                toast.message(
                  t(
                    "updates.action.drainMessage",
                    undefined,
                    "Drain and update requires selecting workers and confirming drain first."
                  )
                )
              }
            >
              {t(
                "updates.action.drainAndUpdate",
                undefined,
                "Drain and update"
              )}
            </Button>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>
            {t("updates.candidates.title", undefined, "Update candidates")}
          </CardTitle>
          <CardDescription>
            {t(
              "updates.candidates.description",
              undefined,
              "Impact preview separates workers that can update now from workers that should wait."
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            items={candidates}
            columns={[
              {
                header: t("updates.column.worker", undefined, "Worker"),
                cell: (node) => <code>{short(node.nodeId)}</code>,
              },
              {
                header: t(
                  "updates.column.currentVersion",
                  undefined,
                  "Current version"
                ),
                cell: (node) => node.version || "-",
              },
              {
                header: t(
                  "updates.column.impactPreview",
                  undefined,
                  "Impact preview"
                ),
                cell: (node) => (
                  <StatusBadge
                    value={
                      node.state === "online" && !node.quarantined
                        ? "update now"
                        : "wait"
                    }
                  />
                ),
              },
              {
                header: t("updates.column.reason", undefined, "Reason"),
                cell: (node) =>
                  human(
                    node.quarantined
                      ? node.quarantineReason
                      : node.state !== "online"
                        ? "worker not online"
                        : "idle or admin selected",
                    t
                  ),
              },
            ]}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>
            {t("updates.records.title", undefined, "Update records")}
          </CardTitle>
          <CardDescription>
            {t(
              "updates.records.description",
              undefined,
              "Created update attempts, status, artifact, and failure reason."
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            items={state.updates ?? []}
            empty={t("updates.records.empty", undefined, "No update records")}
            columns={[
              {
                header: t("updates.column.update", undefined, "Update"),
                cell: (item: UpdateRecord) => (
                  <code>{short(item.updateId)}</code>
                ),
              },
              {
                header: t("updates.column.kind", undefined, "Kind"),
                cell: (item) => item.kind || "-",
              },
              {
                header: t("updates.column.version", undefined, "Version"),
                cell: (item) => item.version || "-",
              },
              {
                header: t("updates.column.artifact", undefined, "Artifact"),
                cell: (item) => <code>{short(item.artifactId)}</code>,
              },
              {
                header: t("updates.column.status", undefined, "Status"),
                cell: (item) => <StatusBadge value={item.status} />,
              },
              {
                header: t("updates.column.failure", undefined, "Failure"),
                cell: (item) => human(item.failure, t),
              },
            ]}
          />
        </CardContent>
      </Card>
    </>
  )
}

function confirmUpdate(
  actions: Actions,
  t: TFunction,
  version: string,
  artifactId: string,
  sha256: string,
  nodes: string
) {
  actions.setConfirm({
    title: t(
      "updates.confirm.trigger.title",
      undefined,
      "Trigger Worker update"
    ),
    body: t(
      "updates.confirm.trigger.body",
      {
        target:
          nodes ||
          t(
            "updates.confirm.selectedTargets",
            undefined,
            "the selected target set"
          ),
        artifact:
          artifactId ||
          t(
            "updates.confirm.providedArtifact",
            undefined,
            "the provided artifact"
          ),
      },
      `This creates an update for ${nodes || "the selected target set"} using ${artifactId || "the provided artifact"}.`
    ),
    confirmLabel: t(
      "updates.confirm.trigger.label",
      undefined,
      "Trigger update"
    ),
    variant: "default",
    run: () => triggerUpdate(actions, version, artifactId, sha256, nodes),
  })
}

function confirmUpdateIdle(
  actions: Actions,
  t: TFunction,
  candidates: StateResponse["nodes"],
  version: string,
  artifactId: string,
  sha256: string
) {
  const nodes = (candidates ?? [])
    .filter((node) => node.state === "online" && !node.quarantined)
    .map((node) => node.nodeId)
    .join(",")
  actions.setConfirm({
    title: t(
      "updates.confirm.idle.title",
      undefined,
      "Update all idle workers"
    ),
    body: t(
      "updates.confirm.idle.body",
      {
        target:
          nodes ||
          t(
            "updates.confirm.noIdleWorkers",
            undefined,
            "no currently idle workers"
          ),
      },
      `This creates a Worker update for ${nodes || "no currently idle nodes"}.`
    ),
    confirmLabel: t(
      "updates.confirm.idle.label",
      undefined,
      "Update idle workers"
    ),
    variant: "default",
    run: () => triggerUpdate(actions, version, artifactId, sha256, nodes),
  })
}
