package comrad

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrometheusMetricsEndpoint(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.sqlite"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(manager.Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	for _, want := range []string{
		"# HELP comrad_queue_limit",
		"comrad_queue_limit",
		"comrad_tasks_total",
		`comrad_storage_backend_info{backend="sqlite"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "prompt") || strings.Contains(strings.ToLower(body), "response") {
		t.Fatalf("metrics leak prompt/response labels:\n%s", body)
	}
}

func TestPrometheusMetricsExposeAdminStateWebSocketState(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.sqlite"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := manager.addAdminStateSubscriber()
	defer manager.removeAdminStateSubscriber(id)
	manager.publishAdminState()

	body := metricsBody(t, manager)
	for _, want := range []string{
		"comrad_admin_state_ws_clients 1",
		"comrad_admin_state_ws_connects_total 1",
		"comrad_admin_state_ws_broadcasts_total 1",
		"comrad_admin_state_ws_last_broadcast_subscribers 1",
		"comrad_admin_state_ws_last_snapshot_bytes",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
}

func metricsBody(t *testing.T, manager *Manager) string {
	t.Helper()
	server := httptest.NewServer(manager.Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.StatusCode, string(b))
	}
	return string(b)
}
