package comrad

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProfileYAMLUsesMinimalRuntimeConfig(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	body := []byte(`
profileId: llm.chat/gemma-4-e2b/context-512
model: gemma-4-e2b
kind: llm.chat
runtime:
  adapter: llama.cpp-metal
  modelArtifacts:
    - sha256:model
    - sha256:mmproj
  contextTokens: 512
  llamaCpp:
    args: ["-ngl", "42", "--threads", "6"]
requirements:
  target: darwin-arm64-metal
  unifiedMemoryBytes: 6442450944
  diskBytes: 8589934592
warmable: true
`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/admin/profiles", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("profile create status = %d", resp.StatusCode)
	}
	var profile WorkloadProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		t.Fatal(err)
	}
	assertMinimalYAMLProfile(t, profile)
}

func TestProfileYAMLRequiresModelArtifactsForLlamaCpp(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	body := []byte(`
profileId: llm.chat/gemma-4-e2b/context-512
model: gemma-4-e2b
kind: llm.chat
runtime:
  adapter: llama.cpp-metal
  contextTokens: 512
requirements:
  target: darwin-arm64-metal
warmable: true
`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/admin/profiles", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("profile create status = %d", resp.StatusCode)
	}
}

func TestProfileYAMLRejectsManagedLlamaServerArgs(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	body := []byte(`
profileId: llm.chat/gemma-4-e2b/context-512
model: gemma-4-e2b
kind: llm.chat
runtime:
  adapter: llama.cpp-metal
  modelArtifacts: [sha256:model]
  contextTokens: 512
  llamaCpp:
    args: ["--host", "0.0.0.0"]
requirements:
  target: darwin-arm64-metal
warmable: true
`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/admin/profiles", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("profile create status = %d", resp.StatusCode)
	}
}

func TestProfileYAMLUpsertEditsExistingModel(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	createProfileFromYAML(t, server.URL, `
profileId: llm.chat/gemma-4-e2b/context-512
model: gemma-4-e2b
kind: llm.chat
runtime:
  adapter: llama.cpp-metal
  modelArtifacts: [sha256:model]
  contextTokens: 512
requirements:
  target: darwin-arm64-metal
warmable: true
`)
	profile := createProfileFromYAML(t, server.URL, `
profileId: llm.chat/gemma-4-e2b/context-512
model: e4b
kind: llm.chat
computeCost: 3
runtime:
  adapter: llama.cpp-metal
  modelArtifacts: [sha256:model]
  contextTokens: 512
  llamaCpp:
    args: ["-ngl", "99"]
requirements:
  target: darwin-arm64-metal
warmable: true
`)

	if profile.ID != "llm.chat/gemma-4-e2b/context-512" {
		t.Fatalf("profile ID changed during edit: %q", profile.ID)
	}
	if got := ProfileLogicalModel(profile); got != "e4b" {
		t.Fatalf("logical model after edit = %q", got)
	}
	if profile.Version != 2 {
		t.Fatalf("profile version after edit = %d", profile.Version)
	}
	if profile.ComputeCost != 3 {
		t.Fatalf("compute cost after edit = %d", profile.ComputeCost)
	}
}

func assertMinimalYAMLProfile(t *testing.T, profile WorkloadProfile) {
	t.Helper()
	if ProfileLogicalModel(profile) != "gemma-4-e2b" {
		t.Fatalf("logical model = %q", ProfileLogicalModel(profile))
	}
	if strings.Join(profile.Artifacts, ",") != "sha256:model,sha256:mmproj" {
		t.Fatalf("artifacts = %+v", profile.Artifacts)
	}
	if profile.LLM == nil || profile.LLM.ContextTokens != 512 {
		t.Fatalf("llm = %+v", profile.LLM)
	}
	if profile.Requirements == nil || profile.Requirements.RuntimeAdapter != "" {
		t.Fatalf("requirements duplicate runtime adapter: %+v", profile.Requirements)
	}
	if strings.Join(profile.Runtime.LlamaCpp.Args, " ") != "-ngl 42 --threads 6" {
		t.Fatalf("llama args = %+v", profile.Runtime.LlamaCpp.Args)
	}
	raw, _ := json.Marshal(profile)
	for _, removed := range []string{"quantization", "modelSha256", "tokenizerHash", "modelFamily", "runtimeArtifact"} {
		if strings.Contains(string(raw), removed) {
			t.Fatalf("profile response contains removed field %q: %s", removed, raw)
		}
	}
}

func createProfileFromYAML(t *testing.T, baseURL string, body string) WorkloadProfile {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/admin/profiles", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("profile upsert status = %d", resp.StatusCode)
	}
	var profile WorkloadProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestLlamaServerArgsComeFromProfile(t *testing.T) {
	t.Setenv("COMRAD_LLAMA_CPP_EXTRA_ARGS", "--bad-env-arg")
	profile := WorkloadProfile{
		ID:             "llm.chat/test/context-512",
		RuntimeAdapter: "llama.cpp-metal",
		LLM:            &LLMProfile{ContextTokens: 512},
		Runtime:        RuntimeParameters{LlamaCpp: LlamaCppParameters{Args: []string{"-ngl", "42", "--threads", "6"}}},
	}
	args, err := llamaServerArgs(profile, "/models/test.gguf", nil, 18081)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--host 127.0.0.1", "--port 18081", "--model /models/test.gguf", "--ctx-size 512", "-ngl 42", "--threads 6"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "--bad-env-arg") {
		t.Fatalf("args unexpectedly include Worker env fallback: %s", joined)
	}
}

func TestLlamaServerArgsRejectManagedFlags(t *testing.T) {
	cases := [][]string{
		{"--host", "0.0.0.0"},
		{"--port=9000"},
		{"--model", "/tmp/other.gguf"},
		{"-m", "/tmp/other.gguf"},
		{"--mmproj", "/tmp/mmproj.gguf"},
		{"--ctx-size", "4096"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			profile := WorkloadProfile{
				ID:             "llm.chat/test/context-512",
				RuntimeAdapter: "llama.cpp-metal",
				LLM:            &LLMProfile{ContextTokens: 512},
				Runtime:        RuntimeParameters{LlamaCpp: LlamaCppParameters{Args: args}},
			}
			if _, err := llamaServerArgs(profile, "/models/test.gguf", nil, 18081); err == nil {
				t.Fatalf("llamaServerArgs accepted managed args: %+v", args)
			}
		})
	}
}

func newProfileConfigManager(t *testing.T) *Manager {
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
	return manager
}

func seedProfileConfigArtifacts(t *testing.T, manager *Manager) {
	t.Helper()
	now := time.Now().UTC()
	if err := manager.store.Update(func(db *Database) error {
		db.Artifacts["sha256:model"] = Artifact{ID: "sha256:model", SHA256: "sha256:model", Kind: "model_gguf", Name: "model.gguf", CreatedAt: now}
		db.Artifacts["sha256:mmproj"] = Artifact{ID: "sha256:mmproj", SHA256: "sha256:mmproj", Kind: "model_mmproj", Name: "mmproj.gguf", CreatedAt: now}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
