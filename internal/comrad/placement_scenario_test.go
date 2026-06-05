package comrad

import (
	"slices"
	"testing"
	"time"
)

func TestPlanPlacementMultiWorkerCapacityScenarioReplansUnavailableNode(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	db := placementTestDatabase(now)
	addPlacementProfile(&db, "giant-scarce", "sha256:giant", 12, 12, now)
	addPlacementProfile(&db, "medium-steady", "sha256:medium", 8, 8, now)
	addPlacementProfile(&db, "archive-cached", "sha256:archive", 6, 6, now)
	addPlacementProfile(&db, "hot-small", "sha256:hot", 2, 2, now)
	addPlacementPolicy(&db, "giant-scarce", PlacementPolicy{ID: "pol-giant", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addPlacementPolicy(&db, "medium-steady", PlacementPolicy{ID: "pol-medium", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addPlacementPolicy(&db, "archive-cached", PlacementPolicy{ID: "pol-archive", CachedCount: 1, WarmCount: 0, CreatedAt: now, UpdatedAt: now})
	addPlacementPolicy(&db, "hot-small", PlacementPolicy{ID: "pol-hot", AutoBalance: true, MaxCachedCount: 3, MaxWarmCount: 3, CreatedAt: now, UpdatedAt: now})
	addPlacementNode(&db, "node-a-large", 16, 28, 2)
	addPlacementNode(&db, "node-b-mid", 10, 20, 2)
	addPlacementNode(&db, "node-c-small", 6, 12, 1)
	addQueuedPlacementDemand(&db, "hot-small", 3, now)

	plan := PlanPlacement(db)
	if got := placementNodeFor(plan, "giant-scarce"); got != "node-a-large" {
		t.Fatalf("giant placement node = %q, want node-a-large; plan=%+v", got, plan)
	}
	if got := placementNodeFor(plan, "medium-steady"); got != "node-b-mid" {
		t.Fatalf("medium placement node = %q, want node-b-mid; plan=%+v", got, plan)
	}
	if got := cachedOnlyPlacement(plan, "archive-cached"); got.NodeID != "node-a-large" || got.SlotID != "" {
		t.Fatalf("cached-only placement = %+v, want node-level cache on node-a-large without a warm slot", got)
	}
	assertWarmPlacementNodes(t, plan, "hot-small", []string{"node-a-large", "node-b-mid", "node-c-small"})

	node := db.Nodes["node-c-small"]
	node.State = NodeStateOffline
	db.Nodes[node.ID] = node
	slot := db.Slots["node-c-small/slot0"]
	slot.State = SlotStateUnavailable
	db.Slots[slot.ID] = slot

	replanned := PlanPlacement(db)
	assertNoAssignmentsOnNode(t, replanned, "node-c-small")
	assertWarmPlacementNodes(t, replanned, "hot-small", []string{"node-a-large", "node-b-mid"})
	if got := missingPlacementCount(replanned, "hot-small"); got != 1 {
		t.Fatalf("hot-small missing placements = %d, want 1 after node-c-small goes offline; plan=%+v", got, replanned)
	}
}

func addQueuedPlacementDemand(db *Database, profileID string, count int, now time.Time) {
	for i := 0; i < count; i++ {
		id := NewID("task")
		db.Tasks[id] = Task{ID: id, ProfileID: profileID, Status: TaskStatusQueued, CreatedAt: now, UpdatedAt: now}
	}
}

func cachedOnlyPlacement(plan []PlacementAssignment, profileID string) PlacementAssignment {
	for _, assignment := range plan {
		if assignment.ProfileID == profileID && assignment.DesiredCached && !assignment.DesiredWarm {
			return assignment
		}
	}
	return PlacementAssignment{}
}

func assertWarmPlacementNodes(t *testing.T, plan []PlacementAssignment, profileID string, want []string) {
	t.Helper()
	got := warmPlacementNodes(plan, profileID)
	if !slices.Equal(got, want) {
		t.Fatalf("%s warm placement nodes = %v, want %v; plan=%+v", profileID, got, want, plan)
	}
}

func warmPlacementNodes(plan []PlacementAssignment, profileID string) []string {
	nodes := []string{}
	for _, assignment := range plan {
		if assignment.ProfileID == profileID && assignment.NodeID != "" && assignment.DesiredWarm && assignment.MismatchReason == "" {
			nodes = append(nodes, assignment.NodeID)
		}
	}
	slices.Sort(nodes)
	return nodes
}

func assertNoAssignmentsOnNode(t *testing.T, plan []PlacementAssignment, nodeID string) {
	t.Helper()
	for _, assignment := range plan {
		if assignment.NodeID == nodeID {
			t.Fatalf("unexpected assignment on unavailable node %s: %+v; plan=%+v", nodeID, assignment, plan)
		}
	}
}

func missingPlacementCount(plan []PlacementAssignment, profileID string) int {
	count := 0
	for _, assignment := range plan {
		if assignment.ProfileID == profileID && assignment.NodeID == "" && assignment.MismatchReason != "" {
			count++
		}
	}
	return count
}
