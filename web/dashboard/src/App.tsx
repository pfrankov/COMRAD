import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"

import {
  CheckCircle2Icon,
  LoaderCircleIcon,
  SearchIcon,
  WifiIcon,
  WifiOffIcon,
} from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { ArtifactsPage } from "@/pages/artifacts"
import { NodesPage } from "@/pages/nodes"
import { OverviewPage } from "@/pages/overview"
import { PlacementPage } from "@/pages/placement"
import { ProfilesPage } from "@/pages/profiles"
import { SettingsPage } from "@/pages/settings"
import { TasksPage } from "@/pages/tasks"
import { UpdatesPage } from "@/pages/updates"
import { UsersPage } from "@/pages/users"
import { cn } from "@/lib/utils"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import {
  uploadArtifactRequest,
  type Actions,
  type ConfirmAction,
} from "@/comrad/actions"
import { commandItems, useSections, type Section } from "@/comrad/navigation"
import type { StateResponse } from "@/types"

type ConnectionStatus =
  | "idle"
  | "connecting"
  | "connected"
  | "reconnecting"
  | "disconnected"

export function App() {
  const { t } = useI18n()
  const sections = useSections()
  const [active, setActive] = useState(
    () => window.location.hash.slice(1) || "overview"
  )
  const [token, setToken] = useState(
    () => localStorage.getItem("comrad.adminToken") || ""
  )
  const [state, setState] = useState<StateResponse | null>(null)
  const [commandOpen, setCommandOpen] = useState(false)
  const [confirm, setConfirm] = useState<ConfirmAction | null>(null)
  const { connectionStatus, lastStateAt } = useAdminStateStream(token, setState)

  const show = useCallback((id: string) => {
    setActive(id)
    window.history.pushState(null, "", `#${id}`)
  }, [])

  const saveAdminToken = useCallback((nextToken: string) => {
    const trimmed = nextToken.trim()
    if (trimmed) localStorage.setItem("comrad.adminToken", trimmed)
    else localStorage.removeItem("comrad.adminToken")
    setToken(trimmed)
  }, [])

  const fetchJSON = useCallback(
    async <T,>(path: string, init?: RequestInit): Promise<T> => {
      const headers = {
        Authorization: `Bearer ${token}`,
        ...((init?.headers as Record<string, string>) || {}),
      }
      const res = await fetch(path, { ...init, headers })
      if (!res.ok) throw new Error(await res.text())
      return (await res.json()) as T
    },
    [token]
  )

  const fetchText = useCallback(
    async (path: string, init?: RequestInit): Promise<string> => {
      const headers = {
        Authorization: `Bearer ${token}`,
        ...((init?.headers as Record<string, string>) || {}),
      }
      const res = await fetch(path, { ...init, headers })
      if (!res.ok) throw new Error(await res.text())
      return res.text()
    },
    [token]
  )

  const api = useCallback(
    async (path: string, init?: RequestInit) => {
      const headers = {
        Authorization: `Bearer ${token}`,
        ...((init?.headers as Record<string, string>) || {}),
      }
      const res = await fetch(path, { ...init, headers })
      if (!res.ok) throw new Error(await res.text())
    },
    [token]
  )

  const uploadArtifact = useCallback(
    (
      file: File,
      kind: string,
      onUploadProgress?: Parameters<Actions["uploadArtifact"]>[2]
    ) => uploadArtifactRequest(token, file, kind, onUploadProgress),
    [token]
  )

  useEffect(() => {
    const onHash = () => setActive(window.location.hash.slice(1) || "overview")
    window.addEventListener("hashchange", onHash)
    return () => window.removeEventListener("hashchange", onHash)
  }, [])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault()
        setCommandOpen(true)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [])

  const actions = useMemo<Actions>(
    () => ({ api, fetchJSON, fetchText, uploadArtifact, show, setConfirm }),
    [api, fetchJSON, fetchText, show, uploadArtifact]
  )
  const commands = useMemo(
    () => commandItems(state, show, t, sections),
    [sections, state, show, t]
  )
  const content = state ? (
    renderPage(active, state, actions, {
      adminToken: token,
      saveAdminToken,
    })
  ) : active === "settings" ? (
    <SettingsPage
      state={{} as StateResponse}
      actions={actions}
      adminToken={token}
      saveAdminToken={saveAdminToken}
    />
  ) : (
    <LoadingState />
  )

  return (
    <TooltipProvider>
      <div className="min-h-svh bg-background text-foreground">
        <div className="flex min-h-svh flex-col lg:grid lg:grid-cols-[260px_minmax(0,1fr)]">
          <Sidebar active={active} state={state} show={show} />
          <div className="min-w-0">
            <Topbar
              connectionStatus={connectionStatus}
              lastStateAt={lastStateAt}
              openCommand={() => setCommandOpen(true)}
            />
            <main className="mx-auto flex max-w-[1440px] flex-col gap-6 px-4 py-6 lg:px-8 lg:py-8">
              {content}
            </main>
          </div>
        </div>
        <CommandPalette
          open={commandOpen}
          setOpen={setCommandOpen}
          commands={commands}
        />
        <ConfirmDialog confirm={confirm} setConfirm={setConfirm} />
        <Toaster />
      </div>
    </TooltipProvider>
  )
}

