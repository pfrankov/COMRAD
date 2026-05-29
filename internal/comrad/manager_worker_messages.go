package comrad

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (m *Manager) handleWorkerEnvelope(s *workerSession, msg Envelope) error {
	if err := validateWorkerEnvelopeSession(s, msg); err != nil {
		return err
	}
	handlers := workerMessageHandlers()
	if handler, ok := handlers[msg.Type]; ok {
		return handler(m, s, msg)
	}
	return fmt.Errorf("unknown message type %s", msg.Type)
}

type workerMessageHandler func(*Manager, *workerSession, Envelope) error

func workerMessageHandlers() map[string]workerMessageHandler {
	return map[string]workerMessageHandler{
		MsgHello:          handleWorkerHello,
		MsgFullState:      handleWorkerFullState,
		MsgHeartbeat:      handleWorkerHeartbeat,
		MsgSlotState:      handleWorkerSlotState,
		MsgArtifactState:  handleWorkerArtifactState,
		MsgToken:          handleWorkerToken,
		MsgAttemptStarted: handleWorkerAttemptStarted,
		MsgAttemptLease:   handleWorkerAttemptLease,
		MsgAttemptFailed:  handleWorkerAttemptFailed,
		MsgComputeReport:  handleWorkerComputeReport,
		MsgAck:            handleWorkerAck,
		MsgTelemetry:      handleWorkerTelemetry,
	}
}

func handleWorkerHello(m *Manager, s *workerSession, msg Envelope) error {
	var payload HelloPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	return m.upsertWorkerState(s, payload.Node, payload.Slots, nil, nil, payload.NodeToken)
}

func handleWorkerFullState(m *Manager, s *workerSession, msg Envelope) error {
	var payload FullStatePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	return m.upsertWorkerState(s, payload.Node, payload.Slots, payload.Cached, payload.WarmProfiles, payload.NodeToken)
}

func handleWorkerHeartbeat(m *Manager, s *workerSession, msg Envelope) error {
	if s.nodeID == "" {
		return nil
	}
	return m.store.Update(func(db *Database) error {
		node := db.Nodes[s.nodeID]
		node.LastSeen = time.Now().UTC()
		node.State = NodeStateOnline
		db.Nodes[node.ID] = node
		return nil
	})
}

func handleWorkerSlotState(m *Manager, s *workerSession, msg Envelope) error {
	var payload SlotStatePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	err := m.store.Update(func(db *Database) error { return updateSlotState(db, s.nodeID, payload) })
	if err == nil {
		m.replanAndDispatch()
	}
	return err
}

func updateSlotState(db *Database, nodeID string, payload SlotStatePayload) error {
	slot := db.Slots[payload.SlotID]
	if slot.ID == "" {
		return fmt.Errorf("slot %s not found", payload.SlotID)
	}
	if slot.NodeID != nodeID {
		return fmt.Errorf("slot %s does not belong to worker %s", payload.SlotID, nodeID)
	}
	applySlotStatePayload(&slot, payload)
	db.Slots[slot.ID] = slot
	return nil
}

func applySlotStatePayload(slot *Slot, payload SlotStatePayload) {
	slot.State = payload.State
	slot.ProfileID = payload.ProfileID
	applySlotModelPayload(slot, payload)
	slot.ActiveTaskID = payload.ActiveTaskID
	slot.MismatchReason = payload.MismatchReason
	slot.AcceptsNew = payload.State == SlotStateReady && !slot.Quarantined
	if slot.Quarantined {
		slot.MismatchReason = FailureQuarantined + ": " + slot.QuarantineReason
	}
	if payload.State == SlotStateReady {
		slot.LastReady = time.Now().UTC()
	}
}

func applySlotModelPayload(slot *Slot, payload SlotStatePayload) {
	if payload.ProfileVersion > 0 {
		slot.ProfileVersion = payload.ProfileVersion
	}
	if payload.LogicalModel != "" {
		slot.LogicalModel = payload.LogicalModel
	}
	if payload.RuntimeVariantID != "" {
		slot.RuntimeVariantID = payload.RuntimeVariantID
	}
	if payload.ModelArtifactID != "" {
		slot.ModelArtifactID = payload.ModelArtifactID
	}
	if payload.ModelSHA256 != "" {
		slot.ModelSHA256 = payload.ModelSHA256
	}
}

