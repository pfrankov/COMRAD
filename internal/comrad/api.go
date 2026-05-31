package comrad

import "time"

type StateResponse struct {
	Version           string                   `json:"version"`
	SchemaVersion     int                      `json:"schemaVersion"`
	Nodes             []Node                   `json:"nodes"`
	Slots             []Slot                   `json:"slots"`
	Artifacts         []Artifact               `json:"artifacts"`
	ArtifactEvictions []ArtifactEvictionRecord `json:"artifactEvictions"`
	Profiles          []WorkloadProfile        `json:"profiles"`
	Policies          []PlacementPolicy        `json:"policies"`
	Assignments       []PlacementAssignment    `json:"assignments"`
	FitMatrix         []FitResult              `json:"fitMatrix"`
	RuntimeSummary    RuntimeSummary           `json:"runtimeSummary"`
	CachePlans        []CachePlan              `json:"cachePlans"`
	Tasks             []Task                   `json:"tasks"`
	Attempts          []Attempt                `json:"attempts"`
	Reports           []ComputeReport          `json:"reports"`
	TaskSummary       TaskSummary              `json:"taskSummary"`
	TaskPageLimit     int                      `json:"taskPageLimit"`
	TasksTruncated    bool                     `json:"tasksTruncated"`
	Updates           []UpdateRecord           `json:"updates"`
	Users             []User                   `json:"users"`
	APIKeys           []APIKeyView             `json:"apiKeys"`
	ComputeLedger     []ComputeLedgerEntry     `json:"computeLedger"`
	Queue             QueueState               `json:"queue"`
	AuditTail         []AuditEvent             `json:"auditTail"`
}

type TaskSummary struct {
	Total            int               `json:"total"`
	Queued           int               `json:"queued"`
	Running          int               `json:"running"`
	Completed        int               `json:"completed"`
	Failed           int               `json:"failed"`
	Cancelled        int               `json:"cancelled"`
	FailuresLastHour int               `json:"failuresLastHour"`
	ByUser           []TaskUserSummary `json:"byUser,omitempty"`
}

type TaskUserSummary struct {
	UserID      string `json:"userId"`
	Total       int    `json:"total"`
	Queued      int    `json:"queued"`
	Running     int    `json:"running"`
	Completed   int    `json:"completed"`
	Failed      int    `json:"failed"`
	Cancelled   int    `json:"cancelled"`
	ComputeCost int64  `json:"computeCost"`
}

type TaskListResponse struct {
	Items    []Task          `json:"items"`
	Attempts []Attempt       `json:"attempts"`
	Reports  []ComputeReport `json:"reports"`
	Total    int             `json:"total"`
	Limit    int             `json:"limit"`
	Offset   int             `json:"offset"`
	HasMore  bool            `json:"hasMore"`
	Summary  TaskSummary     `json:"summary"`
}

type QueueState struct {
	Limit  int `json:"limit"`
	InUse  int `json:"inUse"`
	Queued int `json:"queued"`
}

