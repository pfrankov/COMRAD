package comrad

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeletingProfileQueuesWorkerArtifactEviction(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	seedCachedProfilePolicy(t, m, profile, "node-a")
	m.replanAndDispatch()

	server := httptest.NewServer(m.Handler())
	defer server.Close()
	req, err := http.NewRequest(http.MethodDelete, server.URL+"/api/admin/profiles?profileId="+profile.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}

	msg := nextEvict(t, session)
	if msg.NodeID != "node-a" {
		t.Fatalf("evict node = %s", msg.NodeID)
	}
	var payload EvictArtifactPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ArtifactID != "sha256:model" {
		t.Fatalf("evict artifact = %s", payload.ArtifactID)
	}
	state := m.store.Snapshot()
	if _, ok := state.Profiles[profile.ID]; ok {
		t.Fatal("profile still exists after delete")
	}
	if len(state.Policies) != 0 || len(state.Assignments) != 0 {
		t.Fatalf("profile policy or assignment remained: policies=%d assignments=%d", len(state.Policies), len(state.Assignments))
	}
}

func TestZeroCapacityEvictsNoLongerDesiredWorkerArtifact(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	seedCachedProfilePolicy(t, m, profile, "node-a")
	m.replanAndDispatch()
	drainSessionMessages(session)

	if err := m.store.Update(func(db *Database) error {
		policy := db.Policies["pol"]
		policy.CachedCount = 0
		policy.WarmCount = 0
		policy.UpdatedAt = time.Now().UTC()
		db.Policies[policy.ID] = policy
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.replanAndDispatch()

	payload := decodeEvictPayload(t, nextEvict(t, session))
	if payload.ArtifactID != "sha256:model" {
		t.Fatalf("evicted artifact = %s", payload.ArtifactID)
	}
}

func TestAdminCanEvictStaleArtifactFromSelectedWorker(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	if err := m.store.Update(func(db *Database) error {
		node := db.Nodes["node-a"]
		node.CachedArtifacts = []string{"sha256:model"}
		db.Nodes[node.ID] = node
		slot := db.Slots["node-a/slot0"]
		clearEvictedSlot(&slot)
		db.Slots[slot.ID] = slot
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/api/admin/nodes/node-a/artifacts/sha256:model", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("evict status = %d", resp.StatusCode)
	}
	payload := decodeEvictPayload(t, nextEvict(t, session))
	if payload.ArtifactID != "sha256:model" {
		t.Fatalf("evicted artifact = %s", payload.ArtifactID)
	}
}

func TestAdminCanRequestStaleIdleCacheEviction(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	seedStaleCachedArtifact(t, m, "node-a", "node-a/slot0", SlotStateIdle)
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	resp := postCacheArtifactAction(t, server.URL, "node-a", "sha256:model", "evict")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("evict action status = %d", resp.StatusCode)
	}
	payload := decodeEvictPayload(t, nextEvict(t, session))
	if payload.ArtifactID != "sha256:model" || payload.Reason != "admin_requested" {
		t.Fatalf("evict payload = %+v", payload)
	}
}

func TestAdminCannotEvictActiveStaleCache(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	seedStaleCachedArtifact(t, m, "node-a", "node-a/slot0", SlotStateServing)
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	resp := postCacheArtifactAction(t, server.URL, "node-a", "sha256:model", "evict")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("active evict status = %d", resp.StatusCode)
	}
	assertNoEvictQueued(t, session)
}

func TestKeptStaleCacheIsNotIncludedInEvictionPlan(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	addReadySession(t, m, "node-a", "node-a/slot0", profile)
	seedStaleCachedArtifact(t, m, "node-a", "node-a/slot0", SlotStateIdle)
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	resp := postCacheArtifactAction(t, server.URL, "node-a", "sha256:model", "keep")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("keep action status = %d", resp.StatusCode)
	}
	targets := staleArtifactEvictions(m.store.Snapshot())
	if len(targets) != 0 {
		t.Fatalf("kept stale artifact was planned for eviction: %+v", targets)
	}
}

