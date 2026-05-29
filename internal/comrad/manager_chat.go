package comrad

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (m *Manager) collectChat(ctx context.Context, task Task, profile WorkloadProfile, messages []ChatMessage, maxTokens int, temperature float64) (string, int, error) {
	var b strings.Builder
	for {
		events, cleanup, err := m.startAttempt(ctx, task, profile, messages, maxTokens, temperature)
		if err != nil {
			if chatCancelled(err) {
				m.cancelTask(task.ID, FailureCancelledByClient)
				return "", 499, err
			}
			return "", http.StatusServiceUnavailable, err
		}
		result := m.collectAttempt(ctx, task, events, cleanup, &b)
		if result.retry {
			b.Reset()
			continue
		}
		return result.content, result.status, result.err
	}
}

type collectResult struct {
	content string
	status  int
	err     error
	retry   bool
}

func (m *Manager) collectAttempt(ctx context.Context, task Task, events <-chan streamEvent, cleanup func(), b *strings.Builder) collectResult {
	for {
		select {
		case <-ctx.Done():
			cleanup()
			m.cancelTask(task.ID, FailureCancelledByClient)
			return collectResult{status: 499, err: ctx.Err()}
		case ev := <-events:
			result := handleCollectEvent(ev, b)
			if result.status != 0 || result.retry {
				cleanup()
				return result
			}
		}
	}
}

func handleCollectEvent(ev streamEvent, b *strings.Builder) collectResult {
	switch ev.kind {
	case "token":
		b.WriteString(ev.token.Token)
	case "failed":
		return collectFailureResult(ev.failed)
	case "report":
		return collectReportResult(ev.report, b.String())
	case "error":
		return collectErrorResult(ev.err, b.Len() == 0)
	}
	return collectResult{}
}

func collectFailureResult(failed AttemptFailedPayload) collectResult {
	if !failed.FirstOutputSent && failed.CanRetry {
		return collectResult{retry: true}
	}
	return collectResult{status: http.StatusServiceUnavailable, err: errors.New(failed.FailureReason)}
}

func collectReportResult(report ComputeReport, content string) collectResult {
	if report.Status == TaskStatusCompleted {
		return collectResult{content: content, status: http.StatusOK}
	}
	if report.Status == TaskStatusFailed && report.CanRetry {
		return collectResult{retry: true}
	}
	return collectResult{status: http.StatusServiceUnavailable, err: errors.New(report.FailureReason)}
}

func collectErrorResult(err error, retry bool) collectResult {
	if retry {
		return collectResult{retry: true}
	}
	return collectResult{status: http.StatusServiceUnavailable, err: err}
}

func (m *Manager) streamChat(ctx context.Context, w http.ResponseWriter, task Task, profile WorkloadProfile, messages []ChatMessage, maxTokens int, temperature float64) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	for {
		events, cleanup, err := m.startAttempt(ctx, task, profile, messages, maxTokens, temperature)
		if err != nil {
			if chatCancelled(err) {
				m.cancelTask(task.ID, FailureCancelledByClient)
				return
			}
			writeSSE(w, "error", ErrorResponse{Error: ErrorBody{Code: errorCode(err), Message: err.Error()}})
			flushSSE(flusher)
			return
		}
		if m.streamAttempt(ctx, w, flusher, task, profile, events, cleanup) {
			continue
		}
		return
	}
}

func (m *Manager) streamAttempt(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, task Task, profile WorkloadProfile, events <-chan streamEvent, cleanup func()) bool {
	defer cleanup()
	for {
		select {
		case <-ctx.Done():
			m.cancelTask(task.ID, FailureCancelledByClient)
			return false
		case ev := <-events:
			done, retry := m.writeStreamEvent(w, flusher, task, profile, ev)
			if done || retry {
				return retry
			}
		}
	}
}

func (m *Manager) writeStreamEvent(w http.ResponseWriter, flusher http.Flusher, task Task, profile WorkloadProfile, ev streamEvent) (bool, bool) {
	switch ev.kind {
	case "token":
		writeSSE(w, "message", tokenChunk(task.ID, profile.ID, ev.token.Token))
		flushSSE(flusher)
	case "failed":
		return writeStreamFailure(w, flusher, ev.failed)
	case "report":
		return writeStreamReport(w, flusher, task.ID, profile.ID, ev.report), false
	case "error":
		if !taskHasFirstOutput(m.store.Snapshot(), task.ID) {
			return false, true
		}
		return writeStreamError(w, flusher, errorCode(ev.err), ev.err.Error()), false
	}
	return false, false
}

