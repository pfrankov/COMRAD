package comrad

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestManagerRequiresExplicitProductionTokens(t *testing.T) {
	dir := t.TempDir()
	_, err := NewManager(ManagerConfig{
		Addr:        "127.0.0.1:1922",
		DBPath:      filepath.Join(dir, "manager.json"),
		ArtifactDir: filepath.Join(dir, "artifacts"),
	})
	if err == nil || !strings.Contains(err.Error(), "COMRAD_ADMIN_TOKEN") {
		t.Fatalf("NewManager err = %v, want missing token error", err)
	}
}

func TestManagerRejectsDevTokensWithoutLocalOverride(t *testing.T) {
	dir := t.TempDir()
	_, err := NewManager(ManagerConfig{
		Addr:         "127.0.0.1:1922",
		DBPath:       filepath.Join(dir, "manager.json"),
		ArtifactDir:  filepath.Join(dir, "artifacts"),
		AdminToken:   "dev-admin-token",
		ClientAPIKey: "dev-client-key",
		WorkerToken:  "dev-worker-token",
	})
	if err == nil || !strings.Contains(err.Error(), "COMRAD_ALLOW_DEV_DEFAULTS") {
		t.Fatalf("NewManager err = %v, want dev token rejection", err)
	}
}

func TestManagerRejectsDevTokensOnPublicBind(t *testing.T) {
	dir := t.TempDir()
	_, err := NewManager(ManagerConfig{
		Addr:           "0.0.0.0:8080",
		DBPath:         filepath.Join(dir, "manager.json"),
		ArtifactDir:    filepath.Join(dir, "artifacts"),
		AllowDevTokens: true,
	})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("NewManager err = %v, want public bind rejection", err)
	}
}

func TestAuthProtectedSurfacesRejectMissingTokens(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(dir, "manager.json"),
		ArtifactDir:  filepath.Join(dir, "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
		AutoApprove:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/api/admin/state", want: http.StatusUnauthorized},
		{path: "/v1/models", want: http.StatusUnauthorized},
		{path: "/api/worker/artifacts/sha256:missing", want: http.StatusUnauthorized},
	} {
		resp, err := http.Get(server.URL + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("%s returned %d, want %d", tc.path, resp.StatusCode, tc.want)
		}
	}
}

func TestAdminRoutesRejectQueryToken(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(dir, "manager.json"),
		ArtifactDir:  filepath.Join(dir, "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/admin/state?admin_token=admin")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("query token status = %d, want 401", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/admin/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bearer token status = %d, want 200", resp.StatusCode)
	}
}

func TestDashboardServesOperatorControlCenter(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(dir, "manager.json"),
		ArtifactDir:  filepath.Join(dir, "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
		AutoApprove:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en;q=0.7")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard returned %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("dashboard cache control = %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	assertTextContainsAll(t, html, "dashboard", "COMRAD Manager", `id="root"`, "/dashboard/assets/")
	assertTextContainsAll(t, html, "dashboard", `window.__COMRAD_SYSTEM_LOCALE__ = "ru"`)
	if strings.Contains(html, "function renderArtifacts") {
		t.Fatal("dashboard still serves the legacy inline renderer")
	}
	js := fetchDashboardBundle(t, server.URL, html)
	assertTextContainsAll(t, js, "dashboard bundle", "Needs attention", "Quarantine", "Add a model", "Edit model", "Existing models", "Linked model files", "Storage inventory", "llama.cpp server args", "application/yaml", "API clients", "Compute cost", "One-time API key", "Find an API client", "Lookup key", "Edit client", "Top up balance", "Issue API key", "Server-side filters", "Capacity", "Set ready copies", "COMRAD operating model", "Serve models", "Supply capacity", "Control access", "Observe requests", "Admin workflows", "How COMRAD works", "Upload model files", "Worker software updates", "Info hash", "Delete artifact", "unused artifacts only", "Readiness triage", "Capacity planner")
	assertTextContainsNone(t, js, "dashboard bundle", "model-quant", "COMRAD_LLAMA_CPP_ENABLED", "COMRAD_LLAMA_CPP_PATH", "llama-cli", "Placement policy editor", "runtime archive", "Rename or tune", "Supporting model files", "Manager-local GGUF path", "Or use a Manager-local")
}

func assertTextContainsAll(t *testing.T, text, name string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("%s missing %q", name, want)
		}
	}
}

func assertTextContainsNone(t *testing.T, text, name string, rejects ...string) {
	t.Helper()
	for _, reject := range rejects {
		if strings.Contains(text, reject) {
			t.Fatalf("%s unexpectedly contains %q", name, reject)
		}
	}
}

func fetchDashboardBundle(t *testing.T, baseURL, html string) string {
	t.Helper()
	re := regexp.MustCompile(`/dashboard/assets/[^"]+\.js`)
	path := re.FindString(html)
	if path == "" {
		t.Fatalf("dashboard has no JS asset: %s", html)
	}
	resp, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard asset returned %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("dashboard asset cache control = %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
