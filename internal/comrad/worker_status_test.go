package comrad

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkerStatusSnapshotReflectsSlots(t *testing.T) {
	mock := &mockWorkerP2P{
		available: false,
		snapshot:  &WorkerP2PStatus{Available: false},
	}
	worker := newMockP2PWorkerForTest(t, mock, WorkerConfig{
		NodeID:                 "node-status-slots",
		StatePath:              filepath.Join(t.TempDir(), "state.json"),
		CacheDir:               filepath.Join(t.TempDir(), "cache"),
		SlotCount:              2,
		MaxConcurrentDownloads: 1,
	})

	snap := worker.StatusSnapshot()

	if len(snap.Slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(snap.Slots))
	}
	for _, s := range snap.Slots {
		if s.ID == "" {
			t.Fatal("slot ID is empty")
		}
		if s.State != SlotStateIdle {
			t.Fatalf("expected slot state %s, got %s", SlotStateIdle, s.State)
		}
	}
}

func TestWorkerStatusSnapshotConnectedFlag(t *testing.T) {
	mock := &mockWorkerP2P{
		available: false,
		snapshot:  &WorkerP2PStatus{Available: false},
	}
	worker := newMockP2PWorkerForTest(t, mock, WorkerConfig{
		NodeID:                 "node-status-connected",
		StatePath:              filepath.Join(t.TempDir(), "state.json"),
		CacheDir:               filepath.Join(t.TempDir(), "cache"),
		SlotCount:              1,
		MaxConcurrentDownloads: 1,
	})

	snap := worker.StatusSnapshot()
	if snap.Connected {
		t.Fatal("expected Connected=false for fresh worker")
	}

	worker.mu.Lock()
	worker.connected = true
	worker.mu.Unlock()

	snap = worker.StatusSnapshot()
	if !snap.Connected {
		t.Fatal("expected Connected=true after setting worker.connected")
	}
}

func TestWorkerStatusSnapshotCachedAndWarmCounts(t *testing.T) {
	mock := &mockWorkerP2P{
		available: false,
		snapshot:  &WorkerP2PStatus{Available: false},
	}
	worker := newMockP2PWorkerForTest(t, mock, WorkerConfig{
		NodeID:                 "node-status-counts",
		StatePath:              filepath.Join(t.TempDir(), "state.json"),
		CacheDir:               filepath.Join(t.TempDir(), "cache"),
		SlotCount:              1,
		MaxConcurrentDownloads: 1,
	})

	worker.mu.Lock()
	worker.cache["artifact-1"] = "/path/a"
	worker.cache["artifact-2"] = "/path/b"
	worker.warm["profile-1"] = WorkloadProfile{ID: "profile-1"}
	worker.mu.Unlock()

	snap := worker.StatusSnapshot()
	if snap.CachedCount != 2 {
		t.Fatalf("expected CachedCount=2, got %d", snap.CachedCount)
	}
	if snap.WarmCount != 1 {
		t.Fatalf("expected WarmCount=1, got %d", snap.WarmCount)
	}
}

func TestWorkerServeStatusEndpoint(t *testing.T) {
	mock := &mockWorkerP2P{
		available: false,
		snapshot:  &WorkerP2PStatus{Available: false},
	}
	worker := newMockP2PWorkerForTest(t, mock, WorkerConfig{
		NodeID:                 "node-status-http",
		StatePath:              filepath.Join(t.TempDir(), "state.json"),
		CacheDir:               filepath.Join(t.TempDir(), "cache"),
		SlotCount:              1,
		MaxConcurrentDownloads: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := startStatusServer(t, ctx, worker)
	checkStatusEndpoint(t, addr)
	checkHealthzEndpoint(t, addr)
}

func startStatusServer(t *testing.T, ctx context.Context, w *Worker) string {
	t.Helper()
	addrCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		if err := w.serveStatus(ctx, "127.0.0.1:0", addrCh); err != nil {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		t.Fatalf("serveStatus failed: %v", err)
	case addr := <-addrCh:
		return addr
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for serveStatus to bind")
	}
	return ""
}

func checkStatusEndpoint(t *testing.T, addr string) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://%s/status", addr))
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /status status = %d, want 200", resp.StatusCode)
	}
	var snap WorkerStatusSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode /status JSON: %v", err)
	}
	if snap.NodeID == "" {
		t.Fatal("snapshot NodeID is empty")
	}
}

func checkHealthzEndpoint(t *testing.T, addr string) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want 200", resp.StatusCode)
	}
}

func TestWorkerServeStatusRejectsNonLoopback(t *testing.T) {
	mock := &mockWorkerP2P{
		available: false,
		snapshot:  &WorkerP2PStatus{Available: false},
	}
	worker := newMockP2PWorkerForTest(t, mock, WorkerConfig{
		NodeID:                 "node-status-reject",
		StatePath:              filepath.Join(t.TempDir(), "state.json"),
		CacheDir:               filepath.Join(t.TempDir(), "cache"),
		SlotCount:              1,
		MaxConcurrentDownloads: 1,
	})

	ctx := context.Background()
	err := worker.serveStatus(ctx, "0.0.0.0:0", nil)
	if err == nil {
		t.Fatal("expected error for non-loopback address, got nil")
	}
}