func tokenChunk(taskID, profileID, token string) ChatCompletionChunk {
	return ChatCompletionChunk{ID: taskID, Object: "chat.completion.chunk", Model: profileID, Choices: []ChatCompletionChoice{{Index: 0, Delta: map[string]string{"content": token}}}}
}

func writeStreamFailure(w http.ResponseWriter, flusher http.Flusher, failed AttemptFailedPayload) (bool, bool) {
	if !failed.FirstOutputSent && failed.CanRetry {
		return false, true
	}
	writeSSE(w, "error", ErrorResponse{Error: ErrorBody{Code: failed.FailureReason, Message: failed.Phase}})
	flushSSE(flusher)
	return true, false
}

func writeStreamReport(w http.ResponseWriter, flusher http.Flusher, taskID, profileID string, report ComputeReport) bool {
	if report.Status == TaskStatusCompleted {
		chunk := ChatCompletionChunk{ID: taskID, Object: "chat.completion.chunk", Model: profileID, Choices: []ChatCompletionChoice{{Index: 0, Delta: map[string]string{}, FinishReason: "stop"}}}
		writeSSE(w, "message", chunk)
		fmt.Fprint(w, "data: [DONE]\n\n")
	} else {
		writeSSE(w, "error", ErrorResponse{Error: ErrorBody{Code: report.FailureReason, Message: report.Phase}})
	}
	flushSSE(flusher)
	return true
}

func writeStreamError(w http.ResponseWriter, flusher http.Flusher, code, message string) bool {
	writeSSE(w, "error", ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
	flushSSE(flusher)
	return true
}

func flushSSE(flusher http.Flusher) {
	if flusher != nil {
		flusher.Flush()
	}
}

func (m *Manager) startAttempt(ctx context.Context, task Task, profile WorkloadProfile, messages []ChatMessage, maxTokens int, temperature float64) (<-chan streamEvent, func(), error) {
	deadline := time.Now().Add(m.cfg.StreamWait)
	for {
		if ctx.Err() != nil {
			return nil, func() {}, ctx.Err()
		}
		if taskIsCancelled(m.store.Snapshot(), task.ID) {
			return nil, func() {}, errTaskCancelled
		}
		slot, effective, sess, ok := m.selectReadySlot(profile, task.ID)
		if ok {
			events, cleanup, err := m.beginAttempt(ctx, task, profile, effective, slot, sess, messages, maxTokens, temperature)
			if errors.Is(err, errSlotUnavailable) {
				time.Sleep(25 * time.Millisecond)
				continue
			}
			if err != nil {
				return nil, func() {}, err
			}
			return events, cleanup, nil
		}
		if time.Now().After(deadline) {
			m.failTask(task.ID, FailureNoCapacity)
			return nil, func() {}, fmt.Errorf(FailureNoCapacity)
		}
		select {
		case <-ctx.Done():
			return nil, func() {}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *Manager) beginAttempt(ctx context.Context, task Task, profile, effective WorkloadProfile, slot Slot, sess *workerSession, messages []ChatMessage, maxTokens int, temperature float64) (<-chan streamEvent, func(), error) {
	now := time.Now().UTC()
	attempt := newAttempt(task, profile, effective, slot, now)
	active, ctxAttempt, cancel := newActiveAttempt(ctx, task.ID, attempt.ID, now)
	if err := m.reserveAttemptSlot(task, profile, effective, slot, attempt, now); err != nil {
		cancel()
		return nil, func() {}, err
	}
	m.registerStream(attempt.ID, active)
	if !m.dispatchAttempt(sess, task, effective, slot, attempt, messages, maxTokens, temperature) {
		m.unregisterStream(attempt.ID)
		cancel()
		m.markAttemptFailed(attempt.ID, "dispatch", FailureWorkerDisconnected, true, false)
		return nil, func() {}, fmt.Errorf(FailureWorkerDisconnected)
	}
	go m.cancelWorkerOnContext(ctx, ctxAttempt, slot.NodeID, task.ID, attempt.ID)
	return active.events, cleanupAttempt(active, cancel, m, attempt.ID), nil
}

func newAttempt(task Task, profile, effective WorkloadProfile, slot Slot, now time.Time) Attempt {
	return Attempt{ID: NewID("att"), TaskID: task.ID, UserID: task.UserID, NodeID: slot.NodeID, SlotID: slot.ID, ProfileID: profile.ID, ProfileVersion: task.ProfileVersion, LogicalModel: ProfileLogicalModel(effective), RuntimeVariantID: effective.RuntimeVariantID, RuntimeAdapter: effective.RuntimeAdapter, ComputeCost: task.ComputeCost, Status: TaskStatusRunning, Phase: "assigned", CanRetry: true, LeaseExpiresAt: now.Add(30 * time.Second), StartedAt: now}
}

func newActiveAttempt(ctx context.Context, taskID, attemptID string, now time.Time) (*activeAttempt, context.Context, context.CancelFunc) {
	events := make(chan streamEvent, 256)
	ctxAttempt, cancel := context.WithCancel(ctx)
	active := &activeAttempt{taskID: taskID, attemptID: attemptID, events: events, createdAt: now, cancelFn: cancel}
	return active, ctxAttempt, cancel
}

func (m *Manager) reserveAttemptSlot(task Task, profile, effective WorkloadProfile, slot Slot, attempt Attempt, now time.Time) error {
	return m.store.Update(func(db *Database) error {
		return reserveAttemptInDB(db, task, profile, effective, slot, attempt, now)
	})
}

func reserveAttemptInDB(db *Database, task Task, profile, effective WorkloadProfile, slot Slot, attempt Attempt, now time.Time) error {
	storedTask := db.Tasks[task.ID]
	if storedTask.Status == TaskStatusCancelled {
		return errTaskCancelled
	}
	if storedTask.Status != TaskStatusQueued {
		return errSlotUnavailable
	}
	current := db.Slots[slot.ID]
	if current.State != SlotStateReady || current.ActiveTaskID != "" || !current.AcceptsNew || current.Quarantined {
		return errSlotUnavailable
	}
	if current.ProfileID != profile.ID || current.RuntimeVariantID != effective.RuntimeVariantID {
		return errSlotUnavailable
	}
	current.State = SlotStateServing
	current.ActiveTaskID = task.ID
	current.LastTaskAt = now
	current.AcceptsNew = false
	db.Slots[current.ID] = current
	task = db.Tasks[task.ID]
	task.Status = TaskStatusRunning
	task.ProfileID = profile.ID
	task.ProfileVersion = profileVersion(profile)
	task.LogicalModel = ProfileLogicalModel(effective)
	task.RuntimeVariantID = effective.RuntimeVariantID
	task.ComputeCost = profile.ComputeCost
	task.UpdatedAt = now
	db.Tasks[task.ID] = task
	db.Attempts[attempt.ID] = attempt
	db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "attempt.assigned", Actor: "manager", Subject: attempt.ID, CreatedAt: now})
	return nil
}

func chatCancelled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errTaskCancelled)
}

