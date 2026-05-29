package comrad

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkerNodeTokenPreventsSessionHijack(t *testing.T) {
	manager := newWorkerSecurityManager(t)
	legit := testWorkerSession(manager, "legit")
	hello := Envelope{Type: MsgHello, Payload: MarshalPayload(HelloPayload{Node: Node{ID: "node-a", Approved: true}, Slots: []Slot{{ID: "node-a/slot0"}}})}
	if err := manager.handleWorkerEnvelope(legit, hello); err != nil {
		t.Fatal(err)
	}
	nodeToken := readWorkerNodeToken(t, legit)
	if nodeToken == "" {
		t.Fatal("registration did not return a node token")
	}

	attacker := testWorkerSession(manager, "attacker")
	if err := manager.handleWorkerEnvelope(attacker, hello); err == nil {
		t.Fatal("second session claimed node without node token")
	}

	reconnect := testWorkerSession(manager, "reconnect")
	withToken := Envelope{Type: MsgHello, Payload: MarshalPayload(HelloPayload{NodeToken: nodeToken, Node: Node{ID: "node-a", Approved: true}, Slots: []Slot{{ID: "node-a/slot0"}}})}
	if err := manager.handleWorkerEnvelope(reconnect, withToken); err != nil {
		t.Fatalf("reconnect with node token failed: %v", err)
	}
}