func handleWorkerArtifactState(m *Manager, s *workerSession, msg Envelope) error {
	var payload ArtifactStatePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	return m.updateArtifactState(s.nodeID, payload)
}

func handleWorkerToken(m *Manager, s *workerSession, msg Envelope) error {
	var payload TokenPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	return m.handleToken(s.nodeID, payload)
}

func handleWorkerAttemptStarted(m *Manager, s *workerSession, msg Envelope) error {
	return m.store.Update(func(db *Database) error {
		if _, err := requireWorkerAttempt(*db, s.nodeID, msg.Attempt, msg.TaskID); err != nil {
			return err
		}
		attempt := db.Attempts[msg.Attempt]
		attempt.Phase = "running"
		attempt.Status = TaskStatusRunning
		db.Attempts[attempt.ID] = attempt
		return nil
	})
}

func handleWorkerAttemptLease(m *Manager, s *workerSession, msg Envelope) error {
	return m.store.Update(func(db *Database) error {
		if _, err := requireWorkerAttempt(*db, s.nodeID, msg.Attempt, msg.TaskID); err != nil {
			return err
		}
		attempt := db.Attempts[msg.Attempt]
		attempt.LeaseExpiresAt = time.Now().UTC().Add(30 * time.Second)
		db.Attempts[attempt.ID] = attempt
		return nil
	})
}

func handleWorkerAttemptFailed(m *Manager, s *workerSession, msg Envelope) error {
	var payload AttemptFailedPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	return m.handleAttemptFailed(s.nodeID, payload)
}

func handleWorkerComputeReport(m *Manager, s *workerSession, msg Envelope) error {
	var report ComputeReport
	if err := json.Unmarshal(msg.Payload, &report); err != nil {
		return err
	}
	return m.handleComputeReport(s.nodeID, report)
}

func handleWorkerAck(m *Manager, s *workerSession, msg Envelope) error {
	return nil
}

func handleWorkerTelemetry(m *Manager, s *workerSession, msg Envelope) error {
	return m.store.Audit("worker.telemetry", "worker", s.nodeID, map[string]any{"messageId": msg.ID})
}

func (m *Manager) upsertWorkerState(s *workerSession, node Node, slots []Slot, cached []string, warm []string, nodeToken string) error {
	now := time.Now().UTC()
	node = normalizeWorkerNode(node, s.id, cached, warm, now)
	if s.nodeID != "" && s.nodeID != node.ID {
		return fmt.Errorf("worker session already registered as %s", s.nodeID)
	}
	var issuedNodeToken string
	err := m.store.Update(func(db *Database) error {
		token, err := authorizeWorkerNode(db, s, node.ID, nodeToken)
		if err != nil {
			return err
		}
		issuedNodeToken = token
		node = mergeExistingNode(node, db.Nodes[node.ID], m.cfg.AutoApprove, now)
		db.Nodes[node.ID] = node
		upsertWorkerSlots(db, node, slots, now)
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "worker.connected", Actor: "worker", Subject: node.ID, CreatedAt: now})
		return nil
	})
	if err != nil {
		return err
	}
	s.nodeID = node.ID
	var old *workerSession
	m.mu.Lock()
	old = m.sessions[node.ID]
	m.sessions[node.ID] = s
	m.mu.Unlock()
	if old != nil && old != s {
		old.close()
	}
	ack := WorkerRegistrationAck{Status: "registered", NodeToken: issuedNodeToken}
	s.enqueue(Envelope{ID: NewID("msg"), Type: MsgAck, NodeID: node.ID, Payload: MarshalPayload(ack)})
	m.replanAndDispatch()
	return nil
}

