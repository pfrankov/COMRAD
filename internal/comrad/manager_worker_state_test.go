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
