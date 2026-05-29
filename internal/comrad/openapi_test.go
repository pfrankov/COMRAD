package comrad

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAPISpecIsAdminOnlyAndDescribesCoreRoutes(t *testing.T) {
	manager := newOpenAPITestManager(t)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	assertOpenAPIRequiresAdmin(t, server.URL)
	body, spec := getOpenAPISpec(t, server.URL)
	if spec["openapi"] != "3.1.0" {
		t.Fatalf("openapi version = %v", spec["openapi"])
	}
	assertOpenAPIPaths(t, spec)
	assertNoOpenAPISecrets(t, string(body))
}

func TestOpenAPIDocsPage(t *testing.T) {
	manager := newOpenAPITestManager(t)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	body := getOpenAPIDocs(t, server.URL)
	for _, want := range []string{"COMRAD API Reference", "OpenAPI 3.1", "/api/admin/openapi.json"} {
		if !strings.Contains(body, want) {
			t.Fatalf("docs missing %q", want)
		}
	}
	if strings.Contains(body, "e2e-admin-token") || strings.Contains(body, "Bearer admin") || strings.Contains(body, "admin_token") {
		t.Fatal("docs page leaks a configured token")
	}
}

func assertOpenAPIRequiresAdmin(t *testing.T, serverURL string) {
	t.Helper()
	resp, err := http.Get(serverURL + "/api/admin/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("openapi without admin token = %d", resp.StatusCode)
	}
}

func getOpenAPISpec(t *testing.T, serverURL string) ([]byte, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/admin/openapi.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("openapi status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatal(err)
	}
	return body, spec
}

func assertOpenAPIPaths(t *testing.T, spec map[string]any) {
	t.Helper()
	paths := spec["paths"].(map[string]any)
	for _, path := range []string{"/v1/models", "/v1/chat/completions", "/api/admin/state/ws-ticket", "/api/admin/config.yaml", "/api/admin/artifacts/upload", "/api/admin/artifacts/{artifactId}", "/api/admin/nodes/{nodeId}/artifacts/{artifactId}", "/api/admin/profiles", "/api/admin/api-keys/lookup", "/api/worker/ws"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("spec missing path %s", path)
		}
	}
	if _, ok := paths["/api/admin/profiles"].(map[string]any)["delete"]; !ok {
		t.Fatal("spec missing profile delete operation")
	}
	if _, ok := paths["/api/admin/nodes/{nodeId}/artifacts/{artifactId}"].(map[string]any)["delete"]; !ok {
		t.Fatal("spec missing worker artifact eviction operation")
	}
}

func assertNoOpenAPISecrets(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{"admin", "client", "worker"} {
		if strings.Contains(text, "Bearer "+secret) {
			t.Fatalf("spec leaks configured token %q", secret)
		}
	}
}

func getOpenAPIDocs(t *testing.T, serverURL string) string {
	t.Helper()
	ticket := issueStateWSTicket(t, serverURL, "admin")
	resp, err := http.Get(serverURL + "/api/admin/docs?ticket=" + url.QueryEscape(ticket))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("docs status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func newOpenAPITestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(dir, "comrad.json"),
		ArtifactDir:  filepath.Join(dir, "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
		AutoApprove:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
