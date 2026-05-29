package comrad

import (
	"net/http"
	"strings"
	"time"
)

type artifactEvictionTarget struct {
	nodeID     string
	artifactID string
}

func (m *Manager) dispatchStaleArtifactEvictions(reason string) {
	db := m.store.Snapshot()
	for _, target := range staleArtifactEvictions(db) {
		m.dispatchArtifactEviction(target.nodeID, target.artifactID, reason)
	}
}

func staleArtifactEvictions(db Database) []artifactEvictionTarget {
	allowed := desiredArtifactsByNode(db)
	active := activeArtifactsByNode(db)
	targets := []artifactEvictionTarget{}
	for _, node := range db.Nodes {
		for _, artifactID := range node.CachedArtifacts {
			artifactID = NormalizeSHA256(artifactID)
			if artifactID == "" || allowed[node.ID][artifactID] || active[node.ID][artifactID] {
				continue
			}
			targets = append(targets, artifactEvictionTarget{nodeID: node.ID, artifactID: artifactID})
		}
	}
	return targets
}

func desiredArtifactsByNode(db Database) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, assignment := range db.Assignments {
		if assignment.NodeID == "" || !assignment.DesiredCached {
			continue
		}
		profile, ok := db.Profiles[assignment.ProfileID]
		if !ok {
			continue
		}
		addProfileArtifacts(out, assignment.NodeID, effectiveProfileForAssignment(profile, assignment))
	}
	for _, update := range db.Updates {
		for _, nodeID := range updateTargets(db, update) {
			addArtifact(out, nodeID, update.ArtifactID)
		}
	}
	return out
}

func activeArtifactsByNode(db Database) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, attempt := range db.Attempts {
		if attempt.Status != TaskStatusRunning {
			continue
		}
		if profile, ok := db.Profiles[attempt.ProfileID]; ok {
			addProfileArtifacts(out, attempt.NodeID, profile)
		}
		slot := db.Slots[attempt.SlotID]
		addArtifact(out, attempt.NodeID, slot.ModelArtifactID)
	}
	for _, slot := range db.Slots {
		if slot.State == SlotStateServing {
			addArtifact(out, slot.NodeID, slot.ModelArtifactID)
		}
	}
	return out
}

func updateTargets(db Database, update UpdateRecord) []string {
	if len(update.TargetNodes) > 0 {
		return update.TargetNodes
	}
	targets := make([]string, 0, len(db.Nodes))
	for id := range db.Nodes {
		targets = append(targets, id)
	}
	return targets
}

func addProfileArtifacts(out map[string]map[string]bool, nodeID string, profile WorkloadProfile) {
	for _, id := range profileArtifactIDs(profile) {
		addArtifact(out, nodeID, id)
	}
}

func addArtifact(out map[string]map[string]bool, nodeID, artifactID string) {
	artifactID = NormalizeSHA256(artifactID)
	if nodeID == "" || artifactID == "" {
		return
	}
	if out[nodeID] == nil {
		out[nodeID] = map[string]bool{}
	}
	out[nodeID][artifactID] = true
}

func (m *Manager) handleAdminNodeArtifactByPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	nodeID, artifactID, ok := parseNodeArtifactPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "node artifact path required")
		return
	}
	if err := m.validateManualArtifactEviction(nodeID, artifactID); err != nil {
		writeManualArtifactEvictionError(w, err)
		return
	}
	if !m.dispatchArtifactEviction(nodeID, artifactID, "admin_requested") {
		writeError(w, http.StatusConflict, "worker_offline", "worker is not connected")
		return
	}
	_ = m.store.Audit("artifact.evict_requested", "admin", artifactID, map[string]any{"nodeId": nodeID})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "eviction_queued"})
}

func parseNodeArtifactPath(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, "/api/admin/nodes/")
	parts := strings.Split(rest, "/artifacts/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], NormalizeSHA256(parts[1]), true
}

func (m *Manager) validateManualArtifactEviction(nodeID, artifactID string) error {
	db := m.store.Snapshot()
	node, ok := db.Nodes[nodeID]
	if !ok {
		return artifactDeleteError{code: "not_found", message: "node not found"}
	}
	if !containsArtifactID(node.CachedArtifacts, artifactID) {
		return artifactDeleteError{code: "not_found", message: "artifact is not cached on this worker"}
	}
	if desiredArtifactsByNode(db)[nodeID][artifactID] || activeArtifactsByNode(db)[nodeID][artifactID] {
		return artifactDeleteError{code: "artifact_in_use", message: "artifact is assigned or active on this worker"}
	}
	return nil
}

func writeManualArtifactEvictionError(w http.ResponseWriter, err error) {
	if e, ok := err.(artifactDeleteError); ok {
		status := http.StatusNotFound
		if e.code == "artifact_in_use" {
			status = http.StatusConflict
		}
		writeError(w, status, e.code, e.message)
		return
	}
	writeError(w, http.StatusInternalServerError, "evict_failed", err.Error())
}

func (m *Manager) dispatchArtifactEviction(nodeID, artifactID, reason string) bool {
	artifactID = NormalizeSHA256(artifactID)
	now := time.Now().UTC()
	m.mu.Lock()
	sess := m.sessions[nodeID]
	m.mu.Unlock()
	if sess == nil {
		m.recordArtifactEviction(nodeID, artifactID, reason, ArtifactEvictionBlocked, "worker_offline", now)
		return false
	}
	payload := EvictArtifactPayload{ArtifactID: artifactID, Reason: reason}
	msg := Envelope{ID: NewID("msg"), Type: MsgEvictArtifact, NodeID: nodeID, Payload: MarshalPayload(payload)}
	if !sess.enqueue(msg) {
		m.recordArtifactEviction(nodeID, artifactID, reason, ArtifactEvictionBlocked, "worker_send_queue_full", now)
		return false
	}
	m.recordArtifactEviction(nodeID, artifactID, reason, ArtifactEvictionQueued, "", now)
	return true
}

func containsArtifactID(items []string, artifactID string) bool {
	artifactID = NormalizeSHA256(artifactID)
	for _, item := range items {
		if NormalizeSHA256(item) == artifactID {
			return true
		}
	}
	return false
}

func removeArtifactID(items []string, artifactID string) []string {
	artifactID = NormalizeSHA256(artifactID)
	out := items[:0]
	for _, existing := range items {
		if NormalizeSHA256(existing) != artifactID {
			out = append(out, existing)
		}
	}
	return out
}
