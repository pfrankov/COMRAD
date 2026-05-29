export function MetricCard({
  label,
  value,
  detail,
}: {
  label: string
  value: string | number
  detail?: string
}) {
  return (
    <div className="rounded-lg border bg-muted/50 p-4">
      <div className="font-mono text-xs font-normal text-muted-foreground uppercase">
        {label}
      </div>
      <div className="mt-3 text-3xl leading-none font-semibold">{value}</div>
      {detail ? (
        <p className="mt-2 text-sm text-muted-foreground">{detail}</p>
      ) : null}
    </div>
  )
}
