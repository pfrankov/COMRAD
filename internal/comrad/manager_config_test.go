package comrad

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminConfigYAMLRedactsSecrets(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		Addr:           "127.0.0.1:1922",
		DBPath:         filepath.Join(t.TempDir(), "comrad.sqlite"),
		DatabaseURL:    "postgres://comrad:postgres-secret@postgres:5432/comrad?sslmode=disable",
		StorageMode:    "auto",
		ArtifactDir:    filepath.Join(t.TempDir(), "artifacts"),
		AdminToken:     "admin-secret",
		ClientAPIKey:   "client-secret",
		WorkerToken:    "worker-secret",
		EnforceBalance: true,
		QueueLimit:     7,
		StreamWait:     3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.store.Update(func(db *Database) error {
		db.Nodes["node-a"] = Node{ID: "node-a", State: NodeStateOnline, Approved: true, P2P: &WorkerP2PStatus{Available: true, Port: 39011, MaxUploads: 12, DownloadTimeoutSeconds: 45}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/admin/config.yaml", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	for _, want := range []string{
		"version:",
		"manager:",
		"listen: 127.0.0.1:1922",
		"storage:",
		"mode: auto",
		"backend:",
		"auth:",
		"adminToken: '<redacted: configured>'",
		"scheduler:",
		"queueLimit: 7",
		"streamWaitSeconds: 3",
		"autoBalanceScaleDownCooldownSeconds: 300",
		"workerFlap:",
		"cooldownSeconds: 300",
		"workers:",
		"connection: outboundWebSocket",
		"p2p:",
		"mode: bestEffortPublicBitTorrent",
		"defaultPort: 6881",
		"availableWorkers: 1",
		"effectivePorts:",
		"- 39011",
		"observability:",
		"dashboardStateStream: websocket",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("config yaml missing %q:\n%s", want, body)
		}
	}
	for _, secret := range []string{"admin-secret", "client-secret", "worker-secret", "postgres-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("config yaml leaked secret %q:\n%s", secret, body)
		}
	}
}
