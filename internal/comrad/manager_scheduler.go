package comrad

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	errSlotUnavailable = errors.New("slot_unavailable")
	errTaskCancelled   = errors.New(FailureCancelledByClient)
)

func (m *Manager) selectReadySlot(profile WorkloadProfile, taskID string) (Slot, WorkloadProfile, *workerSession, bool) {
	m.clearExpiredQuarantines()
	m.clearExpiredWorkerSuppressions(time.Now().UTC())
	m.expireWorkerHeartbeats(time.Now().UTC())
	db := m.store.Snapshot()
	candidates := readySlotCandidates(db, profile, failedSlotsForTask(db, taskID))
	sortReadySlotCandidates(candidates)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range candidates {
		sess := m.sessions[c.slot.NodeID]
		if sess != nil {
			return c.slot, c.effective, sess, true
		}
	}
	return Slot{}, WorkloadProfile{}, nil, false
}

func failedSlotsForTask(db Database, taskID string) map[string]bool {
	failed := map[string]bool{}
	for _, slotID := range db.Tasks[taskID].FailedSlots {
		failed[slotID] = true
	}
	for _, attempt := range db.Attempts {
		if attempt.TaskID == taskID && attempt.Status == TaskStatusFailed {
			failed[attempt.SlotID] = true
		}
	}
	return failed
}

func readySlotCandidates(db Database, profile WorkloadProfile, failed map[string]bool) []placementCandidate {
	candidates := []placementCandidate{}
	for _, slot := range db.Slots {
		node := db.Nodes[slot.NodeID]
		if !slotReadyForProfile(slot, node, profile.ID, failed) {
			continue
		}
		candidates = append(candidates, variantCandidates(profile, node, slot)...)
	}
	return candidates
}

func slotReadyForProfile(slot Slot, node Node, profileID string, failed map[string]bool) bool {
	if slot.ProfileID != profileID || slot.State != SlotStateReady || slot.ActiveTaskID != "" || !slot.AcceptsNew {
		return false
	}
	if slot.Quarantined || failed[slot.ID] {
		return false
	}
	return node.State == NodeStateOnline && node.Approved && !node.Quarantined
}

func variantCandidates(profile WorkloadProfile, node Node, slot Slot) []placementCandidate {
	out := []placementCandidate{}
	for _, variant := range ProfileRuntimeVariants(profile) {
		effective := EffectiveProfileForVariant(profile, variant)
		if slot.RuntimeVariantID != "" && slot.RuntimeVariantID != effective.RuntimeVariantID {
			continue
		}
		fit := FitProfileToSlot(effective, node, slot)
		if fit.Fits {
			if slotProfileCurrent(slot, profile.ID, effective) {
				out = append(out, placementCandidate{slot: slot, node: node, effective: effective, fit: fit})
			}
		}
	}
	return out
}

func sortReadySlotCandidates(candidates []placementCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].slot.FailureCount == candidates[j].slot.FailureCount {
			return candidates[i].slot.LastTaskAt.Before(candidates[j].slot.LastTaskAt)
		}
		return candidates[i].slot.FailureCount < candidates[j].slot.FailureCount
	})
}

func (m *Manager) unregisterStream(attemptID string) {
	m.mu.Lock()
	delete(m.streams, attemptID)
	m.mu.Unlock()
}

func (m *Manager) streamForAttempt(attemptID string) *activeAttempt {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streams[attemptID]
}

func (m *Manager) failTask(taskID, reason string) {
	now := time.Now().UTC()
	_ = m.store.Update(func(db *Database) error {
		task := db.Tasks[taskID]
		if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed || task.Status == TaskStatusCancelled {
			return nil
		}
		task.Status = TaskStatusFailed
		task.FailureReason = reason
		task.UpdatedAt = now
		task.CompletedAt = &now
		db.Tasks[taskID] = task
		return nil
	})
}

func (m *Manager) markAttemptFailed(attemptID, phase, reason string, canRetry, firstOutput bool) {
	now := time.Now().UTC()
	_ = m.store.Update(func(db *Database) error {
		attempt := db.Attempts[attemptID]
		attempt.Status = TaskStatusFailed
		attempt.Phase = phase
		attempt.FailureReason = reason
		attempt.CanRetry = canRetry
		attempt.FirstOutputSent = firstOutput
		attempt.CompletedAt = &now
		db.Attempts[attemptID] = attempt
		slot := db.Slots[attempt.SlotID]
		slot.State = SlotStateReady
		slot.ActiveTaskID = ""
		recordSlotFailure(db, &slot, reason, now, m.cfg.QuarantineThreshold, m.cfg.QuarantineDuration)
		db.Slots[slot.ID] = slot
		task := db.Tasks[attempt.TaskID]
		task.FailureReason = reason
		task.UpdatedAt = now
		task.FailedSlots = appendUnique(task.FailedSlots, attempt.SlotID)
		if canRetry && !firstOutput {
			task.Status = TaskStatusQueued
			task.RuntimeVariantID = ""
			task.CompletedAt = nil
		} else {
			task.Status = TaskStatusFailed
			task.CompletedAt = &now
		}
		db.Tasks[task.ID] = task
		return nil
	})
}

