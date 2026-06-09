import { useEffect, useMemo, useState } from "react"

import {
  KeyIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  WalletCardsIcon,
} from "lucide-react"
import { toast } from "sonner"

import { DataTable } from "@/components/comrad/data-table"
import { KeyValues, PageTitle } from "@/components/comrad/dashboard-primitives"
import { StatusBadge } from "@/components/comrad/status-badge"
import { Badge } from "@/components/ui/badge"
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
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useI18n, type TFunction } from "@/i18n/i18n-provider"
import { human, short, timeAgo } from "@/lib/comrad"
import type { Actions } from "@/comrad/actions"
import type {
  APIKey,
  APIKeyLookupResponse,
  ComputeLedgerEntry,
  StateResponse,
  User,
} from "@/types"

type UserDialog = "create" | "edit" | "key" | "topup" | null

export function UsersPage({
  state,
  actions,
}: {
  state: StateResponse
  actions: Actions
}) {
  const { t } = useI18n()
  const users = useMemo(() => state.users ?? [], [state.users])
  const apiKeys = useMemo(() => state.apiKeys ?? [], [state.apiKeys])
  const [query, setQuery] = useState("")
  const [detailUserId, setDetailUserId] = useState("")
  const [dialog, setDialog] = useState<UserDialog>(null)
  const [issuedToken, setIssuedToken] = useState("")
  const stats = useMemo(
    () => userStats(state.computeLedger ?? []),
    [state.computeLedger]
  )
  const filteredUsers = useMemo(
    () => filterUsers(users, apiKeys, query),
    [apiKeys, query, users]
  )
  const selectedUser = users.find((user) => user.userId === detailUserId)
  const openDetails = (user: User) => setDetailUserId(user.userId)
  async function lookupRawKey() {
    const token = query.trim()
    if (!token) {
      toast.error(
        t("users.toast.pasteKey", undefined, "Paste an API key first")
      )
      return
    }
    try {
      const found = await actions.fetchJSON<APIKeyLookupResponse>(
        "/api/admin/api-keys/lookup",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ token }),
        }
      )
      setDetailUserId(found.user.userId)
      setQuery(found.apiKey.apiKeyId)
      toast.success(t("users.toast.clientFound", undefined, "API client found"))
    } catch {
      toast.error(
        t(
          "users.toast.keyNotFound",
          undefined,
          "No active API key matched that token"
        )
      )
    }
  }

  return (
    <>
      <PageTitle
        eyebrow={t("nav.group.govern", undefined, "Admin")}
        title={t("users.title", undefined, "API clients")}
        description={t(
          "users.description",
          undefined,
          "Find a client, open View, then edit access, issue keys, or top up balance."
        )}
        actions={
          <Button onClick={() => setDialog("create")}>
            <PlusIcon data-icon="inline-start" />
            {t("users.createClient", undefined, "Create client")}
          </Button>
        }
      />
      <UserDirectory
        users={filteredUsers}
        allKeys={apiKeys}
        stats={stats}
        query={query}
        setQuery={setQuery}
        openDetails={openDetails}
        lookupRawKey={lookupRawKey}
        t={t}
      />
      {issuedToken ? <IssuedToken token={issuedToken} t={t} /> : null}
      <UserDetailDialog
        user={selectedUser}
        state={state}
        apiKeys={apiKeys}
        stats={stats}
        entries={state.computeLedger ?? []}
        open={detailUserId !== ""}
        setOpen={(open) => !open && setDetailUserId("")}
        openDialog={setDialog}
        t={t}
      />
      <UserActionDialog
        dialog={dialog}
        setDialog={setDialog}
        selectedUser={selectedUser}
        actions={actions}
        setIssuedToken={setIssuedToken}
        t={t}
      />
    </>
  )
}

