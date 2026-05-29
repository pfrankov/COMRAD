import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { human, statusTone } from "@/lib/comrad"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"

const toneClass: Record<string, string> = {
  ready: "border-transparent bg-status-ready-bg text-status-ready",
  waiting: "border-transparent bg-status-waiting-bg text-status-waiting",
  failed: "border-transparent bg-status-failed-bg text-status-failed",
  running: "border-transparent bg-status-running-bg text-status-running",
  offline: "border-transparent bg-status-offline-bg text-status-offline",
  updating: "border-transparent bg-status-updating-bg text-status-updating",
}

export function StatusBadge({
  value,
  className,
}: {
  value?: string | boolean
  className?: string
}) {
  const { t } = useI18n()
  const text = String(value ?? "unknown")
  const tone = statusTone(text)
  return (
    <Badge
      variant="outline"
      className={cn("font-normal capitalize", toneClass[tone], className)}
    >
      {statusLabel(text, t)}
    </Badge>
  )
}

function statusLabel(value: string, t: TFunction) {
  const normalized = value.toLowerCase().replace(/[^a-z0-9]+/g, ".")
  if (normalized === "active") return t("value.active", undefined, "active")
  if (normalized === "assigned")
    return t("value.assigned", undefined, "assigned")
  if (normalized === "cancelled")
    return t("value.cancelled", undefined, "cancelled")
  if (normalized === "cached") return t("value.cached", undefined, "cached")
  if (normalized === "completed")
    return t("value.completed", undefined, "completed")
  if (normalized === "disabled")
    return t("value.disabled", undefined, "disabled")
  if (normalized === "failed") return t("value.failed", undefined, "failed")
  if (normalized === "false") return t("value.false", undefined, "false")
  if (normalized === "healthy") return t("value.healthy", undefined, "healthy")
  if (normalized === "offline") return t("value.offline", undefined, "offline")
  if (normalized === "online") return t("value.online", undefined, "online")
  if (normalized === "quarantined")
    return t("value.quarantined", undefined, "quarantined")
  if (normalized === "queued") return t("value.queued", undefined, "queued")
  if (normalized === "ready") return t("value.ready", undefined, "ready")
  if (normalized === "running") return t("value.running", undefined, "running")
  if (normalized === "unknown") return t("value.unknown", undefined, "unknown")
  if (normalized === "update.now")
    return t("value.updateNow", undefined, "update now")
  if (normalized === "verified")
    return t("value.verified", undefined, "verified")
  if (normalized === "wait") return t("value.wait", undefined, "wait")
  if (normalized === "waiting") return t("value.waiting", undefined, "waiting")
  return human(value, t)
}
