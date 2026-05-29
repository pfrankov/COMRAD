import { PlusIcon, Trash2Icon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { PageTitle } from "@/components/comrad/dashboard-primitives"
import { DataTable } from "@/components/comrad/data-table"
import { StatusBadge } from "@/components/comrad/status-badge"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import { fmtBytes, profileLabel, short } from "@/lib/comrad"
import { deleteArtifact, type Actions } from "@/comrad/actions"
import type { Artifact, Profile, StateResponse } from "@/types"

export function ArtifactsPage({
  state,
  actions,
}: {
  state: StateResponse
  actions: Actions
}) {
  const { t } = useI18n()
  return (
    <>
      <PageTitle
        eyebrow={t("nav.group.serve", undefined, "Resources")}
        title={t("artifacts.title", undefined, "Storage")}
        description={t(
          "artifacts.description",
          undefined,
          "Verified artifacts cached by Workers and used by models."
        )}
        actions={
          <Button onClick={() => actions.show("profiles")}>
            <PlusIcon data-icon="inline-start" />
            {t("profiles.addModel", undefined, "Add a model")}
          </Button>
        }
      />
      <Card>
        <CardHeader>
          <CardTitle>
            {t("artifacts.inventory.title", undefined, "Storage inventory")}
          </CardTitle>
          <CardDescription>
            {t(
              "artifacts.inventory.description",
              undefined,
              "Artifacts are created from the Models page, verified by sha256, and deletion is available for unused artifacts only."
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            items={state.artifacts ?? []}
            empty={t("artifacts.empty", undefined, "No artifacts registered")}
            columns={[
              {
                header: t("artifacts.column.artifact", undefined, "Artifact"),
                cell: (artifact: Artifact) => (
                  <code>{short(artifact.artifactId)}</code>
                ),
              },
              {
                header: t("artifacts.column.kind", undefined, "Kind"),
                cell: (artifact) => artifact.kind || "-",
              },
              {
                header: t("artifacts.column.size", undefined, "Size"),
                cell: (artifact) => fmtBytes(artifact.sizeBytes),
              },
              {
                header: t("artifacts.column.sha256", undefined, "SHA256"),
                cell: (artifact) => <code>{short(artifact.sha256)}</code>,
              },
              {
                header: t(
                  "artifacts.column.cachedWorkers",
                  undefined,
                  "Cached workers"
                ),
                cell: (artifact) => cachedNodes(artifact, state),
              },
              {
                header: t(
                  "artifacts.column.modelsUsingIt",
                  undefined,
                  "Models using it"
                ),
                cell: (artifact) => modelsUsingArtifact(artifact, state),
              },
              {
                header: t(
                  "artifacts.column.verification",
                  undefined,
                  "Verification"
                ),
                cell: () => <StatusBadge value="verified" />,
              },
              {
                header: t("artifacts.column.action", undefined, "Action"),
                cell: (artifact) => (
                  <DeleteArtifactButton
                    artifact={artifact}
                    state={state}
                    actions={actions}
                    t={t}
                  />
                ),
              },
            ]}
          />
        </CardContent>
      </Card>
      <Alert>
        <AlertTitle>
          {t("artifacts.p2p.title", undefined, "P2P is not implemented yet")}
        </AlertTitle>
        <AlertDescription>
          {t(
            "artifacts.p2p.description",
            undefined,
            "Current delivery is Manager to Worker over authenticated artifact URLs. There is no enable switch in this build; P2P requires a delivery subsystem before these peer metrics can exist."
          )}
        </AlertDescription>
      </Alert>
    </>
  )
}

function DeleteArtifactButton({
  artifact,
  state,
  actions,
  t,
}: {
  artifact: Artifact
  state: StateResponse
  actions: Actions
  t: TFunction
}) {
  const usedBy = artifactUsage(artifact, state, t)
  const disabled = usedBy !== "-"
  return (
    <Button
      size="sm"
      variant="destructive"
      disabled={disabled}
      title={
        disabled
          ? t("artifacts.usedBy", { value: usedBy }, `Used by ${usedBy}`)
          : t("artifacts.delete", undefined, "Delete artifact")
      }
      onClick={() => deleteArtifact(artifact.artifactId, actions)}
    >
      <Trash2Icon data-icon="inline-start" />
      {t("artifacts.delete", undefined, "Delete artifact")}
    </Button>
  )
}

function cachedNodes(artifact: Artifact, state: StateResponse) {
  return (
    (state.nodes ?? [])
      .filter((node) => node.cachedArtifacts?.includes(artifact.artifactId))
      .map((node) => short(node.nodeId))
      .join(", ") || "-"
  )
}

function modelsUsingArtifact(artifact: Artifact, state: StateResponse) {
  return (
    (state.profiles ?? [])
      .filter((profile) => profileUsesArtifact(profile, artifact.artifactId))
      .map(profileLabel)
      .join(", ") || "-"
  )
}

function artifactUsage(artifact: Artifact, state: StateResponse, t: TFunction) {
  const uses = []
  const models = modelsUsingArtifact(artifact, state)
  if (models !== "-") uses.push(models)
  for (const update of state.updates ?? []) {
    if (update.artifactId === artifact.artifactId)
      uses.push(
        t(
          "artifacts.usedByUpdate",
          { id: short(update.updateId) },
          `update ${short(update.updateId)}`
        )
      )
  }
  return uses.join(", ") || "-"
}

function profileUsesArtifact(profile: Profile, artifactId: string) {
  if (profile.artifacts?.includes(artifactId)) return true
  return (profile.runtimeVariants ?? []).some((variant) =>
    variant.artifacts?.includes(artifactId)
  )
}