func taskIsCancelled(db Database, taskID string) bool {
	return db.Tasks[taskID].Status == TaskStatusCancelled
}

func (m *Manager) registerStream(attemptID string, active *activeAttempt) {
	m.mu.Lock()
	m.streams[attemptID] = active
	m.mu.Unlock()
}

func (m *Manager) dispatchAttempt(sess *workerSession, task Task, effective WorkloadProfile, slot Slot, attempt Attempt, messages []ChatMessage, maxTokens int, temperature float64) bool {
	artifacts := m.artifactSpecs(effective, sess.baseURL)
	payload := ExecuteTaskPayload{TaskID: task.ID, AttemptID: attempt.ID, SlotID: slot.ID, Profile: effective, Artifacts: artifacts, Messages: messages, MaxTokens: maxTokens, Temperature: temperature, RequestedAt: time.Now().Unix()}
	msg := Envelope{ID: NewID("msg"), Type: MsgExecuteTask, NodeID: slot.NodeID, TaskID: task.ID, Attempt: attempt.ID, Payload: MarshalPayload(payload)}
	return sess.enqueue(msg)
}

func (m *Manager) cancelWorkerOnContext(parent context.Context, attempt context.Context, nodeID, taskID, attemptID string) {
	<-attempt.Done()
	if parent.Err() != nil {
		m.cancelAttempt(nodeID, taskID, attemptID, FailureCancelledByClient)
	}
}

func cleanupAttempt(active *activeAttempt, cancel context.CancelFunc, m *Manager, attemptID string) func() {
	return func() {
		active.cancelOnce.Do(func() {
			cancel()
			m.unregisterStream(attemptID)
		})
	}
}
