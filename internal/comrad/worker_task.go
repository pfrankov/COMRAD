package comrad

import (
	"context"
	"strings"
	"time"
)

func (w *Worker) executeTask(payload ExecuteTaskPayload) {
	start := time.Now().UTC()
	ctx, cancel, ok := w.registerActiveAttempt(payload.AttemptID)
	if !ok {
		return
	}
	defer w.unregisterActiveAttempt(payload.AttemptID)
	defer cancel()
	if failure, failed := w.admitTask(payload); failed {
		w.failExecution(payload, start, failure.phase, failure.reason, true, false, 0, nil)
		return
	}
	w.setSlotServing(payload.SlotID, payload.Profile.ID, payload.TaskID)
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgAttemptStarted, NodeID: w.node.ID, TaskID: payload.TaskID, Attempt: payload.AttemptID})
	leaseCtx, leaseCancel := context.WithCancel(ctx)
	defer leaseCancel()
	go w.renewLease(leaseCtx, payload.TaskID, payload.AttemptID)
	first := false
	var firstAt *time.Time
	sent := 0
	stats, runtimeErr := w.streamRuntimeTokens(ctx, payload, func(token string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if !first {
			now := time.Now().UTC()
			firstAt = &now
			first = true
		}
		w.sendTaskToken(payload, token, sent)
		sent++
		return nil
	})
	if runtimeErr != nil {
		go w.recoverRuntimeSlot(payload.SlotID, payload.Profile, runtimeErr.Error())
		if ctx.Err() != nil {
			w.sendReport(payload, start, "execution", FailureCancelledByClient, TaskStatusCancelled, false, first, fallbackRuntimeStats(sent, payload.Messages, firstAt))
			return
		}
		w.failExecution(payload, start, "runtime", FailureRuntimeError, !first, first, sent, firstAt)
		return
	}
	w.setSlotState(payload.SlotID, SlotStateReady, payload.Profile.ID, "")
	w.sendReport(payload, start, "completed", "", TaskStatusCompleted, false, first, completeRuntimeStats(stats, payload.Messages, firstAt))
}

type taskAdmissionFailure struct {
	phase  string
	reason string
}

func (w *Worker) registerActiveAttempt(attemptID string) (context.Context, context.CancelFunc, bool) {
	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.active[attemptID]; exists {
		cancel()
		return nil, nil, false
	}
	w.active[attemptID] = cancel
	return ctx, cancel, true
}

func (w *Worker) unregisterActiveAttempt(attemptID string) {
	w.mu.Lock()
	delete(w.active, attemptID)
	w.mu.Unlock()
}

func (w *Worker) admitTask(payload ExecuteTaskPayload) (taskAdmissionFailure, bool) {
	slot, ok := w.getSlot(payload.SlotID)
	if !ok {
		return taskAdmissionFailure{phase: "admission", reason: FailureAdmissionCheckFailed}, true
	}
	if slot.ActiveTaskID != "" || slot.State != SlotStateReady || !slotProfileCurrent(slot, payload.Profile.ID, payload.Profile) {
		return taskAdmissionFailure{phase: "admission", reason: FailureAdmissionCheckFailed}, true
	}
	_, fit := w.localFit(payload.Profile, payload.SlotID)
	if !fit.Fits {
		return taskAdmissionFailure{phase: "admission", reason: strings.Join(fit.Reasons, ",")}, true
	}
	if err := w.verifyProfileArtifacts(payload.Profile); err != nil {
		return taskAdmissionFailure{phase: "artifact_verification", reason: FailureArtifactDigestMismatch}, true
	}
	return taskAdmissionFailure{}, false
}

func (w *Worker) failExecution(payload ExecuteTaskPayload, start time.Time, phase, reason string, canRetry, first bool, completionTokens int, firstTokenAt *time.Time) {
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgAttemptFailed, NodeID: w.node.ID, TaskID: payload.TaskID, Attempt: payload.AttemptID, Payload: MarshalPayload(AttemptFailedPayload{TaskID: payload.TaskID, AttemptID: payload.AttemptID, Phase: phase, FailureReason: reason, CanRetry: canRetry, FirstOutputSent: first})})
	w.sendReport(payload, start, phase, reason, TaskStatusFailed, canRetry, first, fallbackRuntimeStats(completionTokens, payload.Messages, firstTokenAt))
}

