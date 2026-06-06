package comrad

import "time"

var Version = "dev"

const (
	TargetDarwinArm64Metal = "darwin-arm64-metal"

	NodeStateExpected   = "expected"
	NodeStateRegistered = "registered"
	NodeStateOnline     = "online"
	NodeStateOffline    = "offline"
	NodeStateDisabled   = "disabled"
	NodeStateError      = "error"

	SlotStateUnavailable    = "unavailable"
	SlotStateIdle           = "idle"
	SlotStateDownloadQueued = "download_queued"
	SlotStateDownloading    = "downloading_artifact"
	SlotStateCached         = "cached"
	SlotStateLoading        = "loading_runtime"
	SlotStateWarming        = "warming"
	SlotStateReady          = "ready"
	SlotStateServing        = "serving"
	SlotStateError          = "error"

	TaskStatusQueued    = "queued"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"

	FailureNoCapacity             = "no_capacity"
	FailureUnknownRequirements    = "unknown_requirements"
	FailureTargetUnsupported      = "target_unsupported"
	FailureRuntimeAdapterMissing  = "runtime_adapter_missing"
	FailureResourceExhaustedRAM   = "resource_exhausted_ram"
	FailureResourceExhaustedVRAM  = "resource_exhausted_vram"
	FailureResourceExhaustedDisk  = "resource_exhausted_disk"
	FailureAdmissionCheckFailed   = "admission_check_failed"
	FailureArtifactDigestMismatch = "artifact_digest_mismatch"
	FailureCancelledByClient      = "cancelled_by_client"
	FailureWorkerDisconnected     = "worker_disconnected"
	FailureRuntimeError           = "runtime_error"
	FailureFirstOutputTimeout     = "first_output_timeout"
	FailureLeaseExpired           = "lease_expired"
	FailureQuarantined            = "quarantined"
	FailureWorkerFlapping         = "worker_flapping"

	APIKeyStatusActive  = "active"
	APIKeyStatusRevoked = "revoked"

	LedgerConsumeCompute  = "consume_compute"
	LedgerProduceCompute  = "produce_compute"
	LedgerAdminAdjustment = "admin_adjustment"
	LedgerPurchaseCompute = "purchase_compute"
	LedgerDebit           = "debit"
	LedgerCredit          = "credit"
)

type ResourceBudget struct {
	RAMBytes           int64 `json:"ramBytes"`
	VRAMBytes          int64 `json:"vramBytes"`
	UnifiedMemoryBytes int64 `json:"unifiedMemoryBytes"`
	DiskBytes          int64 `json:"diskBytes"`
	SlotCount          int   `json:"slotCount"`
}

type DownloadPressure struct {
	MaxConcurrent int `json:"maxConcurrent"`
	Active        int `json:"active"`
	Queued        int `json:"queued"`
}

type WorkerP2PStatus struct {
	Available              bool       `json:"available"`
	Port                   int        `json:"port,omitempty"`
	MaxUploads             int        `json:"maxUploads,omitempty"`
	DownloadTimeoutSeconds int64      `json:"downloadTimeoutSeconds,omitempty"`
	SeedingCount           int        `json:"seedingCount,omitempty"`
	DownloadingCount       int        `json:"downloadingCount,omitempty"`
	PeerCount              int        `json:"peers,omitempty"`
	FallbackCount          int        `json:"fallbackCount,omitempty"`
	LastFailure            string     `json:"lastFailure,omitempty"`
	LastFailureAt          *time.Time `json:"lastFailureAt,omitempty"`
}