function UserDirectory({
  users,
  allKeys,
  stats,
  query,
  setQuery,
  openDetails,
  lookupRawKey,
  t,
}: {
  users: User[]
  allKeys: APIKey[]
  stats: Record<string, { consumed: number; produced: number }>
  query: string
  setQuery: (value: string) => void
  openDetails: (user: User) => void
  lookupRawKey: () => void
  t: TFunction
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t("users.directory.title", undefined, "Find an API client")}
        </CardTitle>
        <CardDescription>
          {t(
            "users.directory.description",
            undefined,
            "Search by client name, user id, API key id, or key label. Paste a raw key and use lookup when the client only sent you the bearer token."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="relative min-w-0 flex-1">
            <SearchIcon
              data-icon="inline-start"
              className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2"
            />
            <Input
              className="pl-9"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t(
                "users.search.placeholder",
                undefined,
                "Search clients, key ids, or key labels"
              )}
            />
          </div>
          <Button
            className="h-10"
            variant="outline"
            onClick={lookupRawKey}
            disabled={!query.trim()}
          >
            <KeyIcon data-icon="inline-start" />
            {t("users.lookupKey", undefined, "Lookup key")}
          </Button>
        </div>
        <DataTable
          items={users}
          empty={t("users.empty", undefined, "No API clients")}
          rowKey={(u) => u.userId}
          columns={[
            {
              header: t("users.column.client", undefined, "API client"),
              cell: (user) => (
                <div className="flex min-w-0 flex-col gap-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-medium">
                      {user.name ||
                        t("users.unnamedClient", undefined, "Unnamed client")}
                    </span>
                    {user.disabled ? (
                      <Badge variant="destructive">
                        {t("value.disabled", undefined, "Disabled")}
                      </Badge>
                    ) : null}
                  </div>
                  <code>{short(user.userId)}</code>
                </div>
              ),
            },
            {
              header: t("users.column.balance", undefined, "Balance"),
              cell: (user) => (
                <Badge variant="secondary">{user.computeBalance ?? 0}</Badge>
              ),
            },
            {
              header: t("users.column.activity", undefined, "Activity"),
              cell: (user) => (
                <KeyValues
                  values={[
                    [
                      t("users.metric.consumed", undefined, "Consumed"),
                      String(stats[user.userId]?.consumed ?? 0),
                    ],
                    [
                      t("users.metric.produced", undefined, "Produced"),
                      String(stats[user.userId]?.produced ?? 0),
                    ],
                  ]}
                />
              ),
            },
            {
              header: t("users.column.apiKeys", undefined, "API keys"),
              cell: (user) => (
                <ClientKeysSummary
                  keys={allKeys.filter((key) => key.userId === user.userId)}
                  t={t}
                />
              ),
            },
            {
              header: t("users.column.action", undefined, "Action"),
              cell: (user) => (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => openDetails(user)}
                >
                  {t("users.view", undefined, "View")}
                </Button>
              ),
            },
          ]}
        />
      </CardContent>
    </Card>
  )
}

function ClientKeysSummary({ keys, t }: { keys: APIKey[]; t: TFunction }) {
  if (!keys.length)
    return (
      <span className="text-muted-foreground">
        {t("users.noKeys", undefined, "No keys")}
      </span>
    )
  const visible = keys.slice(0, 2)
  return (
    <div className="flex min-w-0 flex-col gap-1">
      {visible.map((key) => (
        <div key={key.apiKeyId} className="flex min-w-0 items-center gap-2">
          <StatusBadge value={key.status || "unknown"} />
          <span className="truncate">
            {key.name || t("users.unnamedKey", undefined, "Unnamed key")}
          </span>
          <code>{short(key.apiKeyId)}</code>
        </div>
      ))}
      {keys.length > visible.length ? (
        <span className="text-xs text-muted-foreground">
          {t(
            "users.moreKeys",
            { count: keys.length - visible.length },
            `+${keys.length - visible.length} more`
          )}
        </span>
      ) : null}
    </div>
  )
}

