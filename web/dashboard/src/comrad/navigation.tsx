import {
  ArchiveIcon,
  BoxesIcon,
  CpuIcon,
  HomeIcon,
  ListChecksIcon,
  SettingsIcon,
  SlidersHorizontalIcon,
  UploadCloudIcon,
  UsersIcon,
} from "lucide-react"
import { useMemo } from "react"

import { profileLabel } from "@/lib/comrad"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import type { StateResponse } from "@/types"

export const sectionDefinitions = [
  {
    id: "overview",
    groupKey: "nav.group.operate",
    labelKey: "nav.overview.label",
    descriptionKey: "nav.overview.description",
    icon: HomeIcon,
  },
  {
    id: "tasks",
    groupKey: "nav.group.operate",
    labelKey: "nav.tasks.label",
    descriptionKey: "nav.tasks.description",
    icon: ListChecksIcon,
  },
  {
    id: "profiles",
    groupKey: "nav.group.serve",
    labelKey: "nav.profiles.label",
    descriptionKey: "nav.profiles.description",
    icon: BoxesIcon,
  },
  {
    id: "placement",
    groupKey: "nav.group.serve",
    labelKey: "nav.placement.label",
    descriptionKey: "nav.placement.description",
    icon: SlidersHorizontalIcon,
  },
  {
    id: "nodes",
    groupKey: "nav.group.serve",
    labelKey: "nav.nodes.label",
    descriptionKey: "nav.nodes.description",
    icon: CpuIcon,
  },
  {
    id: "artifacts",
    groupKey: "nav.group.serve",
    labelKey: "nav.artifacts.label",
    descriptionKey: "nav.artifacts.description",
    icon: ArchiveIcon,
  },
  {
    id: "users",
    groupKey: "nav.group.govern",
    labelKey: "nav.users.label",
    descriptionKey: "nav.users.description",
    icon: UsersIcon,
  },
  {
    id: "updates",
    groupKey: "nav.group.govern",
    labelKey: "nav.updates.label",
    descriptionKey: "nav.updates.description",
    icon: UploadCloudIcon,
  },
  {
    id: "settings",
    groupKey: "nav.group.govern",
    labelKey: "nav.settings.label",
    descriptionKey: "nav.settings.description",
    icon: SettingsIcon,
  },
]

export type Section = (typeof sectionDefinitions)[number] & {
  group: string
  label: string
  description: string
}

export function useSections() {
  const { t } = useI18n()
  return useMemo(
    () =>
      sectionDefinitions.map((section) => ({
        ...section,
        group: t(section.groupKey),
        label: t(section.labelKey),
        description: t(section.descriptionKey),
      })),
    [t]
  )
}

export function commandItems(
  state: StateResponse | null,
  show: (id: string) => void,
  t: TFunction,
  sections: Section[]
) {
  const items = sections.map((section) => ({
    label: section.label,
    meta: t("shell.commandMeta.section"),
    run: () => show(section.id),
  }))
  if (!state) return items
  items.push(
    {
      label: t("shell.command.setModelCapacity"),
      meta: t("shell.commandMeta.action"),
      run: () => show("placement"),
    },
    {
      label: t("shell.command.triggerUpdate"),
      meta: t("shell.commandMeta.action"),
      run: () => show("updates"),
    }
  )
  for (const node of state.nodes ?? [])
    items.push({
      label: node.name || node.nodeId,
      meta: t("shell.commandMeta.node", { state: node.state || "-" }),
      run: () => show("nodes"),
    })
  for (const profile of state.profiles ?? [])
    items.push({
      label: profileLabel(profile),
      meta: t("shell.commandMeta.profile", { kind: profile.kind || "-" }),
      run: () => show("profiles"),
    })
  for (const user of state.users ?? [])
    items.push({
      label: user.name || user.userId,
      meta: t("shell.commandMeta.userBalance", {
        balance: user.computeBalance ?? 0,
      }),
      run: () => show("users"),
    })
  for (const task of (state.tasks ?? []).slice(-10))
    items.push({
      label: task.taskId,
      meta: t("shell.commandMeta.recentTask", { status: task.status || "-" }),
      run: () => show("tasks"),
    })
  for (const artifact of state.artifacts ?? [])
    items.push({
      label: artifact.artifactId,
      meta: t("shell.commandMeta.artifact", { kind: artifact.kind || "-" }),
      run: () => show("artifacts"),
    })
  return items
}