type Node struct {
	ID                             string            `json:"nodeId"`
	OwnerUserID                    string            `json:"ownerUserId,omitempty"`
	Name                           string            `json:"name"`
	OS                             string            `json:"os"`
	Arch                           string            `json:"arch"`
	Target                         string            `json:"target"`
	Mode                           string            `json:"mode"`
	Tags                           []string          `json:"tags"`
	State                          string            `json:"state"`
	Version                        string            `json:"version"`
	RuntimeAdapters                []string          `json:"runtimeAdapters"`
	Budgets                        ResourceBudget    `json:"budgets"`
	DownloadPressure               DownloadPressure  `json:"downloadPressure"`
	P2P                            *WorkerP2PStatus  `json:"p2p,omitempty"`
	CachedArtifacts                []string          `json:"cachedArtifacts"`
	WarmProfiles                   []string          `json:"warmProfiles"`
	LastSeen                       time.Time         `json:"lastSeen"`
	Approved                       bool              `json:"approved"`
	UpdateRequired                 bool              `json:"updateRequired"`
	UpdateStatus                   string            `json:"updateStatus,omitempty"`
	LastFailure                    string            `json:"lastFailure,omitempty"`
	LastFailureAt                  *time.Time        `json:"lastFailureAt,omitempty"`
	Quarantined                    bool              `json:"quarantined"`
	QuarantineReason               string            `json:"quarantineReason,omitempty"`
	QuarantineUntil                *time.Time        `json:"quarantineUntil,omitempty"`
	RecentFlapEvents               []WorkerFlapEvent `json:"recentFlapEvents,omitempty"`
	WarmPlacementSuppressed        bool              `json:"warmPlacementSuppressed,omitempty"`
	WarmPlacementSuppressionReason string            `json:"warmPlacementSuppressionReason,omitempty"`
	WarmPlacementSuppressionUntil  *time.Time        `json:"warmPlacementSuppressionUntil,omitempty"`
	Conditions                     []Condition       `json:"conditions,omitempty"`
	ConnectedSession               string            `json:"-"`
}

type WorkerFlapEvent struct {
	Type string    `json:"type"`
	At   time.Time `json:"at"`
}

type Slot struct {
	ID               string         `json:"slotId"`
	NodeID           string         `json:"nodeId"`
	Target           string         `json:"target"`
	RuntimeAdapter   string         `json:"runtimeAdapter"`
	Resources        ResourceBudget `json:"resources"`
	State            string         `json:"state"`
	ProfileID        string         `json:"profileId,omitempty"`
	ProfileVersion   int            `json:"profileVersion,omitempty"`
	LogicalModel     string         `json:"logicalModel,omitempty"`
	RuntimeVariantID string         `json:"runtimeVariantId,omitempty"`
	ModelArtifactID  string         `json:"modelArtifactId,omitempty"`
	ModelSHA256      string         `json:"modelSha256,omitempty"`
	ActiveTaskID     string         `json:"activeTaskId,omitempty"`
	AcceptsNew       bool           `json:"acceptsNewTasks"`
	MismatchReason   string         `json:"mismatchReason,omitempty"`
	LastReady        time.Time      `json:"lastReady,omitempty"`
	LastTaskAt       time.Time      `json:"lastTaskAt,omitempty"`
	FailureCount     int            `json:"failureCount"`
	FailureCounters  map[string]int `json:"failureCounters,omitempty"`
	LastFailure      string         `json:"lastFailure,omitempty"`
	LastFailureAt    *time.Time     `json:"lastFailureAt,omitempty"`
	Quarantined      bool           `json:"quarantined"`
	QuarantineReason string         `json:"quarantineReason,omitempty"`
	QuarantineUntil  *time.Time     `json:"quarantineUntil,omitempty"`
	Conditions       []Condition    `json:"conditions,omitempty"`
}

type ArtifactTorrent struct {
	InfoHash      string `json:"infoHash"`
	MagnetURI     string `json:"magnetUri"`
	PieceLength   int64  `json:"pieceLength"`
	MetaInfoPath  string `json:"metainfoPath,omitempty"`
	MetaInfoBytes []byte `json:"metainfoBytes,omitempty"`
}

type Artifact struct {
	ID        string           `json:"artifactId"`
	Kind      string           `json:"kind"`
	Name      string           `json:"name"`
	Path      string           `json:"path"`
	SHA256    string           `json:"sha256"`
	SizeBytes int64            `json:"sizeBytes"`
	Torrent   *ArtifactTorrent `json:"torrent,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
}

type ArtifactEvictionRecord struct {
	ID          string      `json:"evictionId"`
	NodeID      string      `json:"nodeId"`
	ArtifactID  string      `json:"artifactId"`
	Reason      string      `json:"reason"`
	Status      string      `json:"status"`
	Failure     string      `json:"failure,omitempty"`
	RequestedAt time.Time   `json:"requestedAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	Conditions  []Condition `json:"conditions,omitempty"`
}