func (w *Worker) sendTaskToken(payload ExecuteTaskPayload, token string, index int) {
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgToken, NodeID: w.node.ID, TaskID: payload.TaskID, Attempt: payload.AttemptID, Payload: MarshalPayload(TokenPayload{TaskID: payload.TaskID, AttemptID: payload.AttemptID, Token: token, Index: index})})
}

func (w *Worker) renewLease(ctx context.Context, taskID, attemptID string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.enqueue(Envelope{ID: NewID("msg"), Type: MsgAttemptLease, NodeID: w.node.ID, TaskID: taskID, Attempt: attemptID})
		}
	}
}

func (w *Worker) sendReport(payload ExecuteTaskPayload, start time.Time, phase, reason, status string, canRetry, first bool, stats runtimeStreamStats) {
	now := time.Now().UTC()
	totalMS := now.Sub(start).Milliseconds()
	generationMS := reportGenerationMS(stats, now, totalMS)
	tps := reportTokensPerSecond(stats, generationMS)
	ttftMS := reportTTFT(start, stats.FirstTokenAt)
	report := ComputeReport{
		ID:               NewID("rep"),
		TaskID:           payload.TaskID,
		AttemptID:        payload.AttemptID,
		NodeID:           w.node.ID,
		SlotID:           payload.SlotID,
		ProfileID:        payload.Profile.ID,
		LogicalModel:     ProfileLogicalModel(payload.Profile),
		RuntimeVariantID: payload.Profile.RuntimeVariantID,
		RuntimeAdapter:   payload.Profile.RuntimeAdapter,
		Status:           status,
		Phase:            phase,
		FailureReason:    reason,
		CanRetry:         canRetry,
		Cancelled:        status == TaskStatusCancelled,
		Timing: TimingReport{
			TimeToFirstTokenMS: ttftMS,
			GenerationMS:       generationMS,
			TotalAttemptMS:     totalMS,
		},
		LLM: LLMMetrics{
			PromptTokens:     stats.PromptTokens,
			CompletionTokens: stats.CompletionTokens,
			TotalTokens:      stats.PromptTokens + stats.CompletionTokens,
			TokensPerSecond:  tps,
			ContextTokens:    0,
		},
		Resources: ResourceMetrics{
			PeakRAMBytes:           w.cfg.RAMBytes,
			PeakVRAMBytes:          w.cfg.VRAMBytes,
			PeakUnifiedMemoryBytes: w.cfg.UnifiedBytes,
		},
		CreatedAt: now,
	}
	if payload.Profile.LLM != nil {
		report.LLM.ContextTokens = payload.Profile.LLM.ContextTokens
	}
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgComputeReport, NodeID: w.node.ID, TaskID: payload.TaskID, Attempt: payload.AttemptID, Payload: MarshalPayload(report)})
}

func completeRuntimeStats(stats runtimeStreamStats, messages []ChatMessage, firstTokenAt *time.Time) runtimeStreamStats {
	if firstTokenAt != nil {
		stats.FirstTokenAt = firstTokenAt
	}
	if !stats.HasPromptTokens {
		stats.PromptTokens = promptTokens(messages)
	}
	return stats
}

func fallbackRuntimeStats(completionTokens int, messages []ChatMessage, firstTokenAt *time.Time) runtimeStreamStats {
	return runtimeStreamStats{
		FirstTokenAt:     firstTokenAt,
		PromptTokens:     promptTokens(messages),
		CompletionTokens: completionTokens,
	}
}

func reportGenerationMS(stats runtimeStreamStats, now time.Time, totalMS int64) int64 {
	if stats.HasGeneration {
		return stats.GenerationMS
	}
	if stats.FirstTokenAt == nil {
		return totalMS
	}
	generationMS := now.Sub(*stats.FirstTokenAt).Milliseconds()
	if generationMS < 0 {
		return 0
	}
	return generationMS
}

func reportTokensPerSecond(stats runtimeStreamStats, generationMS int64) float64 {
	if stats.HasTokensPerSec {
		return stats.TokensPerSecond
	}
	if generationMS <= 0 {
		return 0
	}
	return float64(stats.CompletionTokens) / (float64(generationMS) / 1000.0)
}

func reportTTFT(start time.Time, firstTokenAt *time.Time) int64 {
	if firstTokenAt == nil {
		return 0
	}
	ttftMS := firstTokenAt.Sub(start).Milliseconds()
	if ttftMS < 0 {
		return 0
	}
	return ttftMS
}

