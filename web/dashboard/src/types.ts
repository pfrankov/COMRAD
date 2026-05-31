export type ResourceBudget = {
  ramBytes?: number
  vramBytes?: number
  unifiedMemoryBytes?: number
  diskBytes?: number
  slotCount?: number
}

export type Condition = {
  type: string
  status?: string
  reason?: string
  message?: string
  lastTransitionTime?: string
}

export type Node = {
  nodeId: string
  ownerUserId?: string
  name?: string
  os?: string
  arch?: string
  target?: string
  mode?: string
  tags?: string[]
  state?: string
  version?: string
  runtimeAdapters?: string[]
  budgets?: ResourceBudget
  cachedArtifacts?: string[]
  warmProfiles?: string[]
  lastSeen?: string
  approved?: boolean
  updateRequired?: boolean
  updateStatus?: string
  lastFailure?: string
  quarantined?: boolean
  quarantineReason?: string
  quarantineUntil?: string
  conditions?: Condition[]
}

export type Slot = {
  slotId: string
  nodeId: string
  target?: string
  runtimeAdapter?: string
  resources?: ResourceBudget
  state?: string
  profileId?: string
  profileVersion?: number
  logicalModel?: string
  runtimeVariantId?: string
  modelArtifactId?: string
  acceptsNewTasks?: boolean
  activeTaskId?: string
  mismatchReason?: string
  failureCount?: number
  failureCounters?: Record<string, number>
  lastFailure?: string
  quarantined?: boolean
  quarantineReason?: string
  quarantineUntil?: string
  conditions?: Condition[]
}

export type Artifact = {
  artifactId: string
  kind?: string
  name?: string
  sha256?: string
  sizeBytes?: number
}

export type ArtifactEvictionRecord = {
  evictionId: string
  nodeId: string
  artifactId: string
  reason?: string
  status?: string
  failure?: string
  requestedAt?: string
  updatedAt?: string
  conditions?: Condition[]
}

export type Requirements = {
  target?: string
  runtimeAdapter?: string
  ramBytes?: number
  vramBytes?: number
  unifiedMemoryBytes?: number
  diskBytes?: number
  requireTags?: string[]
  minimumWorkerVersion?: string
}

export type RuntimeParameters = {
  llamaCpp?: { args?: string[] }
}

export type RuntimeVariant = {
  variantId?: string
  name?: string
  target?: string
  runtimeAdapter?: string
  artifacts?: string[]
  requirements?: Requirements
  llm?: { contextTokens?: number }
  runtime?: RuntimeParameters
}

export type Profile = {
  profileId: string
  profileVersion?: number
  computeCost?: number
  name?: string
  alias?: string
  logicalModel?: string
  runtimeVariantId?: string
  kind?: string
  runtimeAdapter?: string
  artifacts?: string[]
  requirements?: Requirements
  llm?: { contextTokens?: number }
  runtime?: RuntimeParameters
  runtimeVariants?: RuntimeVariant[]
  warmable?: boolean
  conditions?: Condition[]
}

export type RuntimeSummary = {
  apiVersion?: string
  kind?: string
  items?: RuntimeSummaryItem[]
}

export type RuntimeSummaryItem = {
  metadata?: { name?: string }
  spec?: {
    adapter?: string
    modelFormats?: string[]
    taskKinds?: string[]
    runtimeBinary?: { source?: string; command?: string }
    managedArgs?: string[]
  }
  status?: { availableWorkers?: number; readySlots?: number }
}

export type CachePlan = {
  profileRef: string
  artifacts?: string[]
  requireTags?: string[]
  desiredCopies?: number
  actualCopies?: number
  staleCopies?: number
  evictionsPending?: number
  workers?: CacheWorkerStatus[]
  conditions?: Condition[]
}

export type CacheWorkerStatus = {
  nodeId: string
  cached?: boolean
  warm?: boolean
  active?: boolean
  eviction?: {
    status?: string
    reason?: string
    failure?: string
    updatedAt?: string
  }
}

export type Assignment = {
  assignmentId: string
  profileId: string
  logicalModel?: string
  runtimeVariantId?: string
  modelArtifactId?: string
  nodeId?: string
  slotId?: string
  desiredCached?: boolean
  desiredWarm?: boolean
  actualCached?: boolean
  actualWarm?: boolean
  ready?: boolean
  mismatchReason?: string
}