func TestWorkerCannotClaimExistingNodeWithoutNodeToken(t *testing.T) {
	manager := newWorkerSecurityManager(t)
	err := manager.store.Update(func(db *Database) error {
		db.Nodes["node-a"] = Node{ID: "node-a", Approved: true}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	session := testWorkerSession(manager, "attacker")
	hello := Envelope{Type: MsgHello, Payload: MarshalPayload(HelloPayload{Node: Node{ID: "node-a"}, Slots: []Slot{{ID: "node-a/slot0"}}})}
	if err := manager.handleWorkerEnvelope(session, hello); err == nil {
		t.Fatal("existing node was claimed without node token")
	}
}

func TestWorkerSessionCannotSwitchNodeID(t *testing.T) {
	manager := newWorkerSecurityManager(t)
	session := testWorkerSession(manager, "session")
	helloA := Envelope{Type: MsgHello, Payload: MarshalPayload(HelloPayload{Node: Node{ID: "node-a"}, Slots: []Slot{{ID: "node-a/slot0"}}})}
	if err := manager.handleWorkerEnvelope(session, helloA); err != nil {
		t.Fatal(err)
	}
	helloB := Envelope{Type: MsgHello, Payload: MarshalPayload(HelloPayload{Node: Node{ID: "node-b"}, Slots: []Slot{{ID: "node-b/slot0"}}})}
	if err := manager.handleWorkerEnvelope(session, helloB); err == nil {
		t.Fatal("registered worker session switched node id")
	}
	if got := manager.sessions["node-a"]; got != session {
		t.Fatal("original node session mapping changed")
	}
	if _, ok := manager.sessions["node-b"]; ok {
		t.Fatal("new node session mapping was created")
	}
}

func TestWorkerMessagesMustBelongToSessionNode(t *testing.T) {
	manager := newWorkerSecurityManager(t)
	seedWorkerOwnershipState(t, manager)
	session := testWorkerSession(manager, "session")
	session.nodeID = "node-a"

	slotMsg := Envelope{Type: MsgSlotState, NodeID: "node-a", Payload: MarshalPayload(SlotStatePayload{SlotID: "node-b/slot0", State: SlotStateReady})}
	if err := manager.handleWorkerEnvelope(session, slotMsg); err == nil {
		t.Fatal("worker updated a slot owned by another node")
	}

	report := ComputeReport{TaskID: "task-b", AttemptID: "att-b", NodeID: "node-b", SlotID: "node-b/slot0", Status: TaskStatusCompleted}
	reportMsg := Envelope{Type: MsgComputeReport, NodeID: "node-a", TaskID: "task-b", Attempt: "att-b", Payload: MarshalPayload(report)}
	if err := manager.handleWorkerEnvelope(session, reportMsg); err == nil {
		t.Fatal("worker completed an attempt owned by another node")
	}
}

func TestWorkerArtifactDownloadRequiresNodeToken(t *testing.T) {
	manager := newWorkerSecurityManager(t)
	artifact := seedAssignedArtifact(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/worker/artifacts/"+artifact.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer worker")
	req.Header.Set("X-COMRAD-Node-ID", "node-a")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("artifact without node token status = %d, want 403", resp.StatusCode)
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/api/worker/artifacts/"+artifact.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer worker")
	req.Header.Set("X-COMRAD-Node-ID", "node-a")
	req.Header.Set("X-COMRAD-Node-Token", "node-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("artifact with node token status = %d, want 200", resp.StatusCode)
	}
}

func TestWorkerWebSocketAuthUsesBearerHeader(t *testing.T) {
	wsURL, err := workerWSURL("https://manager.example/base?token=worker-secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(wsURL, "worker-secret") || strings.Contains(wsURL, "token=") {
		t.Fatalf("worker websocket URL leaked token material: %s", wsURL)
	}
	headers := workerWSHeaders("worker-secret")
	if got := headers.Get("Authorization"); got != "Bearer worker-secret" {
		t.Fatalf("Authorization header = %q", got)
	}
}

func newWorkerSecurityManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	manager, err := NewManager(ManagerConfig{DBPath: filepath.Join(dir, "manager.json"), ArtifactDir: filepath.Join(dir, "artifacts"), AdminToken: "admin", ClientAPIKey: "client", WorkerToken: "worker", AutoApprove: true})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func testWorkerSession(manager *Manager, id string) *workerSession {
	return &workerSession{id: id, manager: manager, send: make(chan Envelope, 8), done: make(chan struct{})}
}

func readWorkerNodeToken(t *testing.T, session *workerSession) string {
	t.Helper()
	select {
	case msg := <-session.send:
		var ack WorkerRegistrationAck
		if err := json.Unmarshal(msg.Payload, &ack); err != nil {
			t.Fatal(err)
		}
		return ack.NodeToken
	default:
		t.Fatal("missing registration ack")
	}
	return ""
}

func seedWorkerOwnershipState(t *testing.T, manager *Manager) {
	t.Helper()
	err := manager.store.Update(func(db *Database) error {
		db.Nodes["node-a"] = Node{ID: "node-a", State: NodeStateOnline, Approved: true}
		db.Nodes["node-b"] = Node{ID: "node-b", State: NodeStateOnline, Approved: true}
		db.Slots["node-b/slot0"] = Slot{ID: "node-b/slot0", NodeID: "node-b", State: SlotStateServing, ActiveTaskID: "task-b"}
		db.Tasks["task-b"] = Task{ID: "task-b", UserID: "user-b", Status: TaskStatusRunning}
		db.Attempts["att-b"] = Attempt{ID: "att-b", TaskID: "task-b", UserID: "user-b", NodeID: "node-b", SlotID: "node-b/slot0", Status: TaskStatusRunning, StartedAt: time.Now().UTC()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func seedAssignedArtifact(t *testing.T, manager *Manager) Artifact {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "model.gguf")
	if err := writeTestFile(path, "model"); err != nil {
		t.Fatal(err)
	}
	sha, size, err := FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	artifact := Artifact{ID: sha, Kind: "model_gguf", Name: "model.gguf", Path: path, SHA256: sha, SizeBytes: size, CreatedAt: time.Now().UTC()}
	err = manager.store.Update(func(db *Database) error {
		db.NodeTokenHashes["node-a"] = apiTokenHash("node-token")
		db.Nodes["node-a"] = Node{ID: "node-a", State: NodeStateOnline, Approved: true}
		db.Artifacts[artifact.ID] = artifact
		db.Profiles["profile"] = WorkloadProfile{ID: "profile", Artifacts: []string{artifact.ID}}
		db.Assignments["assign"] = PlacementAssignment{ID: "assign", NodeID: "node-a", ProfileID: "profile", DesiredCached: true}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func writeTestFile(path, body string) error {
	return os.WriteFile(path, []byte(strings.TrimSpace(body)), 0o600)
}