func (m *Manager) updateArtifactState(nodeID string, payload ArtifactStatePayload) error {
	err := m.store.Update(func(db *Database) error {
		now := time.Now().UTC()
		node := db.Nodes[nodeID]
		if payload.State == "verified" && !Contains(node.CachedArtifacts, payload.ArtifactID) {
			node.CachedArtifacts = append(node.CachedArtifacts, payload.ArtifactID)
		}
		if payload.State == "evicted" {
			node.CachedArtifacts = removeArtifactID(node.CachedArtifacts, payload.ArtifactID)
		}
		if status := evictionStatusForArtifactState(payload.State); status != "" {
			upsertArtifactEvictionRecord(db, nodeID, payload.ArtifactID, "", status, payload.Error, now)
		}
		db.Nodes[node.ID] = node
		db.Audit = append(db.Audit, AuditEvent{
			ID:        NewID("aud"),
			Type:      "worker.artifact_" + payload.State,
			Actor:     "worker",
			Subject:   payload.ArtifactID,
			Metadata:  map[string]any{"nodeId": nodeID, "error": payload.Error},
			CreatedAt: now,
		})
		return nil
	})
	if err == nil {
		m.replanAndDispatch()
	}
	return err
}

func (m *Manager) handleToken(nodeID string, payload TokenPayload) error {
	active := m.streamForAttempt(payload.AttemptID)
	if active == nil {
		return m.validateTokenAttempt(nodeID, payload)
	}
	if active.taskID != payload.TaskID {
		return fmt.Errorf("attempt %s does not belong to task %s", payload.AttemptID, payload.TaskID)
	}
	if active.firstOutput {
		active.events <- streamEvent{kind: "token", token: payload}
		return nil
	}
	if err := m.recordFirstOutput(nodeID, payload); err != nil {
		return err
	}
	active.firstOutput = true
	active.events <- streamEvent{kind: "token", token: payload}
	return nil
}

func (m *Manager) validateTokenAttempt(nodeID string, payload TokenPayload) error {
	return m.store.View(func(db Database) error {
		_, err := requireWorkerAttempt(db, nodeID, payload.AttemptID, payload.TaskID)
		return err
	})
}

func (m *Manager) recordFirstOutput(nodeID string, payload TokenPayload) error {
	now := time.Now().UTC()
	return m.store.Update(func(db *Database) error {
		attempt, err := requireWorkerAttempt(*db, nodeID, payload.AttemptID, payload.TaskID)
		if err != nil {
			return err
		}
		payload.TaskID = attempt.TaskID
		attempt = db.Attempts[payload.AttemptID]
		if !attempt.FirstOutputSent {
			attempt.FirstOutputSent = true
			attempt.FirstOutputAt = &now
			attempt.CanRetry = false
			db.Attempts[attempt.ID] = attempt
			db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "attempt.first_output", Actor: "worker", Subject: attempt.ID, CreatedAt: now})
		}
		return nil
	})
}

func (m *Manager) handleAttemptFailed(nodeID string, payload AttemptFailedPayload) error {
	now := time.Now().UTC()
	if err := m.store.Update(func(db *Database) error {
		authoritative, err := requireWorkerAttempt(*db, nodeID, payload.AttemptID, payload.TaskID)
		if err != nil {
			return err
		}
		payload.TaskID = authoritative.TaskID
		attempt := db.Attempts[payload.AttemptID]
		attempt.Status = TaskStatusFailed
		attempt.Phase = payload.Phase
		attempt.FailureReason = payload.FailureReason
		attempt.CanRetry = payload.CanRetry
		attempt.FirstOutputSent = payload.FirstOutputSent
		attempt.CompletedAt = &now
		db.Attempts[attempt.ID] = attempt
		slot := db.Slots[attempt.SlotID]
		slot.ActiveTaskID = ""
		if slot.ProfileID == attempt.ProfileID {
			slot.State = SlotStateReady
			slot.AcceptsNew = true
		} else {
			slot.State = SlotStateIdle
		}
		recordSlotFailure(db, &slot, payload.FailureReason, now, m.cfg.QuarantineThreshold, m.cfg.QuarantineDuration)
		db.Slots[slot.ID] = slot
		task := db.Tasks[payload.TaskID]
		task.FailureReason = payload.FailureReason
		task.UpdatedAt = now
		task.FailedSlots = appendUnique(task.FailedSlots, attempt.SlotID)
		if payload.CanRetry && !payload.FirstOutputSent {
			task.Status = TaskStatusQueued
			task.RuntimeVariantID = ""
			task.CompletedAt = nil
		} else {
			task.Status = TaskStatusFailed
			task.CompletedAt = &now
		}
		db.Tasks[task.ID] = task
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "attempt.failed", Actor: "worker", Subject: attempt.ID, CreatedAt: now})
		return nil
	}); err != nil {
		return err
	}
	if active := m.streamForAttempt(payload.AttemptID); active != nil {
		active.events <- streamEvent{kind: "failed", failed: payload}
	}
	return nil
}

