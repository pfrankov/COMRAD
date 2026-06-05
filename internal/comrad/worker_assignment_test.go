package comrad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWorkerSerializesCachedOnlyDownloadsWithDefaultLimit(t *testing.T) {
	server := newArtifactGateServer(t, map[string]string{
		"model-a.gguf": "model-a",
		"model-b.gguf": "model-b",
	})
	defer server.Close()
	worker := newDownloadAssignmentWorker(t, 0, 1)
	payloadA := cachedOnlyDownloadPayload("llm.chat/a", server.spec("model-a.gguf"))
	payloadB := cachedOnlyDownloadPayload("llm.chat/b", server.spec("model-b.gguf"))

	doneA := runAssignment(t, worker, payloadA)
	server.waitStarted(t, "model-a.gguf")
	doneB := runAssignment(t, worker, payloadB)

	server.assertNotStarted(t, "model-b.gguf")
	server.release("model-a.gguf")
	waitAssignment(t, doneA)
	server.waitStarted(t, "model-b.gguf")
	server.release("model-b.gguf")
	waitAssignment(t, doneB)

	if server.maxActive() != 1 {
		t.Fatalf("max active downloads = %d, want 1", server.maxActive())
	}
}

func TestWarmAssignmentQueuesBehindDownloadPressureOnAssignedSlot(t *testing.T) {
	server := newArtifactGateServer(t, map[string]string{
		"cached.gguf": "cached-only",
		"warm.gguf":   "warm-model",
	})
	defer server.Close()
	worker := newDownloadAssignmentWorker(t, 1, 2)
	cached := cachedOnlyDownloadPayload("llm.chat/cached", server.spec("cached.gguf"))
	warm := warmDownloadPayload("llm.chat/warm", server.spec("warm.gguf"), worker.node.Target)
	warm.SlotID = "node-a/metal1"

	doneCached := runAssignment(t, worker, cached)
	server.waitStarted(t, "cached.gguf")
	doneWarm := runAssignmentAllowError(worker, warm)

	waitForWorkerSlotState(t, worker, "node-a/metal1", SlotStateDownloadQueued)
	slot0, _ := worker.getSlot("node-a/metal0")
	if slot0.State != SlotStateIdle {
		t.Fatalf("unassigned slot state = %s, want %s", slot0.State, SlotStateIdle)
	}
	server.assertNotStarted(t, "warm.gguf")

	server.release("cached.gguf")
	waitAssignment(t, doneCached)
	server.waitStarted(t, "warm.gguf")
	server.release("warm.gguf")
	waitAssignmentAllowError(t, doneWarm)
}

func TestWorkerConfigDefaultsToOneConcurrentModelDownload(t *testing.T) {
	worker := newDownloadAssignmentWorker(t, 0, 1)
	if worker.cfg.MaxConcurrentDownloads != 1 {
		t.Fatalf("MaxConcurrentDownloads = %d, want 1", worker.cfg.MaxConcurrentDownloads)
	}
	if worker.node.DownloadPressure.MaxConcurrent != 1 {
		t.Fatalf("node max concurrent downloads = %d, want 1", worker.node.DownloadPressure.MaxConcurrent)
	}
}

type artifactGateServer struct {
	server *httptest.Server
	gates  map[string]*artifactGate
	active int
	peak   int
	mu     sync.Mutex
}

type artifactGate struct {
	id      string
	body    string
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newArtifactGateServer(t *testing.T, bodies map[string]string) *artifactGateServer {
	t.Helper()
	g := &artifactGateServer{gates: map[string]*artifactGate{}}
	for name, body := range bodies {
		sha := sha256ForTest(t, body)
		g.gates[name] = &artifactGate{id: sha, body: body, started: make(chan struct{}), release: make(chan struct{})}
	}
	g.server = httptest.NewServer(http.HandlerFunc(g.handle))
	return g
}

func (g *artifactGateServer) Close() {
	g.server.Close()
}

func (g *artifactGateServer) spec(name string) ArtifactSpec {
	gate := g.gates[name]
	return ArtifactSpec{
		ID:     gate.id,
		Name:   name,
		Kind:   "model_gguf",
		SHA256: gate.id,
		URL:    g.server.URL + "/artifacts/" + gate.id,
	}
}

func (g *artifactGateServer) handle(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/artifacts/")
	gate := g.gateByID(id)
	if gate == nil {
		http.NotFound(w, r)
		return
	}
	gate.once.Do(func() { close(gate.started) })
	g.mu.Lock()
	g.active++
	if g.active > g.peak {
		g.peak = g.active
	}
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.active--
		g.mu.Unlock()
	}()
	select {
	case <-gate.release:
	case <-r.Context().Done():
		return
	}
	_, _ = w.Write([]byte(gate.body))
}

