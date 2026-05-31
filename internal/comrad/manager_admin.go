package comrad

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (m *Manager) handleAdminState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, m.stateResponse())
}

func (m *Manager) handleAdminNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, SortedNodes(m.store.Snapshot()))
	case http.MethodPost:
		m.handleAdminNodeUpdate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *Manager) handleAdminNodeUpdate(w http.ResponseWriter, r *http.Request) {
	var req UpdateNodeRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "nodeId is required")
		return
	}
	if err := m.store.Update(func(db *Database) error { return upsertAdminNode(db, req) }); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	m.replanAndDispatch()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func upsertAdminNode(db *Database, req UpdateNodeRequest) error {
	node, ok := db.Nodes[req.ID]
	if !ok {
		node = Node{ID: req.ID, State: NodeStateExpected}
	}
	if req.Approved != nil {
		node.Approved = *req.Approved
	}
	if req.OwnerUserID != "" {
		if _, ok := db.Users[req.OwnerUserID]; !ok {
			return fmt.Errorf("user %s not found", req.OwnerUserID)
		}
		node.OwnerUserID = req.OwnerUserID
	}
	if req.Mode != "" {
		node.Mode = req.Mode
	}
	if req.Tags != nil {
		node.Tags = req.Tags
	}
	if req.State != "" {
		node.State = req.State
	}
	db.Nodes[node.ID] = node
	db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "admin.node.updated", Actor: "admin", Subject: node.ID, CreatedAt: time.Now().UTC()})
	return nil
}

func (m *Manager) handleAdminSlots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, SortedSlots(m.store.Snapshot()))
}

func (m *Manager) handleAdminArtifacts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, SortedArtifacts(m.store.Snapshot()))
	case http.MethodPost:
		var req CreateArtifactRequest
		if err := readConfig(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if req.Path == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "path is required")
			return
		}
		sha, size, err := FileSHA256(req.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, "artifact_unreadable", err.Error())
			return
		}
		if req.SHA256 != "" && NormalizeSHA256(req.SHA256) != sha {
			writeError(w, http.StatusBadRequest, FailureArtifactDigestMismatch, fmt.Sprintf("expected %s got %s", NormalizeSHA256(req.SHA256), sha))
			return
		}
		name := req.Name
		if name == "" {
			name = filepath.Base(req.Path)
		}
		kind := req.Kind
		if kind == "" {
			kind = "model_gguf"
		}
		abs, _ := filepath.Abs(req.Path)
		artifact := Artifact{ID: "sha256:" + strings.TrimPrefix(sha, "sha256:"), Kind: kind, Name: name, Path: abs, SHA256: sha, SizeBytes: size, CreatedAt: time.Now().UTC()}
		err = m.store.Update(func(db *Database) error {
			db.Artifacts[artifact.ID] = artifact
			db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "artifact.registered", Actor: "admin", Subject: artifact.ID, CreatedAt: time.Now().UTC()})
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, artifact)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *Manager) handleAdminProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, SortedProfiles(m.store.Snapshot()))
	case http.MethodPost:
		m.handleAdminProfileUpsert(w, r)
	case http.MethodDelete:
		m.handleAdminProfileDelete(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *Manager) handleAdminProfileUpsert(w http.ResponseWriter, r *http.Request) {
	if isYAMLRequest(r) {
		m.handleAdminProfileConfigUpsert(w, r)
		return
	}
	var req CreateProfileRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	profile := profileFromRequest(req)
	var saved WorkloadProfile
	if err := m.store.Update(func(db *Database) error {
		if err := upsertProfile(db, profile); err != nil {
			return err
		}
		saved = db.Profiles[profile.ID]
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "profile_invalid", err.Error())
		return
	}
	m.replanAndDispatch()
	writeJSON(w, http.StatusCreated, saved)
}

func (m *Manager) handleAdminProfileConfigUpsert(w http.ResponseWriter, r *http.Request) {
	var req ProfileConfig
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	profile, err := profileFromConfig(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "profile_invalid", err.Error())
		return
	}
	profile.CreatedAt = time.Now().UTC()
	var saved WorkloadProfile
	if err := m.store.Update(func(db *Database) error {
		if err := upsertProfile(db, profile); err != nil {
			return err
		}
		saved = db.Profiles[profile.ID]
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "profile_invalid", err.Error())
		return
	}
	m.replanAndDispatch()
	writeJSON(w, http.StatusCreated, saved)
}

