package comrad

import (
	"testing"
	"time"
)

func TestPlanPlacementReservesLargestScarceProfileFirst(t *testing.T) {
	now := time.Now().UTC()
	db := placementTestDatabase(now)
	addPlacementProfile(&db, "zz-big", "sha256:big", 12, 12, now)
	addPlacementProfile(&db, "aa-small", "sha256:small", 2, 2, now)
	addPlacementPolicy(&db, "aa-small", PlacementPolicy{ID: "pol-aa-small", ProfileID: "aa-small", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addPlacementPolicy(&db, "zz-big", PlacementPolicy{ID: "pol-zz-big", ProfileID: "zz-big", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addPlacementNode(&db, "node-a-power", 16, 30, 1)
	addPlacementNode(&db, "node-b-weak", 4, 10, 1)

	plan := PlanPlacement(db)

	if got := placementNodeFor(plan, "zz-big"); got != "node-a-power" {
		t.Fatalf("big profile node = %q, want node-a-power; plan=%+v", got, plan)
	}
	if got := placementNodeFor(plan, "aa-small"); got != "node-b-weak" {
		t.Fatalf("small profile node = %q, want node-b-weak; plan=%+v", got, plan)
	}
}

func TestPlanPlacementUsesAggregateNodeBudgets(t *testing.T) {
	now := time.Now().UTC()
	db := placementTestDatabase(now)
	addPlacementProfile(&db, "alpha", "sha256:alpha", 8, 8, now)
	addPlacementProfile(&db, "beta", "sha256:beta", 8, 8, now)
	addPlacementPolicy(&db, "alpha", PlacementPolicy{ID: "pol-alpha", ProfileID: "alpha", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addPlacementPolicy(&db, "beta", PlacementPolicy{ID: "pol-beta", ProfileID: "beta", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addPlacementNode(&db, "node-a", 12, 30, 2)

	plan := PlanPlacement(db)

	if got := desiredWarmOnNode(plan, "node-a"); got != 1 {
		t.Fatalf("warm placements on node-a = %d, want 1 due aggregate RAM budget; plan=%+v", got, plan)
	}
	if !hasMissingPlacement(plan) {
		t.Fatalf("expected one missing placement due aggregate RAM budget; plan=%+v", plan)
	}
}

func TestCachedOnlyPlacementDoesNotOccupyWarmSlot(t *testing.T) {
	now := time.Now().UTC()
	db := placementTestDatabase(now)
	addPlacementProfile(&db, "warm", "sha256:warm", 4, 4, now)
	addPlacementProfile(&db, "cached", "sha256:cached", 4, 4, now)
	addPlacementPolicy(&db, "warm", PlacementPolicy{ID: "pol-warm", ProfileID: "warm", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addPlacementPolicy(&db, "cached", PlacementPolicy{ID: "pol-cached", ProfileID: "cached", CachedCount: 1, WarmCount: 0, CreatedAt: now, UpdatedAt: now})
	addPlacementNode(&db, "node-a", 16, 30, 1)

	plan := PlanPlacement(db)
	cached := placementForProfile(plan, "cached")
	if cached.NodeID != "node-a" || cached.SlotID != "" || !cached.DesiredCached || cached.DesiredWarm {
		t.Fatalf("cached-only assignment = %+v, want node-level cached assignment without warm slot", cached)
	}
}

func TestAutoBalanceRespectsLimitsAndDoesNotStealMinimumCapacity(t *testing.T) {
	now := time.Now().UTC()
	db := placementTestDatabase(now)
	addPlacementProfile(&db, "big", "sha256:big", 12, 12, now)
	addPlacementProfile(&db, "hot", "sha256:hot", 2, 2, now)
	addPlacementPolicy(&db, "big", PlacementPolicy{ID: "pol-big", ProfileID: "big", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addPlacementPolicy(&db, "hot", PlacementPolicy{ID: "pol-hot", ProfileID: "hot", AutoBalance: true, MinWarmCount: 0, MaxWarmCount: 2, MinCachedCount: 0, MaxCachedCount: 2, CreatedAt: now, UpdatedAt: now})
	addPlacementNode(&db, "node-a-power", 12, 30, 1)
	addPlacementNode(&db, "node-b-small", 4, 10, 1)
	for i := 0; i < 8; i++ {
		id := NewID("task")
		db.Tasks[id] = Task{ID: id, ProfileID: "hot", Status: TaskStatusCompleted, CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute)}
	}

	plan := PlanPlacement(db)

	if got := placementNodeFor(plan, "big"); got != "node-a-power" {
		t.Fatalf("big minimum placement node = %q, want node-a-power; plan=%+v", got, plan)
	}
	if got := desiredWarmForProfile(plan, "hot"); got != 2 {
		t.Fatalf("hot desired warm = %d, want 2 from auto-balance demand cap; plan=%+v", got, plan)
	}
	if got := warmReadyNodeCount(plan, "hot"); got != 1 {
		t.Fatalf("hot concrete warm nodes = %d, want 1 after preserving big minimum; plan=%+v", got, plan)
	}
}

func TestHeartbeatExpiredWorkerIsExcludedAndReplanned(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	if err := m.store.Update(func(db *Database) error {
		db.Policies["pol"] = PlacementPolicy{ID: "pol", ProfileID: profile.ID, CachedCount: 1, WarmCount: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		node := db.Nodes["node-a"]
		node.LastSeen = time.Now().UTC().Add(-2 * time.Minute)
		db.Nodes[node.ID] = node
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	m.expireWorkerHeartbeats(time.Now().UTC())

	state := m.store.Snapshot()
	if state.Nodes["node-a"].State != NodeStateOffline {
		t.Fatalf("node state = %s, want offline", state.Nodes["node-a"].State)
	}
	if state.Slots["node-a/slot0"].State != SlotStateUnavailable {
		t.Fatalf("slot state = %s, want unavailable", state.Slots["node-a/slot0"].State)
	}
	if _, _, _, ok := m.selectReadySlot(profile, "task-new"); ok {
		t.Fatal("expired worker was still schedulable")
	}
	select {
	case <-session.done:
	default:
		t.Fatal("expired session was not closed")
	}
}

func placementTestDatabase(now time.Time) Database {
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

func addPlacementProfile(db *Database, id, artifact string, memoryGiB, diskGiB int64, now time.Time) {
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

func addPlacementPolicy(db *Database, profileID string, policy PlacementPolicy) {
	if policy.ID == "" {
		policy.ID = "pol-" + profileID
	}
	policy.ProfileID = profileID
	db.Policies[policy.ID] = policy
}

func addPlacementNode(db *Database, id string, memoryGiB, diskGiB int64, slots int) {
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

func placementNodeFor(plan []PlacementAssignment, profileID string) string {
	for _, assignment := range plan {
		if assignment.ProfileID == profileID && assignment.NodeID != "" && assignment.DesiredWarm && assignment.MismatchReason == "" {
			return assignment.NodeID
		}
	}
	return ""
}

func placementForProfile(plan []PlacementAssignment, profileID string) PlacementAssignment {
	for _, assignment := range plan {
		if assignment.ProfileID == profileID && assignment.NodeID != "" {
			return assignment
		}
	}
	return PlacementAssignment{}
}

func desiredWarmOnNode(plan []PlacementAssignment, nodeID string) int {
	count := 0
	for _, assignment := range plan {
		if assignment.NodeID == nodeID && assignment.DesiredWarm {
			count++
		}
	}
	return count
}

func desiredWarmForProfile(plan []PlacementAssignment, profileID string) int {
	count := 0
	for _, assignment := range plan {
		if assignment.ProfileID == profileID && assignment.DesiredWarm {
			count++
		}
	}
	return count
}

func warmReadyNodeCount(plan []PlacementAssignment, profileID string) int {
	count := 0
	for _, assignment := range plan {
		if assignment.ProfileID == profileID && assignment.DesiredWarm && assignment.NodeID != "" && assignment.MismatchReason == "" {
			count++
		}
	}
	return count
}

func hasMissingPlacement(plan []PlacementAssignment) bool {
	for _, assignment := range plan {
		if assignment.NodeID == "" && assignment.MismatchReason == "insufficient_compatible_slots" {
			return true
		}
	}
	return false
}
