package comrad

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkerWarmsLlamaServerAndStreamsChat(t *testing.T) {
	worker, profile, argsPath := newRuntimeServerTestWorker(t, "")
	defer worker.stopAllRuntimeServers()
	if err := worker.handleAssignment(context.Background(), runtimeServerAssignment(profile, worker.cache)); err != nil {
		t.Fatal(err)
	}
	assertWorkerSlotReady(t, worker, profile.ID)
	assertFakeServerArgs(t, argsPath, "--model", "--mmproj", "--ctx-size 512", "-ngl 42")
	drainWorkerMessages(worker.send)

	worker.executeTask(runtimeServerExecutePayload(profile))
	tokens, report := readWorkerTokensAndReport(t, worker.send)
	if tokens != "first second" {
		t.Fatalf("tokens = %q", tokens)
	}
	if report.Status != TaskStatusCompleted || report.LLM.CompletionTokens != 4 || report.LLM.PromptTokens != 11 {
		t.Fatalf("report = %+v", report)
	}
	if report.Timing.TimeToFirstTokenMS <= 0 || report.Timing.GenerationMS != 250 || report.LLM.TokensPerSecond != 16 {
		t.Fatalf("report = %+v", report)
	}
}

func TestExecuteTaskDoesNotRetryLlamaServerFailureAfterFirstOutput(t *testing.T) {
	worker, profile, _ := newRuntimeServerTestWorker(t, "fail_after_first")
	defer worker.stopAllRuntimeServers()
	if err := worker.handleAssignment(context.Background(), runtimeServerAssignment(profile, worker.cache)); err != nil {
		t.Fatal(err)
	}
	drainWorkerMessages(worker.send)

	worker.executeTask(runtimeServerExecutePayload(profile))
	failed, report := readWorkerFailureAndReport(t, worker.send)
	if failed.CanRetry || !failed.FirstOutputSent {
		t.Fatalf("failed payload = %+v, want non-retryable after first output", failed)
	}
	if report.Status != TaskStatusFailed || report.LLM.CompletionTokens != 1 || report.Timing.TimeToFirstTokenMS <= 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestExecuteTaskRestartsLlamaServerAfterProcessExit(t *testing.T) {
	worker, profile, _ := newRuntimeServerTestWorker(t, "exit_on_chat")
	defer worker.stopAllRuntimeServers()
	if err := worker.handleAssignment(context.Background(), runtimeServerAssignment(profile, worker.cache)); err != nil {
		t.Fatal(err)
	}
	old := currentRuntimeProcess(t, worker)
	drainWorkerMessages(worker.send)

	worker.executeTask(runtimeServerExecutePayload(profile))
	failed, report := readWorkerFailureAndReport(t, worker.send)
	if !failed.CanRetry || failed.FirstOutputSent {
		t.Fatalf("failed payload = %+v, want retryable failure before output", failed)
	}
	if report.Status != TaskStatusFailed || report.CanRetry != true {
		t.Fatalf("report = %+v", report)
	}
	waitForRuntimeReplacement(t, worker, old, SlotStateReady)
}

func TestRuntimeWatcherRestartsExitedReadyLlamaServer(t *testing.T) {
	worker, profile, _ := newRuntimeServerTestWorker(t, "")
	defer worker.stopAllRuntimeServers()
	if err := worker.handleAssignment(context.Background(), runtimeServerAssignment(profile, worker.cache)); err != nil {
		t.Fatal(err)
	}
	old := currentRuntimeProcess(t, worker)
	if err := old.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeReplacement(t, worker, old, SlotStateReady)
}

func TestFakeLlamaServer(t *testing.T) {
	if os.Getenv("COMRAD_FAKE_LLAMA_SERVER") != "1" {
		return
	}
	runFakeLlamaServer()
	os.Exit(0)
}

func newRuntimeServerTestWorker(t *testing.T, mode string) (*Worker, WorkloadProfile, string) {
	t.Helper()
	dir := t.TempDir()
	serverPath := writeFakeLlamaServer(t, dir)
	argsPath := filepath.Join(dir, "server-args.txt")
	t.Setenv("COMRAD_FAKE_LLAMA_SERVER_BIN", os.Args[0])
	t.Setenv("COMRAD_FAKE_LLAMA_SERVER_ARGS_PATH", argsPath)
	t.Setenv("COMRAD_FAKE_LLAMA_SERVER_MODE", mode)
	modelPath, modelSHA := writeRuntimeArtifact(t, dir, "model.gguf", "GGUF")
	mmprojPath, mmprojSHA := writeRuntimeArtifact(t, dir, "mmproj.gguf", "MMPROJ")
	profile := runtimeServerProfile(modelSHA, mmprojSHA)
	slot := runtimeServerSlot(profile)
	worker := &Worker{
		cfg:             WorkerConfig{CacheDir: dir, LlamaServerPath: serverPath, RuntimeStartWait: 2 * time.Second},
		client:          &http.Client{Timeout: 0},
		node:            Node{ID: "node-a", State: NodeStateOnline, Approved: true, Target: TargetDarwinArm64Metal, RuntimeAdapters: []string{"llama.cpp-metal"}},
		slots:           map[string]Slot{slot.ID: slot},
		assigns:         map[string]AssignmentPayload{},
		cache:           map[string]string{modelSHA: modelPath, mmprojSHA: mmprojPath},
		warm:            map[string]WorkloadProfile{},
		active:          map[string]context.CancelFunc{},
		runtimes:        map[string]*llamaServerProcess{},
		runtimeRestarts: map[string]int{},
		send:            make(chan Envelope, 32),
	}
	return worker, profile, argsPath
}

func writeFakeLlamaServer(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "llama-server")
	script := "#!/bin/sh\nCOMRAD_FAKE_LLAMA_SERVER=1 exec \"$COMRAD_FAKE_LLAMA_SERVER_BIN\" -test.run=TestFakeLlamaServer -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeRuntimeArtifact(t *testing.T, dir, name, data string) (string, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, _, err := FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, sha
}