func profileFromRequest(req CreateProfileRequest) WorkloadProfile {
	normalizeProfileRequest(&req)
	profile := WorkloadProfile{ID: req.ID, Name: req.Name, Alias: req.Alias, LogicalModel: req.LogicalModel, Kind: req.Kind, RuntimeAdapter: req.RuntimeAdapter, Artifacts: req.Artifacts, Requirements: req.Requirements, LLM: req.LLM, Runtime: req.Runtime, RuntimeVariants: req.RuntimeVariants, ComputeCost: req.ComputeCost, Warmable: req.Warmable, MaxConcurrencyPerSlot: req.MaxConcurrencyPerSlot, CreatedAt: time.Now().UTC()}
	normalizeProfile(&profile)
	return profile
}

func normalizeProfileRequest(req *CreateProfileRequest) {
	if req.Kind == "" {
		req.Kind = "llm.chat"
	}
	if req.ID == "" {
		req.ID = profileIDFromRequest(*req)
	}
	if req.RuntimeAdapter == "" && req.Requirements != nil {
		req.RuntimeAdapter = req.Requirements.RuntimeAdapter
	}
	if req.MaxConcurrencyPerSlot == 0 {
		req.MaxConcurrencyPerSlot = 1
	}
}

func profileIDFromRequest(req CreateProfileRequest) string {
	base := req.Name
	if base == "" {
		base = req.Alias
	}
	if base == "" {
		base = req.Kind
	}
	id := strings.NewReplacer(" ", "-", "/", ".", ":", "-").Replace(base)
	if req.LLM != nil && req.LLM.ContextTokens > 0 {
		id += fmt.Sprintf("/context-%d", req.LLM.ContextTokens)
	}
	return id
}

func normalizeProfile(profile *WorkloadProfile) {
	if profile.Version == 0 {
		profile.Version = 1
	}
	if profile.Name == "" {
		profile.Name = profile.ID
	}
	if profile.Alias == "" {
		profile.Alias = profile.Name
	}
	if profile.LogicalModel == "" {
		profile.LogicalModel = ProfileLogicalModel(*profile)
	}
	for i, variant := range profile.RuntimeVariants {
		profile.RuntimeVariants[i] = normalizeVariant(*profile, variant)
	}
}

func upsertProfile(db *Database, profile WorkloadProfile) error {
	if profile.ComputeCost < 0 {
		return fmt.Errorf("computeCost must be non-negative")
	}
	if err := validateWorkloadProfileRuntime(profile); err != nil {
		return err
	}
	for _, id := range profileArtifactIDs(profile) {
		if _, ok := db.Artifacts[id]; !ok {
			return fmt.Errorf("artifact %s not found", id)
		}
	}
	if old, ok := db.Profiles[profile.ID]; ok {
		profile.CreatedAt = old.CreatedAt
		profile.Version = old.Version + 1
	}
	if profile.Version == 0 {
		profile.Version = 1
	}
	db.Profiles[profile.ID] = profile
	db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "profile.upserted", Actor: "admin", Subject: profile.ID, CreatedAt: time.Now().UTC()})
	return nil
}

func (m *Manager) handleAdminPolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, SortedPolicies(m.store.Snapshot()))
	case http.MethodPost:
		var req UpsertPolicyRequest
		if err := readConfig(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if req.ProfileID == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "profileId is required")
			return
		}
		now := time.Now().UTC()
		if req.ID == "" {
			req.ID = "pol_" + strings.NewReplacer("/", "_", ":", "_").Replace(req.ProfileID)
		}
		policy := PlacementPolicy{
			ID:                       req.ID,
			ProfileID:                req.ProfileID,
			CachedCount:              req.CachedCount,
			WarmCount:                req.WarmCount,
			AutoBalance:              req.AutoBalance,
			MinCachedCount:           req.MinCachedCount,
			MaxCachedCount:           req.MaxCachedCount,
			MinWarmCount:             req.MinWarmCount,
			MaxWarmCount:             req.MaxWarmCount,
			MaxCachedProfilesPerNode: req.MaxCachedProfilesPerNode,
			MaxWarmProfilesPerNode:   req.MaxWarmProfilesPerNode,
			Constraints:              req.Constraints,
			HardPinnedSlots:          req.HardPinnedSlots,
			CreatedAt:                now,
			UpdatedAt:                now,
		}
		err := m.store.Update(func(db *Database) error {
			if _, ok := db.Profiles[policy.ProfileID]; !ok {
				return fmt.Errorf("profile %s not found", policy.ProfileID)
			}
			if err := validatePlacementPolicy(policy); err != nil {
				return err
			}
			if old, ok := db.Policies[policy.ID]; ok {
				policy.CreatedAt = old.CreatedAt
			}
			db.Policies[policy.ID] = policy
			db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "placement.policy.upserted", Actor: "admin", Subject: policy.ID, CreatedAt: now})
			return nil
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "policy_invalid", err.Error())
			return
		}
		m.replanAndDispatch()
		writeJSON(w, http.StatusCreated, policy)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *Manager) handleAdminPlacement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	db := m.store.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"fitMatrix":   BuildFitMatrix(db),
		"assignments": SortedAssignments(db),
		"plan":        PlanPlacement(db),
	})
}

