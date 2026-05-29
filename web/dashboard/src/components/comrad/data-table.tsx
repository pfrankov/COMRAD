import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { useI18n } from "@/i18n/i18n-provider"
import type { ReactNode } from "react"

type Column<T> = {
  header: string
  cell: (item: T) => ReactNode
}

export function DataTable<T>({
  columns,
  items,
  empty,
}: {
  columns: Column<T>[]
  items: T[]
  empty?: string
}) {
  const { t } = useI18n()
  if (!items.length) {
    return (
      <Empty className="border border-dashed bg-card shadow-card">
        <EmptyHeader>
          <EmptyTitle>{empty ?? t("common.noData")}</EmptyTitle>
          <EmptyDescription>{t("common.noDataDescription")}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <div className="overflow-hidden rounded-lg border bg-card shadow-card">
      <Table className="min-w-[720px]">
        <TableHeader>
          <TableRow>
            {columns.map((column) => (
              <TableHead key={column.header}>{column.header}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((item, index) => (
            <TableRow key={index}>
              {columns.map((column) => (
                <TableCell
                  key={column.header}
                  className="max-w-80 align-top whitespace-normal"
                >
                  {column.cell(item)}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