async function issueAdminStateWSTicket(token: string) {
  const res = await fetch("/api/admin/state/ws-ticket", {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) throw new Error(await res.text())
  const body = (await res.json()) as { ticket: string }
  if (!body.ticket) throw new Error("missing websocket ticket")
  return body.ticket
}

function adminStateWebSocketURL(ticket: string) {
  const url = new URL("/api/admin/state/ws", window.location.href)
  url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
  url.searchParams.set("ticket", ticket)
  return url
}

function useAdminStateStream(
  token: string,
  setState: (state: StateResponse | null) => void
) {
  const { t } = useI18n()
  const [connectionStatus, setConnectionStatus] =
    useState<ConnectionStatus>("idle")
  const [lastStateAt, setLastStateAt] = useState<number | null>(null)

  useEffect(() => {
    if (!token) {
      setState(null)
      setLastStateAt(null)
      setConnectionStatus("idle")
      return undefined
    }

    let closed = false
    let hasConnected = false
    let retryTimer: number | undefined
    let socket: WebSocket | null = null

    const reconnect = (status: ConnectionStatus = "reconnecting") => {
      if (closed) return
      setConnectionStatus(status)
      retryTimer = window.setTimeout(connect, 2000)
    }

    const connect = async () => {
      setConnectionStatus(hasConnected ? "reconnecting" : "connecting")
      try {
        const ticket = await issueAdminStateWSTicket(token)
        if (closed) return
        const nextSocket = new WebSocket(adminStateWebSocketURL(ticket))
        socket = nextSocket
        nextSocket.onopen = () => {
          hasConnected = true
          setConnectionStatus("connected")
        }
        nextSocket.onmessage = (event) => {
          try {
            setState(JSON.parse(event.data) as StateResponse)
            setLastStateAt(Date.now())
          } catch {
            toast.error(
              t(
                "shell.stateStreamInvalid",
                undefined,
                "Manager sent an unreadable state update"
              )
            )
          }
        }
        nextSocket.onerror = () => nextSocket.close()
        nextSocket.onclose = () => reconnect("reconnecting")
      } catch {
        reconnect(hasConnected ? "reconnecting" : "disconnected")
      }
    }

    connect()
    return () => {
      closed = true
      if (retryTimer) window.clearTimeout(retryTimer)
      socket?.close()
    }
  }, [setState, t, token])

  return { connectionStatus, lastStateAt }
}

function SidebarText({
  children,
  className,
  title,
}: {
  children: ReactNode
  className?: string
  title?: string
}) {
  const ref = useRef<HTMLSpanElement>(null)
  const [overflowing, setOverflowing] = useState(false)

  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return undefined

    let cancelled = false
    const measure = () => {
      if (cancelled) return
      setOverflowing(element.scrollWidth > element.clientWidth + 1)
    }

    measure()
    const animationFrame = window.requestAnimationFrame(measure)
    void document.fonts?.ready.then(measure).catch(() => undefined)
    let observer: ResizeObserver | null = null
    if (typeof ResizeObserver !== "undefined") {
      observer = new ResizeObserver(measure)
      observer.observe(element)
    } else {
      window.addEventListener("resize", measure)
    }

    return () => {
      cancelled = true
      window.cancelAnimationFrame(animationFrame)
      observer?.disconnect()
      window.removeEventListener("resize", measure)
    }
  }, [children])

  const overflowTitle =
    title ?? (typeof children === "string" ? children : undefined)

  return (
    <span
      ref={ref}
      className={cn("sidebar-fade", className)}
      data-overflow={overflowing}
      title={overflowing ? overflowTitle : undefined}
    >
      {children}
    </span>
  )
}