func runtimeServerProfile(modelSHA, mmprojSHA string) WorkloadProfile {
	return WorkloadProfile{
		ID:             "llm.chat/test/context-512",
		Version:        1,
		Name:           "test",
		Alias:          "test",
		LogicalModel:   "test",
		Kind:           "llm.chat",
		RuntimeAdapter: "llama.cpp-metal",
		Artifacts:      []string{modelSHA, mmprojSHA},
		Requirements:   &Requirements{Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal", UnifiedMemoryBytes: 1, DiskBytes: 1},
		LLM:            &LLMProfile{ContextTokens: 512},
		Runtime:        RuntimeParameters{LlamaCpp: LlamaCppParameters{Args: []string{"-ngl", "42"}}},
		Warmable:       true,
	}
}

func runtimeServerSlot(profile WorkloadProfile) Slot {
	return Slot{ID: "node-a/metal0", NodeID: "node-a", Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal", Resources: ResourceBudget{UnifiedMemoryBytes: 8, DiskBytes: 8}, State: SlotStateIdle, AcceptsNew: false}
}

func runtimeServerAssignment(profile WorkloadProfile, cache map[string]string) AssignmentPayload {
	specs := make([]ArtifactSpec, 0, len(profile.Artifacts))
	for _, id := range profile.Artifacts {
		specs = append(specs, ArtifactSpec{ID: id, Kind: artifactKind(id, profile), Name: filepath.Base(cache[id]), SHA256: id})
	}
	return AssignmentPayload{Profile: profile, Artifacts: specs, Warm: true}
}

func artifactKind(id string, profile WorkloadProfile) string {
	if len(profile.Artifacts) > 1 && id == profile.Artifacts[1] {
		return "model_mmproj"
	}
	return "model_gguf"
}

func runtimeServerExecutePayload(profile WorkloadProfile) ExecuteTaskPayload {
	return ExecuteTaskPayload{TaskID: "task-a", AttemptID: "attempt-a", SlotID: "node-a/metal0", Profile: profile, Messages: []ChatMessage{{Role: "user", Content: "hello"}}, MaxTokens: 8, Temperature: 0.2}
}

func assertWorkerSlotReady(t *testing.T, worker *Worker, profileID string) {
	t.Helper()
	slot, ok := worker.getSlot("node-a/metal0")
	if !ok || slot.State != SlotStateReady || slot.ProfileID != profileID {
		t.Fatalf("slot = %+v ready=%t", slot, ok)
	}
}

func currentRuntimeProcess(t *testing.T, worker *Worker) *llamaServerProcess {
	t.Helper()
	worker.mu.Lock()
	defer worker.mu.Unlock()
	proc := worker.runtimes["node-a/metal0"]
	if proc == nil {
		t.Fatal("runtime process is not registered")
	}
	return proc
}

func waitForRuntimeReplacement(t *testing.T, worker *Worker, old *llamaServerProcess, state string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		worker.mu.Lock()
		proc := worker.runtimes["node-a/metal0"]
		slot := worker.slots["node-a/metal0"]
		worker.mu.Unlock()
		if proc != nil && proc != old && proc.alive() && slot.State == state {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	worker.mu.Lock()
	proc := worker.runtimes["node-a/metal0"]
	slot := worker.slots["node-a/metal0"]
	worker.mu.Unlock()
	t.Fatalf("runtime was not replaced; proc=%p old=%p slot=%+v", proc, old, slot)
}

func assertFakeServerArgs(t *testing.T, path string, wants ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	args := string(data)
	for _, want := range wants {
		if !strings.Contains(args, want) {
			t.Fatalf("server args missing %q: %s", want, args)
		}
	}
}

func drainWorkerMessages(ch <-chan Envelope) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func readWorkerTokensAndReport(t *testing.T, ch <-chan Envelope) (string, ComputeReport) {
	t.Helper()
	var tokens strings.Builder
	var report ComputeReport
	deadline := time.After(time.Second)
	for report.ID == "" {
		select {
		case msg := <-ch:
			switch msg.Type {
			case MsgToken:
				var token TokenPayload
				if err := json.Unmarshal(msg.Payload, &token); err != nil {
					t.Fatal(err)
				}
				tokens.WriteString(token.Token)
			case MsgComputeReport:
				if err := json.Unmarshal(msg.Payload, &report); err != nil {
					t.Fatal(err)
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for report; tokens=%q report=%+v", tokens.String(), report)
		}
	}
	return tokens.String(), report
}

func readWorkerFailureAndReport(t *testing.T, ch <-chan Envelope) (AttemptFailedPayload, ComputeReport) {
	t.Helper()
	var failed AttemptFailedPayload
	var report ComputeReport
	deadline := time.After(time.Second)
	for failed.AttemptID == "" || report.ID == "" {
		select {
		case msg := <-ch:
			switch msg.Type {
			case MsgAttemptFailed:
				if err := json.Unmarshal(msg.Payload, &failed); err != nil {
					t.Fatal(err)
				}
			case MsgComputeReport:
				if err := json.Unmarshal(msg.Payload, &report); err != nil {
					t.Fatal(err)
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for failure and report; got failed=%+v report=%+v", failed, report)
		}
	}
	return failed, report
}

func runFakeLlamaServer() {
	args := argsAfterDashDash(os.Args)
	port := argValue(args, "--port")
	if path := os.Getenv("COMRAD_FAKE_LLAMA_SERVER_ARGS_PATH"); path != "" {
		_ = os.WriteFile(path, []byte(strings.Join(args, " ")), 0o644)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/v1/chat/completions", fakeChatCompletions)
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func fakeChatCompletions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	time.Sleep(100 * time.Millisecond)
	if os.Getenv("COMRAD_FAKE_LLAMA_SERVER_MODE") == "exit_on_chat" {
		if flusher != nil {
			flusher.Flush()
		}
		os.Exit(3)
	}
	writeFakeChunk(w, flusher, "first ")
	if os.Getenv("COMRAD_FAKE_LLAMA_SERVER_MODE") == "fail_after_first" {
		return
	}
	writeFakeChunk(w, flusher, "second")
	_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"stop\",\"index\":0,\"delta\":{}}],\"timings\":{\"prompt_n\":11,\"predicted_n\":4,\"predicted_ms\":250,\"predicted_per_second\":16}}\n\n")
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func writeFakeChunk(w http.ResponseWriter, flusher http.Flusher, token string) {
	_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", token)
	if flusher != nil {
		flusher.Flush()
	}
}

func argsAfterDashDash(args []string) []string {
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			return args[i+1:]
		}
	}
	return nil
}

func argValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
