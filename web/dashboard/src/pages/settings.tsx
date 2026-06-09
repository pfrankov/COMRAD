import { useEffect, useState } from "react"

import {
  CheckIcon,
  CopyIcon,
  CpuIcon,
  EyeIcon,
  EyeOffIcon,
  FileCode2Icon,
  KeyRoundIcon,
  MonitorIcon,
  MoonIcon,
  NetworkIcon,
  RefreshCwIcon,
  SunIcon,
} from "lucide-react"
import { toast } from "sonner"

import { useTheme, type Theme } from "@/components/theme-provider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { KeyValues, PageTitle } from "@/components/comrad/dashboard-primitives"
import { useI18n } from "@/i18n/i18n-provider"
import { locales, localeNames, type LanguagePreference } from "@/i18n/config"
import type { Actions } from "@/comrad/actions"
import type { RuntimeSummaryItem, StateResponse } from "@/types"

export function SettingsPage({
  state,
  actions,
  adminToken,
  saveAdminToken,
}: {
  state: StateResponse
  actions: Actions
  adminToken: string
  saveAdminToken: (token: string) => void
}) {
  const { t } = useI18n()
  return (
    <>
      <PageTitle
        eyebrow={t("nav.group.govern")}
        title={t("settings.title")}
        description={t("settings.description")}
      />
      <div className="grid gap-4 lg:grid-cols-3">
        <AdminTokenCard
          adminToken={adminToken}
          saveAdminToken={saveAdminToken}
        />
        <ClientKeyCard actions={actions} adminToken={adminToken} />
        <WorkerConnectionCard actions={actions} adminToken={adminToken} />
        <P2PCard state={state} actions={actions} />
        <ThemeCard />
      </div>
      <LanguageCard />
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.how.title")}</CardTitle>
          <CardDescription>{t("settings.how.description")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-3">
          <ComponentPurpose
            title={t("common.models")}
            body={t("settings.models.body")}
          />
          <ComponentPurpose
            title={t("common.capacity")}
            body={t("settings.capacity.body")}
          />
          <ComponentPurpose
            title={t("nav.nodes.label")}
            body={t("settings.nodes.body")}
          />
          <ComponentPurpose
            title={t("nav.tasks.label")}
            body={t("settings.tasks.body")}
          />
          <ComponentPurpose
            title={t("common.users")}
            body={t("settings.users.body")}
          />
          <ComponentPurpose
            title={t("nav.artifacts.label")}
            body={t("settings.storage.body")}
          />
        </CardContent>
      </Card>
      <div className="grid gap-4 lg:grid-cols-[1fr_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>{t("settings.where.title")}</CardTitle>
            <CardDescription>{t("settings.where.description")}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2">
            <SettingsLink
              label={t("common.models")}
              detail={t("settings.models.detail")}
              onClick={() => actions.show("profiles")}
            />
            <SettingsLink
              label={t("common.capacity")}
              detail={t("settings.capacity.detail")}
              onClick={() => actions.show("placement")}
            />
            <SettingsLink
              label={t("common.users")}
              detail={t("settings.users.detail")}
              onClick={() => actions.show("users")}
            />
            <SettingsLink
              label={t("nav.nodes.label")}
              detail={t("settings.nodes.detail")}
              onClick={() => actions.show("nodes")}
            />
            <SettingsLink
              label={t("nav.updates.label")}
              detail={t("settings.updates.detail")}
              onClick={() => actions.show("updates")}
            />
            <SettingsLink
              label={t("common.apiReference")}
              detail={t("settings.apiReference.detail")}
              onClick={() => void openAPIReference(adminToken)}
            />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t("settings.safety.title")}</CardTitle>
            <CardDescription>
              {t("settings.safety.description")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <KeyValues
              values={[
                [t("settings.managerVersion"), state.version || "-"],
                [t("common.adminApi"), t("settings.adminApiValue")],
                [t("common.clientApi"), t("settings.clientApiValue")],
                [t("settings.workerControl"), t("settings.workerControlValue")],
                [
                  t("settings.promptVisibility"),
                  t("settings.promptVisibilityValue"),
                ],
                [t("settings.validationGate"), "make validate"],
              ]}
            />
          </CardContent>
        </Card>
      </div>
      <RuntimeSummaryCard state={state} />
      <RuntimeConfigCard actions={actions} adminToken={adminToken} />
    </>
  )
}

function RuntimeSummaryCard({ state }: { state: StateResponse }) {
  const { t } = useI18n()
  const runtimes = state.runtimeSummary?.items ?? []
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CpuIcon data-icon="inline-start" />
          {t("settings.adapterSummary.title", undefined, "Runtime summary")}
        </CardTitle>
        <CardDescription>
          {t(
            "settings.adapterSummary.description",
            undefined,
            "Read-only runtime adapters derived from profiles, Workers, and slots."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {!runtimes.length ? (
          <p className="text-sm text-muted-foreground">
            {t(
              "settings.adapterSummary.empty",
              undefined,
              "No runtime adapters reported yet."
            )}
          </p>
        ) : (
          runtimes.map((runtime) => (
            <RuntimeSummaryRow
              key={runtime.metadata?.name || runtime.spec?.adapter}
              runtime={runtime}
            />
          ))
        )}
      </CardContent>
    </Card>
  )
}

function RuntimeSummaryRow({ runtime }: { runtime: RuntimeSummaryItem }) {
  const { t } = useI18n()
  return (
    <div className="grid gap-3 rounded-md border p-3 lg:grid-cols-[1fr_1fr_1fr]">
      <div className="min-w-0">
        <div className="font-medium">
          {runtime.metadata?.name || runtime.spec?.adapter || "-"}
        </div>
        <div className="mt-1 text-xs text-muted-foreground">
          {runtime.spec?.runtimeBinary?.command || "-"} ·{" "}
          {runtime.spec?.runtimeBinary?.source || "-"}
        </div>
      </div>
      <KeyValues
        values={[
          [
            t("settings.adapterSummary.formats", undefined, "Formats"),
            runtime.spec?.modelFormats?.join(", ") || "-",
          ],
          [
            t("settings.adapterSummary.tasks", undefined, "Tasks"),
            runtime.spec?.taskKinds?.join(", ") || "-",
          ],
        ]}
      />
      <KeyValues
        values={[
          [
            t("settings.adapterSummary.workers", undefined, "Workers"),
            String(runtime.status?.availableWorkers ?? 0),
          ],
          [
            t("settings.adapterSummary.readySlots", undefined, "Ready slots"),
            String(runtime.status?.readySlots ?? 0),
          ],
        ]}
      />
    </div>
  )
}

type ConfigStatus = "idle" | "loading" | "loaded" | "error"

function RuntimeConfigCard({
  actions,
  adminToken,
}: {
  actions: Actions
  adminToken: string
}) {
  const { t } = useI18n()
  const [yaml, setYaml] = useState("")
  const [status, setStatus] = useState<ConfigStatus>("idle")
  const [error, setError] = useState("")
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    if (!adminToken) {
      setYaml("")
      setStatus("idle")
      setError("")
      return undefined
    }
    setStatus("loading")
    setError("")
    actions
      .fetchText("/api/admin/config.yaml")
      .then((body) => {
        if (cancelled) return
        setYaml(body)
        setStatus("loaded")
      })
      .catch((err) => {
        if (cancelled) return
        setYaml("")
        setError(err instanceof Error ? err.message : String(err))
        setStatus("error")
      })
    return () => {
      cancelled = true
    }
  }, [actions, adminToken, reloadKey])

  const statusText =
    status === "loading"
      ? t("settings.config.loading", undefined, "Loading")
      : status === "loaded"
        ? t("settings.config.loaded", undefined, "Loaded")
        : status === "error"
          ? t("settings.config.failed", undefined, "Failed")
          : t("settings.config.waiting", undefined, "Waiting")

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2">
              <FileCode2Icon data-icon="inline-start" />
              {t("settings.config.title", undefined, "Runtime YAML config")}
            </CardTitle>
            <CardDescription>
              {t(
                "settings.config.description",
                undefined,
                "Sanitized Manager configuration exactly as this process is running it."
              )}
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="rounded-md font-normal">
              {t("settings.config.readOnly", undefined, "Read-only")}
            </Badge>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={!adminToken || status === "loading"}
              onClick={() => setReloadKey((value) => value + 1)}
            >
              <RefreshCwIcon data-icon="inline-start" />
              {t("settings.config.refresh", undefined, "Refresh")}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {!adminToken ? (
          <p className="text-sm text-muted-foreground">
            {t(
              "settings.config.noToken",
              undefined,
              "Save the admin token to load the runtime config."
            )}
          </p>
        ) : status === "error" ? (
          <p className="text-sm text-status-failed">
            {t(
              "settings.config.error",
              { error },
              "Config load failed: {error}"
            )}
          </p>
        ) : yaml ? (
          <YamlCodeBlock value={yaml} />
        ) : (
          <p className="text-sm text-muted-foreground">{statusText}</p>
        )}
      </CardContent>
    </Card>
  )
}