export type Policy = {
  policyId: string
  profileId: string
  cachedCount?: number
  warmCount?: number
  autoBalance?: boolean
  minCachedCount?: number
  maxCachedCount?: number
  minWarmCount?: number
  maxWarmCount?: number
  maxCachedProfilesPerNode?: number
  maxWarmProfilesPerNode?: number
  effectiveCachedCount?: number
  effectiveWarmCount?: number
  demandQueued?: number
  demandRunning?: number
  demandRecent?: number
  conditions?: Condition[]
}

export type FitResult = {
  profileId: string
  logicalModel?: string
  runtimeVariantId?: string
  slotId?: string
  nodeId?: string
  fits?: boolean
  reasons?: string[]
}

export type Task = {
  taskId: string
  userId?: string
  kind?: string
  model?: string
  profileId?: string
  profileVersion?: number
  logicalModel?: string
  runtimeVariantId?: string
  computeCost?: number
  failedSlots?: string[]
  status?: string
  failureReason?: string
  createdAt?: string
  updatedAt?: string
}

export type Attempt = {
  attemptId: string
  taskId: string
  userId?: string
  nodeId?: string
  slotId?: string
  profileId?: string
  profileVersion?: number
  runtimeAdapter?: string
  computeCost?: number
  status?: string
  phase?: string
  failureReason?: string
  canRetry?: boolean
  firstOutputSent?: boolean
  startedAt?: string
}

export type Report = {
  reportId: string
  taskId: string
  attemptId?: string
  userId?: string
  nodeId?: string
  slotId?: string
  profileId?: string
  profileVersion?: number
  computeCost?: number
  status?: string
  phase?: string
  failureReason?: string
  timing?: {
    timeToFirstTokenMs?: number
    generationMs?: number
    totalAttemptMs?: number
  }
  llm?: {
    completionTokens?: number
    tokensPerSecond?: number
    contextTokens?: number
  }
  createdAt?: string
}

export type TaskUserSummary = {
  userId: string
  total?: number
  queued?: number
  running?: number
  completed?: number
  failed?: number
  cancelled?: number
  computeCost?: number
}

export type TaskSummary = {
  total?: number
  queued?: number
  running?: number
  completed?: number
  failed?: number
  cancelled?: number
  failuresLastHour?: number
  byUser?: TaskUserSummary[]
}

export type TaskListResponse = {
  items?: Task[]
  attempts?: Attempt[]
  reports?: Report[]
  total?: number
  limit?: number
  offset?: number
  hasMore?: boolean
  summary?: TaskSummary
}

export type User = {
  userId: string
  name?: string
  computeBalance?: number
  disabled?: boolean
  createdAt?: string
}

export type APIKey = {
  apiKeyId: string
  userId: string
  name?: string
  status?: string
  createdAt?: string
  revokedAt?: string
  lastUsedAt?: string
}

export type APIKeyLookupResponse = {
  apiKey: APIKey
  user: User
}

export type ComputeLedgerEntry = {
  ledgerEntryId: string
  type?: string
  userId: string
  taskId?: string
  attemptId?: string
  reportId?: string
  nodeId?: string
  slotId?: string
  profileId?: string
  profileVersion?: number
  computeCost?: number
  amount?: number
  direction?: string
  reason?: string
  createdAt?: string
}

export type UpdateRecord = {
  updateId: string
  kind?: string
  version?: string
  artifactId?: string
  status?: string
  failure?: string
}

export type StateResponse = {
  version?: string
  nodes?: Node[]
  slots?: Slot[]
  artifacts?: Artifact[]
  artifactEvictions?: ArtifactEvictionRecord[]
  profiles?: Profile[]
  policies?: Policy[]
  assignments?: Assignment[]
  fitMatrix?: FitResult[]
  runtimeSummary?: RuntimeSummary
  cachePlans?: CachePlan[]
  tasks?: Task[]
  attempts?: Attempt[]
  reports?: Report[]
  taskSummary?: TaskSummary
  taskPageLimit?: number
  tasksTruncated?: boolean
  updates?: UpdateRecord[]
  users?: User[]
  apiKeys?: APIKey[]
  computeLedger?: ComputeLedgerEntry[]
  queue?: { limit?: number; inUse?: number; queued?: number }
}
