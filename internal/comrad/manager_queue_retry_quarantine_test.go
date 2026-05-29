package comrad

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogicalModelRuntimeVariantsSelectConcreteSlotArtifact(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	now := time.Now().UTC()
	profile := WorkloadProfile{
		ID:           "llm.chat/gemma-4-e2b",
		Name:         "Gemma 4 E2B",
		Alias:        "gemma-4-e2b",
		LogicalModel: "gemma-4-e2b",
		Kind:         "llm.chat",
		Warmable:     true,
		RuntimeVariants: []RuntimeModelVariant{
			{
				ID:             "gemma-4-e2b-mlx",
				Target:         TargetDarwinArm64Metal,
				RuntimeAdapter: "mlx-metal",
				Artifacts:      []string{"sha256:mlx"},
				Requirements:   &Requirements{Target: TargetDarwinArm64Metal, RuntimeAdapter: "mlx-metal", UnifiedMemoryBytes: 1},
				LLM:            &LLMProfile{ContextTokens: 4096},
			},
			{
				ID:             "gemma-4-e2b-gguf-q4_k_m",
				Target:         TargetDarwinArm64Metal,
				RuntimeAdapter: "llama.cpp-metal",
				Artifacts:      []string{"sha256:gguf"},
				Requirements:   &Requirements{Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal", UnifiedMemoryBytes: 1},
				LLM:            &LLMProfile{ContextTokens: 8192},
			},
		},
		CreatedAt: now,
	}
	if err := m.store.Update(func(db *Database) error {
		db.Artifacts["sha256:mlx"] = Artifact{ID: "sha256:mlx", SHA256: "sha256:mlx", Kind: "model_mlx", CreatedAt: now}
		db.Artifacts["sha256:gguf"] = Artifact{ID: "sha256:gguf", SHA256: "sha256:gguf", Kind: "model_gguf", CreatedAt: now}
		db.Profiles[profile.ID] = profile
		db.Policies["pol"] = PlacementPolicy{ID: "pol", ProfileID: profile.ID, CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now}
		db.Nodes["node-metal"] = Node{ID: "node-metal", State: NodeStateOnline, Approved: true, RuntimeAdapters: []string{"llama.cpp-metal"}}
		db.Slots["node-metal/metal0"] = Slot{ID: "node-metal/metal0", NodeID: "node-metal", Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal", Resources: ResourceBudget{UnifiedMemoryBytes: 8}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	plan := PlanPlacement(m.store.Snapshot())
	if len(plan) != 1 {
		t.Fatalf("plan length = %d", len(plan))
	}
	if plan[0].LogicalModel != "gemma-4-e2b" {
		t.Fatalf("logical model = %q", plan[0].LogicalModel)
	}
	if plan[0].RuntimeVariantID != "gemma-4-e2b-gguf-q4_k_m" {
		t.Fatalf("variant = %q", plan[0].RuntimeVariantID)
	}
	if plan[0].ModelArtifactID != "sha256:gguf" || plan[0].ModelSHA256 != "sha256:gguf" {
		t.Fatalf("concrete artifact = %q sha = %q", plan[0].ModelArtifactID, plan[0].ModelSHA256)
	}
	resolved, err := ResolveLLMProfile(m.store.Snapshot(), "gemma-4-e2b", 6000)
	if err != nil {
		t.Fatal(err)
	}
	effective, fit := BestVariantForSlot(resolved, m.store.Snapshot().Nodes["node-metal"], m.store.Snapshot().Slots["node-metal/metal0"])
	if !fit.Fits || effective.RuntimeVariantID != "gemma-4-e2b-gguf-q4_k_m" {
		t.Fatalf("fit=%v effective=%s", fit, effective.RuntimeVariantID)
	}
}

func TestQueueWaitsForReadyWorkerAndIsDrained(t *testing.T) {
	m := newTestManager(t, 4, 2*time.Second, 3)
	profile := seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	result := startStreamingChat(t, server.URL, "client", "assistant", context.Background())
	waitFor(t, time.Second, func() bool {
		state := getState(t, server.URL, "admin")
		return state.Queue.Queued == 1 && len(state.Tasks) == 1 && state.Tasks[0].Status == TaskStatusQueued
	})

	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	payload := nextExecute(t, session)
	completePayload(m, payload, "queued drained")
	out := <-result
	if out.status != http.StatusOK || !strings.Contains(out.body, "data: [DONE]") {
		t.Fatalf("stream failed: status=%d body=%s", out.status, out.body)
	}
	state := getState(t, server.URL, "admin")
	if state.Queue.Queued != 0 {
		t.Fatalf("queued = %d", state.Queue.Queued)
	}
	if state.Tasks[0].Status != TaskStatusCompleted {
		t.Fatalf("task status = %s", state.Tasks[0].Status)
	}
}

func TestMultipleWorkersDrainQueueWithoutSharingSlots(t *testing.T) {
	m := newTestManager(t, 4, 2*time.Second, 3)
	profile := seedBasicProfile(t, m)
	s1 := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	s2 := addReadySession(t, m, "node-b", "node-b/slot0", profile)
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	r1 := startStreamingChat(t, server.URL, "client", "assistant", context.Background())
	r2 := startStreamingChat(t, server.URL, "client", "assistant", context.Background())
	p1 := nextExecuteFromEither(t, s1, s2)
	p2 := nextExecuteFromEither(t, s1, s2)
	if p1.SlotID == p2.SlotID {
		t.Fatalf("same slot assigned twice: %s", p1.SlotID)
	}
	completePayload(m, p1, "one")
	completePayload(m, p2, "two")
	for _, r := range []chatResult{<-r1, <-r2} {
		if r.status != http.StatusOK || !strings.Contains(r.body, "data: [DONE]") {
			t.Fatalf("stream failed: status=%d body=%s", r.status, r.body)
		}
	}
}

func TestQueueOverflowReturnsNoCapacityAndStateIsObservable(t *testing.T) {
	m := newTestManager(t, 1, 2*time.Second, 3)
	seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := startStreamingChat(t, server.URL, "client", "assistant", ctx)
	waitFor(t, time.Second, func() bool {
		state := getState(t, server.URL, "admin")
		return state.Queue.InUse == 1 && state.Queue.Queued == 1 && state.Queue.Limit == 1
	})
	second := doChatOnce(t, server.URL, "client", "assistant")
	if second.status != http.StatusServiceUnavailable || !strings.Contains(second.body, FailureNoCapacity) {
		t.Fatalf("second status=%d body=%s", second.status, second.body)
	}
	cancel()
	<-first
}

func TestWorkerDisconnectBeforeFirstOutputRetriesOnAnotherSlot(t *testing.T) {
	m := newTestManager(t, 4, 2*time.Second, 3)
	profile := seedBasicProfile(t, m)
	s1 := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	s2 := addReadySession(t, m, "node-b", "node-b/slot0", profile)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := startStreamingChat(t, server.URL, "client", "assistant", ctx)

	first := nextExecuteFromEither(t, s1, s2)
	retrySession := s2
	failedSession := s1
	if first.SlotID == "node-b/slot0" {
		retrySession = s1
		failedSession = s2
	}
	failedSession.close()
	second := nextExecute(t, retrySession)
	if second.SlotID == first.SlotID {
		t.Fatalf("retried failed slot %s", second.SlotID)
	}
	completePayload(m, second, "retried")
	out := <-result
	if out.status != http.StatusOK || !strings.Contains(out.body, "data: [DONE]") {
		t.Fatalf("stream failed: status=%d body=%s", out.status, out.body)
	}
	state := getState(t, server.URL, "admin")
	if len(state.Attempts) != 2 || state.Attempts[0].Status != TaskStatusFailed || state.Attempts[1].Status != TaskStatusCompleted {
		t.Fatalf("attempts = %+v", state.Attempts)
	}
	if !Contains(state.Tasks[0].FailedSlots, first.SlotID) {
		t.Fatalf("failed slot was not recorded: %+v", state.Tasks[0].FailedSlots)
	}
}

func TestWorkerDisconnectWithoutAlternativeFailsByPolicy(t *testing.T) {
	m := newTestManager(t, 4, 500*time.Millisecond, 3)
	profile := seedBasicProfile(t, m)
	s1 := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	result := startStreamingChat(t, server.URL, "client", "assistant", context.Background())
	first := nextExecute(t, s1)
	s1.close()
	waitFor(t, 200*time.Millisecond, func() bool {
		state := getState(t, server.URL, "admin")
		return state.Tasks[0].Status == TaskStatusQueued && Contains(state.Tasks[0].FailedSlots, first.SlotID)
	})
	out := <-result
	if out.status != http.StatusOK || !strings.Contains(out.body, FailureNoCapacity) {
		t.Fatalf("expected no_capacity stream failure after %s, got status=%d body=%s", first.SlotID, out.status, out.body)
	}
	state := getState(t, server.URL, "admin")
	if state.Tasks[0].Status != TaskStatusFailed {
		t.Fatalf("task status = %s", state.Tasks[0].Status)
	}
}

func TestFailureAfterFirstOutputDoesNotRetry(t *testing.T) {
	m := newTestManager(t, 4, 2*time.Second, 3)
	profile := seedBasicProfile(t, m)
	s1 := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	s2 := addReadySession(t, m, "node-b", "node-b/slot0", profile)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	result := startStreamingChat(t, server.URL, "client", "assistant", context.Background())
	payload := nextExecuteFromEither(t, s1, s2)
	nodeID := strings.Split(payload.SlotID, "/")[0]
	if err := m.handleToken(nodeID, TokenPayload{TaskID: payload.TaskID, AttemptID: payload.AttemptID, Token: "partial", Index: 0}); err != nil {
		t.Fatal(err)
	}
	if err := m.handleAttemptFailed(nodeID, AttemptFailedPayload{TaskID: payload.TaskID, AttemptID: payload.AttemptID, Phase: "runtime", FailureReason: FailureRuntimeError, CanRetry: true, FirstOutputSent: true}); err != nil {
		t.Fatal(err)
	}
	out := <-result
	if !strings.Contains(out.body, FailureRuntimeError) {
		t.Fatalf("expected runtime error, got %s", out.body)
	}
	deadline := time.After(150 * time.Millisecond)
	for {
		select {
		case msg := <-s1.send:
			if msg.Type == MsgExecuteTask {
				t.Fatalf("unexpected retry assigned after first output: %+v", msg)
			}
		case msg := <-s2.send:
			if msg.Type == MsgExecuteTask {
				t.Fatalf("unexpected retry assigned after first output: %+v", msg)
			}
		case <-deadline:
			return
		}
	}
}

func TestClientDisconnectCancelsQueuedChat(t *testing.T) {
	m := newTestManager(t, 4, 5*time.Second, 3)
	seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := startStreamingChat(t, server.URL, "client", "assistant", ctx)
	waitFor(t, time.Second, func() bool {
		for _, task := range m.store.Snapshot().Tasks {
			return task.Status == TaskStatusQueued
		}
		return false
	})

	cancel()
	<-result
	waitFor(t, time.Second, func() bool {
		for _, task := range m.store.Snapshot().Tasks {
			return task.Status == TaskStatusCancelled
		}
		return false
	})
	state := m.store.Snapshot()
	if len(state.Tasks) != 1 {
		t.Fatalf("tasks = %d", len(state.Tasks))
	}
	for _, task := range state.Tasks {
		if task.Status != TaskStatusCancelled || task.FailureReason != FailureCancelledByClient {
			t.Fatalf("task after disconnect = %+v", task)
		}
	}
}

func TestWorkerFailureQuarantineAndManualUnban(t *testing.T) {
	m := newTestManager(t, 4, 2*time.Second, 2)
	profile := seedBasicProfile(t, m)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	for i := 0; i < 2; i++ {
		taskID := "task-q-" + string(rune('a'+i))
		attemptID := "att-q-" + string(rune('a'+i))
		if err := m.store.Update(func(db *Database) error {
			db.Tasks[taskID] = Task{ID: taskID, Kind: "llm.chat", Model: "assistant", ProfileID: profile.ID, Status: TaskStatusRunning, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			db.Attempts[attemptID] = Attempt{ID: attemptID, TaskID: taskID, NodeID: "node-a", SlotID: "node-a/slot0", ProfileID: profile.ID, RuntimeVariantID: profile.RuntimeVariantID, RuntimeAdapter: profile.RuntimeAdapter, Status: TaskStatusRunning, StartedAt: time.Now().UTC()}
			slot := db.Slots["node-a/slot0"]
			slot.State = SlotStateServing
			slot.ActiveTaskID = taskID
			slot.AcceptsNew = false
			db.Slots[slot.ID] = slot
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := m.handleAttemptFailed("node-a", AttemptFailedPayload{TaskID: taskID, AttemptID: attemptID, Phase: "runtime", FailureReason: FailureRuntimeError, CanRetry: true, FirstOutputSent: false}); err != nil {
			t.Fatal(err)
		}
	}
	state := m.stateResponse()
	if !state.Slots[0].Quarantined || state.Slots[0].QuarantineReason != FailureRuntimeError {
		t.Fatalf("slot was not quarantined: %+v", state.Slots[0])
	}
	if _, _, _, ok := m.selectReadySlot(profile, "new-task"); ok {
		t.Fatal("quarantined slot was schedulable")
	}

	server := httptest.NewServer(m.Handler())
	defer server.Close()
	body := bytes.NewBufferString(`{"slotId":"node-a/slot0"}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/admin/quarantine/unban", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unban returned %d", resp.StatusCode)
	}
	_ = session
	state = m.stateResponse()
	if state.Slots[0].Quarantined {
		t.Fatalf("slot still quarantined: %+v", state.Slots[0])
	}
	if _, _, _, ok := m.selectReadySlot(profile, "new-task"); !ok {
		t.Fatal("unbanned ready slot was not schedulable")
	}
}

type chatResult struct {
	status int
	body   string
}

func newTestManager(t *testing.T, queueLimit int, streamWait time.Duration, quarantineThreshold int) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewManager(ManagerConfig{
		DBPath:              filepath.Join(dir, "comrad.json"),
		ArtifactDir:         filepath.Join(dir, "artifacts"),
		AdminToken:          "admin",
		ClientAPIKey:        "client",
		WorkerToken:         "worker",
		QueueLimit:          queueLimit,
		StreamWait:          streamWait,
		QuarantineThreshold: quarantineThreshold,
		QuarantineDuration:  time.Minute,
		AutoApprove:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func seedBasicProfile(t *testing.T, m *Manager) WorkloadProfile {
	t.Helper()
	profile := EffectiveProfileForVariant(WorkloadProfile{
		ID:             "llm.chat/assistant",
		Name:           "assistant",
		Alias:          "assistant",
		LogicalModel:   "assistant",
		Kind:           "llm.chat",
		RuntimeAdapter: "llama.cpp-metal",
		Artifacts:      []string{"sha256:model"},
		Requirements:   &Requirements{Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal", UnifiedMemoryBytes: 1, DiskBytes: 1},
		LLM:            &LLMProfile{ContextTokens: 4096},
		Warmable:       true,
		CreatedAt:      time.Now().UTC(),
	}, RuntimeModelVariant{})
	if err := m.store.Update(func(db *Database) error {
		db.Artifacts["sha256:model"] = Artifact{ID: "sha256:model", SHA256: "sha256:model", Kind: "model_gguf", CreatedAt: profile.CreatedAt}
		db.Profiles[profile.ID] = profile
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return profile
}

func addReadySession(t *testing.T, m *Manager, nodeID, slotID string, profile WorkloadProfile) *workerSession {
	t.Helper()
	session := &workerSession{id: NewID("ses"), nodeID: nodeID, baseURL: "http://manager.test", manager: m, send: make(chan Envelope, 16), done: make(chan struct{})}
	now := time.Now().UTC()
	if err := m.store.Update(func(db *Database) error {
		db.Nodes[nodeID] = Node{ID: nodeID, State: NodeStateOnline, Approved: true, RuntimeAdapters: []string{profile.RuntimeAdapter}, LastSeen: now}
		db.Slots[slotID] = Slot{
			ID:               slotID,
			NodeID:           nodeID,
			Target:           TargetDarwinArm64Metal,
			RuntimeAdapter:   profile.RuntimeAdapter,
			Resources:        ResourceBudget{UnifiedMemoryBytes: 8, DiskBytes: 8},
			State:            SlotStateReady,
			ProfileID:        profile.ID,
			ProfileVersion:   profileVersion(profile),
			LogicalModel:     ProfileLogicalModel(profile),
			RuntimeVariantID: profile.RuntimeVariantID,
			ModelArtifactID:  ConcreteModelArtifactID(profile),
			ModelSHA256:      ConcreteModelSHA256(profile),
			AcceptsNew:       true,
			LastReady:        now,
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.sessions[nodeID] = session
	m.mu.Unlock()
	return session
}

func startStreamingChat(t *testing.T, baseURL, token, model string, ctx context.Context) <-chan chatResult {
	t.Helper()
	out := make(chan chatResult, 1)
	go func() {
		out <- doStreamingChat(t, baseURL, token, model, ctx)
	}()
	return out
}

func doChatOnce(t *testing.T, baseURL, token, model string) chatResult {
	t.Helper()
	return doStreamingChat(t, baseURL, token, model, context.Background())
}

func doStreamingChat(t *testing.T, baseURL, token, model string, ctx context.Context) chatResult {
	t.Helper()
	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(ChatCompletionRequest{Model: model, Stream: true, MaxTokens: 4, Messages: []ChatMessage{{Role: "user", Content: "hello"}}}); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return chatResult{status: 0, body: err.Error()}
	}
	defer resp.Body.Close()
	var b strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		b.WriteString(line)
		b.WriteByte('\n')
		if line == "data: [DONE]" || strings.Contains(line, `"error"`) {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		b.WriteString(err.Error())
	}
	return chatResult{status: resp.StatusCode, body: b.String()}
}

func nextExecute(t *testing.T, session *workerSession) ExecuteTaskPayload {
	t.Helper()
	select {
	case msg := <-session.send:
		if msg.Type != MsgExecuteTask {
			t.Fatalf("message type = %s", msg.Type)
		}
		var payload ExecuteTaskPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		return payload
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for execute")
	}
	return ExecuteTaskPayload{}
}

func nextExecuteFromEither(t *testing.T, a, b *workerSession) ExecuteTaskPayload {
	t.Helper()
	select {
	case msg := <-a.send:
		if msg.Type != MsgExecuteTask {
			t.Fatalf("message type = %s", msg.Type)
		}
		var payload ExecuteTaskPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		return payload
	case msg := <-b.send:
		if msg.Type != MsgExecuteTask {
			t.Fatalf("message type = %s", msg.Type)
		}
		var payload ExecuteTaskPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		return payload
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for execute")
	}
	return ExecuteTaskPayload{}
}

func completePayload(m *Manager, payload ExecuteTaskPayload, token string) {
	nodeID := strings.Split(payload.SlotID, "/")[0]
	if err := m.handleToken(nodeID, TokenPayload{TaskID: payload.TaskID, AttemptID: payload.AttemptID, Token: token, Index: 0}); err != nil {
		panic(err)
	}
	if err := m.handleComputeReport(nodeID, ComputeReport{
		ID:               NewID("rep"),
		TaskID:           payload.TaskID,
		AttemptID:        payload.AttemptID,
		NodeID:           strings.Split(payload.SlotID, "/")[0],
		SlotID:           payload.SlotID,
		ProfileID:        payload.Profile.ID,
		LogicalModel:     ProfileLogicalModel(payload.Profile),
		RuntimeVariantID: payload.Profile.RuntimeVariantID,
		RuntimeAdapter:   payload.Profile.RuntimeAdapter,
		Status:           TaskStatusCompleted,
		Phase:            "completed",
		LLM:              LLMMetrics{CompletionTokens: 1, TotalTokens: 1, ContextTokens: payload.Profile.LLM.ContextTokens},
		CreatedAt:        time.Now().UTC(),
	}); err != nil {
		panic(err)
	}
}