function YamlCodeBlock({ value }: { value: string }) {
  const lines = value.trimEnd().split("\n")
  return (
    <pre className="max-h-[640px] overflow-auto rounded-md border bg-muted/50 p-4 text-xs leading-5">
      <code>
        {lines.map((line, index) => (
          <span
            key={`${index}-${line}`}
            className="block min-h-5 whitespace-pre"
          >
            <YamlLine line={line} />
          </span>
        ))}
      </code>
    </pre>
  )
}

function YamlLine({ line }: { line: string }) {
  const match = line.match(/^(\s*)([A-Za-z0-9_.-]+)(:)(.*)$/)
  if (!match) return <>{line}</>
  const [, indent, key, colon, value] = match
  return (
    <>
      {indent}
      <span className="text-primary">{key}</span>
      <span className="text-muted-foreground">{colon}</span>
      <YamlValue value={value} />
    </>
  )
}

function YamlValue({ value }: { value: string }) {
  const trimmed = value.trim()
  if (!trimmed) return <>{value}</>
  const leading = value.slice(0, value.indexOf(trimmed))
  const isString =
    trimmed.startsWith("'") || trimmed.startsWith('"') || trimmed === "<unset>"
  const isBooleanOrNumber = /^(true|false|null|[-+]?\d+(\.\d+)?)$/i.test(
    trimmed
  )
  const className = isString
    ? "text-status-running"
    : isBooleanOrNumber
      ? "text-status-ready"
      : "text-foreground"
  return (
    <>
      {leading}
      <span className={className}>{trimmed}</span>
    </>
  )
}

