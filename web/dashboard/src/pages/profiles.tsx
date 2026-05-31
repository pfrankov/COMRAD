import { useEffect, useState } from "react"

import {
  BoxIcon,
  FileTextIcon,
  PencilIcon,
  PlusIcon,
  Settings2Icon,
  Trash2Icon,
} from "lucide-react"
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { Toggle } from "@/components/ui/toggle"
import { DataTable } from "@/components/comrad/data-table"
import {
  ConditionList,
  conditionBadgeValue,
  conditionSummary,
} from "@/components/comrad/conditions"
import { KeyValues, PageTitle } from "@/components/comrad/dashboard-primitives"
import { StatusBadge } from "@/components/comrad/status-badge"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import {
  assignmentCounts,
  fmtBytes,
  human,
  profileLabel,
  profileVariants,
  short,
} from "@/lib/comrad"
import {
  deleteModel,
  saveModel,
  type Actions,
  type ModelEditorForm,
  type UploadProgress,
} from "@/comrad/actions"
import type { Artifact, Assignment, Policy, Profile, StateResponse } from "@/types"

type EditorMode = "add" | "edit"

export function ProfilesPage({
  state,
  actions,
}: {
  state: StateResponse
  actions: Actions
}) {
  const { t } = useI18n()
  const [form, setForm] = useState<ModelEditorForm>(() => emptyModelForm())
  const [editorOpen, setEditorOpen] = useState(false)
  const [editorMode, setEditorMode] = useState<EditorMode>("add")
  const [detailProfile, setDetailProfile] = useState<Profile | null>(null)
  const profiles = state.profiles ?? []

  const startAdd = () => {
    setForm(emptyModelForm())
    setEditorMode("add")
    setEditorOpen(true)
  }
  const startEdit = (profile: Profile) => {
    setForm(formFromProfile(profile, state))
    setEditorMode("edit")
    setEditorOpen(true)
  }

  return (
    <>
      <PageTitle
        eyebrow={t("nav.group.serve", undefined, "Resources")}
        title={t("profiles.title", undefined, "Models")}
        description={t(
          "profiles.description",
          undefined,
          "Upload model files, edit client-facing model settings, and choose how many copies stay ready."
        )}
        actions={
          profiles.length ? (
          <Button onClick={startAdd}>
            <PlusIcon data-icon="inline-start" />
            {t("profiles.addModel", undefined, "Add a model")}
          </Button>
          ) : undefined
        }
      />
      <ModelIntro state={state} t={t} />
      <ExistingModels
        state={state}
        startAdd={startAdd}
        startEdit={startEdit}
        deleteModel={(profile) =>
          deleteModel(profile.profileId, profileLabel(profile), actions)
        }
        openDetails={setDetailProfile}
        t={t}
      />
      <ModelTechnicalDetailsDialog
        profile={detailProfile}
        state={state}
        open={detailProfile !== null}
        setOpen={(open) => !open && setDetailProfile(null)}
        t={t}
      />
      <ModelEditorDialog
        open={editorOpen}
        setOpen={setEditorOpen}
        mode={editorMode}
        form={form}
        setForm={setForm}
        actions={actions}
        artifacts={state.artifacts ?? []}
        t={t}
      />
    </>
  )
}

