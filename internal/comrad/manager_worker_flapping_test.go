package comrad

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkerFlappingSuppressesWarmPlacement(t *testing.T) {
	m := newFlapTestManager(t, 3, time.Minute, 2*time.Minute)
	profile := seedBasicProfile(t, m)
	seedFlapPlacementPolicy(t, m, profile, time.Now().UTC())

	nodeToken := connectFlapWorker(t, m, "node-a", "")
	disconnectFlapWorker(t, m, "node-a")
	connectFlapWorker(t, m, "node-a", nodeToken)
	disconnectFlapWorker(t, m, "node-a")
	connectFlapWorker(t, m, "node-a", nodeToken)

	state := m.stateResponse()
	node := stateNode(t, state, "node-a")
	if !node.WarmPlacementSuppressed || node.WarmPlacementSuppressionReason != FailureWorkerFlapping || node.WarmPlacementSuppressionUntil == nil {
		t.Fatalf("node suppression = suppressed:%t reason:%q until:%v", node.WarmPlacementSuppressed, node.WarmPlacementSuppressionReason, node.WarmPlacementSuppressionUntil)
	}
	assertCondition(t, node.Conditions, "WarmPlacementSuppressed", "True", "WorkerFlapping")

	m.replanAndDispatch()
	plan := SortedAssignments(m.store.Snapshot())
	if desiredWarmOnNode(plan, "node-a") != 0 {
		t.Fatalf("suppressed worker received warm placement: %+v", plan)
	}
	if !hasMissingPlacement(plan) {
		t.Fatalf("suppressed only worker should leave warm placement missing: %+v", plan)
	}
}

