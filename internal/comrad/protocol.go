package comrad

import "encoding/json"

const (
	MsgHello            = "worker.hello"
	MsgFullState        = "worker.full_state"
	MsgHeartbeat        = "worker.heartbeat"
	MsgTelemetry        = "worker.telemetry"
	MsgSlotState        = "worker.slot_state"
	MsgArtifactState    = "worker.artifact_state"
	MsgAssignProfile    = "manager.assign_profile"
	MsgExecuteTask      = "manager.execute_task"
	MsgCancelTask       = "manager.cancel_task"
	MsgEvictArtifact    = "manager.evict_artifact"
	MsgUpdateWorker     = "manager.update_worker"
	MsgAck              = "control.ack"
	MsgToken            = "attempt.token"
	MsgAttemptStarted   = "attempt.started"
	MsgAttemptLease     = "attempt.lease_renewed"
	MsgAttemptFailed    = "attempt.failed"
	MsgAttemptCompleted = "attempt.completed"
	MsgComputeReport    = "attempt.compute_report"
	MsgError            = "control.error"
)

type Envelope struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	NodeID  string          `json:"nodeId,omitempty"`
	TaskID  string          `json:"taskId,omitempty"`
	Attempt string          `json:"attemptId,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type HelloPayload struct {
	NodeToken string `json:"nodeToken,omitempty"`
	Node      Node   `json:"node"`
	Slots     []Slot `json:"slots"`
}

type FullStatePayload struct {
	NodeToken    string                `json:"nodeToken,omitempty"`
	Node         Node                  `json:"node"`
	Slots        []Slot                `json:"slots"`
	Cached       []string              `json:"cachedArtifacts"`
	WarmProfiles []string              `json:"warmProfiles"`
	Assignments  []PlacementAssignment `json:"assignments,omitempty"`
	ProcessedIDs []string              `json:"processedMessageIds,omitempty"`
}

type WorkerRegistrationAck struct {
	Status    string `json:"status"`
	NodeToken string `json:"nodeToken,omitempty"`
}

type ArtifactSpec struct {
	ID        string           `json:"artifactId"`
	Kind      string           `json:"kind"`
	Name      string           `json:"name"`
	SHA256    string           `json:"sha256"`
	SizeBytes int64            `json:"sizeBytes"`
	URL       string           `json:"url"`
	Torrent   *ArtifactTorrent `json:"torrent,omitempty"`
	P2PPeers  []string         `json:"p2pPeers,omitempty"`
}

type AssignmentPayload struct {
	Profile   WorkloadProfile `json:"profile"`
	Artifacts []ArtifactSpec  `json:"artifacts"`
	SlotID    string          `json:"slotId,omitempty"`
	Cached    bool            `json:"cached"`
	Warm      bool            `json:"warm"`
}

type ExecuteTaskPayload struct {
	TaskID      string          `json:"taskId"`
	AttemptID   string          `json:"attemptId"`
	SlotID      string          `json:"slotId"`
	Profile     WorkloadProfile `json:"profile"`
	Artifacts   []ArtifactSpec  `json:"artifacts"`
	Messages    []ChatMessage   `json:"messages"`
	MaxTokens   int             `json:"maxTokens"`
	Temperature float64         `json:"temperature"`
	RequestedAt int64           `json:"requestedAt"`
}

type CancelTaskPayload struct {
	TaskID        string `json:"taskId"`
	AttemptID     string `json:"attemptId"`
	FailureReason string `json:"failureReason"`
}

type EvictArtifactPayload struct {
	ArtifactID string `json:"artifactId"`
	Reason     string `json:"reason,omitempty"`
}

type TokenPayload struct {
	TaskID       string `json:"taskId"`
	AttemptID    string `json:"attemptId"`
	Token        string `json:"token"`
	Index        int    `json:"index"`
	FinishReason string `json:"finishReason,omitempty"`
}

type SlotStatePayload struct {
	SlotID           string `json:"slotId"`
	State            string `json:"state"`
	ProfileID        string `json:"profileId,omitempty"`
	ProfileVersion   int    `json:"profileVersion,omitempty"`
	LogicalModel     string `json:"logicalModel,omitempty"`
	RuntimeVariantID string `json:"runtimeVariantId,omitempty"`
	ModelArtifactID  string `json:"modelArtifactId,omitempty"`
	ModelSHA256      string `json:"modelSha256,omitempty"`
	ActiveTaskID     string `json:"activeTaskId,omitempty"`
	MismatchReason   string `json:"mismatchReason,omitempty"`
}

type ArtifactStatePayload struct {
	ArtifactID string `json:"artifactId"`
	State      string `json:"state"`
	SHA256     string `json:"sha256"`
	Error      string `json:"error,omitempty"`
}

type AttemptFailedPayload struct {
	TaskID          string `json:"taskId"`
	AttemptID       string `json:"attemptId"`
	Phase           string `json:"phase"`
	FailureReason   string `json:"failureReason"`
	CanRetry        bool   `json:"canRetry"`
	FirstOutputSent bool   `json:"firstOutputSent"`
}

type UpdatePayload struct {
	Update   UpdateRecord `json:"update"`
	Artifact ArtifactSpec `json:"artifact"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func MarshalPayload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
