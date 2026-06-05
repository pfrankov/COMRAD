package comrad

import (
	"reflect"
	"testing"
	"time"
)

func TestExplainPlacementShowsScarceReservationRejection(t *testing.T) {
	now := time.Now().UTC()
	db := explainTestDatabase(now)
	addExplainPlacementProfile(&db, "zz-big", "sha256:big", 12, 12, now)
	addExplainPlacementProfile(&db, "aa-small", "sha256:small", 6, 6, now)
	addExplainPlacementPolicy(&db, "aa-small", PlacementPolicy{ID: "pol-aa-small", ProfileID: "aa-small", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addExplainPlacementPolicy(&db, "zz-big", PlacementPolicy{ID: "pol-zz-big", ProfileID: "zz-big", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addExplainPlacementNode(&db, "node-a-power", 16, 30, 2)
	addExplainPlacementNode(&db, "node-b-weak", 8, 10, 1)

	explain := ExplainPlacement(db)

	big := explanationForProfile(t, explain, "zz-big")
	assertExplainSelected(t, big, "node-a-power", "node-a-power/slot0", "warm", "selected_for_warm_copy")
	small := explanationForProfile(t, explain, "aa-small")
	assertExplainSelected(t, small, "node-b-weak", "node-b-weak/slot0", "warm", "selected_for_warm_copy")
	assertExplainRejected(t, small, "node-a-power", "node-a-power/slot1", "warm", "resource_exhausted_unified_memory")
}

func TestExplainPlacementDryRunMatchesPlanner(t *testing.T) {
	now := time.Now().UTC()
	db := explainTestDatabase(now)
	addExplainPlacementProfile(&db, "profile", "sha256:profile", 4, 4, now)
	addExplainPlacementPolicy(&db, "profile", PlacementPolicy{ID: "pol-profile", ProfileID: "profile", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addExplainPlacementNode(&db, "node-good", 16, 30, 1)

	explain := explainPlacementWithCooldown(db, now, 17*time.Minute)
	plan := planPlacementWithCooldown(db, now, nil, 17*time.Minute)

	if !reflect.DeepEqual(explain.Plan, plan) {
		t.Fatalf("explain plan = %+v, want planner plan %+v", explain.Plan, plan)
	}
}

func TestExplainPlacementReportsNodeRAMAndDiskRejections(t *testing.T) {
	now := time.Now().UTC()
	db := explainTestDatabase(now)
	addExplainPlacementProfile(&db, "profile", "sha256:profile", 8, 8, now)
	addExplainPlacementPolicy(&db, "profile", PlacementPolicy{ID: "pol-profile", ProfileID: "profile", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addExplainPlacementNode(&db, "node-low-ram", 4, 30, 1)
	addExplainPlacementNode(&db, "node-low-disk", 16, 4, 1)
	addExplainPlacementNode(&db, "node-good", 16, 30, 1)

	explain := ExplainPlacement(db)
	profile := explanationForProfile(t, explain, "profile")

	assertExplainSelected(t, profile, "node-good", "node-good/slot0", "warm", "selected_for_warm_copy")
	assertExplainRejected(t, profile, "node-low-ram", "node-low-ram/slot0", "warm", "resource_exhausted_unified_memory")
	assertExplainRejected(t, profile, "node-low-disk", "node-low-disk/slot0", "warm", FailureResourceExhaustedDisk)
}

func TestExplainPlacementReportsOfflineNodeExcluded(t *testing.T) {
	now := time.Now().UTC()
	db := explainTestDatabase(now)
	addExplainPlacementProfile(&db, "profile", "sha256:profile", 4, 4, now)
	addExplainPlacementPolicy(&db, "profile", PlacementPolicy{ID: "pol-profile", ProfileID: "profile", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addExplainPlacementNode(&db, "node-offline", 16, 30, 1)
	addExplainPlacementNode(&db, "node-good", 16, 30, 1)
	offline := db.Nodes["node-offline"]
	offline.State = NodeStateOffline
	db.Nodes[offline.ID] = offline
	slot := db.Slots["node-offline/slot0"]
	slot.State = SlotStateUnavailable
	slot.AcceptsNew = false
	slot.MismatchReason = FailureWorkerDisconnected
	db.Slots[slot.ID] = slot

	explain := ExplainPlacement(db)
	profile := explanationForProfile(t, explain, "profile")

	assertExplainSelected(t, profile, "node-good", "node-good/slot0", "warm", "selected_for_warm_copy")
	assertExplainRejected(t, profile, "node-offline", "node-offline/slot0", "warm", "node_offline")
}

func explanationForProfile(t *testing.T, explain PlacementExplainResponse, profileID string) PlacementProfileExplanation {
	t.Helper()
	for _, profile := range explain.Profiles {
		if profile.ProfileID == profileID {
			return profile
		}
	}
	t.Fatalf("profile explanation %q missing: %+v", profileID, explain.Profiles)
	return PlacementProfileExplanation{}
}

func assertExplainSelected(t *testing.T, profile PlacementProfileExplanation, nodeID, slotID, phase, reason string) {
	t.Helper()
	for _, candidate := range profile.Selected {
		if candidate.NodeID == nodeID && candidate.SlotID == slotID && candidate.Phase == phase && Contains(candidate.Reasons, reason) {
			return
		}
	}
	t.Fatalf("selected %s/%s phase %s reason %s missing: %+v", nodeID, slotID, phase, reason, profile.Selected)
}

func assertExplainRejected(t *testing.T, profile PlacementProfileExplanation, nodeID, slotID, phase, reason string) {
	t.Helper()
	for _, candidate := range profile.Rejected {
		if candidate.NodeID == nodeID && candidate.SlotID == slotID && candidate.Phase == phase && Contains(candidate.Reasons, reason) {
			return
		}
	}
	t.Fatalf("rejected %s/%s phase %s reason %s missing: %+v", nodeID, slotID, phase, reason, profile.Rejected)
}

func explainTestDatabase(now time.Time) Database {
	return Database{
		SchemaVersion: CurrentSchemaVersion,
		Profiles:      map[string]WorkloadProfile{},
		Policies:      map[string]PlacementPolicy{},
		Nodes:         map[string]Node{},
		Slots:         map[string]Slot{},
		Artifacts:     map[string]Artifact{},
		Tasks:         map[string]Task{},
	}
}

func addExplainPlacementProfile(db *Database, id, artifact string, memoryGiB, diskGiB int64, now time.Time) {
	db.Artifacts[artifact] = Artifact{ID: artifact, SHA256: artifact, Kind: "model_gguf", SizeBytes: diskGiB << 30, CreatedAt: now}
	db.Profiles[id] = WorkloadProfile{
		ID:             id,
		Name:           id,
		Alias:          id,
		LogicalModel:   id,
		Kind:           "llm.chat",
		RuntimeAdapter: "llama.cpp-metal",
		Artifacts:      []string{artifact},
		Requirements: &Requirements{
			Target:             TargetDarwinArm64Metal,
			RuntimeAdapter:     "llama.cpp-metal",
			UnifiedMemoryBytes: memoryGiB << 30,
			DiskBytes:          diskGiB << 30,
		},
		LLM:      &LLMProfile{ContextTokens: 4096},
		Warmable: true,
	}
}

func addExplainPlacementPolicy(db *Database, profileID string, policy PlacementPolicy) {
	if policy.ID == "" {
		policy.ID = "pol-" + profileID
	}
	policy.ProfileID = profileID
	db.Policies[policy.ID] = policy
}

func addExplainPlacementNode(db *Database, id string, memoryGiB, diskGiB int64, slots int) {
	db.Nodes[id] = Node{
		ID:              id,
		State:           NodeStateOnline,
		Approved:        true,
		Target:          TargetDarwinArm64Metal,
		RuntimeAdapters: []string{"llama.cpp-metal"},
		Budgets:         ResourceBudget{UnifiedMemoryBytes: memoryGiB << 30, DiskBytes: diskGiB << 30, SlotCount: slots},
	}
	for i := 0; i < slots; i++ {
		slotID := id + "/slot" + string(rune('0'+i))
		db.Slots[slotID] = Slot{
			ID:             slotID,
			NodeID:         id,
			Target:         TargetDarwinArm64Metal,
			RuntimeAdapter: "llama.cpp-metal",
			Resources:      ResourceBudget{UnifiedMemoryBytes: memoryGiB << 30, DiskBytes: diskGiB << 30},
			State:          SlotStateIdle,
		}
	}
}