function Sidebar({
  active,
  state,
  show,
}: {
  active: string
  state: StateResponse | null
  show: (id: string) => void
}) {
  const { t } = useI18n()
  const sections = useSections()
  const versionText = state?.version || t("app.waiting")
  const groups = sections.reduce<Array<{ label: string; items: Section[] }>>(
    (out, section) => {
      const group = out.find((item) => item.label === section.group)
      if (group) group.items.push(section)
      else out.push({ label: section.group, items: [section] })
      return out
    },
    []
  )
  return (
    <aside className="border-b border-sidebar-border bg-sidebar text-sidebar-foreground lg:sticky lg:top-0 lg:h-svh lg:overflow-y-auto lg:border-r lg:border-b-0">
      <div className="flex min-h-16 items-center gap-3 border-b border-sidebar-border px-5">
        <div className="grid size-8 place-items-center rounded-md bg-sidebar-primary text-sidebar-primary-foreground">
          <CheckCircle2Icon />
        </div>
        <div className="min-w-0">
          <div className="text-sm font-medium">COMRAD Manager</div>
          <div className="text-xs leading-snug text-muted-foreground">
            {t("app.controlPlane")} ·{" "}
            <span className="font-mono">{versionText}</span>
          </div>
        </div>
      </div>
      <nav className="flex gap-2 overflow-x-auto p-3 lg:grid lg:gap-4 lg:overflow-visible">
        {groups.map((group) => (
          <div
            key={group.label}
            className="flex shrink-0 gap-2 lg:grid lg:gap-1"
          >
            <div className="hidden px-3 font-mono text-xs font-normal text-muted-foreground uppercase lg:block">
              {group.label}
            </div>
            {group.items.map((section) => (
              <Button
                key={section.id}
                variant="ghost"
                className={cn(
                  "h-11 w-36 justify-start gap-3 rounded-md border-l-2 px-3 text-left text-muted-foreground lg:h-auto lg:w-full lg:min-w-0 lg:py-2.5",
                  active === section.id &&
                    "border-l-primary bg-sidebar-accent text-foreground"
                )}
                onClick={() => show(section.id)}
              >
                <section.icon data-icon="inline-start" />
                <span className="min-w-0 flex-1">
                  <span className="block">{section.label}</span>
                  <SidebarText
                    className="hidden text-xs font-normal text-muted-foreground lg:block"
                    title={section.description}
                  >
                    {section.description}
                  </SidebarText>
                </span>
              </Button>
            ))}
          </div>
        ))}
      </nav>
    </aside>
  )
}

function Topbar({
  connectionStatus,
  lastStateAt,
  openCommand,
}: {
  connectionStatus: ConnectionStatus
  lastStateAt: number | null
  openCommand: () => void
}) {
  const { t } = useI18n()

  return (
    <header className="sticky top-0 z-10 border-b border-border bg-card/90 px-4 py-3 backdrop-blur lg:px-8">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <Button
          type="button"
          variant="outline"
          className="h-10 w-full justify-start sm:max-w-md"
          onClick={openCommand}
        >
          <SearchIcon data-icon="inline-start" />
          {t("common.search")}
        </Button>
        <ConnectionBadge status={connectionStatus} lastStateAt={lastStateAt} />
      </div>
    </header>
  )
}

function ConnectionBadge({
  status,
  lastStateAt,
}: {
  status: ConnectionStatus
  lastStateAt: number | null
}) {
  const { t } = useI18n()
  const Icon = connectionStatusIcon(status)
  const label = connectionStatusLabel(status, t)
  const detail = connectionStatusDetail(lastStateAt, t)

  return (
    <Badge
      variant="outline"
      title={detail}
      className={cn(
        "h-8 gap-1.5 self-start rounded-md px-2.5 font-normal sm:self-auto",
        connectionStatusClass(status)
      )}
    >
      <Icon
        data-icon="inline-start"
        className={cn(
          status !== "connected" && status !== "idle" && "animate-spin"
        )}
      />
      {label}
    </Badge>
  )
}

function connectionStatusIcon(status: ConnectionStatus) {
  if (status === "connected") return WifiIcon
  if (status === "idle") return WifiOffIcon
  if (status === "disconnected") return WifiOffIcon
  return LoaderCircleIcon
}

