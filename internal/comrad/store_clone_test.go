package comrad

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreSnapshotDeepCopiesMutableFields(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "clone.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.Update(func(db *Database) error {
		db.Nodes["node-a"] = Node{ID: "node-a", Tags: []string{"fast"}, CachedArtifacts: []string{"sha256:a"}, ConnectedSession: "session-a"}
		db.Slots["slot-a"] = Slot{ID: "slot-a", FailureCounters: map[string]int{"runtime": 1}, LastFailureAt: &now}
		db.Profiles["profile-a"] = WorkloadProfile{
			ID:           "profile-a",
			Artifacts:    []string{"sha256:a"},
			Requirements: &Requirements{RequireTags: []string{"fast"}},
			Runtime:      RuntimeParameters{LlamaCpp: LlamaCppParameters{Args: []string{"-ngl", "99"}}},
			RuntimeVariants: []RuntimeModelVariant{{
				ID:           "variant-a",
				Artifacts:    []string{"sha256:a"},
				Requirements: &Requirements{RequireTags: []string{"fast"}},
				Metadata:     map[string]string{"tier": "test"},
			}},
		}
		db.Policies["policy-a"] = PlacementPolicy{ID: "policy-a", Constraints: PlacementConstraints{RequireTags: []string{"fast"}}, HardPinnedSlots: []string{"slot-a"}}
		db.Tasks["task-a"] = Task{ID: "task-a", FailedSlots: []string{"slot-a"}, Metadata: map[string]any{"reason": "test"}, CompletedAt: &now}
		db.Attempts["attempt-a"] = Attempt{ID: "attempt-a", FirstOutputAt: &now, CompletedAt: &now}
		db.Updates["update-a"] = UpdateRecord{ID: "update-a", TargetNodes: []string{"node-a"}}
		db.APIKeys["key-a"] = APIKey{ID: "key-a", LastUsedAt: &now}
		db.ComputeLedger = append(db.ComputeLedger, ComputeLedgerEntry{ID: "ledger-a"})
		db.Audit = append(db.Audit, AuditEvent{ID: "audit-a", Metadata: map[string]any{"reason": "test"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	snapshot := store.Snapshot()
	snapshot.Nodes["node-a"].Tags[0] = "mutated"
	snapshot.Nodes["node-a"].CachedArtifacts[0] = "sha256:mutated"
	snapshot.Slots["slot-a"].FailureCounters["runtime"] = 9
	snapshot.Profiles["profile-a"].Artifacts[0] = "sha256:mutated"
	snapshot.Profiles["profile-a"].Requirements.RequireTags[0] = "mutated"
	snapshot.Profiles["profile-a"].Runtime.LlamaCpp.Args[1] = "1"
	snapshot.Profiles["profile-a"].RuntimeVariants[0].Metadata["tier"] = "mutated"
	snapshot.Policies["policy-a"].Constraints.RequireTags[0] = "mutated"
	snapshot.Tasks["task-a"].FailedSlots[0] = "mutated"
	snapshot.Tasks["task-a"].Metadata["reason"] = "mutated"
	snapshot.Updates["update-a"].TargetNodes[0] = "mutated"
	snapshot.Audit[len(snapshot.Audit)-1].Metadata["reason"] = "mutated"

	fresh := store.Snapshot()
	if fresh.Nodes["node-a"].ConnectedSession != "" {
		t.Fatalf("snapshot exposed connected session: %+v", fresh.Nodes["node-a"])
	}
	if fresh.Nodes["node-a"].Tags[0] != "fast" ||
		fresh.Nodes["node-a"].CachedArtifacts[0] != "sha256:a" ||
		fresh.Slots["slot-a"].FailureCounters["runtime"] != 1 ||
		fresh.Profiles["profile-a"].Artifacts[0] != "sha256:a" ||
		fresh.Profiles["profile-a"].Requirements.RequireTags[0] != "fast" ||
		fresh.Profiles["profile-a"].Runtime.LlamaCpp.Args[1] != "99" ||
		fresh.Profiles["profile-a"].RuntimeVariants[0].Metadata["tier"] != "test" ||
		fresh.Policies["policy-a"].Constraints.RequireTags[0] != "fast" ||
		fresh.Tasks["task-a"].FailedSlots[0] != "slot-a" ||
		fresh.Tasks["task-a"].Metadata["reason"] != "test" ||
		fresh.Updates["update-a"].TargetNodes[0] != "node-a" ||
		fresh.Audit[len(fresh.Audit)-1].Metadata["reason"] != "test" {
		t.Fatalf("snapshot mutation leaked into store: %+v", fresh)
	}
}

func BenchmarkStoreSnapshotLargeState(b *testing.B) {
	store, err := OpenStore(filepath.Join(b.TempDir(), "large.json"))
	if err != nil {
		b.Fatal(err)
	}
	seedLargeStoreState(b, store, 2000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db := store.Snapshot()
		if len(db.Tasks) != 2000 {
			b.Fatalf("tasks = %d", len(db.Tasks))
		}
	}
}

func seedLargeStoreState(tb testing.TB, store *Store, count int) {
	tb.Helper()
	now := time.Now().UTC()
	if err := store.Update(func(db *Database) error {
		db.Nodes["node-a"] = Node{ID: "node-a", Tags: []string{"fast"}, CachedArtifacts: []string{"sha256:a"}}
		db.Slots["node-a/slot0"] = Slot{ID: "node-a/slot0", NodeID: "node-a", FailureCounters: map[string]int{"runtime": 1}}
		db.Profiles["profile-a"] = WorkloadProfile{ID: "profile-a", Artifacts: []string{"sha256:a"}, Requirements: &Requirements{RequireTags: []string{"fast"}}}
		for i := 0; i < count; i++ {
			id := NewID("task")
			attID := NewID("att")
			repID := NewID("rep")
			db.Tasks[id] = Task{ID: id, UserID: "user-a", Status: TaskStatusCompleted, CreatedAt: now, UpdatedAt: now, CompletedAt: &now, FailedSlots: []string{"node-b/slot0"}}
			db.Attempts[attID] = Attempt{ID: attID, TaskID: id, NodeID: "node-a", SlotID: "node-a/slot0", Status: TaskStatusCompleted, FirstOutputAt: &now, CompletedAt: &now}
			db.Reports[repID] = ComputeReport{ID: repID, TaskID: id, AttemptID: attID, NodeID: "node-a", SlotID: "node-a/slot0", Status: TaskStatusCompleted, CreatedAt: now}
		}
		return nil
	}); err != nil {
		tb.Fatal(err)
	}
}
