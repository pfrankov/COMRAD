package comrad

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestAdminStateWebSocketStreamsInitialStateAndUpdates(t *testing.T) {
	manager, server := newStateWSServer(t)
	defer server.Close()
	ticket := issueStateWSTicket(t, server.URL, "admin")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/admin/state/ws?ticket=" + url.QueryEscape(ticket)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var initial StateResponse
	if err := conn.ReadJSON(&initial); err != nil {
		t.Fatal(err)
	}
	if initial.Version == "" {
		t.Fatal("initial state was not sent")
	}

	createUser(t, server.URL, "client-a")
	var next StateResponse
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := conn.ReadJSON(&next); err != nil {
		t.Fatal(err)
	}
	if !stateHasUser(next, "client-a") {
		t.Fatalf("websocket update did not include created client: %+v", next.Users)
	}
	if !stateHasUser(manager.stateResponse(), "client-a") {
		t.Fatal("control state did not include created client")
	}
}

func TestAdminStateWebSocketRejectsAdminTokenQuery(t *testing.T) {
	_, server := newStateWSServer(t)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/admin/state/ws?admin_token=admin"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("websocket connected with admin token in query")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", respStatus(resp))
	}
}

func TestAdminStateWebSocketTicketIsSingleUse(t *testing.T) {
	_, server := newStateWSServer(t)
	defer server.Close()
	ticket := issueStateWSTicket(t, server.URL, "admin")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/admin/state/ws?ticket=" + url.QueryEscape(ticket)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("websocket reused a consumed ticket")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", respStatus(resp))
	}
}

func TestAdminStateWebSocketTicketRequiresBearerToken(t *testing.T) {
	_, server := newStateWSServer(t)
	defer server.Close()
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/admin/state/ws-ticket?admin_token=admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ticket status = %d, want 401", resp.StatusCode)
	}
}

func TestAdminStateWebSocketRequiresAdminToken(t *testing.T) {
	_, server := newStateWSServer(t)
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/admin/state/ws"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("websocket connected without admin token")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", respStatus(resp))
	}
}

func newStateWSServer(t *testing.T) (*Manager, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(dir, "comrad.json"),
		ArtifactDir:  filepath.Join(dir, "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
		AutoApprove:  true,
		QueueLimit:   2,
		StreamWait:   time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager, httptest.NewServer(manager.Handler())
}

func issueStateWSTicket(t *testing.T, baseURL, token string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/admin/state/ws-ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("ticket status = %d", resp.StatusCode)
	}
	var out AdminStateWSTicketResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Ticket == "" || out.ExpiresAt.IsZero() {
		t.Fatalf("bad ticket response: %+v", out)
	}
	return out.Ticket
}

func createUser(t *testing.T, baseURL, userID string) {
	t.Helper()
	body, _ := json.Marshal(CreateUserRequest{ID: userID, Name: "Client A"})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/admin/users", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user status = %d", resp.StatusCode)
	}
}

func stateHasUser(state StateResponse, userID string) bool {
	for _, user := range state.Users {
		if user.ID == userID {
			return true
		}
	}
	return false
}

func respStatus(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}
