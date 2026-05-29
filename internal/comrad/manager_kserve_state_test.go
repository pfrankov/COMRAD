package comrad

import (
	"testing"
	"time"
)

func TestAdminStateExposesConditionsRuntimeSummaryAndCachePlans(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	addReadySession(t, m, "node-a", "node-a/slot0", profile)
	seedCachedProfilePolicy(t, m, profile, "node-a")
	m.replanAndDispatch()
	if err := m.store.Update(func(db *Database) error {
		db.Nodes["node-b"] = Node{
			ID:              "node-b",
			State:           NodeStateOffline,
			Approved:        true,
			RuntimeAdapters: []string{"llama.cpp-metal"},
			CachedArtifacts: []string{"sha256:model"},
		}
		db.Slots["node-b/slot0"] = Slot{
			ID:             "node-b/slot0",
			NodeID:         "node-b",
			Target:         TargetDarwinArm64Metal,
			RuntimeAdapter: "llama.cpp-metal",
			State:          SlotStateIdle,
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if m.dispatchArtifactEviction("node-b", "sha256:model", "not_desired") {
		t.Fatal("offline eviction unexpectedly queued")
	}

	state := m.stateResponse()
	profileView := state.Profiles[0]
	assertCondition(t, profileView.Conditions, "Ready", "True", "DesiredWarmCopiesReady")
	assertCondition(t, profileView.Conditions, "Schedulable", "True", "CompatibleSlotAvailable")
	assertCondition(t, profileView.Conditions, "ArtifactsAvailable", "True", "AllArtifactsRegistered")
	policyView := state.Policies[0]
	assertCondition(t, policyView.Conditions, "Cached", "True", "DesiredCopiesAvailable")
	assertCondition(t, policyView.Conditions, "Warm", "True", "DesiredWarmCopiesReady")
	assertCondition(t, policyView.Conditions, "PlacementSatisfied", "True", "PlacementApplied")
	assertCondition(t, state.Nodes[0].Conditions, "Connected", "True", "WorkerOnline")
	assertCondition(t, state.Nodes[0].Conditions, "Compatible", "True", "RuntimeAdapterAvailable")
	assertCondition(t, state.Slots[0].Conditions, "Ready", "True", "SlotReady")
	assertCondition(t, state.ArtifactEvictions[0].Conditions, "Blocked", "True", "WorkerOffline")

	if state.RuntimeSummary.Kind != "RuntimeSummary" {
		t.Fatalf("runtime summary kind = %q", state.RuntimeSummary.Kind)
	}
	runtime := state.RuntimeSummary.Items[0]
	if runtime.Metadata.Name != "llama.cpp-metal" || runtime.Status.AvailableWorkers != 1 || runtime.Status.ReadySlots != 1 {
		t.Fatalf("runtime summary = %+v", runtime)
	}
	if len(state.CachePlans) != 1 {
		t.Fatalf("cache plans = %+v", state.CachePlans)
	}
	plan := state.CachePlans[0]
	if plan.ProfileRef != profile.ID || plan.DesiredCopies != 1 || plan.ActualCopies != 2 || plan.StaleCopies != 1 || plan.EvictionsPending != 1 {
		t.Fatalf("cache plan = %+v", plan)
	}
	worker := cacheWorkerStatus(plan.Workers, "node-a")
	if !worker.Cached || !worker.Warm || worker.Active {
		t.Fatalf("node-a cache worker status = %+v", worker)
	}
	worker = cacheWorkerStatus(plan.Workers, "node-b")
	if !worker.Cached || worker.Warm || worker.Eviction.Status != ArtifactEvictionBlocked {
		t.Fatalf("node-b cache worker status = %+v", worker)
	}
}

func TestCachePlanWarmStateUsesManagerAssignments(t *testing.T) {
	db := emptyDatabase()
	now := time.Now().UTC()
	profile := WorkloadProfile{
		ID:             "llm.chat/base",
		Kind:           "llm.chat",
		RuntimeAdapter: "llama.cpp-metal",
		Artifacts:      []string{"sha256:model"},
		Requirements:   &Requirements{Target: TargetDarwinArm64Metal},
		LLM:            &LLMProfile{ContextTokens: 1024},
		CreatedAt:      now,
	}
	db.Profiles[profile.ID] = profile
	db.Artifacts["sha256:model"] = Artifact{ID: "sha256:model", SHA256: "sha256:model", Kind: "model_gguf", CreatedAt: now}
	db.Policies["pol"] = PlacementPolicy{ID: "pol", ProfileID: profile.ID, CachedCount: 1, WarmCount: 1, UpdatedAt: now}
	db.Nodes["node-a"] = Node{ID: "node-a", State: NodeStateOnline, Approved: true, RuntimeAdapters: []string{"llama.cpp-metal"}, CachedArtifacts: []string{"sha256:model"}}
	db.Slots["node-a/slot0"] = Slot{ID: "node-a/slot0", NodeID: "node-a", State: SlotStateReady, ProfileID: profile.ID, RuntimeVariantID: profile.ID, ModelArtifactID: "sha256:model", ModelSHA256: "sha256:model", AcceptsNew: true}
	db.Assignments["asg"] = PlacementAssignment{ID: "asg", ProfileID: profile.ID, NodeID: "node-a", SlotID: "node-a/slot0", DesiredCached: true, DesiredWarm: true, ActualCached: true, ActualWarm: true, Ready: true}

	plans := BuildCachePlans(db)
	decorateAdminStateConditions(&db, BuildFitMatrix(db), plans)

	if len(plans) != 1 || !cacheWorkerStatus(plans[0].Workers, "node-a").Warm {
		t.Fatalf("cache plan did not use assignment warm state: %+v", plans)
	}
	assertCondition(t, db.Policies["pol"].Conditions, "Warm", "True", "DesiredWarmCopiesReady")
}

func assertCondition(t *testing.T, conditions []Condition, typ, status, reason string) {
	t.Helper()
	for _, condition := range conditions {
		if condition.Type == typ {
			if condition.Status != status || condition.Reason != reason {
				t.Fatalf("condition %s = %+v", typ, condition)
			}
			if condition.LastTransitionTime.IsZero() {
				t.Fatalf("condition %s has zero transition time", typ)
			}
			return
		}
	}
	t.Fatalf("missing condition %s in %+v", typ, conditions)
}

func cacheWorkerStatus(workers []CacheWorkerStatus, nodeID string) CacheWorkerStatus {
	for _, worker := range workers {
		if worker.NodeID == nodeID {
			return worker
		}
	}
	return CacheWorkerStatus{}
}
