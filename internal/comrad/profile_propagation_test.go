package comrad

import (
	"testing"
	"time"
)

func TestPlanPlacementInvalidatesWarmSlotWhenProfileVersionChanges(t *testing.T) {
	now := time.Now().UTC()
	profile := propagationTestProfile(2)
	db := propagationTestDatabase(profile, now)
	db.Slots["node-a/slot0"] = propagationTestReadySlot(profile, 1, "gemma-old")

	plan := PlanPlacement(db)
	if len(plan) != 1 {
		t.Fatalf("plan length = %d", len(plan))
	}
	if plan[0].ActualWarm || plan[0].Ready {
		t.Fatalf("stale profile version treated as ready: %+v", plan[0])
	}

	slot := db.Slots["node-a/slot0"]
	slot.ProfileVersion = profileVersion(profile)
	slot.LogicalModel = ProfileLogicalModel(profile)
	db.Slots[slot.ID] = slot
	plan = PlanPlacement(db)
	if len(plan) != 1 || !plan[0].ActualWarm || !plan[0].Ready {
		t.Fatalf("current profile version not ready: %+v", plan)
	}
}

func TestWorkerAssignmentAlreadySatisfiedChecksProfileVersion(t *testing.T) {
	profile := EffectiveProfileForVariant(propagationTestProfile(2), RuntimeModelVariant{})
	worker := &Worker{
		slots: map[string]Slot{
			"node-a/slot0": propagationTestReadySlot(profile, 1, "gemma-old"),
		},
		runtimes: map[string]*llamaServerProcess{},
	}
	if worker.assignmentAlreadySatisfied(AssignmentPayload{Profile: profile}) {
		t.Fatal("worker ignored updated profile version")
	}

	slot := worker.slots["node-a/slot0"]
	slot.ProfileVersion = profileVersion(profile)
	slot.LogicalModel = ProfileLogicalModel(profile)
	worker.slots[slot.ID] = slot
	worker.runtimes[slot.ID] = &llamaServerProcess{profileKey: assignmentKey(profile), done: make(chan struct{})}
	if !worker.assignmentAlreadySatisfied(AssignmentPayload{Profile: profile}) {
		t.Fatal("worker did not recognize the current profile version")
	}
}

func propagationTestDatabase(profile WorkloadProfile, now time.Time) Database {
	return Database{
		SchemaVersion: CurrentSchemaVersion,
		Profiles: map[string]WorkloadProfile{
			profile.ID: profile,
		},
		Policies: map[string]PlacementPolicy{
			"pol": {
				ID:          "pol",
				ProfileID:   profile.ID,
				CachedCount: 1,
				WarmCount:   1,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
		Nodes: map[string]Node{
			"node-a": {
				ID:              "node-a",
				State:           NodeStateOnline,
				Approved:        true,
				RuntimeAdapters: []string{"llama.cpp-metal"},
			},
		},
		Slots: map[string]Slot{},
		Artifacts: map[string]Artifact{
			"sha256:model": {
				ID:     "sha256:model",
				SHA256: "sha256:model",
				Kind:   "model_gguf",
			},
		},
	}
}

func propagationTestProfile(version int) WorkloadProfile {
	return WorkloadProfile{
		ID:             "llm.chat/gemma-4-e2b/context-512",
		Version:        version,
		Name:           "Gemma 4 E2B",
		LogicalModel:   "gemma-4-e2b",
		Kind:           "llm.chat",
		RuntimeAdapter: "llama.cpp-metal",
		Artifacts:      []string{"sha256:model"},
		Requirements: &Requirements{
			Target:             TargetDarwinArm64Metal,
			RuntimeAdapter:     "llama.cpp-metal",
			UnifiedMemoryBytes: 1,
			DiskBytes:          1,
		},
		LLM:      &LLMProfile{ContextTokens: 512},
		Warmable: true,
	}
}

func propagationTestReadySlot(profile WorkloadProfile, version int, logical string) Slot {
	return Slot{
		ID:               "node-a/slot0",
		NodeID:           "node-a",
		Target:           TargetDarwinArm64Metal,
		RuntimeAdapter:   "llama.cpp-metal",
		Resources:        ResourceBudget{UnifiedMemoryBytes: 8, DiskBytes: 8},
		State:            SlotStateReady,
		ProfileID:        profile.ID,
		ProfileVersion:   version,
		LogicalModel:     logical,
		RuntimeVariantID: profile.ID,
		ModelArtifactID:  ConcreteModelArtifactID(profile),
		ModelSHA256:      ConcreteModelSHA256(profile),
		AcceptsNew:       true,
	}
}
