package comrad

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrometheusMetricsEndpoint(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.sqlite"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(manager.Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	for _, want := range []string{
		"# HELP comrad_queue_limit",
		"comrad_queue_limit",
		"comrad_tasks_total",
		`comrad_storage_backend_info{backend="sqlite"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "prompt") || strings.Contains(strings.ToLower(body), "response") {
		t.Fatalf("metrics leak prompt/response labels:\n%s", body)
	}
}

func TestPrometheusMetricsExposeAdminStateWebSocketState(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.sqlite"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := manager.addAdminStateSubscriber()
	defer manager.removeAdminStateSubscriber(id)
	manager.publishAdminState()

	body := metricsBody(t, manager)
	for _, want := range []string{
		"comrad_admin_state_ws_clients 1",
		"comrad_admin_state_ws_connects_total 1",
		"comrad_admin_state_ws_broadcasts_total 1",
		"comrad_admin_state_ws_last_broadcast_subscribers 1",
		"comrad_admin_state_ws_last_snapshot_bytes",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
}

func TestPrometheusMetricsExposeCapacityPlanState(t *testing.T) {
	manager := newTestManager(t, 4, time.Second, 3)
	seedMetricsCapacityState(t, manager)

	body := metricsBody(t, manager)
	for _, want := range []string{
		`comrad_capacity_desired_cached{model="assistant",profile="llm.chat/assistant"} 4`,
		`comrad_capacity_actual_cached{model="assistant",profile="llm.chat/assistant"} 2`,
		`comrad_capacity_desired_warm{model="assistant",profile="llm.chat/assistant"} 3`,
		`comrad_capacity_actual_warm{model="assistant",profile="llm.chat/assistant"} 1`,
		`comrad_capacity_warming{model="assistant",profile="llm.chat/assistant"} 1`,
		`comrad_capacity_failed{model="assistant",profile="llm.chat/assistant"} 1`,
		`comrad_capacity_blocked{model="assistant",profile="llm.chat/assistant"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
	for _, leak := range []string{"sha256:model", "/private/admin-token", "prompt", "response"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(leak)) {
			t.Fatalf("metrics leaked sensitive or unbounded value %q:\n%s", leak, body)
		}
	}
}

func metricsBody(t *testing.T, manager *Manager) string {
	t.Helper()
	server := httptest.NewServer(manager.Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.StatusCode, string(b))
	}
	return string(b)
}

func seedMetricsCapacityState(t *testing.T, manager *Manager) {
	t.Helper()
	now := time.Now().UTC()
	profile := EffectiveProfileForVariant(WorkloadProfile{
		ID:             "llm.chat/assistant",
		Name:           "assistant",
		Alias:          "assistant",
		LogicalModel:   "assistant",
		Kind:           "llm.chat",
		RuntimeAdapter: "llama.cpp-metal",
		Artifacts:      []string{"sha256:model"},
		Requirements:   &Requirements{Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal", UnifiedMemoryBytes: 1, DiskBytes: 1},
		LLM:            &LLMProfile{ContextTokens: 4096},
		Warmable:       true,
		CreatedAt:      now,
	}, RuntimeModelVariant{})
	if err := manager.store.Update(func(db *Database) error {
		db.Artifacts["sha256:model"] = Artifact{ID: "sha256:model", SHA256: "sha256:model", Kind: "model_gguf", Path: "/private/admin-token/model.gguf", CreatedAt: now}
		db.Profiles[profile.ID] = profile
		db.Policies["pol"] = PlacementPolicy{ID: "pol", ProfileID: profile.ID, CachedCount: 4, WarmCount: 3, CreatedAt: now, UpdatedAt: now}
		seedMetricsNode(db, profile, "node-ready", "node-ready/slot0", SlotStateReady, true)
		seedMetricsNode(db, profile, "node-warming", "node-warming/slot0", SlotStateWarming, true)
		seedMetricsNode(db, profile, "node-failed", "node-failed/slot0", SlotStateError, false)
		db.Assignments["asg-ready"] = metricsAssignment(profile, "node-ready", "node-ready/slot0", true, true, true)
		db.Assignments["asg-warming"] = metricsAssignment(profile, "node-warming", "node-warming/slot0", true, false, false)
		db.Assignments["asg-failed"] = metricsAssignment(profile, "node-failed", "node-failed/slot0", false, false, false)
		db.Assignments["asg-missing"] = PlacementAssignment{ID: "asg-missing", ProfileID: profile.ID, DesiredCached: true, MismatchReason: "insufficient_compatible_slots", UpdatedAt: now}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func seedMetricsNode(db *Database, profile WorkloadProfile, nodeID, slotID, state string, cached bool) {
	cachedArtifacts := []string{}
	if cached {
		cachedArtifacts = []string{"sha256:model"}
	}
	db.Nodes[nodeID] = Node{ID: nodeID, State: NodeStateOnline, Approved: true, RuntimeAdapters: []string{profile.RuntimeAdapter}, CachedArtifacts: cachedArtifacts}
	db.Slots[slotID] = Slot{
		ID:               slotID,
		NodeID:           nodeID,
		Target:           TargetDarwinArm64Metal,
		RuntimeAdapter:   profile.RuntimeAdapter,
		State:            state,
		ProfileID:        profile.ID,
		ProfileVersion:   profileVersion(profile),
		LogicalModel:     ProfileLogicalModel(profile),
		RuntimeVariantID: profile.RuntimeVariantID,
		ModelArtifactID:  ConcreteModelArtifactID(profile),
		ModelSHA256:      ConcreteModelSHA256(profile),
	}
}

func metricsAssignment(profile WorkloadProfile, nodeID, slotID string, actualCached, actualWarm, ready bool) PlacementAssignment {
	return PlacementAssignment{
		ID:               "asg-" + nodeID,
		ProfileID:        profile.ID,
		LogicalModel:     ProfileLogicalModel(profile),
		NodeID:           nodeID,
		SlotID:           slotID,
		DesiredCached:    true,
		DesiredWarm:      true,
		ActualCached:     actualCached,
		ActualWarm:       actualWarm,
		Ready:            ready,
		RuntimeVariantID: profile.RuntimeVariantID,
		ModelArtifactID:  ConcreteModelArtifactID(profile),
		ModelSHA256:      ConcreteModelSHA256(profile),
		UpdatedAt:        time.Now().UTC(),
	}
}
