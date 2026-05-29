import { StatusBadge } from "@/components/comrad/status-badge"
import { human, timeAgo } from "@/lib/comrad"
import type { TFunction } from "@/i18n/i18n-provider"
import type { Condition } from "@/types"

export function ConditionList({
  conditions,
  empty,
  t,
}: {
  conditions?: Condition[]
  empty: string
  t: TFunction
}) {
  const visible = conditions ?? []
  if (!visible.length) {
    return <p className="text-sm text-muted-foreground">{empty}</p>
  }
  return (
    <div className="grid gap-2">
      {visible.map((condition) => (
        <div
          key={`${condition.type}-${condition.reason}`}
          className="grid gap-2 rounded-md border p-3 sm:grid-cols-[140px_90px_1fr] sm:items-start"
        >
          <div className="font-mono text-xs text-muted-foreground uppercase">
            {condition.type}
          </div>
          <StatusBadge value={condition.status} />
          <div className="min-w-0 text-sm">
            <div className="truncate font-medium">
              {human(condition.reason, t)}
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              {condition.message || human(condition.reason, t)}
              {condition.lastTransitionTime
                ? ` · ${timeAgo(condition.lastTransitionTime, t)}`
                : ""}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

export function conditionBadgeValue(conditions: Condition[] | undefined) {
  const ready = conditionByType(conditions, "Ready")
  if (!ready) return "unknown"
  return ready.status === "True" ? "ready" : ready.reason || "waiting"
}

export function conditionSummary(
  conditions: Condition[] | undefined,
  type: string,
  t: TFunction,
  fallback: string
) {
  const condition = conditionByType(conditions, type)
  if (!condition) return fallback
  return condition.message || human(condition.reason, t)
}

export function conditionByType(
  conditions: Condition[] | undefined,
  type: string
) {
  return (conditions ?? []).find((condition) => condition.type === type)
}
