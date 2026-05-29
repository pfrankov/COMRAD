package comrad

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestManagerWorkerStreamingPath(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("local acceptance path targets darwin arm64 metal")
	}
	h := newIntegrationHarness(t)
	defer h.cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	done := runWorkerForTest(newIntegrationWorker(t, h), ctx)
	defer stopWorkerForTest(cancel, done)
	waitForProfileReady(t, h)
	stream := streamChatForTest(t, h.server.URL)
	if !strings.Contains(stream, "first") || !strings.Contains(stream, "second") {
		t.Fatalf("stream did not include generated tokens: %s", stream)
	}
	waitForCompletedReport(t, h.server.URL)
}

type integrationHarness struct {
	dir        string
	server     *httptest.Server
	profile    WorkloadProfile
	serverPath string
}

func newIntegrationHarness(t *testing.T) integrationHarness {
	t.Helper()
	dir, modelPath, serverPath := integrationTempArtifacts(t)
	t.Setenv("COMRAD_FAKE_LLAMA_SERVER_BIN", os.Args[0])
	t.Setenv("COMRAD_FAKE_LLAMA_SERVER_ARGS_PATH", filepath.Join(dir, "server-args.txt"))
	t.Setenv("COMRAD_FAKE_LLAMA_SERVER_MODE", "")
	manager, err := newIntegrationManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(manager.Handler())
	profile := registerTinyProfile(t, server.URL, modelPath)
	return integrationHarness{dir: dir, server: server, profile: profile, serverPath: serverPath}
}

func (h integrationHarness) cleanup() {
	h.server.Close()
	_ = os.RemoveAll(h.dir)
}

func integrationTempArtifacts(t *testing.T) (string, string, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "comrad-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(dir, "tiny.gguf")
	if err := os.WriteFile(modelPath, []byte("GGUF tiny test model"), 0o644); err != nil {
		t.Fatal(err)
	}
	serverPath := writeFakeLlamaServer(t, dir)
	return dir, modelPath, serverPath
}

func newIntegrationManager(dir string) (*Manager, error) {
	return NewManager(ManagerConfig{DBPath: filepath.Join(dir, "manager.json"), ArtifactDir: filepath.Join(dir, "artifacts"), AdminToken: "admin", ClientAPIKey: "client", WorkerToken: "worker", AutoApprove: true, QueueLimit: 2, StreamWait: 5 * time.Second})
}

func registerTinyProfile(t *testing.T, baseURL, modelPath string) WorkloadProfile {
	t.Helper()
	artifact := adminJSON[Artifact](t, baseURL, "admin", "/api/admin/artifacts", CreateArtifactRequest{Path: modelPath, Kind: "model_gguf", Name: "tiny.gguf"})
	profile := adminJSON[WorkloadProfile](t, baseURL, "admin", "/api/admin/profiles", tinyProfileRequest(artifact))
	_ = adminJSON[PlacementPolicy](t, baseURL, "admin", "/api/admin/policies", UpsertPolicyRequest{ProfileID: profile.ID, CachedCount: 1, WarmCount: 1})
	return profile
}

func tinyProfileRequest(artifact Artifact) CreateProfileRequest {
	return CreateProfileRequest{
		ID:             "llm.chat/tiny/context-4096",
		Name:           "tiny",
		Alias:          "assistant-default",
		Kind:           "llm.chat",
		RuntimeAdapter: "llama.cpp-metal",
		Artifacts:      []string{artifact.ID},
		Requirements:   &Requirements{Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal", UnifiedMemoryBytes: 1, DiskBytes: 1},
		LLM:            &LLMProfile{ContextTokens: 4096},
		Warmable:       true,
	}
}

func newIntegrationWorker(t *testing.T, h integrationHarness) *Worker {
	t.Helper()
	worker, err := NewWorker(WorkerConfig{ManagerURL: h.server.URL, Token: "worker", StatePath: filepath.Join(h.dir, "worker-state.json"), CacheDir: filepath.Join(h.dir, "cache"), LlamaServerPath: h.serverPath, RuntimeStartWait: 2 * time.Second, SlotCount: 1, RAMBytes: 8 << 30, UnifiedBytes: 8 << 30, DiskBytes: 8 << 30})
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func runWorkerForTest(worker *Worker, ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = worker.Run(ctx)
	}()
	return done
}

func stopWorkerForTest(cancel context.CancelFunc, done <-chan struct{}) {
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

func waitForProfileReady(t *testing.T, h integrationHarness) {
	t.Helper()
	waitFor(t, 5*time.Second, func() bool {
		state := getState(t, h.server.URL, "admin")
		for _, slot := range state.Slots {
			if slot.ProfileID == h.profile.ID && slot.State == SlotStateReady {
				return true
			}
		}
		return false
	})
}

func streamChatForTest(t *testing.T, baseURL string) string {
	t.Helper()
	req := newChatRequest(t, baseURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chat status %d", resp.StatusCode)
	}
	return readSSEUntilDone(t, resp)
}

func newChatRequest(t *testing.T, baseURL string) *http.Request {
	t.Helper()
	body := bytes.NewBuffer(nil)
	err := json.NewEncoder(body).Encode(ChatCompletionRequest{Model: "assistant-default", Stream: true, MaxTokens: 32, Messages: []ChatMessage{{Role: "user", Content: "say hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer client")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func readSSEUntilDone(t *testing.T, resp *http.Response) string {
	t.Helper()
	var stream strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		stream.WriteString(line)
		if line == "data: [DONE]" {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return stream.String()
}

func waitForCompletedReport(t *testing.T, baseURL string) {
	t.Helper()
	waitFor(t, 3*time.Second, func() bool {
		state := getState(t, baseURL, "admin")
		for _, report := range state.Reports {
			if report.Status == TaskStatusCompleted {
				return true
			}
		}
		return false
	})
}

func adminJSON[T any](t *testing.T, baseURL, token, path string, in any) T {
	return adminMethodJSON[T](t, http.MethodPost, baseURL, token, path, in)
}

func adminMethodJSON[T any](t *testing.T, method, baseURL, token, path string, in any) T {
	t.Helper()
	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(in); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("admin %s %s returned %d", method, path, resp.StatusCode)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func getState(t *testing.T, baseURL, token string) StateResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/admin/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("state returned %d", resp.StatusCode)
	}
	var state StateResponse
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func waitFor(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition timed out")
}