func TestWorkerWarmSuppressionExpires(t *testing.T) {
	m := newFlapTestManager(t, 2, time.Minute, time.Minute)
	profile := seedBasicProfile(t, m)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Second)
	if err := m.store.Update(func(db *Database) error {
		db.Policies["pol"] = PlacementPolicy{ID: "pol", ProfileID: profile.ID, CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now}
		db.Nodes["node-a"] = Node{
			ID:                             "node-a",
			State:                          NodeStateOnline,
			Approved:                       true,
			RuntimeAdapters:                []string{profile.RuntimeAdapter},
			WarmPlacementSuppressed:        true,
			WarmPlacementSuppressionReason: FailureWorkerFlapping,
			WarmPlacementSuppressionUntil:  &expired,
		}
		db.Slots["node-a/slot0"] = Slot{
			ID:             "node-a/slot0",
			NodeID:         "node-a",
			Target:         TargetDarwinArm64Metal,
			RuntimeAdapter: profile.RuntimeAdapter,
			Resources:      ResourceBudget{UnifiedMemoryBytes: 8, DiskBytes: 8},
			State:          SlotStateIdle,
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	m.clearExpiredWorkerSuppressions(now)
	m.replanAndDispatch()

	state := m.stateResponse()
	node := stateNode(t, state, "node-a")
	if node.WarmPlacementSuppressed || node.WarmPlacementSuppressionUntil != nil || node.WarmPlacementSuppressionReason != "" {
		t.Fatalf("expired suppression remained: %+v", node)
	}
	assertCondition(t, node.Conditions, "WarmPlacementSuppressed", "False", "WorkerTrusted")
	if got := placementNodeFor(SortedAssignments(m.store.Snapshot()), profile.ID); got != "node-a" {
		t.Fatalf("warm placement node = %q, want node-a", got)
	}
}

func TestStableWorkerIsNotWarmSuppressed(t *testing.T) {
	m := newFlapTestManager(t, 3, time.Minute, time.Minute)
	profile := seedBasicProfile(t, m)
	now := time.Now().UTC()
	seedFlapPlacementPolicy(t, m, profile, now)

	connectFlapWorker(t, m, "node-a", "")
	session := currentWorkerSession(t, m, "node-a")
	for i := 0; i < 3; i++ {
		if err := handleWorkerHeartbeat(m, session, Envelope{Type: MsgHeartbeat, NodeID: "node-a"}); err != nil {
			t.Fatal(err)
		}
	}

	state := m.stateResponse()
	node := stateNode(t, state, "node-a")
	if node.WarmPlacementSuppressed || node.WarmPlacementSuppressionUntil != nil {
		t.Fatalf("stable worker was suppressed: %+v", node)
	}
	assertCondition(t, node.Conditions, "WarmPlacementSuppressed", "False", "WorkerTrusted")
	if got := placementNodeFor(SortedAssignments(m.store.Snapshot()), profile.ID); got != "node-a" {
		t.Fatalf("warm placement node = %q, want node-a", got)
	}
}

func newFlapTestManager(t *testing.T, threshold int, window, cooldown time.Duration) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewManager(ManagerConfig{
		DBPath:                 filepath.Join(dir, "comrad.json"),
		ArtifactDir:            filepath.Join(dir, "artifacts"),
		AdminToken:             "admin",
		ClientAPIKey:           "client",
		WorkerToken:            "worker",
		QueueLimit:             4,
		StreamWait:             time.Second,
		QuarantineThreshold:    3,
		QuarantineDuration:     time.Minute,
		WorkerFlapThreshold:    threshold,
		WorkerFlapWindow:       window,
		WorkerFlapCooldown:     cooldown,
		WorkerHeartbeatTimeout: time.Second,
		AutoApprove:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func seedFlapPlacementPolicy(t *testing.T, m *Manager, profile WorkloadProfile, now time.Time) {
	t.Helper()
	if err := m.store.Update(func(db *Database) error {
		db.Policies["pol"] = PlacementPolicy{ID: "pol", ProfileID: profile.ID, CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func connectFlapWorker(t *testing.T, m *Manager, nodeID, nodeToken string) string {
	t.Helper()
	session := testWorkerSession(m, NewID("ses"))
	node := Node{
		ID:              nodeID,
		State:           NodeStateRegistered,
		OS:              "darwin",
		Arch:            "arm64",
		Target:          TargetDarwinArm64Metal,
		RuntimeAdapters: []string{"llama.cpp-metal"},
		Budgets:         ResourceBudget{UnifiedMemoryBytes: 8, DiskBytes: 8, SlotCount: 1},
	}
	slots := []Slot{{
		ID:             nodeID + "/slot0",
		NodeID:         nodeID,
		Target:         TargetDarwinArm64Metal,
		RuntimeAdapter: "llama.cpp-metal",
		Resources:      ResourceBudget{UnifiedMemoryBytes: 8, DiskBytes: 8},
		State:          SlotStateIdle,
	}}
	if err := m.upsertWorkerState(session, node, slots, nil, nil, nodeToken); err != nil {
		t.Fatal(err)
	}
	issued := readWorkerNodeTokenMaybe(t, session)
	if issued != "" {
		return issued
	}
	return nodeToken
}

func disconnectFlapWorker(t *testing.T, m *Manager, nodeID string) {
	t.Helper()
	currentWorkerSession(t, m, nodeID).close()
}

func currentWorkerSession(t *testing.T, m *Manager, nodeID string) *workerSession {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[nodeID]
	if session == nil {
		t.Fatalf("missing worker session for %s", nodeID)
	}
	return session
}

func readWorkerNodeTokenMaybe(t *testing.T, session *workerSession) string {
	t.Helper()
	select {
	case msg := <-session.send:
		var ack WorkerRegistrationAck
		if err := json.Unmarshal(msg.Payload, &ack); err != nil {
			t.Fatal(err)
		}
		return ack.NodeToken
	default:
		return ""
	}
}

func stateNode(t *testing.T, state StateResponse, nodeID string) Node {
	t.Helper()
	for _, node := range state.Nodes {
		if node.ID == nodeID {
			return node
		}
	}
	t.Fatalf("node %s missing from state: %+v", nodeID, state.Nodes)
	return Node{}
}