type AdminStateWSTicketResponse struct {
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type CreateArtifactRequest struct {
	Kind   string `json:"kind" yaml:"kind"`
	Name   string `json:"name" yaml:"name"`
	Path   string `json:"path" yaml:"path"`
	SHA256 string `json:"sha256" yaml:"sha256"`
}

type CreateProfileRequest struct {
	ID                    string                `json:"profileId" yaml:"profileId"`
	ComputeCost           int64                 `json:"computeCost" yaml:"computeCost"`
	Name                  string                `json:"name" yaml:"name"`
	Alias                 string                `json:"alias" yaml:"alias"`
	LogicalModel          string                `json:"logicalModel" yaml:"logicalModel"`
	Kind                  string                `json:"kind" yaml:"kind"`
	RuntimeAdapter        string                `json:"runtimeAdapter" yaml:"runtimeAdapter"`
	Artifacts             []string              `json:"artifacts" yaml:"artifacts"`
	Requirements          *Requirements         `json:"requirements" yaml:"requirements"`
	LLM                   *LLMProfile           `json:"llm" yaml:"llm"`
	Runtime               RuntimeParameters     `json:"runtime" yaml:"runtime"`
	RuntimeVariants       []RuntimeModelVariant `json:"runtimeVariants" yaml:"runtimeVariants"`
	Warmable              bool                  `json:"warmable" yaml:"warmable"`
	MaxConcurrencyPerSlot int                   `json:"maxConcurrencyPerSlot" yaml:"maxConcurrencyPerSlot"`
}

type SetProfileComputeCostRequest struct {
	ProfileID   string `json:"profileId" yaml:"profileId"`
	ComputeCost int64  `json:"computeCost" yaml:"computeCost"`
}

type CreateUserRequest struct {
	ID   string `json:"userId" yaml:"userId"`
	Name string `json:"name" yaml:"name"`
}

type UpdateUserRequest struct {
	ID       string `json:"userId" yaml:"userId"`
	Name     string `json:"name" yaml:"name"`
	Disabled bool   `json:"disabled" yaml:"disabled"`
}

type IssueAPIKeyRequest struct {
	UserID string `json:"userId" yaml:"userId"`
	Name   string `json:"name" yaml:"name"`
}

type IssueAPIKeyResponse struct {
	APIKey APIKeyView `json:"apiKey"`
	Token  string     `json:"token"`
}

type APIKeyLookupRequest struct {
	Token string `json:"token" yaml:"token"`
}

type APIKeyLookupResponse struct {
	APIKey APIKeyView `json:"apiKey"`
	User   User       `json:"user"`
}

type RevokeAPIKeyRequest struct {
	ID string `json:"apiKeyId" yaml:"apiKeyId"`
}

type AdminBalanceAdjustmentRequest struct {
	UserID string `json:"userId" yaml:"userId"`
	Amount int64  `json:"amount" yaml:"amount"`
	Reason string `json:"reason" yaml:"reason"`
}

type WorkerJoinResponse struct {
	ManagerURL     string `json:"managerUrl"`
	WorkerToken    string `json:"workerToken"`
	InstallCommand string `json:"installCommand"`
}

type UpsertPolicyRequest struct {
	ID                       string               `json:"policyId" yaml:"policyId"`
	ProfileID                string               `json:"profileId" yaml:"profileId"`
	CachedCount              int                  `json:"cachedCount" yaml:"cachedCount"`
	WarmCount                int                  `json:"warmCount" yaml:"warmCount"`
	AutoBalance              bool                 `json:"autoBalance" yaml:"autoBalance"`
	MinCachedCount           int                  `json:"minCachedCount" yaml:"minCachedCount"`
	MaxCachedCount           int                  `json:"maxCachedCount" yaml:"maxCachedCount"`
	MinWarmCount             int                  `json:"minWarmCount" yaml:"minWarmCount"`
	MaxWarmCount             int                  `json:"maxWarmCount" yaml:"maxWarmCount"`
	MaxCachedProfilesPerNode int                  `json:"maxCachedProfilesPerNode" yaml:"maxCachedProfilesPerNode"`
	MaxWarmProfilesPerNode   int                  `json:"maxWarmProfilesPerNode" yaml:"maxWarmProfilesPerNode"`
	Constraints              PlacementConstraints `json:"constraints" yaml:"constraints"`
	HardPinnedSlots          []string             `json:"hardPinnedSlots" yaml:"hardPinnedSlots"`
}

type UpdateNodeRequest struct {
	ID          string   `json:"nodeId" yaml:"nodeId"`
	OwnerUserID string   `json:"ownerUserId,omitempty" yaml:"ownerUserId,omitempty"`
	Approved    *bool    `json:"approved,omitempty" yaml:"approved,omitempty"`
	Mode        string   `json:"mode,omitempty" yaml:"mode,omitempty"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	State       string   `json:"state,omitempty" yaml:"state,omitempty"`
}

type UnbanRequest struct {
	NodeID string `json:"nodeId,omitempty" yaml:"nodeId,omitempty"`
	SlotID string `json:"slotId,omitempty" yaml:"slotId,omitempty"`
}

type ApplyWorkerUpdateRequest struct {
	Kind        string   `json:"kind" yaml:"kind"`
	Version     string   `json:"version" yaml:"version"`
	ArtifactID  string   `json:"artifactId" yaml:"artifactId"`
	SHA256      string   `json:"sha256" yaml:"sha256"`
	Signature   string   `json:"signature" yaml:"signature"`
	PublicKey   string   `json:"publicKey" yaml:"publicKey"`
	TargetNodes []string `json:"targetNodes" yaml:"targetNodes"`
}

type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	MinContext  int           `json:"min_context_tokens"`
	Comrad      struct {
		MinContextTokens int `json:"minContextTokens"`
	} `json:"comrad"`
}

type ChatCompletionChunk struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
}

type ChatCompletionChoice struct {
	Index        int               `json:"index"`
	Delta        map[string]string `json:"delta,omitempty"`
	Message      *ChatMessage      `json:"message,omitempty"`
	FinishReason string            `json:"finish_reason,omitempty"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
