package comrad

import (
	"testing"
	"time"
)

func TestNormalizeWorkerNodeDoesNotInventRuntimeAdapters(t *testing.T) {
	node := normalizeWorkerNode(Node{ID: "node-a", OS: "darwin", Arch: "arm64"}, "session-a", nil, nil, time.Now().UTC())
	if node.Target != TargetDarwinArm64Metal {
		t.Fatalf("target = %q, want %q", node.Target, TargetDarwinArm64Metal)
	}
	if len(node.RuntimeAdapters) != 0 {
		t.Fatalf("runtime adapters = %+v, want none when Worker did not report one", node.RuntimeAdapters)
	}

	slot := normalizeWorkerSlot(Slot{ID: "node-a/slot0"}, node)
	if slot.RuntimeAdapter != "" {
		t.Fatalf("slot runtime adapter = %q, want empty without reported node adapter", slot.RuntimeAdapter)
	}
}

func TestCopyNodeFailureStatePreservesFreshP2P(t *testing.T) {
	freshP2P := &WorkerP2PStatus{
		Available:              true,
		Port:                   6881,
		MaxUploads:             8,
		DownloadTimeoutSeconds: 120,
		SeedingCount:           3,
		PeerCount:              7,
	}
	existingP2P := &WorkerP2PStatus{
		Available: false,
		Port:      6882,
	}

	node := Node{ID: "node-a", P2P: freshP2P}
	existing := Node{ID: "node-a", P2P: existingP2P, Quarantined: true, QuarantineReason: "test"}

	copyNodeFailureState(&node, existing)

	if node.P2P != freshP2P {
		t.Fatal("copyNodeFailureState overwrote fresh worker P2P with stale DB data")
	}
	if node.P2P.Available != true || node.P2P.Port != 6881 || node.P2P.SeedingCount != 3 {
		t.Fatalf("P2P data corrupted: %+v", node.P2P)
	}
	if !node.Quarantined || node.QuarantineReason != "test" {
		t.Fatal("copyNodeFailureState did not preserve quarantine state")
	}
}

func TestCopyNodeFailureStateFallsBackToExistingP2P(t *testing.T) {
	existingP2P := &WorkerP2PStatus{
		Available:  false,
		Port:       6882,
		LastFailure: "old error",
	}

	node := Node{ID: "node-a", P2P: nil}
	existing := Node{ID: "node-a", P2P: existingP2P}

	copyNodeFailureState(&node, existing)

	if node.P2P == nil {
		t.Fatal("expected existing P2P to be preserved when worker sends nil")
	}
	if node.P2P.Port != 6882 {
		t.Fatalf("expected existing P2P port 6882, got %d", node.P2P.Port)
	}
}

func TestMergeExistingNodePreservesWorkerP2POnReconnect(t *testing.T) {
	now := time.Now().UTC()
	workerP2P := &WorkerP2PStatus{Available: true, Port: 6881, SeedingCount: 5}
	staleP2P := &WorkerP2PStatus{Available: false, Port: 6882}

	incoming := Node{ID: "node-a", P2P: workerP2P}
	existing := Node{ID: "node-a", P2P: staleP2P, Quarantined: true, QuarantineReason: "old"}

	merged := mergeExistingNode(incoming, existing, true, now)

	if merged.P2P != workerP2P {
		t.Fatalf("merge overwrote worker P2P: got %+v, want %+v", merged.P2P, workerP2P)
	}
	if !merged.Quarantined {
		t.Fatal("merge lost quarantine state from existing node")
	}
}