func (w *Worker) cancelAttempt(attemptID string) {
	w.mu.Lock()
	cancel := w.active[attemptID]
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (w *Worker) getSlot(slotID string) (Slot, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	slot, ok := w.slots[slotID]
	return slot, ok
}

func (w *Worker) setAnySlotState(state, profileID, reason string) {
	w.mu.Lock()
	var id string
	for _, slot := range w.slots {
		id = slot.ID
		break
	}
	w.mu.Unlock()
	if id != "" {
		w.setSlotState(id, state, profileID, reason)
	}
}

func (w *Worker) setSlotServing(slotID, profileID, taskID string) {
	profile := w.warmProfileForSlot(slotID, profileID)
	w.mu.Lock()
	slot := w.slots[slotID]
	slot.State = SlotStateServing
	slot.ProfileID = profileID
	slot.ProfileVersion = profileVersion(profile)
	slot.LogicalModel = ProfileLogicalModel(profile)
	slot.RuntimeVariantID = profile.RuntimeVariantID
	slot.ModelArtifactID = ConcreteModelArtifactID(profile)
	slot.ModelSHA256 = ConcreteModelSHA256(profile)
	slot.ActiveTaskID = taskID
	slot.AcceptsNew = false
	w.slots[slotID] = slot
	w.mu.Unlock()
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgSlotState, NodeID: w.node.ID, Payload: MarshalPayload(SlotStatePayload{SlotID: slotID, State: slot.State, ProfileID: profileID, ProfileVersion: slot.ProfileVersion, LogicalModel: slot.LogicalModel, RuntimeVariantID: slot.RuntimeVariantID, ModelArtifactID: slot.ModelArtifactID, ModelSHA256: slot.ModelSHA256, ActiveTaskID: taskID})})
}

func (w *Worker) setSlotState(slotID, state, profileID, reason string) {
	profile := w.warmProfileForSlot(slotID, profileID)
	w.mu.Lock()
	slot := w.slots[slotID]
	slot.State = state
	slot.ProfileID = profileID
	slot.ProfileVersion = profileVersion(profile)
	slot.LogicalModel = ProfileLogicalModel(profile)
	slot.RuntimeVariantID = profile.RuntimeVariantID
	slot.ModelArtifactID = ConcreteModelArtifactID(profile)
	slot.ModelSHA256 = ConcreteModelSHA256(profile)
	slot.ActiveTaskID = ""
	slot.MismatchReason = reason
	slot.AcceptsNew = state == SlotStateReady
	w.slots[slotID] = slot
	w.mu.Unlock()
	reportedState := state
	if w.paused.Load() && reportedState == SlotStateReady {
		reportedState = SlotStateIdle
	}
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgSlotState, NodeID: w.node.ID, Payload: MarshalPayload(SlotStatePayload{SlotID: slotID, State: reportedState, ProfileID: profileID, ProfileVersion: slot.ProfileVersion, LogicalModel: slot.LogicalModel, RuntimeVariantID: slot.RuntimeVariantID, ModelArtifactID: slot.ModelArtifactID, ModelSHA256: slot.ModelSHA256, MismatchReason: reason})})
}

func assignmentKey(profile WorkloadProfile) string {
	if profile.RuntimeVariantID == "" {
		return profile.ID
	}
	return profile.ID + "@" + profile.RuntimeVariantID
}

func (w *Worker) warmProfileForSlot(slotID, profileID string) WorkloadProfile {
	w.mu.Lock()
	slot := w.slots[slotID]
	for _, assignment := range w.assigns {
		if assignment.Profile.ID == profileID && (slot.RuntimeVariantID == "" || assignment.Profile.RuntimeVariantID == slot.RuntimeVariantID) {
			w.mu.Unlock()
			return assignment.Profile
		}
	}
	for _, profile := range w.warm {
		if profile.ID == profileID && (slot.RuntimeVariantID == "" || profile.RuntimeVariantID == slot.RuntimeVariantID) {
			w.mu.Unlock()
			return profile
		}
	}
	w.mu.Unlock()
	return WorkloadProfile{ID: profileID, LogicalModel: profileID}
}

func (w *Worker) sendArtifactState(artifact ArtifactSpec, state, errText string) {
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgArtifactState, NodeID: w.node.ID, Payload: MarshalPayload(ArtifactStatePayload{ArtifactID: artifact.ID, State: state, SHA256: artifact.SHA256, Error: errText})})
}
