package comrad

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminWorkerJoinUsesExternalURLAndRequiresAdmin(t *testing.T) {
	m := newTestManager(t, 2, time.Second, 3)
	m.cfg.ExternalURL = "https://manager.example/base/"
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/admin/worker-join")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("worker join without admin token = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/admin/worker-join", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("worker join status = %d", resp.StatusCode)
	}
	var out WorkerJoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ManagerURL != "https://manager.example/base" {
		t.Fatalf("manager URL = %q", out.ManagerURL)
	}
	for _, want := range []string{"COMRAD_MANAGER_URL='https://manager.example/base'", "COMRAD_WORKER_TOKEN='worker'", "scripts/install-worker-macos.sh"} {
		if !strings.Contains(out.InstallCommand, want) {
			t.Fatalf("install command missing %q: %s", want, out.InstallCommand)
		}
	}
}