func (m *Manager) handleComputeReport(nodeID string, report ComputeReport) error {
	now := time.Now().UTC()
	if report.ID == "" {
		report.ID = NewID("rep")
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = now
	}
	if err := m.store.Update(func(db *Database) error {
		attempt, err := requireWorkerAttempt(*db, nodeID, report.AttemptID, report.TaskID)
		if err != nil {
			return err
		}
		task := db.Tasks[attempt.TaskID]
		applyAuthoritativeReportFields(&report, attempt, task)
		if _, exists := db.Reports[report.ID]; exists {
			return nil
		}
		applyReportAccounting(db, &report, now)
		db.Reports[report.ID] = report
		alreadyFailed := attempt.Status == TaskStatusFailed
		attempt.Status = report.Status
		attempt.Phase = report.Phase
		attempt.FailureReason = report.FailureReason
		attempt.CanRetry = report.CanRetry
		attempt.CompletedAt = &now
		db.Attempts[attempt.ID] = attempt
		task = db.Tasks[report.TaskID]
		task.FailureReason = report.FailureReason
		task.UpdatedAt = now
		if report.Status == TaskStatusFailed && report.CanRetry && !attempt.FirstOutputSent {
			task.Status = TaskStatusQueued
			task.RuntimeVariantID = ""
			task.CompletedAt = nil
			task.FailedSlots = appendUnique(task.FailedSlots, report.SlotID)
		} else {
			task.Status = report.Status
			task.CompletedAt = &now
		}
		db.Tasks[task.ID] = task
		slot := db.Slots[report.SlotID]
		slot.ActiveTaskID = ""
		if report.Status == TaskStatusCompleted && slot.ProfileID == report.ProfileID {
			slot.State = SlotStateReady
			slot.AcceptsNew = true
			slot.MismatchReason = ""
		} else if slot.ProfileID == report.ProfileID {
			slot.State = SlotStateReady
			slot.AcceptsNew = true
			slot.MismatchReason = report.FailureReason
		} else {
			slot.State = SlotStateIdle
			slot.AcceptsNew = false
		}
		if report.Status == TaskStatusFailed && !alreadyFailed {
			recordSlotFailure(db, &slot, report.FailureReason, now, m.cfg.QuarantineThreshold, m.cfg.QuarantineDuration)
		}
		db.Slots[slot.ID] = slot
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "attempt.compute_report", Actor: "worker", Subject: report.AttemptID, CreatedAt: now})
		return nil
	}); err != nil {
		return err
	}
	if active := m.streamForAttempt(report.AttemptID); active != nil {
		active.events <- streamEvent{kind: "report", report: report}
	}
	m.replanAndDispatch()
	return nil
}

func (m *Manager) dispatchUpdate(update UpdateRecord) {
	db := m.store.Snapshot()
	targets := update.TargetNodes
	if len(targets) == 0 {
		for id := range db.Nodes {
			targets = append(targets, id)
		}
	}
	for _, nodeID := range targets {
		m.mu.Lock()
		sess := m.sessions[nodeID]
		m.mu.Unlock()
		if sess == nil {
			continue
		}
		payload := UpdatePayload{Update: update}
		if art, ok := db.Artifacts[update.ArtifactID]; ok {
			payload.URL = strings.TrimRight(sess.baseURL, "/") + "/api/worker/artifacts/" + art.ID
		}
		sess.enqueue(Envelope{ID: NewID("msg"), Type: MsgUpdateWorker, NodeID: nodeID, Payload: MarshalPayload(payload)})
	}
}
