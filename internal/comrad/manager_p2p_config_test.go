package comrad

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func newTestManagerForP2P(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(dir, "comrad.json"),
		ArtifactDir:  filepath.Join(dir, "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
		AutoApprove:  true,
		QueueLimit:   2,
		StreamWait:   0,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func newFakeWorkerSession(nodeID string) (*workerSession, chan Envelope) {
	ch := make(chan Envelope, 16)
	s := &workerSession{
		id:     NewID("sess"),
		nodeID: nodeID,
		send:   ch,
		done:   make(chan struct{}),
	}
	return s, ch
}

func drainSessionSend(ch chan Envelope) []Envelope {
	var out []Envelope
	for {
		select {
		case msg := <-ch:
			out = append(out, msg)
		default:
			return out
		}
	}
}

func TestBroadcastP2PConfigSendsToAllSessions(t *testing.T) {
	m := newTestManagerForP2P(t)

	s1, ch1 := newFakeWorkerSession("node-1")
	s2, ch2 := newFakeWorkerSession("node-2")
	m.mu.Lock()
	m.sessions["node-1"] = s1
	m.sessions["node-2"] = s2
	m.mu.Unlock()

	m.broadcastP2PConfig(false)

	for _, ch := range []chan Envelope{ch1, ch2} {
		msgs := drainSessionSend(ch)
		var found bool
		for _, msg := range msgs {
			if msg.Type != MsgP2PConfig {
				continue
			}
			var p P2PConfigPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				t.Fatalf("unmarshal P2PConfigPayload: %v", err)
			}
			if p.Enabled {
				t.Fatal("expected Enabled=false in broadcast")
			}
			found = true
		}
		if !found {
			t.Fatalf("session did not receive MsgP2PConfig; got %+v", msgs)
		}
	}
}

func TestBroadcastP2PConfigEnabledTrue(t *testing.T) {
	m := newTestManagerForP2P(t)

	s, ch := newFakeWorkerSession("node-a")
	m.mu.Lock()
	m.sessions["node-a"] = s
	m.mu.Unlock()

	m.broadcastP2PConfig(true)

	msgs := drainSessionSend(ch)
	for _, msg := range msgs {
		if msg.Type != MsgP2PConfig {
			continue
		}
		var p P2PConfigPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !p.Enabled {
			t.Fatal("expected Enabled=true in broadcast")
		}
		return
	}
	t.Fatalf("no MsgP2PConfig received; got %+v", msgs)
}

func TestSendP2PConfigToSessionReflectsCurrentSetting(t *testing.T) {
	m := newTestManagerForP2P(t)

	// default is P2PEnabled=true; disable it first
	if err := m.store.Update(func(db *Database) error {
		db.Settings.P2PEnabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	s, ch := newFakeWorkerSession("node-b")
	m.sendP2PConfigToSession(s)

	msgs := drainSessionSend(ch)
	for _, msg := range msgs {
		if msg.Type != MsgP2PConfig {
			continue
		}
		var p P2PConfigPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if p.Enabled {
			t.Fatal("expected Enabled=false matching store setting")
		}
		return
	}
	t.Fatalf("no MsgP2PConfig received; got %+v", msgs)
}

func TestBroadcastP2PConfigNoSessions(t *testing.T) {
	m := newTestManagerForP2P(t)
	// must not panic with no sessions
	m.broadcastP2PConfig(false)
}