func TestArtifactEvictionStateTracksQueuedBlockedAndWorkerResult(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	if err := m.store.Update(func(db *Database) error {
		node := db.Nodes["node-a"]
		node.CachedArtifacts = []string{"sha256:model"}
		db.Nodes[node.ID] = node
		db.Nodes["node-offline"] = Node{ID: "node-offline", State: NodeStateOffline, CachedArtifacts: []string{"sha256:stale"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if !m.dispatchArtifactEviction("node-a", "sha256:model", "admin_requested") {
		t.Fatal("eviction was not queued")
	}
	queued := latestEvictionFor(t, m, "node-a", "sha256:model")
	if queued.Status != "queued" || queued.Reason != "admin_requested" {
		t.Fatalf("queued eviction = %+v", queued)
	}
	_ = nextEvict(t, session)
	if err := m.updateArtifactState("node-a", ArtifactStatePayload{ArtifactID: "sha256:model", State: "evicted"}); err != nil {
		t.Fatal(err)
	}
	evicted := latestEvictionFor(t, m, "node-a", "sha256:model")
	if evicted.Status != "evicted" || evicted.Failure != "" {
		t.Fatalf("evicted record = %+v", evicted)
	}

	if m.dispatchArtifactEviction("node-offline", "sha256:stale", "not_desired") {
		t.Fatal("offline eviction unexpectedly queued")
	}
	blocked := latestEvictionFor(t, m, "node-offline", "sha256:stale")
	if blocked.Status != "blocked" || blocked.Failure != "worker_offline" {
		t.Fatalf("blocked record = %+v", blocked)
	}
	state := m.stateResponse()
	if len(state.ArtifactEvictions) < 2 {
		t.Fatalf("state did not expose eviction records: %+v", state.ArtifactEvictions)
	}
}

func TestWorkerEvictsCachedArtifactAndClearsWarmSlot(t *testing.T) {
	worker, cachePath, profile := newEvictionTestWorker(t, SlotStateReady)
	msg := Envelope{ID: "msg-evict", Type: MsgEvictArtifact, Payload: MarshalPayload(EvictArtifactPayload{ArtifactID: "sha256:model", Reason: "test"})}
	if err := worker.handleEnvelope(context.Background(), msg); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("cache file still exists or stat failed unexpectedly: %v", err)
	}
	worker.mu.Lock()
	_, cached := worker.cache["sha256:model"]
	slot := worker.slots["node-a/metal0"]
	_, assigned := worker.assigns[assignmentKey(profile)]
	_, warm := worker.warm[assignmentKey(profile)]
	worker.mu.Unlock()
	if cached || assigned || warm {
		t.Fatalf("worker retained cache=%v assignment=%v warm=%v", cached, assigned, warm)
	}
	if slot.State != SlotStateIdle || slot.ProfileID != "" || slot.AcceptsNew {
		t.Fatalf("slot after eviction = %+v", slot)
	}
	if !workerSentArtifactState(worker, "evicted") {
		t.Fatal("worker did not report evicted artifact state")
	}
}

func TestWorkerRefusesToEvictArtifactUsedByServingSlot(t *testing.T) {
	worker, cachePath, _ := newEvictionTestWorker(t, SlotStateServing)
	msg := Envelope{ID: "msg-evict", Type: MsgEvictArtifact, Payload: MarshalPayload(EvictArtifactPayload{ArtifactID: "sha256:model"})}
	err := worker.handleEnvelope(context.Background(), msg)
	if err == nil || !strings.Contains(err.Error(), "artifact_in_use") {
		t.Fatalf("evict error = %v", err)
	}
	if _, statErr := os.Stat(cachePath); statErr != nil {
		t.Fatalf("cache file was removed after failed eviction: %v", statErr)
	}
}

func latestEvictionFor(t *testing.T, m *Manager, nodeID, artifactID string) ArtifactEvictionRecord {
	t.Helper()
	var latest ArtifactEvictionRecord
	for _, record := range m.store.Snapshot().ArtifactEvictions {
		if record.NodeID == nodeID && record.ArtifactID == artifactID && record.UpdatedAt.After(latest.UpdatedAt) {
			latest = record
		}
	}
	if latest.ID == "" {
		t.Fatalf("missing eviction record for %s/%s", nodeID, artifactID)
	}
	return latest
}

func seedCachedProfilePolicy(t *testing.T, m *Manager, profile WorkloadProfile, nodeID string) {
	t.Helper()
	if err := m.store.Update(func(db *Database) error {
		node := db.Nodes[nodeID]
		node.CachedArtifacts = []string{"sha256:model"}
		db.Nodes[nodeID] = node
		db.Policies["pol"] = PlacementPolicy{ID: "pol", ProfileID: profile.ID, CachedCount: 1, WarmCount: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func nextEvict(t *testing.T, session *workerSession) Envelope {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case msg := <-session.send:
			if msg.Type == MsgEvictArtifact {
				return msg
			}
		case <-deadline:
			t.Fatal("timed out waiting for eviction")
		}
	}
}

func decodeEvictPayload(t *testing.T, msg Envelope) EvictArtifactPayload {
	t.Helper()
	var payload EvictArtifactPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func postCacheArtifactAction(t *testing.T, serverURL, nodeID, artifactID, action string) *http.Response {
	t.Helper()
	body := strings.NewReader(`{"action":"` + action + `"}`)
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/admin/nodes/"+nodeID+"/artifacts/"+artifactID, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func seedStaleCachedArtifact(t *testing.T, m *Manager, nodeID, slotID, state string) {
	t.Helper()
	if err := m.store.Update(func(db *Database) error {
		node := db.Nodes[nodeID]
		node.CachedArtifacts = []string{"sha256:model"}
		db.Nodes[nodeID] = node
		slot := db.Slots[slotID]
		slot.State = state
		slot.ProfileID = "llm.chat/assistant"
		slot.ModelArtifactID = "sha256:model"
		slot.ModelSHA256 = "sha256:model"
		slot.ActiveTaskID = ""
		slot.AcceptsNew = state == SlotStateReady
		if state == SlotStateServing {
			slot.ActiveTaskID = "task-active"
			slot.AcceptsNew = false
		}
		db.Slots[slot.ID] = slot
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func assertNoEvictQueued(t *testing.T, session *workerSession) {
	t.Helper()
	select {
	case msg := <-session.send:
		if msg.Type == MsgEvictArtifact {
			t.Fatalf("unexpected eviction queued: %+v", msg)
		}
	default:
	}
}

func drainSessionMessages(session *workerSession) {
	for {
		select {
		case <-session.send:
		default:
			return
		}
	}
}

func newEvictionTestWorker(t *testing.T, slotState string) (*Worker, string, WorkloadProfile) {
	t.Helper()
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "sha256_model")
	if err := os.WriteFile(cachePath, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	profile := EffectiveProfileForVariant(WorkloadProfile{
		ID:             "llm.chat/assistant",
		Alias:          "assistant",
		LogicalModel:   "assistant",
		Kind:           "llm.chat",
		RuntimeAdapter: "llama.cpp-metal",
		Artifacts:      []string{"sha256:model"},
		Requirements:   &Requirements{Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal", UnifiedMemoryBytes: 1, DiskBytes: 1},
		LLM:            &LLMProfile{ContextTokens: 512},
		Warmable:       true,
	}, RuntimeModelVariant{})
	slot := Slot{
		ID:               "node-a/metal0",
		NodeID:           "node-a",
		Target:           TargetDarwinArm64Metal,
		RuntimeAdapter:   "llama.cpp-metal",
		State:            slotState,
		ProfileID:        profile.ID,
		ProfileVersion:   profileVersion(profile),
		LogicalModel:     ProfileLogicalModel(profile),
		RuntimeVariantID: profile.RuntimeVariantID,
		ModelArtifactID:  "sha256:model",
		ModelSHA256:      "sha256:model",
		AcceptsNew:       slotState == SlotStateReady,
	}
	worker := &Worker{
		cfg:       WorkerConfig{StatePath: filepath.Join(dir, "worker-state.json"), CacheDir: dir},
		client:    http.DefaultClient,
		node:      Node{ID: "node-a", State: NodeStateOnline, Approved: true, Target: TargetDarwinArm64Metal, RuntimeAdapters: []string{"llama.cpp-metal"}},
		slots:     map[string]Slot{slot.ID: slot},
		assigns:   map[string]AssignmentPayload{assignmentKey(profile): {Profile: profile, Artifacts: []ArtifactSpec{{ID: "sha256:model", SHA256: "sha256:model", Kind: "model_gguf", Name: "model.gguf"}}}},
		cache:     map[string]string{"sha256:model": cachePath},
		warm:      map[string]WorkloadProfile{assignmentKey(profile): profile},
		processed: map[string]time.Time{},
		active:    map[string]context.CancelFunc{},
		runtimes:  map[string]*llamaServerProcess{slot.ID: nil},
		send:      make(chan Envelope, 16),
	}
	return worker, cachePath, profile
}

func workerSentArtifactState(worker *Worker, state string) bool {
	for {
		select {
		case msg := <-worker.send:
			if msg.Type != MsgArtifactState {
				continue
			}
			var payload ArtifactStatePayload
			if err := json.Unmarshal(msg.Payload, &payload); err == nil && payload.State == state {
				return true
			}
		default:
			return false
		}
	}
}