type CacheIntentRecord struct {
	ID          string    `json:"cacheIntentId"`
	NodeID      string    `json:"nodeId"`
	ArtifactID  string    `json:"artifactId"`
	Action      string    `json:"action"`
	RequestedAt time.Time `json:"requestedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Requirements struct {
	Target             string   `json:"target" yaml:"target"`
	RuntimeAdapter     string   `json:"runtimeAdapter,omitempty" yaml:"runtimeAdapter,omitempty"`
	RAMBytes           int64    `json:"ramBytes,omitempty" yaml:"ramBytes,omitempty"`
	VRAMBytes          int64    `json:"vramBytes,omitempty" yaml:"vramBytes,omitempty"`
	UnifiedMemoryBytes int64    `json:"unifiedMemoryBytes,omitempty" yaml:"unifiedMemoryBytes,omitempty"`
	DiskBytes          int64    `json:"diskBytes,omitempty" yaml:"diskBytes,omitempty"`
	RequireTags        []string `json:"requireTags,omitempty" yaml:"requireTags,omitempty"`
	MinimumWorker      string   `json:"minimumWorkerVersion,omitempty" yaml:"minimumWorkerVersion,omitempty"`
}

type LLMProfile struct {
	ContextTokens int `json:"contextTokens" yaml:"contextTokens"`
}

type RuntimeParameters struct {
	LlamaCpp LlamaCppParameters `json:"llamaCpp,omitempty" yaml:"llamaCpp,omitempty"`
}

type LlamaCppParameters struct {
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`
}

type RuntimeModelVariant struct {
	ID             string            `json:"variantId"`
	Name           string            `json:"name,omitempty"`
	Target         string            `json:"target"`
	RuntimeAdapter string            `json:"runtimeAdapter"`
	Artifacts      []string          `json:"artifacts"`
	Requirements   *Requirements     `json:"requirements,omitempty"`
	LLM            *LLMProfile       `json:"llm,omitempty"`
	Runtime        RuntimeParameters `json:"runtime,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type WorkloadProfile struct {
	ID                    string                `json:"profileId"`
	Version               int                   `json:"profileVersion"`
	Name                  string                `json:"name"`
	Alias                 string                `json:"alias"`
	LogicalModel          string                `json:"logicalModel,omitempty"`
	RuntimeVariantID      string                `json:"runtimeVariantId,omitempty"`
	Kind                  string                `json:"kind"`
	RuntimeAdapter        string                `json:"runtimeAdapter"`
	Artifacts             []string              `json:"artifacts"`
	Requirements          *Requirements         `json:"requirements,omitempty"`
	LLM                   *LLMProfile           `json:"llm,omitempty"`
	Runtime               RuntimeParameters     `json:"runtime,omitempty"`
	RuntimeVariants       []RuntimeModelVariant `json:"runtimeVariants,omitempty"`
	ComputeCost           int64                 `json:"computeCost"`
	Warmable              bool                  `json:"warmable"`
	MaxConcurrencyPerSlot int                   `json:"maxConcurrencyPerSlot"`
	CreatedAt             time.Time             `json:"createdAt"`
	Conditions            []Condition           `json:"conditions,omitempty"`
}

type PlacementConstraints struct {
	RequireTags []string `json:"requireTags,omitempty" yaml:"requireTags,omitempty"`
	PreferNodes []string `json:"preferNodes,omitempty" yaml:"preferNodes,omitempty"`
	DenyNodes   []string `json:"denyNodes,omitempty" yaml:"denyNodes,omitempty"`
}

type PlacementPolicy struct {
	ID                       string               `json:"policyId"`
	ProfileID                string               `json:"profileId"`
	CachedCount              int                  `json:"cachedCount"`
	WarmCount                int                  `json:"warmCount"`
	AutoBalance              bool                 `json:"autoBalance,omitempty"`
	MinCachedCount           int                  `json:"minCachedCount,omitempty"`
	MaxCachedCount           int                  `json:"maxCachedCount,omitempty"`
	MinWarmCount             int                  `json:"minWarmCount,omitempty"`
	MaxWarmCount             int                  `json:"maxWarmCount,omitempty"`
	MaxCachedProfilesPerNode int                  `json:"maxCachedProfilesPerNode,omitempty"`
	MaxWarmProfilesPerNode   int                  `json:"maxWarmProfilesPerNode,omitempty"`
	EffectiveCachedCount     int                  `json:"effectiveCachedCount,omitempty"`
	EffectiveWarmCount       int                  `json:"effectiveWarmCount,omitempty"`
	DemandQueued             int                  `json:"demandQueued,omitempty"`
	DemandRunning            int                  `json:"demandRunning,omitempty"`
	DemandRecent             int                  `json:"demandRecent,omitempty"`
	DemandSmoothed           int                  `json:"demandSmoothed,omitempty"`
	Constraints              PlacementConstraints `json:"constraints"`
	HardPinnedSlots          []string             `json:"hardPinnedSlots,omitempty"`
	CreatedAt                time.Time            `json:"createdAt"`
	UpdatedAt                time.Time            `json:"updatedAt"`
	Conditions               []Condition          `json:"conditions,omitempty"`
}

type PlacementAssignment struct {
	ID               string    `json:"assignmentId"`
	ProfileID        string    `json:"profileId"`
	LogicalModel     string    `json:"logicalModel,omitempty"`
	RuntimeVariantID string    `json:"runtimeVariantId,omitempty"`
	ModelArtifactID  string    `json:"modelArtifactId,omitempty"`
	ModelSHA256      string    `json:"modelSha256,omitempty"`
	NodeID           string    `json:"nodeId"`
	SlotID           string    `json:"slotId"`
	DesiredCached    bool      `json:"desiredCached"`
	DesiredWarm      bool      `json:"desiredWarm"`
	ActualCached     bool      `json:"actualCached"`
	ActualWarm       bool      `json:"actualWarm"`
	Ready            bool      `json:"ready"`
	Draining         bool      `json:"draining,omitempty"`
	MismatchReason   string    `json:"mismatchReason,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type FitResult struct {
	ProfileID        string   `json:"profileId"`
	LogicalModel     string   `json:"logicalModel,omitempty"`
	RuntimeVariantID string   `json:"runtimeVariantId,omitempty"`
	SlotID           string   `json:"slotId"`
	NodeID           string   `json:"nodeId"`
	Fits             bool     `json:"fits"`
	Reasons          []string `json:"reasons,omitempty"`
}