func (m *Manager) handleApplyPlacement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	m.replanAndDispatch()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (m *Manager) handleAdminTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, taskListResponse(m.store.Snapshot(), parseTaskListQuery(r.URL.Query())))
}

func (m *Manager) handleAdminAttempts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, SortedAttempts(m.store.Snapshot()))
}

func (m *Manager) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, SortedReports(m.store.Snapshot()))
}

func (m *Manager) handleAdminUnban(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req UnbanRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.NodeID == "" && req.SlotID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "nodeId or slotId is required")
		return
	}
	now := time.Now().UTC()
	if err := m.store.Update(func(db *Database) error {
		if req.NodeID != "" {
			node, ok := db.Nodes[req.NodeID]
			if !ok {
				return fmt.Errorf("node %s not found", req.NodeID)
			}
			node.Quarantined = false
			node.QuarantineReason = ""
			node.QuarantineUntil = nil
			db.Nodes[node.ID] = node
			db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "quarantine.node_unbanned", Actor: "admin", Subject: node.ID, CreatedAt: now})
		}
		if req.SlotID != "" {
			slot, ok := db.Slots[req.SlotID]
			if !ok {
				return fmt.Errorf("slot %s not found", req.SlotID)
			}
			slot.Quarantined = false
			slot.QuarantineReason = ""
			slot.QuarantineUntil = nil
			if slot.State == SlotStateReady && slot.ActiveTaskID == "" {
				slot.AcceptsNew = true
				slot.MismatchReason = ""
			}
			db.Slots[slot.ID] = slot
			db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "quarantine.slot_unbanned", Actor: "admin", Subject: slot.ID, CreatedAt: now})
		}
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "unban_failed", err.Error())
		return
	}
	m.replanAndDispatch()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (m *Manager) handleAdminUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	db := m.store.Snapshot()
	out := make([]UpdateRecord, 0, len(db.Updates))
	for _, u := range db.Updates {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	writeJSON(w, http.StatusOK, out)
}

func (m *Manager) handleApplyWorkerUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req ApplyWorkerUpdateRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.ArtifactID == "" || req.SHA256 == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "artifactId and sha256 are required")
		return
	}
	update := UpdateRecord{ID: NewID("upd"), Kind: req.Kind, Version: req.Version, ArtifactID: req.ArtifactID, SHA256: NormalizeSHA256(req.SHA256), Signature: req.Signature, PublicKey: req.PublicKey, TargetNodes: req.TargetNodes, Status: "pending", CreatedAt: time.Now().UTC()}
	if update.Kind == "" {
		update.Kind = "worker"
	}
	err := m.store.Update(func(db *Database) error {
		artifact, ok := db.Artifacts[update.ArtifactID]
		if !ok {
			return fmt.Errorf("artifact %s not found", update.ArtifactID)
		}
		if NormalizeSHA256(artifact.SHA256) != update.SHA256 {
			return fmt.Errorf("artifact sha mismatch")
		}
		db.Updates[update.ID] = update
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "update.created", Actor: "admin", Subject: update.ID, CreatedAt: time.Now().UTC()})
		for _, nodeID := range update.TargetNodes {
			node := db.Nodes[nodeID]
			node.UpdateRequired = true
			node.UpdateStatus = "pending"
			db.Nodes[nodeID] = node
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_invalid", err.Error())
		return
	}
	m.dispatchUpdate(update)
	writeJSON(w, http.StatusCreated, update)
}

func (m *Manager) handleAdminMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	writePrometheusMetrics(w, m.store.Snapshot(), len(m.queue), cap(m.queue), m.store.BackendName(), m.runtimeMetricsSnapshot())
}
