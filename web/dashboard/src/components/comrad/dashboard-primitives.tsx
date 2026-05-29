import type { ReactNode } from "react"

import { useI18n } from "@/i18n/i18n-provider"

export function PageTitle({
  title,
  description,
  actions,
  eyebrow,
}: {
  title: string
  description: string
  actions?: ReactNode
  eyebrow?: string
}) {
  return (
    <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
      <div className="max-w-3xl">
        {eyebrow ? (
          <div className="mb-2 font-mono text-xs font-normal text-muted-foreground uppercase">
            {eyebrow}
          </div>
        ) : null}
        <h1 className="text-[32px] leading-10 font-semibold">{title}</h1>
        <p className="mt-2 text-base text-muted-foreground">{description}</p>
      </div>
      {actions ? <div className="flex flex-wrap gap-2">{actions}</div> : null}
    </div>
  )
}

export function CodeBlock({
  value,
  detail,
}: {
  value?: string
  detail?: string
}) {
  return (
    <div className="flex flex-col gap-1">
      <code>{value || "-"}</code>
      {detail ? (
        <span className="font-mono text-xs text-muted-foreground">
          {detail}
        </span>
      ) : null}
    </div>
  )
}

export function KeyValues({ values }: { values: Array<[string, string]> }) {
  return (
    <dl className="grid gap-2 text-sm md:grid-cols-[180px_minmax(0,1fr)]">
      {values.map(([key, value]) => (
        <div key={key} className="contents">
          <dt className="text-muted-foreground">{key}</dt>
          <dd className="min-w-0 break-words">{value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function DryRun({ title, items }: { title: string; items: string[] }) {
  const { t } = useI18n()
  return (
    <div className="rounded-lg border bg-muted/50 p-4">
      <div className="text-sm font-medium">{title}</div>
      <div className="mt-3 flex flex-col gap-1 text-sm text-muted-foreground">
        {items.length ? (
          items.slice(0, 8).map((item) => <span key={item}>{item}</span>)
        ) : (
          <span>{t("common.none")}</span>
        )}
      </div>
    </div>
  )
}

export function OperatorPath({
  steps,
}: {
  steps: Array<{ title: string; body: string; action?: ReactNode }>
}) {
  return (
    <div className="grid gap-3 lg:grid-cols-3">
      {steps.map((step, index) => (
        <div key={step.title} className="rounded-lg border bg-muted/50 p-4">
          <div className="flex items-center gap-3">
            <span className="grid size-7 place-items-center rounded-md bg-primary text-xs font-medium text-primary-foreground">
              {index + 1}
            </span>
            <div className="font-medium">{step.title}</div>
          </div>
          <p className="mt-3 text-sm leading-6 text-muted-foreground">
            {step.body}
          </p>
          {step.action ? <div className="mt-4">{step.action}</div> : null}
        </div>
      ))}
    </div>
  )
}
