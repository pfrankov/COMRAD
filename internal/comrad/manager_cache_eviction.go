package comrad

import (
	"net/http"
	"strings"
	"time"
)

const (
	CacheIntentKeep          = "keep"
	CacheIntentEvictWhenIdle = "evict_when_idle"
	cacheActionEvict         = "evict"
)

type artifactEvictionTarget struct {
	nodeID     string
	artifactID string
	reason     string
}

func (m *Manager) dispatchStaleArtifactEvictions(reason string) {
	db := m.store.Snapshot()
	for _, target := range staleArtifactEvictions(db) {
		m.dispatchArtifactEviction(target.nodeID, target.artifactID, firstNonEmpty(target.reason, reason))
	}
}

func staleArtifactEvictions(db Database) []artifactEvictionTarget {
	allowed := desiredArtifactsByNode(db)
	busy := evictionBusyArtifactsByNode(db)
	targets := []artifactEvictionTarget{}
	for _, node := range db.Nodes {
		for _, artifactID := range node.CachedArtifacts {
			artifactID = NormalizeSHA256(artifactID)
			intent := cacheIntentRecord(db, node.ID, artifactID)
			if artifactID == "" || allowed[node.ID][artifactID] || busy[node.ID][artifactID] || intent.Action == CacheIntentKeep {
				continue
			}
			targets = append(targets, artifactEvictionTarget{nodeID: node.ID, artifactID: artifactID, reason: staleEvictionReason(intent)})
		}
	}
	return targets
}

func staleEvictionReason(intent CacheIntentRecord) string {
	if intent.Action == CacheIntentEvictWhenIdle {
		return "admin_requested_idle"
	}
	return ""
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

func evictionBusyArtifactsByNode(db Database) map[string]map[string]bool {
	out := activeArtifactsByNode(db)
	for _, slot := range db.Slots {
		if !slotBusyForEviction(slot) {
			continue
		}
		addArtifact(out, slot.NodeID, slot.ModelArtifactID)
		if profile, ok := db.Profiles[slot.ProfileID]; ok {
			addProfileArtifacts(out, slot.NodeID, profile)
		}
	}
	return out
}

func slotBusyForEviction(slot Slot) bool {
	if slot.ActiveTaskID != "" {
		return true
	}
	switch slot.State {
	case SlotStateDownloading, SlotStateLoading, SlotStateWarming, SlotStateServing:
		return true
	default:
		return false
	}
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
	if r.Method == http.MethodPost {
		m.handleAdminNodeArtifactAction(w, r)
		return
	}
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

func (m *Manager) handleAdminNodeArtifactAction(w http.ResponseWriter, r *http.Request) {
	nodeID, artifactID, ok := parseNodeArtifactPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "node artifact path required")
		return
	}
	var req CacheArtifactActionRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	switch normalizeCacheArtifactAction(req.Action) {
	case CacheIntentKeep:
		m.handleKeepCacheAction(w, nodeID, artifactID)
	case cacheActionEvict:
		m.handleEvictCacheAction(w, nodeID, artifactID)
	case CacheIntentEvictWhenIdle:
		m.handleEvictWhenIdleCacheAction(w, nodeID, artifactID)
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "action must be keep, pin, evict, or evict_when_idle")
	}
}

func (m *Manager) handleKeepCacheAction(w http.ResponseWriter, nodeID, artifactID string) {
	if err := m.recordCacheIntent(nodeID, artifactID, CacheIntentKeep); err != nil {
		writeManualArtifactEvictionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cache_kept"})
}

func (m *Manager) handleEvictCacheAction(w http.ResponseWriter, nodeID, artifactID string) {
	if err := m.validateManualArtifactEviction(nodeID, artifactID); err != nil {
		writeManualArtifactEvictionError(w, err)
		return
	}
	if err := m.recordCacheIntent(nodeID, artifactID, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if !m.dispatchArtifactEviction(nodeID, artifactID, "admin_requested") {
		writeError(w, http.StatusConflict, "worker_offline", "worker is not connected")
		return
	}
	_ = m.store.Audit("artifact.evict_requested", "admin", artifactID, map[string]any{"nodeId": nodeID})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "eviction_queued"})
}

func (m *Manager) handleEvictWhenIdleCacheAction(w http.ResponseWriter, nodeID, artifactID string) {
	if err := m.recordCacheIntent(nodeID, artifactID, CacheIntentEvictWhenIdle); err != nil {
		writeManualArtifactEvictionError(w, err)
		return
	}
	if err := m.validateManualArtifactEviction(nodeID, artifactID); err != nil {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "eviction_deferred"})
		return
	}
	if !m.dispatchArtifactEviction(nodeID, artifactID, "admin_requested_idle") {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "eviction_deferred"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "eviction_queued"})
}

func normalizeCacheArtifactAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case CacheIntentKeep, "pin":
		return CacheIntentKeep
	case cacheActionEvict:
		return cacheActionEvict
	case CacheIntentEvictWhenIdle, "evictwhenidle":
		return CacheIntentEvictWhenIdle
	default:
		return ""
	}
}

func (m *Manager) recordCacheIntent(nodeID, artifactID, action string) error {
	artifactID = NormalizeSHA256(artifactID)
	now := time.Now().UTC()
	return m.store.Update(func(db *Database) error {
		node, ok := db.Nodes[nodeID]
		if !ok {
			return artifactDeleteError{code: "not_found", message: "node not found"}
		}
		if !containsArtifactID(node.CachedArtifacts, artifactID) {
			return artifactDeleteError{code: "not_found", message: "artifact is not cached on this worker"}
		}
		id := cacheIntentID(nodeID, artifactID)
		if action == "" {
			clearCacheIntentRecord(db, nodeID, artifactID)
			return nil
		}
		intent := db.CacheIntents[id]
		if intent.ID == "" {
			intent = CacheIntentRecord{ID: id, NodeID: nodeID, ArtifactID: artifactID, RequestedAt: now}
		}
		intent.Action = action
		intent.UpdatedAt = now
		db.CacheIntents[id] = intent
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "cache.intent." + action, Actor: "admin", Subject: artifactID, Metadata: map[string]any{"nodeId": nodeID}, CreatedAt: now})
		return nil
	})
}

func clearCacheIntentRecord(db *Database, nodeID, artifactID string) {
	delete(db.CacheIntents, cacheIntentID(nodeID, artifactID))
}

func cacheIntentRecord(db Database, nodeID, artifactID string) CacheIntentRecord {
	return db.CacheIntents[cacheIntentID(nodeID, artifactID)]
}

func cacheIntentID(nodeID, artifactID string) string {
	return strings.NewReplacer("/", "_", ":", "_").Replace(nodeID + "_" + NormalizeSHA256(artifactID))
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
	if desiredArtifactsByNode(db)[nodeID][artifactID] || evictionBusyArtifactsByNode(db)[nodeID][artifactID] {
		return artifactDeleteError{code: "artifact_in_use", message: "artifact is assigned, warming, or active on this worker"}
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