func (g *artifactGateServer) gateByID(id string) *artifactGate {
	for _, gate := range g.gates {
		if gate.id == id {
			return gate
		}
	}
	return nil
}

func (g *artifactGateServer) waitStarted(t *testing.T, name string) {
	t.Helper()
	select {
	case <-g.gates[name].started:
	case <-time.After(time.Second):
		t.Fatalf("artifact %s did not start downloading", name)
	}
}

func (g *artifactGateServer) assertNotStarted(t *testing.T, name string) {
	t.Helper()
	select {
	case <-g.gates[name].started:
		t.Fatalf("artifact %s started before download pressure cleared", name)
	case <-time.After(100 * time.Millisecond):
	}
}

func (g *artifactGateServer) release(name string) {
	close(g.gates[name].release)
}

func (g *artifactGateServer) maxActive() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.peak
}

func newDownloadAssignmentWorker(t *testing.T, maxDownloads, slots int) *Worker {
	t.Helper()
	dir := t.TempDir()
	worker, err := NewWorker(WorkerConfig{
		NodeID:                 "node-a",
		StatePath:              filepath.Join(dir, "state.json"),
		CacheDir:               filepath.Join(dir, "cache"),
		SlotCount:              slots,
		MaxConcurrentDownloads: maxDownloads,
	})
	if err != nil {
		t.Fatal(err)
	}
	worker.send = make(chan Envelope, 64)
	return worker
}

func cachedOnlyDownloadPayload(profileID string, artifact ArtifactSpec) AssignmentPayload {
	return AssignmentPayload{
		Profile:   downloadTestProfile(profileID, artifact, ""),
		Artifacts: []ArtifactSpec{artifact},
		Cached:    true,
		Warm:      false,
	}
}

func warmDownloadPayload(profileID string, artifact ArtifactSpec, target string) AssignmentPayload {
	return AssignmentPayload{
		Profile:   downloadTestProfile(profileID, artifact, target),
		Artifacts: []ArtifactSpec{artifact},
		Cached:    true,
		Warm:      true,
	}
}

func downloadTestProfile(profileID string, artifact ArtifactSpec, target string) WorkloadProfile {
	return WorkloadProfile{
		ID:        profileID,
		Version:   1,
		Name:      profileID,
		Alias:     profileID,
		Kind:      "llm.chat",
		Artifacts: []string{artifact.ID},
		Requirements: &Requirements{
			Target:    target,
			DiskBytes: 1,
		},
		LLM:      &LLMProfile{ContextTokens: 128},
		Warmable: true,
	}
}

func runAssignment(t *testing.T, worker *Worker, payload AssignmentPayload) <-chan error {
	t.Helper()
	return runAssignmentAllowError(worker, payload)
}

func runAssignmentAllowError(worker *Worker, payload AssignmentPayload) <-chan error {
	done := make(chan error, 1)
	go func() { done <- worker.handleAssignment(context.Background(), payload) }()
	return done
}

func waitAssignment(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("assignment did not finish")
	}
}

func waitAssignmentAllowError(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("assignment did not finish")
	}
}

func waitForWorkerSlotState(t *testing.T, worker *Worker, slotID, state string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		slot, _ := worker.getSlot(slotID)
		if slot.State == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	slot, _ := worker.getSlot(slotID)
	t.Fatalf("slot %s state = %s, want %s", slotID, slot.State, state)
}

func sha256ForTest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, _, err := FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha
}
