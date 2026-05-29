package comrad

import (
	"testing"
	"time"
)

func TestHandleTokenPersistsOnlyFirstOutputState(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	profile := seedBasicProfile(t, m)
	seedTokenAttempt(t, m, profile)
	counter := countStoreSaves(t, m.store)
	active := &activeAttempt{
		taskID:    "task-token",
		attemptID: "attempt-token",
		events:    make(chan streamEvent, 4),
		createdAt: time.Now().UTC(),
	}
	m.registerStream(active.attemptID, active)
	defer m.unregisterStream(active.attemptID)

	for i, token := range []string{"first", " second", " third"} {
		err := m.handleToken("node-token", TokenPayload{
			TaskID:    active.taskID,
			AttemptID: active.attemptID,
			Token:     token,
			Index:     i,
		})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case ev := <-active.events:
			if ev.kind != "token" || ev.token.Token != token {
				t.Fatalf("event = %+v, want token %q", ev, token)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for token %q", token)
		}
	}
	if counter.saves != 1 {
		t.Fatalf("state saves after three tokens = %d, want 1", counter.saves)
	}
	db := m.store.Snapshot()
	attempt := db.Attempts[active.attemptID]
	if !attempt.FirstOutputSent || attempt.FirstOutputAt == nil || attempt.CanRetry {
		t.Fatalf("attempt first-output state = %+v", attempt)
	}
}

func seedTokenAttempt(t *testing.T, m *Manager, profile WorkloadProfile) {
	t.Helper()
	now := time.Now().UTC()
	if err := m.store.Update(func(db *Database) error {
		db.Nodes["node-token"] = Node{ID: "node-token", State: NodeStateOnline, Approved: true, RuntimeAdapters: []string{profile.RuntimeAdapter}}
		db.Slots["node-token/slot0"] = Slot{ID: "node-token/slot0", NodeID: "node-token", State: SlotStateServing, ProfileID: profile.ID, RuntimeAdapter: profile.RuntimeAdapter, ActiveTaskID: "task-token"}
		db.Tasks["task-token"] = Task{ID: "task-token", Kind: "llm.chat", Model: "assistant", ProfileID: profile.ID, Status: TaskStatusRunning, CreatedAt: now, UpdatedAt: now}
		db.Attempts["attempt-token"] = Attempt{
			ID:             "attempt-token",
			TaskID:         "task-token",
			NodeID:         "node-token",
			SlotID:         "node-token/slot0",
			ProfileID:      profile.ID,
			RuntimeAdapter: profile.RuntimeAdapter,
			Status:         TaskStatusRunning,
			CanRetry:       true,
			StartedAt:      now,
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func countStoreSaves(t *testing.T, store *Store) *countingStoreBackend {
	t.Helper()
	counter := &countingStoreBackend{inner: store.backend}
	store.mu.Lock()
	store.backend = counter
	store.mu.Unlock()
	return counter
}

type countingStoreBackend struct {
	inner storeBackend
	saves int
}

func (b *countingStoreBackend) Name() string {
	return b.inner.Name()
}

func (b *countingStoreBackend) Path() string {
	return b.inner.Path()
}

func (b *countingStoreBackend) Load() (Database, bool, error) {
	return b.inner.Load()
}

func (b *countingStoreBackend) Save(db Database) error {
	b.saves++
	return b.inner.Save(db)
}