function connectionStatusClass(status: ConnectionStatus) {
  if (status === "connected") {
    return "border-transparent bg-status-running-bg text-status-running"
  }
  if (status === "reconnecting" || status === "connecting") {
    return "border-transparent bg-status-waiting-bg text-status-waiting"
  }
  return "border-transparent bg-status-offline-bg text-status-offline"
}

function connectionStatusLabel(status: ConnectionStatus, t: TFunction) {
  switch (status) {
    case "connected":
      return t("shell.connection.connected", undefined, "Live")
    case "connecting":
      return t("shell.connection.connecting", undefined, "Connecting")
    case "reconnecting":
      return t("shell.connection.reconnecting", undefined, "Reconnecting")
    case "disconnected":
      return t("shell.connection.disconnected", undefined, "Disconnected")
    default:
      return t("shell.connection.idle", undefined, "No token")
  }
}

function connectionStatusDetail(lastStateAt: number | null, t: TFunction) {
  if (!lastStateAt) {
    return t("shell.connection.noUpdates", undefined, "No state received yet")
  }
  return t(
    "shell.connection.lastUpdate",
    { time: new Date(lastStateAt).toLocaleTimeString() },
    "Last update {time}"
  )
}

function CommandPalette({
  open,
  setOpen,
  commands,
}: {
  open: boolean
  setOpen: (open: boolean) => void
  commands: ReturnType<typeof commandItems>
}) {
  const { t } = useI18n()
  return (
    <CommandDialog
      open={open}
      onOpenChange={setOpen}
      title={t("shell.commandPaletteTitle")}
      description={t("shell.commandDescription")}
    >
      <Command>
        <CommandInput placeholder={t("shell.commandPlaceholder")} />
        <CommandList>
          <CommandEmpty>{t("shell.commandNoResults")}</CommandEmpty>
          <CommandGroup heading={t("shell.commandGroup")}>
            {commands.map((item) => (
              <CommandItem
                key={`${item.meta}-${item.label}`}
                onSelect={() => {
                  item.run()
                  setOpen(false)
                }}
              >
                <span>{item.label}</span>
                <span className="ml-auto text-xs text-muted-foreground">
                  {item.meta}
                </span>
              </CommandItem>
            ))}
          </CommandGroup>
        </CommandList>
      </Command>
    </CommandDialog>
  )
}

function ConfirmDialog({
  confirm,
  setConfirm,
}: {
  confirm: ConfirmAction | null
  setConfirm: (confirm: ConfirmAction | null) => void
}) {
  const { t } = useI18n()
  return (
    <Dialog open={!!confirm} onOpenChange={(open) => !open && setConfirm(null)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{confirm?.title}</DialogTitle>
          <DialogDescription>{confirm?.body}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setConfirm(null)}>
            {t("common.cancel")}
          </Button>
          <Button
            variant={confirm?.variant ?? "destructive"}
            onClick={() => {
              const action = confirm
              setConfirm(null)
              void action?.run()
            }}
          >
            {confirm?.confirmLabel ?? t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function renderPage(
  active: string,
  state: StateResponse,
  actions: Actions,
  settingsAuth: {
    adminToken: string
    saveAdminToken: (token: string) => void
  }
) {
  switch (active) {
    case "nodes":
      return <NodesPage state={state} actions={actions} />
    case "profiles":
      return <ProfilesPage state={state} actions={actions} />
    case "users":
      return <UsersPage state={state} actions={actions} />
    case "placement":
      return <PlacementPage state={state} actions={actions} />
    case "tasks":
      return <TasksPage state={state} actions={actions} />
    case "artifacts":
      return <ArtifactsPage state={state} actions={actions} />
    case "updates":
      return <UpdatesPage state={state} actions={actions} />
    case "settings":
      return (
        <SettingsPage
          state={state}
          actions={actions}
          adminToken={settingsAuth.adminToken}
          saveAdminToken={settingsAuth.saveAdminToken}
        />
      )
    default:
      return <OverviewPage state={state} actions={actions} />
  }
}

function LoadingState() {
  const { t } = useI18n()
  return (
    <Alert>
      <AlertTitle>{t("shell.loadingTitle")}</AlertTitle>
      <AlertDescription>{t("shell.loadingDescription")}</AlertDescription>
    </Alert>
  )
}

export default App