type Task struct {
	ID               string         `json:"taskId"`
	UserID           string         `json:"userId"`
	Kind             string         `json:"kind"`
	Model            string         `json:"model"`
	ProfileID        string         `json:"profileId"`
	ProfileVersion   int            `json:"profileVersion"`
	LogicalModel     string         `json:"logicalModel,omitempty"`
	RuntimeVariantID string         `json:"runtimeVariantId,omitempty"`
	ComputeCost      int64          `json:"computeCost"`
	FailedSlots      []string       `json:"failedSlots,omitempty"`
	MinContextTokens int            `json:"minContextTokens"`
	Status           string         `json:"status"`
	FailureReason    string         `json:"failureReason,omitempty"`
	Stream           bool           `json:"stream"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	CompletedAt      *time.Time     `json:"completedAt,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type Attempt struct {
	ID               string     `json:"attemptId"`
	TaskID           string     `json:"taskId"`
	UserID           string     `json:"userId"`
	NodeID           string     `json:"nodeId"`
	SlotID           string     `json:"slotId"`
	ProfileID        string     `json:"profileId"`
	ProfileVersion   int        `json:"profileVersion"`
	LogicalModel     string     `json:"logicalModel,omitempty"`
	RuntimeVariantID string     `json:"runtimeVariantId,omitempty"`
	RuntimeAdapter   string     `json:"runtimeAdapter"`
	ComputeCost      int64      `json:"computeCost"`
	Status           string     `json:"status"`
	Phase            string     `json:"phase,omitempty"`
	FailureReason    string     `json:"failureReason,omitempty"`
	CanRetry         bool       `json:"canRetry"`
	FirstOutputSent  bool       `json:"firstOutputSent"`
	LeaseExpiresAt   time.Time  `json:"leaseExpiresAt"`
	StartedAt        time.Time  `json:"startedAt"`
	FirstOutputAt    *time.Time `json:"firstOutputAt,omitempty"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
}

type TimingReport struct {
	TimeToFirstTokenMS int64 `json:"timeToFirstTokenMs"`
	GenerationMS       int64 `json:"generationMs"`
	TotalAttemptMS     int64 `json:"totalAttemptMs"`
}

type LLMMetrics struct {
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
	TokensPerSecond  float64 `json:"tokensPerSecond"`
	ContextTokens    int     `json:"contextTokens"`
}

type ResourceMetrics struct {
	PeakRAMBytes           int64 `json:"peakRamBytes"`
	PeakVRAMBytes          int64 `json:"peakVramBytes"`
	PeakUnifiedMemoryBytes int64 `json:"peakUnifiedMemoryBytes"`
	DiskCacheBytes         int64 `json:"diskCacheBytes"`
	RuntimeExitCode        int   `json:"runtimeExitCode"`
	OOMKilled              bool  `json:"oomKilled"`
}

type ComputeReport struct {
	ID               string          `json:"reportId"`
	TaskID           string          `json:"taskId"`
	AttemptID        string          `json:"attemptId"`
	UserID           string          `json:"userId"`
	NodeID           string          `json:"nodeId"`
	SlotID           string          `json:"slotId"`
	ProfileID        string          `json:"profileId"`
	ProfileVersion   int             `json:"profileVersion"`
	LogicalModel     string          `json:"logicalModel,omitempty"`
	RuntimeVariantID string          `json:"runtimeVariantId,omitempty"`
	RuntimeAdapter   string          `json:"runtimeAdapter"`
	ComputeCost      int64           `json:"computeCost"`
	Status           string          `json:"status"`
	Phase            string          `json:"phase,omitempty"`
	FailureReason    string          `json:"failureReason,omitempty"`
	CanRetry         bool            `json:"canRetry"`
	Cancelled        bool            `json:"cancelled"`
	Timing           TimingReport    `json:"timing"`
	LLM              LLMMetrics      `json:"llm"`
	Resources        ResourceMetrics `json:"resources"`
	CreatedAt        time.Time       `json:"createdAt"`
}

type UpdateRecord struct {
	ID             string    `json:"updateId"`
	Kind           string    `json:"kind"`
	Version        string    `json:"version"`
	ArtifactID     string    `json:"artifactId"`
	SHA256         string    `json:"sha256"`
	Delivery       string    `json:"delivery,omitempty"`
	DeliveryDetail string    `json:"deliveryDetail,omitempty"`
	Signature      string    `json:"signature,omitempty"`
	PublicKey      string    `json:"publicKey,omitempty"`
	TargetNodes    []string  `json:"targetNodes,omitempty"`
	Status         string    `json:"status"`
	Failure        string    `json:"failure,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

type AuditEvent struct {
	ID        string         `json:"auditId"`
	Type      string         `json:"type"`
	Actor     string         `json:"actor"`
	Subject   string         `json:"subject"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

type User struct {
	ID             string    `json:"userId"`
	Name           string    `json:"name"`
	ComputeBalance int64     `json:"computeBalance"`
	Disabled       bool      `json:"disabled,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

type APIKey struct {
	ID         string     `json:"apiKeyId"`
	UserID     string     `json:"userId"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"tokenHash"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type APIKeyView struct {
	ID         string     `json:"apiKeyId"`
	UserID     string     `json:"userId"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type ComputeLedgerEntry struct {
	ID             string    `json:"ledgerEntryId"`
	Type           string    `json:"type"`
	UserID         string    `json:"userId"`
	TaskID         string    `json:"taskId,omitempty"`
	AttemptID      string    `json:"attemptId,omitempty"`
	ReportID       string    `json:"reportId,omitempty"`
	NodeID         string    `json:"nodeId,omitempty"`
	SlotID         string    `json:"slotId,omitempty"`
	ProfileID      string    `json:"profileId,omitempty"`
	ProfileVersion int       `json:"profileVersion,omitempty"`
	ComputeCost    int64     `json:"computeCost"`
	Amount         int64     `json:"amount"`
	Direction      string    `json:"direction"`
	Reason         string    `json:"reason"`
	CreatedAt      time.Time `json:"createdAt"`
}