function UserDetailDialog({
  user,
  state,
  apiKeys,
  stats,
  entries,
  open,
  setOpen,
  openDialog,
  t,
}: {
  user?: User
  state: StateResponse
  apiKeys: APIKey[]
  stats: Record<string, { consumed: number; produced: number }>
  entries: ComputeLedgerEntry[]
  open: boolean
  setOpen: (open: boolean) => void
  openDialog: (dialog: UserDialog) => void
  t: TFunction
}) {
  if (!user) return null
  const keys = apiKeys.filter((key) => key.userId === user.userId)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-[860px]">
        <DialogHeader>
          <div className="flex items-start justify-between gap-3 pr-8">
            <div className="min-w-0">
              <DialogTitle className="truncate">
                {user.name ||
                  t("users.unnamedClient", undefined, "Unnamed client")}
              </DialogTitle>
              <DialogDescription>
                <code>{user.userId}</code>
              </DialogDescription>
            </div>
            <Badge variant={user.disabled ? "destructive" : "secondary"}>
              {user.disabled
                ? t("value.disabled", undefined, "Disabled")
                : t("value.active", undefined, "Active")}
            </Badge>
          </div>
        </DialogHeader>
        <div className="flex flex-col gap-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <UserMetric
            label={t("users.metric.balance", undefined, "Balance")}
            value={String(user.computeBalance ?? 0)}
          />
          <UserMetric
            label={t("users.metric.consumed", undefined, "Consumed")}
            value={String(stats[user.userId]?.consumed ?? 0)}
          />
          <UserMetric
            label={t("users.metric.produced", undefined, "Produced")}
            value={String(stats[user.userId]?.produced ?? 0)}
          />
          <UserMetric
            label={t("users.metric.activeKeys", undefined, "Active keys")}
            value={String(activeKeyCount(keys))}
          />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => openDialog("edit")}>
            <PencilIcon data-icon="inline-start" />
            {t("users.editClient", undefined, "Edit client")}
          </Button>
          <Button onClick={() => openDialog("topup")}>
            <WalletCardsIcon data-icon="inline-start" />
            {t("users.topUpBalance", undefined, "Top up balance")}
          </Button>
          <Button variant="outline" onClick={() => openDialog("key")}>
            <KeyIcon data-icon="inline-start" />
            {t("users.issueApiKey", undefined, "Issue API key")}
          </Button>
        </div>
        <KeyValues
          values={[
            [
              t("users.field.ownedWorkers", undefined, "Owned workers"),
              ownedNodes(state, user.userId),
            ],
            [
              t("users.field.created", undefined, "Created"),
              timeAgo(user.createdAt, t),
            ],
          ]}
        />
        <LedgerTable entries={entries} selectedUserId={user.userId} t={t} />
        <DataTable
          items={keys}
          empty={t(
            "users.keys.emptyForClient",
            undefined,
            "No API keys for this client"
          )}
          rowKey={(k) => k.apiKeyId}
          columns={[
            {
              header: t("users.keys.column.key", undefined, "Key"),
              cell: (key) => <code>{short(key.apiKeyId)}</code>,
            },
            {
              header: t("users.keys.column.name", undefined, "Name"),
              cell: (key) => key.name || "-",
            },
            {
              header: t("users.keys.column.status", undefined, "Status"),
              cell: (key) => <StatusBadge value={key.status} />,
            },
            {
              header: t("users.keys.column.lastUsed", undefined, "Last used"),
              cell: (key) => timeAgo(key.lastUsedAt, t),
            },
          ]}
        />
        </div>
      </DialogContent>
    </Dialog>
  )
}

function UserMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border bg-muted/40 p-3">
      <div className="font-mono text-xs text-muted-foreground uppercase">
        {label}
      </div>
      <div className="mt-1 text-lg font-semibold">{value}</div>
    </div>
  )
}

function IssuedToken({ token, t }: { token: string; t: TFunction }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t("users.issuedToken.title", undefined, "One-time API key")}
        </CardTitle>
        <CardDescription>
          {t(
            "users.issuedToken.description",
            undefined,
            "Store it outside COMRAD. Only key status is kept after this response."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <code className="break-all">{token}</code>
      </CardContent>
    </Card>
  )
}

