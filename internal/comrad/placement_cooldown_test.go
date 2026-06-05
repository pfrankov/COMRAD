package comrad

import (
	"testing"
	"time"
)

func TestAutoBalanceScaleDownWaitsForCooldown(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	db := placementTestDatabase(now)
	addPlacementProfile(&db, "hot", "sha256:hot", 2, 2, now)
	policy := PlacementPolicy{
		ID:             "pol-hot",
		ProfileID:      "hot",
		AutoBalance:    true,
		MaxCachedCount: 4,
		MaxWarmCount:   4,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	addPlacementPolicy(&db, "hot", policy)
	for i := 0; i < 21; i++ {
		created := now.Add(-autoBalanceDemandWindow - time.Minute)
		id := NewID("task")
		db.Tasks[id] = Task{
			ID:          id,
			ProfileID:   "hot",
			Status:      TaskStatusCompleted,
			CreatedAt:   created,
			UpdatedAt:   created,
			CompletedAt: &created,
		}
	}
	runningID := NewID("task")
	db.Tasks[runningID] = Task{
		ID:        runningID,
		ProfileID: "hot",
		Status:    TaskStatusRunning,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}

	capacity := EffectivePolicyCapacity(db, policy, now)
	if capacity.Warm != 2 || capacity.Cached != 2 {
		t.Fatalf("capacity during cooldown = cached:%d warm:%d, want 2/2", capacity.Cached, capacity.Warm)
	}

	delete(db.Tasks, runningID)
	afterCooldown := now.Add(defaultAutoBalanceScaleDownCooldown + time.Minute)
	capacity = EffectivePolicyCapacity(db, policy, afterCooldown)
	if capacity.Warm != 0 || capacity.Cached != 0 {
		t.Fatalf("capacity after cooldown = cached:%d warm:%d, want 0/0", capacity.Cached, capacity.Warm)
	}
}

func TestAutoBalanceScaleUpStillImmediate(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	db := placementTestDatabase(now)
	addPlacementProfile(&db, "hot", "sha256:hot", 2, 2, now)
	policy := PlacementPolicy{
		ID:             "pol-hot",
		ProfileID:      "hot",
		AutoBalance:    true,
		MaxCachedCount: 3,
		MaxWarmCount:   3,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	addPlacementPolicy(&db, "hot", policy)
	for i := 0; i < 3; i++ {
		id := NewID("task")
		db.Tasks[id] = Task{ID: id, ProfileID: "hot", Status: TaskStatusQueued, CreatedAt: now, UpdatedAt: now}
	}

	capacity := EffectivePolicyCapacity(db, policy, now)
	if capacity.Warm != 3 || capacity.Cached != 3 {
		t.Fatalf("capacity on immediate scale-up = cached:%d warm:%d, want 3/3", capacity.Cached, capacity.Warm)
	}
}

func TestPlanPlacementPreservesWarmingCopyAsDrainingDuringScaleDown(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	db := placementTestDatabase(now)
	addPlacementProfile(&db, "hot", "sha256:hot", 2, 2, now)
	addPlacementPolicy(&db, "hot", PlacementPolicy{
		ID:             "pol-hot",
		ProfileID:      "hot",
		AutoBalance:    true,
		MaxCachedCount: 1,
		MaxWarmCount:   1,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	addPlacementNode(&db, "node-a", 8, 8, 1)
	profile := db.Profiles["hot"]
	node := db.Nodes["node-a"]
	node.CachedArtifacts = []string{"sha256:hot"}
	db.Nodes[node.ID] = node
	slot := db.Slots["node-a/slot0"]
	effective := EffectiveProfileForVariant(profile, ProfileRuntimeVariants(profile)[0])
	slot.State = SlotStateWarming
	slot.ProfileID = profile.ID
	slot.ProfileVersion = profileVersion(profile)
	slot.LogicalModel = ProfileLogicalModel(effective)
	slot.RuntimeVariantID = effective.RuntimeVariantID
	slot.ModelArtifactID = ConcreteModelArtifactID(effective)
	slot.ModelSHA256 = ConcreteModelSHA256(effective)
	db.Slots[slot.ID] = slot

	plan := PlanPlacement(db)
	assignment := placementForSlot(plan, "node-a/slot0")
	if assignment.ID == "" {
		t.Fatalf("warming copy was removed from plan: %+v", plan)
	}
	if !assignment.DesiredCached || !assignment.DesiredWarm || !assignment.Draining {
		t.Fatalf("warming assignment = %+v, want cached warm draining assignment", assignment)
	}
}

func placementForSlot(plan []PlacementAssignment, slotID string) PlacementAssignment {
	for _, assignment := range plan {
		if assignment.SlotID == slotID {
			return assignment
		}
	}
	return PlacementAssignment{}
}