func (m *Manager) cancelTask(taskID, reason string) {
	m.markTaskCancelRequested(taskID, reason)
	db := m.store.Snapshot()
	for _, attempt := range db.Attempts {
		if attempt.TaskID == taskID && attempt.Status == TaskStatusRunning {
			m.cancelAttempt(attempt.NodeID, taskID, attempt.ID, reason)
		}
	}
}

func (m *Manager) markTaskCancelRequested(taskID, reason string) {
	now := time.Now().UTC()
	_ = m.store.Update(func(db *Database) error {
		task := db.Tasks[taskID]
		if task.ID == "" || task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed || task.Status == TaskStatusCancelled {
			return nil
		}
		task.Status = TaskStatusCancelled
		task.FailureReason = reason
		task.UpdatedAt = now
		task.CompletedAt = &now
		db.Tasks[task.ID] = task
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "task.cancel_requested", Actor: "client", Subject: task.ID, CreatedAt: now})
		return nil
	})
}

func (m *Manager) cancelAttempt(nodeID, taskID, attemptID, reason string) {
	m.mu.Lock()
	sess := m.sessions[nodeID]
	m.mu.Unlock()
	if sess == nil {
		return
	}
	payload := CancelTaskPayload{TaskID: taskID, AttemptID: attemptID, FailureReason: reason}
	sess.enqueue(Envelope{ID: NewID("msg"), Type: MsgCancelTask, NodeID: nodeID, TaskID: taskID, Attempt: attemptID, Payload: MarshalPayload(payload)})
}

func (m *Manager) replanAndDispatch() {
	m.clearExpiredQuarantines()
	m.clearExpiredWorkerSuppressions(time.Now().UTC())
	db := m.store.Snapshot()
	plan := PlanPlacementWithConfig(db, m.cfg)
	_ = m.store.Update(func(db *Database) error {
		db.Assignments = map[string]PlacementAssignment{}
		for _, a := range plan {
			db.Assignments[a.ID] = a
		}
		applyPlacementDrainState(db)
		return nil
	})
	for _, a := range plan {
		if !a.DesiredCached || a.NodeID == "" || a.MismatchReason != "" {
			continue
		}
		if a.ActualCached && (!a.DesiredWarm || a.ActualWarm) {
			continue
		}
		profile := effectiveProfileForAssignment(db.Profiles[a.ProfileID], a)
		m.mu.Lock()
		sess := m.sessions[a.NodeID]
		m.mu.Unlock()
		if sess == nil {
			continue
		}
		payload := AssignmentPayload{Profile: profile, Artifacts: m.artifactSpecs(profile, sess.baseURL), SlotID: a.SlotID, Cached: a.DesiredCached, Warm: a.DesiredWarm}
		sess.enqueue(Envelope{ID: NewID("msg"), Type: MsgAssignProfile, NodeID: a.NodeID, Payload: MarshalPayload(payload)})
	}
	m.dispatchStaleArtifactEvictions("not_desired")
}

func (m *Manager) artifactSpecs(profile WorkloadProfile, baseURL string) []ArtifactSpec {
	db := m.store.Snapshot()
	p2pEnabled := db.Settings.P2PEnabled
	specs := []ArtifactSpec{}
	for _, id := range profile.Artifacts {
		artifact, ok := db.Artifacts[id]
		if !ok {
			continue
		}
		spec := ArtifactSpec{
			ID:        artifact.ID,
			Kind:      artifact.Kind,
			Name:      artifact.Name,
			SHA256:    artifact.SHA256,
			SizeBytes: artifact.SizeBytes,
			URL:       strings.TrimRight(baseURL, "/") + "/api/worker/artifacts/" + artifact.ID,
		}
		if p2pEnabled {
			spec.Torrent = cloneArtifactTorrent(artifact.Torrent)
			spec.P2PPeers = m.peersForArtifact(db, id)
		}
		specs = append(specs, spec)
	}
	return specs
}

func (m *Manager) peersForArtifact(db Database, artifactID string) []string {
	var peers []string
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, node := range db.Nodes {
		if node.State != NodeStateOnline || node.P2P == nil || !node.P2P.Available || node.P2P.Port <= 0 {
			continue
		}
		if !Contains(node.CachedArtifacts, artifactID) {
			continue
		}
		sess := m.sessions[node.ID]
		if sess == nil || sess.remoteHost == "" {
			continue
		}
		peers = append(peers, fmt.Sprintf("%s:%d", sess.remoteHost, node.P2P.Port))
	}
	return peers
}