function UserActionDialog({
  dialog,
  setDialog,
  selectedUser,
  actions,
  setIssuedToken,
  t,
}: {
  dialog: UserDialog
  setDialog: (dialog: UserDialog) => void
  selectedUser?: User
  actions: Actions
  setIssuedToken: (value: string) => void
  t: TFunction
}) {
  const [name, setName] = useState("")
  const [clientStatus, setClientStatus] = useState("active")
  const [keyName, setKeyName] = useState("")
  const [amount, setAmount] = useState("")
  const [reason, setReason] = useState(() =>
    t("users.defaultTopUpReason", undefined, "manual top-up")
  )
  const close = () => setDialog(null)
  const selectedUserId = selectedUser?.userId ?? ""
  useEffect(() => {
    if (dialog !== "edit" || !selectedUser) return
    setName(selectedUser.name ?? "")
    setClientStatus(selectedUser.disabled ? "disabled" : "active")
  }, [dialog, selectedUser])
  return (
    <Dialog open={dialog !== null} onOpenChange={(open) => !open && close()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{dialogTitle(dialog, t)}</DialogTitle>
          <DialogDescription>{dialogDescription(dialog, t)}</DialogDescription>
        </DialogHeader>
        {dialog === "create" ? (
          <FieldGroup>
            <Field>
              <FieldLabel>
                {t("users.field.displayName", undefined, "Display name")}
              </FieldLabel>
              <Input
                value={name}
                placeholder="operator"
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
          </FieldGroup>
        ) : null}
        {dialog === "edit" ? (
          <FieldGroup>
            <Field>
              <FieldLabel>
                {t("users.field.client", undefined, "Client")}
              </FieldLabel>
              <Input value={selectedUserId} readOnly />
            </Field>
            <Field>
              <FieldLabel>
                {t("users.field.displayName", undefined, "Display name")}
              </FieldLabel>
              <Input
                value={name}
                placeholder="production client"
                onChange={(event) => setName(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>
                {t("users.field.status", undefined, "Status")}
              </FieldLabel>
              <ToggleGroup
                type="single"
                value={clientStatus}
                onValueChange={(value) => value && setClientStatus(value)}
              >
                <ToggleGroupItem value="active">
                  {t("value.active", undefined, "Active")}
                </ToggleGroupItem>
                <ToggleGroupItem value="disabled">
                  {t("value.disabled", undefined, "Disabled")}
                </ToggleGroupItem>
              </ToggleGroup>
              <FieldDescription>
                {t(
                  "users.field.status.description",
                  undefined,
                  "Disabled clients cannot authenticate with active API keys."
                )}
              </FieldDescription>
            </Field>
          </FieldGroup>
        ) : null}
        {dialog === "key" ? (
          <FieldGroup>
            <Field>
              <FieldLabel>
                {t("users.field.client", undefined, "Client")}
              </FieldLabel>
              <Input value={selectedUserId} readOnly />
            </Field>
            <Field>
              <FieldLabel>
                {t("users.field.keyName", undefined, "Key name")}
              </FieldLabel>
              <Input
                value={keyName}
                placeholder="client app"
                onChange={(event) => setKeyName(event.target.value)}
              />
              <FieldDescription>
                {t(
                  "users.field.keyName.description",
                  undefined,
                  "The raw token is shown once after confirmation."
                )}
              </FieldDescription>
            </Field>
          </FieldGroup>
        ) : null}
        {dialog === "topup" ? (
          <FieldGroup>
            <Field>
              <FieldLabel>
                {t("users.field.client", undefined, "Client")}
              </FieldLabel>
              <Input value={selectedUserId} readOnly />
            </Field>
            <Field>
              <FieldLabel>
                {t("users.field.amount", undefined, "Amount")}
              </FieldLabel>
              <Input
                value={amount}
                placeholder="10 or -10"
                onChange={(event) => setAmount(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>
                {t("users.field.reason", undefined, "Reason")}
              </FieldLabel>
              <Input
                value={reason}
                onChange={(event) => setReason(event.target.value)}
              />
            </Field>
          </FieldGroup>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={close}>
            {t("common.cancel", undefined, "Cancel")}
          </Button>
          <Button
            disabled={dialog !== "create" && !selectedUserId}
            onClick={() =>
              runDialogAction({
                dialog,
                actions,
                selectedUserId,
                name,
                clientStatus,
                keyName,
                amount,
                reason,
                close,
                reset: () => {
                  setName("")
                  setClientStatus("active")
                  setKeyName("")
                  setAmount("")
                  setReason(
                    t("users.defaultTopUpReason", undefined, "manual top-up")
                  )
                },
                setIssuedToken,
                t,
              })
            }
          >
            {dialog === "create"
              ? t("users.createClient", undefined, "Create client")
              : dialog === "edit"
                ? t("users.saveClient", undefined, "Save client")
                : dialog === "key"
                  ? t("users.issueApiKey", undefined, "Issue API key")
                  : t("users.topUpBalance", undefined, "Top up balance")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function LedgerTable({
  entries,
  selectedUserId,
  t,
}: {
  entries: ComputeLedgerEntry[]
  selectedUserId?: string
  t: TFunction
}) {
  const visible = selectedUserId
    ? entries.filter((entry) => entry.userId === selectedUserId)
    : []
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t("users.ledger.title", undefined, "Compute ledger")}
        </CardTitle>
        <CardDescription>
          {t(
            "users.ledger.description",
            undefined,
            "Open View for an API client to inspect its activity and adjustments."
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <DataTable
          items={[...visible].reverse()}
          empty={t("users.ledger.empty", undefined, "No ledger entries")}
          rowKey={(e) => e.ledgerEntryId}
          columns={[
            {
              header: t("users.ledger.column.entry", undefined, "Entry"),
              cell: (entry) => <code>{short(entry.ledgerEntryId)}</code>,
            },
            {
              header: t("users.ledger.column.type", undefined, "Type"),
              cell: (entry) => human(entry.type, t),
            },
            {
              header: t("users.ledger.column.client", undefined, "Client"),
              cell: (entry) => <code>{short(entry.userId)}</code>,
            },
            {
              header: t("users.ledger.column.amount", undefined, "Amount"),
              cell: (entry) =>
                `${entry.direction === "debit" ? "-" : "+"}${entry.amount ?? 0}`,
            },
            {
              header: t(
                "users.ledger.column.requestAttempt",
                undefined,
                "Request / attempt"
              ),
              cell: (entry) => (
                <KeyValues
                  values={[
                    [
                      t("users.ledger.field.task", undefined, "Request"),
                      short(entry.taskId),
                    ],
                    [
                      t("users.ledger.field.attempt", undefined, "Attempt"),
                      short(entry.attemptId),
                    ],
                    [
                      t("users.ledger.field.report", undefined, "Report"),
                      short(entry.reportId),
                    ],
                  ]}
                />
              ),
            },
            {
              header: t("users.ledger.column.profile", undefined, "Profile"),
              cell: (entry) => (
                <code>
                  {short(entry.profileId)} v{entry.profileVersion || "-"}
                </code>
              ),
            },
            {
              header: t("users.ledger.column.reason", undefined, "Reason"),
              cell: (entry) => human(entry.reason, t),
            },
          ]}
        />
      </CardContent>
    </Card>
  )
}

function dialogTitle(dialog: UserDialog, t: TFunction) {
  if (dialog === "create")
    return t("users.dialog.create.title", undefined, "Create API client")
  if (dialog === "edit")
    return t("users.dialog.edit.title", undefined, "Edit API client")
  if (dialog === "key")
    return t("users.dialog.key.title", undefined, "Issue API key")
  if (dialog === "topup")
    return t("users.dialog.topup.title", undefined, "Top up balance")
  return t("users.dialog.action.title", undefined, "Client action")
}

function dialogDescription(dialog: UserDialog, t: TFunction) {
  if (dialog === "create")
    return t(
      "users.dialog.create.description",
      undefined,
      "Create a client identity for API access and accounting."
    )
  if (dialog === "edit")
    return t(
      "users.dialog.edit.description",
      undefined,
      "Update the selected client identity."
    )
  if (dialog === "key")
    return t(
      "users.dialog.key.description",
      undefined,
      "Issue a bearer token for the selected client."
    )
  if (dialog === "topup")
    return t(
      "users.dialog.topup.description",
      undefined,
      "Record a manual ledger adjustment for the selected client."
    )
  return t("users.dialog.action.description", undefined, "Choose an action.")
}

function filterUsers(users: User[], keys: APIKey[], query: string) {
  const needle = query.trim().toLowerCase()
  if (!needle) return users
  return users.filter((user) => {
    const userText = `${user.userId} ${user.name}`.toLowerCase()
    const keyText = keys
      .filter((key) => key.userId === user.userId)
      .map((key) => `${key.apiKeyId} ${key.name}`)
      .join(" ")
      .toLowerCase()
    return userText.includes(needle) || keyText.includes(needle)
  })
}

function activeKeyCount(keys: APIKey[]) {
  return keys.filter((key) => key.status === "active").length
}

function runDialogAction(args: {
  dialog: UserDialog
  actions: Actions
  selectedUserId: string
  name: string
  clientStatus: string
  keyName: string
  amount: string
  reason: string
  close: () => void
  reset: () => void
  setIssuedToken: (value: string) => void
  t: TFunction
}) {
  if (args.dialog === "create") {
    confirmCreateUser(args)
  } else if (args.dialog === "edit") {
    confirmEditClient(args)
  } else if (args.dialog === "key") {
    confirmIssueKey(args)
  } else if (args.dialog === "topup") {
    confirmTopUp(args)
  }
}

function confirmCreateUser(args: Parameters<typeof runDialogAction>[0]) {
  args.actions.setConfirm({
    title: args.t("users.confirm.create.title", undefined, "Create API client"),
    body: args.t(
      "users.confirm.create.body",
      {
        name:
          args.name ||
          args.t("users.confirm.newClient", undefined, "a new client"),
      },
      `This registers ${args.name || "a new client"} for API identity and compute accounting.`
    ),
    confirmLabel: args.t("users.createClient", undefined, "Create client"),
    variant: "default",
    run: async () => {
      await args.actions.api("/api/admin/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: args.name }),
      })
      args.reset()
      args.close()
      toast.success(
        args.t(
          "users.toast.clientRegistered",
          undefined,
          "API client registered"
        )
      )
    },
  })
}

function confirmEditClient(args: Parameters<typeof runDialogAction>[0]) {
  args.actions.setConfirm({
    title: args.t("users.confirm.edit.title", undefined, "Edit API client"),
    body: args.t(
      "users.confirm.edit.body",
      {
        client:
          args.selectedUserId ||
          args.t(
            "users.confirm.selectedClient",
            undefined,
            "the selected client"
          ),
      },
      `This updates ${args.selectedUserId || "the selected client"}. Disabled clients cannot authenticate with active API keys.`
    ),
    confirmLabel: args.t("users.saveClient", undefined, "Save client"),
    variant: "default",
    run: async () => {
      await args.actions.api("/api/admin/users", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          userId: args.selectedUserId,
          name: args.name,
          disabled: args.clientStatus === "disabled",
        }),
      })
      args.reset()
      args.close()
      toast.success(
        args.t("users.toast.clientUpdated", undefined, "API client updated")
      )
    },
  })
}

function confirmIssueKey(args: Parameters<typeof runDialogAction>[0]) {
  args.actions.setConfirm({
    title: args.t("users.confirm.key.title", undefined, "Issue API key"),
    body: args.t(
      "users.confirm.key.body",
      {
        client:
          args.selectedUserId ||
          args.t(
            "users.confirm.selectedClient",
            undefined,
            "the selected client"
          ),
      },
      `This creates a bearer token for ${args.selectedUserId || "the selected client"}. The raw key will be shown once.`
    ),
    confirmLabel: args.t("users.confirm.key.label", undefined, "Issue key"),
    variant: "default",
    run: async () => {
      const issued = await args.actions.fetchJSON<{ token: string }>(
        "/api/admin/api-keys",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            userId: args.selectedUserId,
            name: args.keyName,
          }),
        }
      )
      args.setIssuedToken(issued.token)
      await args.actions.api("/api/admin/state")
      args.reset()
      args.close()
      toast.success(
        args.t("users.toast.keyIssued", undefined, "API key issued")
      )
    },
  })
}

function confirmTopUp(args: Parameters<typeof runDialogAction>[0]) {
  args.actions.setConfirm({
    title: args.t("users.confirm.topup.title", undefined, "Top up balance"),
    body: args.t(
      "users.confirm.topup.body",
      {
        amount: args.amount || 0,
        client:
          args.selectedUserId ||
          args.t(
            "users.confirm.selectedClient",
            undefined,
            "the selected client"
          ),
      },
      `This appends a ${args.amount || 0} compute ledger adjustment for ${args.selectedUserId || "the selected client"}.`
    ),
    confirmLabel: args.t("users.confirm.topup.label", undefined, "Top up"),
    variant: "default",
    run: async () => {
      await args.actions.api("/api/admin/users/adjust-balance", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          userId: args.selectedUserId,
          amount: Number(args.amount || 0),
          reason: args.reason,
        }),
      })
      args.reset()
      args.close()
      toast.success(
        args.t("users.toast.balanceUpdated", undefined, "Balance updated")
      )
    },
  })
}

function userStats(entries: ComputeLedgerEntry[]) {
  const out: Record<string, { consumed: number; produced: number }> = {}
  for (const entry of entries) {
    out[entry.userId] ||= { consumed: 0, produced: 0 }
    if (entry.type === "consume_compute") {
      out[entry.userId].consumed += entry.amount ?? 0
    }
    if (entry.type === "produce_compute") {
      out[entry.userId].produced += entry.amount ?? 0
    }
  }
  return out
}

function ownedNodes(state: StateResponse, userId: string) {
  const nodes = (state.nodes ?? []).filter(
    (node) => node.ownerUserId === userId
  )
  return nodes.length ? nodes.map((node) => short(node.nodeId)).join(", ") : "-"
}