function ClientKeyCard({
  actions,
  adminToken,
}: {
  actions: Actions
  adminToken: string
}) {
  const { t } = useI18n()
  const [key, setKey] = useState<string | null>(null)
  const [managerUrl, setManagerUrl] = useState<string | null>(null)
  const [visible, setVisible] = useState(false)
  const [copied, setCopied] = useState(false)
  const [copiedUrl, setCopiedUrl] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    setKey(null)
    setManagerUrl(null)
    setVisible(false)
    setError("")
    if (!adminToken) return
    setLoading(true)
    actions
      .fetchJSON<{ clientKey: string; managerUrl: string }>("/api/admin/client-key")
      .then((body) => {
        setKey(body.clientKey)
        setManagerUrl(body.managerUrl)
      })
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false))
  }, [actions, adminToken])

  const handleCopy = async () => {
    if (!key) return
    await navigator.clipboard.writeText(key)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleCopyUrl = async () => {
    if (!managerUrl) return
    await navigator.clipboard.writeText(managerUrl)
    setCopiedUrl(true)
    setTimeout(() => setCopiedUrl(false), 2000)
  }

  const displayKey = !adminToken
    ? ""
    : loading
      ? t("settings.clientKey.loading", undefined, "Loading…")
      : error
        ? t("settings.clientKey.error", undefined, "Failed to load")
        : visible
          ? (key ?? "")
          : key
            ? "•".repeat(Math.min(key.length, 32))
            : ""

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <KeyRoundIcon data-icon="inline-start" />
          {t("settings.clientKey.title", undefined, "Client connection")}
        </CardTitle>
        <CardDescription>
          {t(
            "settings.clientKey.description",
            undefined,
            "Share these with clients so they can connect to this Manager."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {!adminToken ? (
          <p className="text-sm text-muted-foreground">
            {t(
              "settings.clientKey.noToken",
              undefined,
              "Save the admin token to reveal client connection details."
            )}
          </p>
        ) : (
          <>
            <div className="flex flex-col gap-1">
              <span className="text-xs text-muted-foreground">
                {t("settings.clientKey.urlLabel", undefined, "Manager URL")}
              </span>
              <div className="flex items-center gap-2">
                <code className="flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 font-mono text-xs">
                  {loading
                    ? t("settings.clientKey.loading", undefined, "Loading…")
                    : error
                      ? t("settings.clientKey.error", undefined, "Failed to load")
                      : (managerUrl ?? "")}
                </code>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  disabled={!managerUrl}
                  onClick={() => void handleCopyUrl()}
                  aria-label={t("common.copy", undefined, "Copy")}
                >
                  {copiedUrl ? (
                    <CheckIcon className="size-4 text-status-running" />
                  ) : (
                    <CopyIcon className="size-4" />
                  )}
                </Button>
              </div>
            </div>
            <div className="flex flex-col gap-1">
              <span className="text-xs text-muted-foreground">
                {t("settings.clientKey.keyLabel", undefined, "API key")}
              </span>
              <div className="flex items-center gap-2">
                <code className="flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 font-mono text-xs">
                  {displayKey}
                </code>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  disabled={!key}
                  onClick={() => setVisible((v) => !v)}
                  aria-label={
                    visible
                      ? t("common.hide", undefined, "Hide")
                      : t("common.show", undefined, "Show")
                  }
                >
                  {visible ? (
                    <EyeOffIcon className="size-4" />
                  ) : (
                    <EyeIcon className="size-4" />
                  )}
                </Button>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  disabled={!key}
                  onClick={() => void handleCopy()}
                  aria-label={t("common.copy", undefined, "Copy")}
                >
                  {copied ? (
                    <CheckIcon className="size-4 text-status-running" />
                  ) : (
                    <CopyIcon className="size-4" />
                  )}
                </Button>
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function WorkerConnectionCard({
  actions,
  adminToken,
}: {
  actions: Actions
  adminToken: string
}) {
  const { t } = useI18n()
  const [workerToken, setWorkerToken] = useState<string | null>(null)
  const [managerUrl, setManagerUrl] = useState<string | null>(null)
  const [visible, setVisible] = useState(false)
  const [copied, setCopied] = useState(false)
  const [copiedUrl, setCopiedUrl] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    setWorkerToken(null)
    setManagerUrl(null)
    setVisible(false)
    setError("")
    if (!adminToken) return
    setLoading(true)
    actions
      .fetchJSON<{ managerUrl: string; workerToken: string }>("/api/admin/worker-join")
      .then((body) => {
        setWorkerToken(body.workerToken)
        setManagerUrl(body.managerUrl)
      })
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false))
  }, [actions, adminToken])

  const handleCopy = async () => {
    if (!workerToken) return
    await navigator.clipboard.writeText(workerToken)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleCopyUrl = async () => {
    if (!managerUrl) return
    await navigator.clipboard.writeText(managerUrl)
    setCopiedUrl(true)
    setTimeout(() => setCopiedUrl(false), 2000)
  }

  const displayToken = !adminToken
    ? ""
    : loading
      ? t("settings.clientKey.loading", undefined, "Loading…")
      : error
        ? t("settings.clientKey.error", undefined, "Failed to load")
        : visible
          ? (workerToken ?? "")
          : workerToken
            ? "•".repeat(Math.min(workerToken.length, 32))
            : ""

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <NetworkIcon data-icon="inline-start" />
          {t("settings.workerConnection.title", undefined, "Worker connection")}
        </CardTitle>
        <CardDescription>
          {t(
            "settings.workerConnection.description",
            undefined,
            "Enter these in the tray app settings to connect a worker node."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {!adminToken ? (
          <p className="text-sm text-muted-foreground">
            {t(
              "settings.workerConnection.noToken",
              undefined,
              "Save the admin token to reveal worker connection details."
            )}
          </p>
        ) : (
          <>
            <div className="flex flex-col gap-1">
              <span className="text-xs text-muted-foreground">
                {t("settings.clientKey.urlLabel", undefined, "Manager URL")}
              </span>
              <div className="flex items-center gap-2">
                <code className="flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 font-mono text-xs">
                  {loading
                    ? t("settings.clientKey.loading", undefined, "Loading…")
                    : error
                      ? t("settings.clientKey.error", undefined, "Failed to load")
                      : (managerUrl ?? "")}
                </code>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  disabled={!managerUrl}
                  onClick={() => void handleCopyUrl()}
                  aria-label={t("common.copy", undefined, "Copy")}
                >
                  {copiedUrl ? (
                    <CheckIcon className="size-4 text-status-running" />
                  ) : (
                    <CopyIcon className="size-4" />
                  )}
                </Button>
              </div>
            </div>
            <div className="flex flex-col gap-1">
              <span className="text-xs text-muted-foreground">
                {t("settings.workerConnection.tokenLabel", undefined, "Token")}
              </span>
              <div className="flex items-center gap-2">
                <code className="flex-1 truncate rounded-md border bg-muted/50 px-3 py-2 font-mono text-xs">
                  {displayToken}
                </code>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  disabled={!workerToken}
                  onClick={() => setVisible((v) => !v)}
                  aria-label={
                    visible
                      ? t("common.hide", undefined, "Hide")
                      : t("common.show", undefined, "Show")
                  }
                >
                  {visible ? (
                    <EyeOffIcon className="size-4" />
                  ) : (
                    <EyeIcon className="size-4" />
                  )}
                </Button>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  disabled={!workerToken}
                  onClick={() => void handleCopy()}
                  aria-label={t("common.copy", undefined, "Copy")}
                >
                  {copied ? (
                    <CheckIcon className="size-4 text-status-running" />
                  ) : (
                    <CopyIcon className="size-4" />
                  )}
                </Button>
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function AdminTokenCard({
  adminToken,
  saveAdminToken,
}: {
  adminToken: string
  saveAdminToken: (token: string) => void
}) {
  const { t } = useI18n()
  const [draft, setDraft] = useState("")
  useEffect(() => setDraft(""), [adminToken])
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("common.adminToken")}</CardTitle>
        <CardDescription>
          {t(
            "settings.adminToken.description",
            undefined,
            "Save the admin token here before loading protected Manager state."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault()
            if (!draft.trim() && adminToken) {
              toast.error(
                t(
                  "settings.adminToken.emptyNoChange",
                  undefined,
                  "Paste a new token to replace the saved one."
                )
              )
              return
            }
            saveAdminToken(draft)
            setDraft("")
            toast.success(
              t("settings.adminToken.saved", undefined, "Admin token saved")
            )
          }}
        >
          <input
            type="text"
            name="username"
            autoComplete="username"
            value="admin"
            readOnly
            className="hidden"
          />
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="admin-token">
                {t("common.adminToken")}
              </FieldLabel>
              <Input
                id="admin-token"
                type="password"
                autoComplete="current-password"
                placeholder={
                  adminToken
                    ? t(
                        "settings.adminToken.placeholder",
                        undefined,
                        "Token saved; paste a new token to replace"
                      )
                    : ""
                }
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
              />
            </Field>
          </FieldGroup>
          <Button type="submit" className="w-full sm:w-fit">
            <KeyRoundIcon data-icon="inline-start" />
            {t("shell.saveAdminToken")}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}

function P2PCard({
  state,
  actions,
}: {
  state: StateResponse
  actions: Actions
}) {
  const { t } = useI18n()
  const p2pEnabled = state.settings?.p2pEnabled ?? true
  const [saving, setSaving] = useState(false)

  const toggleP2P = async (enabled: boolean) => {
    setSaving(true)
    try {
      await actions.api("/api/admin/settings", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ p2pEnabled: enabled }),
      })
      toast.success(t("settings.p2p.saved"))
    } catch (err) {
      toast.error(
        t("settings.p2p.saveFailed")
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <NetworkIcon data-icon="inline-start" />
          {t("settings.p2p.title")}
        </CardTitle>
        <CardDescription>
          {t("settings.p2p.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <ToggleGroup
          type="single"
          value={p2pEnabled ? "enabled" : "disabled"}
          onValueChange={(value) => {
            if (value && !saving) toggleP2P(value === "enabled")
          }}
          className="grid w-full grid-cols-2 rounded-full bg-muted p-1 sm:w-auto"
          aria-label={t("settings.p2p.title")}
        >
          <ToggleGroupItem
            value="enabled"
            className="rounded-full px-3"
            disabled={saving}
          >
            {t("settings.p2p.enabled")}
          </ToggleGroupItem>
          <ToggleGroupItem
            value="disabled"
            className="rounded-full px-3"
            disabled={saving}
          >
            {t("settings.p2p.disabled")}
          </ToggleGroupItem>
        </ToggleGroup>
        <div className="font-mono text-xs text-muted-foreground uppercase">
          {p2pEnabled
            ? t("settings.p2p.status")
            : t("settings.p2p.statusOff")}
        </div>
      </CardContent>
    </Card>
  )
}

function ThemeCard() {
  const { theme, resolvedTheme, setTheme } = useTheme()
  const { t } = useI18n()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("theme.title")}</CardTitle>
        <CardDescription>{t("theme.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <ToggleGroup
          type="single"
          value={theme}
          onValueChange={(value) => value && setTheme(value as Theme)}
          className="grid w-full grid-cols-3 rounded-full bg-muted p-1 sm:w-auto"
          aria-label={t("theme.title")}
        >
          <ToggleGroupItem value="system" className="rounded-full px-3">
            <MonitorIcon data-icon="inline-start" />
            {t("theme.system")}
          </ToggleGroupItem>
          <ToggleGroupItem value="light" className="rounded-full px-3">
            <SunIcon data-icon="inline-start" />
            {t("theme.light")}
          </ToggleGroupItem>
          <ToggleGroupItem value="dark" className="rounded-full px-3">
            <MoonIcon data-icon="inline-start" />
            {t("theme.dark")}
          </ToggleGroupItem>
        </ToggleGroup>
        <div className="font-mono text-xs text-muted-foreground uppercase">
          {t("common.active", {
            value: t(`theme.${resolvedTheme}`, undefined, resolvedTheme),
          })}
        </div>
      </CardContent>
    </Card>
  )
}

function LanguageCard() {
  const { language, resolvedLocale, setLanguage, t } = useI18n()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.language.title")}</CardTitle>
        <CardDescription>{t("settings.language.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <Select
          value={language}
          onValueChange={(value) => setLanguage(value as LanguagePreference)}
        >
          <SelectTrigger
            className="w-full sm:w-64"
            aria-label={t("settings.language.title")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="system">
                {t("settings.language.system")}
              </SelectItem>
              {locales.map((locale) => (
                <SelectItem key={locale} value={locale}>
                  {localeNames[locale]}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <div className="font-mono text-xs text-muted-foreground uppercase">
          {t("settings.language.active", {
            value: localeNames[resolvedLocale],
          })}
        </div>
      </CardContent>
    </Card>
  )
}

async function openAPIReference(adminToken: string) {
  if (!adminToken) {
    toast.error("Save the admin token first")
    return
  }
  try {
    const res = await fetch("/api/admin/state/ws-ticket", {
      method: "POST",
      headers: { Authorization: `Bearer ${adminToken}` },
    })
    if (!res.ok) {
      toast.error(`API reference access failed: ${res.status}`)
      return
    }
    const body = (await res.json()) as { ticket?: string }
    if (!body.ticket) {
      toast.error("API reference access failed")
      return
    }
    window.open(
      `/api/admin/docs?ticket=${encodeURIComponent(body.ticket)}`,
      "_blank",
      "noopener,noreferrer"
    )
  } catch (err) {
    toast.error(
      err instanceof Error ? err.message : "API reference access failed"
    )
  }
}

function SettingsLink({
  label,
  detail,
  onClick,
}: {
  label: string
  detail: string
  onClick: () => void
}) {
  return (
    <Button
      variant="ghost"
      className="h-auto justify-start px-3 py-3 text-left"
      onClick={onClick}
    >
      <span>
        <span className="block font-medium">{label}</span>
        <span className="block text-xs text-muted-foreground">{detail}</span>
      </span>
    </Button>
  )
}

function ComponentPurpose({ title, body }: { title: string; body: string }) {
  return (
    <div className="rounded-lg border bg-muted/50 p-4">
      <div className="font-medium">{title}</div>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">{body}</p>
    </div>
  )
}
