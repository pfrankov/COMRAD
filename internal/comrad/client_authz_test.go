package comrad

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientCannotCancelAnotherUsersJob(t *testing.T) {
	manager, server, session := newClientCancelAuthzHarness(t)
	defer server.Close()

	resp := postCancel(t, server.URL, "client-a")
	assertResponseStatus(t, resp, http.StatusNotFound)
	assertTaskStatus(t, manager, "task-b", TaskStatusRunning)
	assertNoWorkerCancel(t, session)
}

func TestClientCanCancelOwnJob(t *testing.T) {
	_, server, session := newClientCancelAuthzHarness(t)
	defer server.Close()

	resp := postCancel(t, server.URL, "client-b")
	assertResponseStatus(t, resp, http.StatusOK)
	assertWorkerCancel(t, session, "task-b")
}

func newClientCancelAuthzHarness(t *testing.T) (*Manager, *httptest.Server, *workerSession) {
	t.Helper()
	manager := newWorkerSecurityManager(t)
	seedCrossUserRunningTask(t, manager)
	session := testWorkerSession(manager, "node-b-session")
	session.nodeID = "node-b"
	manager.mu.Lock()
	manager.sessions["node-b"] = session
	manager.mu.Unlock()
	return manager, httptest.NewServer(manager.Handler()), session
}

func postCancel(t *testing.T, baseURL, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/jobs/task-b/cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func assertResponseStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	_ = resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("cancel status = %d, want %d", resp.StatusCode, want)
	}
}

func assertTaskStatus(t *testing.T, manager *Manager, taskID, want string) {
	t.Helper()
	task := manager.store.Snapshot().Tasks[taskID]
	if task.Status != want {
		t.Fatalf("task status = %q, want %q", task.Status, want)
	}
}

func assertNoWorkerCancel(t *testing.T, session *workerSession) {
	t.Helper()
	select {
	case msg := <-session.send:
		t.Fatalf("cross-user cancel sent worker message: %+v", msg)
	default:
	}
}

func assertWorkerCancel(t *testing.T, session *workerSession, taskID string) {
	t.Helper()
	select {
	case msg := <-session.send:
		if msg.Type != MsgCancelTask || msg.TaskID != taskID {
			t.Fatalf("owner cancel worker message = %+v", msg)
		}
	default:
		t.Fatal("owner cancel did not notify worker")
	}
}

func seedCrossUserRunningTask(t *testing.T, manager *Manager) {
	t.Helper()
	err := manager.store.Update(func(db *Database) error {
		db.Users["user-a"] = User{ID: "user-a", Name: "A", CreatedAt: time.Now().UTC()}
		db.Users["user-b"] = User{ID: "user-b", Name: "B", CreatedAt: time.Now().UTC()}
		db.APIKeys["key-a"] = APIKey{ID: "key-a", UserID: "user-a", TokenHash: apiTokenHash("client-a"), Status: APIKeyStatusActive, CreatedAt: time.Now().UTC()}
		db.APIKeys["key-b"] = APIKey{ID: "key-b", UserID: "user-b", TokenHash: apiTokenHash("client-b"), Status: APIKeyStatusActive, CreatedAt: time.Now().UTC()}
		db.Tasks["task-b"] = Task{ID: "task-b", UserID: "user-b", Status: TaskStatusRunning, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		db.Attempts["att-b"] = Attempt{ID: "att-b", TaskID: "task-b", UserID: "user-b", NodeID: "node-b", Status: TaskStatusRunning, StartedAt: time.Now().UTC()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