function ModelIntro({
  state,
  t,
}: {
  state: StateResponse
  t: TFunction
}) {
  const readyModels = (state.profiles ?? []).filter((profile) =>
    modelReady(profile, state.assignments ?? [])
  ).length
  return (
    <div className="grid gap-4 lg:grid-cols-[1.4fr_1fr]">
      <Card>
        <CardHeader>
          <CardTitle>
            {t("profiles.intro.title", undefined, "What you can do here")}
          </CardTitle>
          <CardDescription>
            {t(
              "profiles.intro.description",
              undefined,
              "Most model work should start on this page."
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-3">
          <WorkflowStep
            title={t("profiles.step.add.title", undefined, "1. Add a model")}
            body={t(
              "profiles.step.add.body",
              undefined,
              "Upload the model files and let the Manager store and verify them."
            )}
          />
          <WorkflowStep
            title={t(
              "profiles.step.capacity.title",
              undefined,
              "2. Set ready copies"
            )}
            body={t(
              "profiles.step.capacity.body",
              undefined,
              "Choose how many slots should keep the model loaded for fast requests."
            )}
          />
          <WorkflowStep
            title={t("profiles.step.edit.title", undefined, "3. Edit model")}
            body={t(
              "profiles.step.edit.body",
              undefined,
              "Change the client model name, context, cost, or llama.cpp server args later."
            )}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>
            {t("profiles.capacity.title", undefined, "Model capacity")}
          </CardTitle>
          <CardDescription>
            {t(
              "profiles.capacity.description",
              undefined,
              "Ready means clients can request the model now."
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div>
            <div className="text-3xl font-semibold">
              {readyModels}/{state.profiles?.length ?? 0}
            </div>
            <p className="text-sm text-muted-foreground">
              {t("profiles.capacity.readyLabel", undefined, "models ready")}
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function WorkflowStep({ title, body }: { title: string; body: string }) {
  return (
    <div className="rounded-lg border bg-muted/50 p-4">
      <div className="font-medium">{title}</div>
      <p className="mt-2 text-sm text-muted-foreground">{body}</p>
    </div>
  )
}

function ExistingModels(props: {
  state: StateResponse
  startAdd: () => void
  startEdit: (profile: Profile) => void
  deleteModel: (profile: Profile) => void
  openDetails: (profile: Profile) => void
  t: TFunction
}) {
  const profiles = props.state.profiles ?? []
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {props.t("profiles.existing.title", undefined, "Existing models")}
        </CardTitle>
        <CardDescription>
          {props.t(
            "profiles.existing.description",
            undefined,
            "Client-facing model names first; exact artifacts and slots are available when needed."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {!profiles.length ? (
          <NoModels startAdd={props.startAdd} t={props.t} />
        ) : (
          profiles.map((profile) => (
            <ModelRow
              key={profile.profileId}
              profile={profile}
              state={props.state}
              edit={() => props.startEdit(profile)}
              deleteModel={() => props.deleteModel(profile)}
              openDetails={() => props.openDetails(profile)}
              t={props.t}
            />
          ))
        )}
      </CardContent>
    </Card>
  )
}

function NoModels({ startAdd, t }: { startAdd: () => void; t: TFunction }) {
  return (
    <Empty className="border border-dashed">
      <EmptyHeader>
        <EmptyTitle>
          {t("profiles.empty.title", undefined, "No models yet")}
        </EmptyTitle>
        <EmptyDescription>
          {t(
            "profiles.empty.description",
            undefined,
            "Add a GGUF model and keep one ready copy available for requests."
          )}
        </EmptyDescription>
      </EmptyHeader>
      <Button onClick={startAdd}>
        <PlusIcon data-icon="inline-start" />
        {t("profiles.addModel", undefined, "Add a model")}
      </Button>
    </Empty>
  )
}

function ModelRow(props: {
  profile: Profile
  state: StateResponse
  edit: () => void
  deleteModel: () => void
  openDetails: () => void
  t: TFunction
}) {
  const counts = assignmentCounts(
    props.profile.profileId,
    props.state.assignments ?? []
  )
  return (
    <div className="rounded-lg border bg-card">
      <div className="grid gap-4 p-4 xl:grid-cols-[minmax(260px,1fr)_170px_170px_220px] xl:items-center">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate text-lg font-semibold">
              {profileLabel(props.profile)}
            </h2>
            <StatusBadge
              value={conditionBadgeValue(props.profile.conditions)}
            />
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {props.t(
              "profiles.row.clientsRequestAs",
              undefined,
              "Clients request this as"
            )}{" "}
            <code>
              {props.profile.logicalModel ||
                props.profile.alias ||
                props.profile.name}
            </code>
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            {conditionSummary(
              props.profile.conditions,
              "Ready",
              props.t,
              props.t(
                "profiles.row.noReadinessCondition",
                undefined,
                "Readiness condition is not reported yet."
              )
            )}
          </p>
        </div>
        <ModelStat
          label={props.t(
            "profiles.stat.readyCopies",
            undefined,
            "Ready copies"
          )}
          value={`${counts.actualWarm}/${counts.desiredWarm}`}
        />
        <ModelStat
          label={props.t(
            "profiles.stat.contextCost",
            undefined,
            "Context / cost"
          )}
          value={`${props.profile.llm?.contextTokens || "-"} / ${props.profile.computeCost ?? 0}`}
        />
        <div className="flex flex-wrap justify-start gap-2 xl:justify-end">
          <Button size="sm" onClick={props.edit}>
            <PencilIcon data-icon="inline-start" />
            {props.t("profiles.editModel", undefined, "Edit model")}
          </Button>
          <Button size="sm" variant="outline" onClick={props.openDetails}>
            <Settings2Icon data-icon="inline-start" />
            {props.t(
              "profiles.technicalDetails",
              undefined,
              "Technical details"
            )}
          </Button>
          <Button size="sm" variant="destructive" onClick={props.deleteModel}>
            <Trash2Icon data-icon="inline-start" />
            {props.t("profiles.deleteModel", undefined, "Delete model")}
          </Button>
        </div>
      </div>
    </div>
  )
}

function ModelStat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="font-mono text-xs font-normal text-muted-foreground uppercase">
        {label}
      </div>
      <div className="mt-1 font-medium">{value}</div>
    </div>
  )
}

function ModelTechnicalDetails({
  profile,
  state,
  artifacts,
  t,
}: {
  profile: Profile
  state: StateResponse
  artifacts: string[]
  t: TFunction
}) {
  return (
    <div className="flex flex-col gap-4 p-4">
      <KeyValues
        values={[
          [
            t("profiles.field.profileId", undefined, "Profile id"),
            profile.profileId,
          ],
          [
            t("profiles.field.artifacts", undefined, "Artifacts"),
            artifacts.join(", ") || "-",
          ],
          [
            t("profiles.field.requirements", undefined, "Requirements"),
            requirementSummary(profile, t),
          ],
        ]}
      />
      <DataTable
        items={(state.assignments ?? []).filter(
          (item) => item.profileId === profile.profileId
        )}
        empty={t(
          "profiles.assignments.empty",
          undefined,
          "No slot assignments"
        )}
        columns={[
          {
            header: t("profiles.assignments.slot", undefined, "Slot"),
            cell: (item) => <code>{short(item.slotId)}</code>,
          },
          {
            header: t(
              "profiles.assignments.artifact",
              undefined,
              "Selected artifact"
            ),
            cell: (item) => <code>{short(item.modelArtifactId)}</code>,
          },
          {
            header: t("profiles.assignments.ready", undefined, "Ready"),
            cell: (item) => (
              <StatusBadge
                value={item.ready ? "ready" : item.mismatchReason || "waiting"}
              />
            ),
          },
          {
            header: t("profiles.assignments.reason", undefined, "Reason"),
            cell: (item) => human(item.mismatchReason, t),
          },
        ]}
      />
      <div>
        <div className="mb-2 text-sm font-medium">
          {t("profiles.conditions.title", undefined, "Readiness conditions")}
        </div>
        <ConditionList
          conditions={profile.conditions}
          empty={t(
            "profiles.conditions.empty",
            undefined,
            "No readiness conditions reported."
          )}
          t={t}
        />
      </div>
    </div>
  )
}

function ModelTechnicalDetailsDialog({
  profile,
  state,
  open,
  setOpen,
  t,
}: {
  profile: Profile | null
  state: StateResponse
  open: boolean
  setOpen: (open: boolean) => void
  t: TFunction
}) {
  const artifacts = profile
    ? concreteArtifacts(profile, state.artifacts ?? [])
    : []
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-[780px]">
        <DialogHeader>
          <DialogTitle>
            {profile
              ? profileLabel(profile)
              : t(
                  "profiles.technicalDetails",
                  undefined,
                  "Technical details"
                )}
          </DialogTitle>
          <DialogDescription>
            {t(
              "profiles.technicalDetails.description",
              undefined,
              "Exact artifacts, requirements, and slot assignments for this model."
            )}
          </DialogDescription>
        </DialogHeader>
        {profile ? (
          <ModelTechnicalDetails
            profile={profile}
            state={state}
            artifacts={artifacts}
            t={t}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function ModelEditorDialog(props: {
  open: boolean
  setOpen: (open: boolean) => void
  mode: EditorMode
  form: ModelEditorForm
  setForm: (form: ModelEditorForm) => void
  actions: Actions
  artifacts: Artifact[]
  t: TFunction
}) {
  const [saving, setSaving] = useState(false)
  const [uploadFiles, setUploadFiles] = useState<File[]>([])
  const [uploadProgress, setUploadProgress] = useState<UploadProgress | null>(
    null
  )
  const editing = props.mode === "edit"
  const update = <K extends keyof ModelEditorForm>(
    key: K,
    value: ModelEditorForm[K]
  ) =>
    props.setForm({ ...props.form, [key]: value })
  useEffect(() => {
    if (props.open) {
      setUploadFiles([])
      setUploadProgress(null)
    }
  }, [props.open, props.form.profileId])
  const submit = async () => {
    setSaving(true)
    setUploadProgress(null)
    try {
      const saved = await saveModel(
        props.actions,
        props.form,
        uploadFiles,
        setUploadProgress
      )
      if (saved) props.setOpen(false)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : props.t("profiles.toast.notSaved", undefined, "Model was not saved")
      )
    } finally {
      setSaving(false)
      setUploadProgress(null)
    }
  }
  const confirmSubmit = () => {
    props.actions.setConfirm({
      title: editing
        ? props.t(
            "profiles.confirm.save.title",
            undefined,
            "Save model changes"
          )
        : props.t("profiles.confirm.add.title", undefined, "Add model"),
      body: editing
        ? props.t(
            "profiles.confirm.save.body",
            undefined,
            "This updates the model profile and desired downloaded or ready copy policy."
          )
        : props.t(
            "profiles.confirm.add.body",
            undefined,
            "This registers the model profile, uploads artifacts, and saves desired capacity."
          ),
      confirmLabel: editing
        ? props.t("profiles.saveChanges", undefined, "Save changes")
        : props.t("profiles.addModel", undefined, "Add model"),
      variant: "default",
      run: submit,
    })
  }

  return (
    <Dialog open={props.open} onOpenChange={props.setOpen}>
      <DialogContent className="grid max-h-[calc(100svh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-[760px]">
        <DialogHeader>
          <DialogTitle>
            {editing
              ? props.t("profiles.editModel", undefined, "Edit model")
              : props.t("profiles.addModel", undefined, "Add a model")}
          </DialogTitle>
          <DialogDescription>
            {editing
              ? props.t(
                  "profiles.editor.editDescription",
                  undefined,
                  "Change how clients request the model or adjust serving settings without uploading a new file."
                )
              : props.t(
                  "profiles.editor.addDescription",
                  undefined,
                  "Upload the model files, choose a client name, and keep one ready copy available."
                )}
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto pr-1">
          <FieldGroup>
            {editing ? (
              <LinkedModelFiles
                artifactIds={props.form.modelArtifactIds ?? []}
                artifacts={props.artifacts}
                t={props.t}
              />
            ) : null}
            <Field>
              <FieldLabel>
                {props.t(
                  "profiles.field.modelName",
                  undefined,
                  "Model name clients use"
                )}
              </FieldLabel>
              <Input
                value={props.form.alias}
                onChange={(event) => update("alias", event.target.value)}
                placeholder="gemma-4-e2b"
              />
              <FieldDescription>
                {props.t(
                  "profiles.field.modelName.description",
                  undefined,
                  "This is the value clients pass as"
                )}{" "}
                <code>model</code>{" "}
                {props.t(
                  "profiles.field.inChatRequests",
                  undefined,
                  "in chat requests."
                )}
              </FieldDescription>
            </Field>
            <FieldGroup className="grid gap-4 md:grid-cols-2">
              <Field>
                <FieldLabel>
                  {props.t(
                    "profiles.field.uploadFiles",
                    undefined,
                    "Upload model files"
                  )}
                </FieldLabel>
                <Input
                  type="file"
                  multiple
                  onChange={(event) =>
                    setUploadFiles(Array.from(event.target.files ?? []))
                  }
                />
                <FieldDescription>
                  {props.t(
                    "profiles.field.uploadFiles.description",
                    undefined,
                    "Upload a new file set only when the model files themselves change. Editing settings keeps the linked files above."
                  )}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel>
                  {props.t(
                    "profiles.field.contextTokens",
                    undefined,
                    "Context tokens"
                  )}
                </FieldLabel>
                <Input
                  type="number"
                  value={props.form.contextTokens}
                  onChange={(event) =>
                    update("contextTokens", event.target.value)
                  }
                />
              </Field>
            </FieldGroup>
            {uploadProgress ? (
              <UploadProgressPanel progress={uploadProgress} t={props.t} />
            ) : null}
            <FieldGroup className="grid gap-4 md:grid-cols-4">
              <Field>
                <FieldLabel>
                  {props.t(
                    "profiles.field.computeCost",
                    undefined,
                    "Compute cost"
                  )}
                </FieldLabel>
                <Input
                  type="number"
                  value={props.form.computeCost}
                  onChange={(event) =>
                    update("computeCost", event.target.value)
                  }
                  />
                </Field>
              <Field>
                <FieldLabel>
                  {props.t(
                    "profiles.field.autoBalance",
                    undefined,
                    "Auto balance"
                  )}
                </FieldLabel>
                <Toggle
                  variant="outline"
                  pressed={props.form.autoBalance}
                  onPressedChange={(value) => update("autoBalance", value)}
                >
                  {props.form.autoBalance
                    ? props.t("common.on", undefined, "On")
                    : props.t("common.off", undefined, "Off")}
                </Toggle>
              </Field>
              <Field>
                <FieldLabel>
                  {props.t(
                    "profiles.field.downloadedCopies",
                    undefined,
                    "Downloaded copies"
                  )}
                </FieldLabel>
                <Input
                  type="number"
                  value={props.form.cachedCount}
                  onChange={(event) =>
                    update("cachedCount", event.target.value)
                  }
                />
              </Field>
              <Field>
                <FieldLabel>
                  {props.t(
                    "profiles.field.readyCopies",
                    undefined,
                    "Ready copies"
                  )}
                </FieldLabel>
                <Input
                  type="number"
                  value={props.form.warmCount}
                  onChange={(event) => update("warmCount", event.target.value)}
                />
              </Field>
            </FieldGroup>
            {props.form.autoBalance ? (
              <FieldGroup className="grid gap-4 md:grid-cols-4">
                <Field>
                  <FieldLabel>
                    {props.t(
                      "profiles.field.minReady",
                      undefined,
                      "Min ready"
                    )}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={props.form.minWarmCount}
                    onChange={(event) =>
                      update("minWarmCount", event.target.value)
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel>
                    {props.t(
                      "profiles.field.maxReady",
                      undefined,
                      "Max ready"
                    )}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={props.form.maxWarmCount}
                    onChange={(event) =>
                      update("maxWarmCount", event.target.value)
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel>
                    {props.t(
                      "profiles.field.minDownloaded",
                      undefined,
                      "Min downloaded"
                    )}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={props.form.minCachedCount}
                    onChange={(event) =>
                      update("minCachedCount", event.target.value)
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel>
                    {props.t(
                      "profiles.field.maxDownloaded",
                      undefined,
                      "Max downloaded"
                    )}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={props.form.maxCachedCount}
                    onChange={(event) =>
                      update("maxCachedCount", event.target.value)
                    }
                  />
                </Field>
              </FieldGroup>
            ) : null}
            <Card className="border-dashed">
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-sm">
                  <BoxIcon data-icon="inline-start" />
                  {props.t(
                    "profiles.advanced.title",
                    undefined,
                    "Advanced llama.cpp server settings"
                  )}
                </CardTitle>
                <CardDescription>
                  {props.t(
                    "profiles.advanced.description",
                    undefined,
                    "Only change these when the model fails to fit, starts slowly, or needs different llama.cpp server flags."
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className="grid gap-4 md:grid-cols-3">
                <Field>
                  <FieldLabel>
                    {props.t(
                      "profiles.field.memoryGib",
                      undefined,
                      "Unified memory GiB"
                    )}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={props.form.ram}
                    onChange={(event) => update("ram", event.target.value)}
                  />
                </Field>
                <Field>
                  <FieldLabel>
                    {props.t("profiles.field.diskGib", undefined, "Disk GiB")}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={props.form.disk}
                    onChange={(event) => update("disk", event.target.value)}
                  />
                </Field>
                <Field>
                  <FieldLabel>
                    {props.t(
                      "profiles.field.llamaArgs",
                      undefined,
                      "llama.cpp server args"
                    )}
                  </FieldLabel>
                  <Input
                    value={props.form.llamaArgs}
                    onChange={(event) =>
                      update("llamaArgs", event.target.value)
                    }
                    placeholder="-ngl 99 --threads 6"
                  />
                </Field>
                <Field>
                  <FieldLabel>
                    {props.t(
                      "profiles.field.maxWarmPerWorker",
                      undefined,
                      "Max warm models per worker"
                    )}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={props.form.maxWarmProfilesPerNode}
                    onChange={(event) =>
                      update("maxWarmProfilesPerNode", event.target.value)
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel>
                    {props.t(
                      "profiles.field.maxCachedPerWorker",
                      undefined,
                      "Max downloaded models per worker"
                    )}
                  </FieldLabel>
                  <Input
                    type="number"
                    value={props.form.maxCachedProfilesPerNode}
                    onChange={(event) =>
                      update("maxCachedProfilesPerNode", event.target.value)
                    }
                  />
                </Field>
              </CardContent>
            </Card>
          </FieldGroup>
        </div>
        <DialogFooter className="border-t pt-4">
          <Button variant="outline" onClick={() => props.setOpen(false)}>
            {props.t("common.cancel", undefined, "Cancel")}
          </Button>
          <Button disabled={saving} onClick={confirmSubmit}>
            {saving
              ? props.t("profiles.saving", undefined, "Saving...")
              : editing
                ? props.t("profiles.saveChanges", undefined, "Save changes")
                : props.t("profiles.addModel", undefined, "Add model")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function UploadProgressPanel({
  progress,
  t,
}: {
  progress: UploadProgress
  t: TFunction
}) {
  const percent = Math.max(0, Math.min(100, progress.percent))
  const speed = `${fmtBytes(progress.bytesPerSecond)}/s`
  return (
    <div className="rounded-lg border bg-muted/40 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        <span className="truncate font-medium">{progress.fileName}</span>
        <span className="font-mono text-xs text-muted-foreground">
          {t(
            "profiles.uploadProgress.percent",
            { percent: Math.round(percent) },
            `${Math.round(percent)}%`
          )}
        </span>
      </div>
      <Progress value={percent} className="mt-3" />
      <div className="mt-2 text-xs text-muted-foreground">
        {t(
          "profiles.uploadProgress.speed",
          { speed },
          `Upload speed ${speed}`
        )}
      </div>
    </div>
  )
}

function emptyModelForm(): ModelEditorForm {
  return {
    alias: "",
    contextTokens: "512",
    ram: "6",
    disk: "8",
    computeCost: "0",
    llamaArgs: "-ngl 99",
    cachedCount: "1",
    warmCount: "1",
    autoBalance: false,
    minCachedCount: "",
    maxCachedCount: "",
    minWarmCount: "",
    maxWarmCount: "",
    maxCachedProfilesPerNode: "",
    maxWarmProfilesPerNode: "",
  }
}

function formFromProfile(
  profile: Profile,
  state: StateResponse
): ModelEditorForm {
  const counts = assignmentCounts(profile.profileId, state.assignments ?? [])
  const policy = policyForProfile(profile.profileId, state.policies ?? [])
  return {
    ...emptyModelForm(),
    profileId: profile.profileId,
    alias: profile.logicalModel || profile.alias || profile.name || "",
    contextTokens: String(profile.llm?.contextTokens || 512),
    ram: bytesToGiB(
      profile.requirements?.unifiedMemoryBytes || profile.requirements?.ramBytes
    ),
    disk: bytesToGiB(profile.requirements?.diskBytes),
    computeCost: String(profile.computeCost ?? 0),
    llamaArgs: profile.runtime?.llamaCpp?.args?.join(" ") || "",
    modelArtifactIds: modelArtifactIds(profile, state.artifacts ?? []),
    cachedCount: String(policy?.cachedCount ?? (counts.desiredCached || 1)),
    warmCount: String(policy?.warmCount ?? (counts.desiredWarm || 1)),
    autoBalance: Boolean(policy?.autoBalance),
    minCachedCount: countField(policy?.minCachedCount),
    maxCachedCount: countField(policy?.maxCachedCount),
    minWarmCount: countField(policy?.minWarmCount),
    maxWarmCount: countField(policy?.maxWarmCount),
    maxCachedProfilesPerNode: countField(policy?.maxCachedProfilesPerNode),
    maxWarmProfilesPerNode: countField(policy?.maxWarmProfilesPerNode),
  }
}

function policyForProfile(profileId: string, policies: Policy[]) {
  return policies.find((policy) => policy.profileId === profileId)
}

function countField(value?: number) {
  return value ? String(value) : ""
}

function modelArtifactIds(profile: Profile, artifacts: Artifact[]) {
  const ids = profile.artifacts ?? []
  const modelIds = ids.filter((id) => {
    const artifact = artifacts.find((item) => item.artifactId === id)
    return !artifact?.kind || artifact.kind.startsWith("model_")
  })
  return modelIds.length ? modelIds : ids
}

function bytesToGiB(value?: number) {
  if (!value) return ""
  return String(Math.round(value / 1073741824))
}

function modelReady(profile: Profile, assignments: Assignment[]) {
  const counts = assignmentCounts(profile.profileId, assignments)
  return counts.desiredWarm > 0 && counts.actualWarm >= counts.desiredWarm
}

function concreteArtifacts(profile: Profile, artifacts: Artifact[]) {
  const ids = profileVariants(profile).flatMap(
    (variant) => variant.artifacts || profile.artifacts || []
  )
  return [...new Set(ids)].map((id) => {
    const artifact = artifacts.find((item) => item.artifactId === id)
    return `${artifact?.kind || "artifact"} ${short(id)}${artifact?.sizeBytes ? ` (${fmtBytes(artifact.sizeBytes)})` : ""}`
  })
}

function LinkedModelFiles({
  artifactIds,
  artifacts,
  t,
}: {
  artifactIds: string[]
  artifacts: Artifact[]
  t: TFunction
}) {
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm">
          <FileTextIcon data-icon="inline-start" />
          {t("profiles.linkedFiles.title", undefined, "Linked model files")}
        </CardTitle>
        <CardDescription>
          {t(
            "profiles.linkedFiles.description",
            undefined,
            "These files stay linked unless you upload a replacement file set."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-sm">
        {artifactIds.length ? (
          artifactIds.map((id) => {
            const artifact = artifacts.find((item) => item.artifactId === id)
            return (
              <div
                key={id}
                className="grid gap-1 rounded-md border bg-muted/40 p-3 md:grid-cols-[minmax(0,1fr)_auto]"
              >
                <div className="min-w-0">
                  <div className="truncate font-medium">
                    {artifact?.name || short(id)}
                  </div>
                  <code>{short(id)}</code>
                </div>
                <div className="text-muted-foreground">
                  {[artifact?.kind, fmtBytes(artifact?.sizeBytes)]
                    .filter(Boolean)
                    .join(" · ") ||
                    t(
                      "profiles.linkedFiles.modelFile",
                      undefined,
                      "model file"
                    )}
                </div>
              </div>
            )
          })
        ) : (
          <span className="text-muted-foreground">
            {t(
              "profiles.linkedFiles.empty",
              undefined,
              "No linked files found."
            )}
          </span>
        )}
      </CardContent>
    </Card>
  )
}

function requirementSummary(profile: Profile, t: TFunction) {
  const req = profile.requirements
  if (!req) return t("common.none", undefined, "none")
  return t(
    "profiles.requirements.summary",
    {
      target: req.target || "-",
      memory: fmtBytes(req.unifiedMemoryBytes || req.ramBytes),
      disk: fmtBytes(req.diskBytes),
    },
    `${req.target || "-"}, memory ${fmtBytes(req.unifiedMemoryBytes || req.ramBytes)}, disk ${fmtBytes(req.diskBytes)}`
  )
}
