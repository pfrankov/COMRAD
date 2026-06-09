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
		db.Artifacts["sha256:linux-model"] = Artifact{ID: "sha256:linux-model", SHA256: "sha256:linux-model", Kind: "model_gguf", Name: "linux-model.gguf", CreatedAt: now}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProfileYAMLWithRuntimeVariants(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	profile := createProfileFromYAML(t, server.URL, `
profileId: llm.chat/multi-target-model
model: multi-target-model
kind: llm.chat
computeCost: 1
runtime:
  adapter: llama.cpp-metal
  modelArtifacts: [sha256:model, sha256:mmproj]
  contextTokens: 512
requirements:
  target: darwin-arm64-metal
  unifiedMemoryBytes: 6442450944
  diskBytes: 8589934592
warmable: true
runtimeVariants:
  - variantId: metal
    target: darwin-arm64-metal
    adapter: llama.cpp-metal
    modelArtifacts: [sha256:model, sha256:mmproj]
  - variantId: cuda
    target: linux-amd64-cuda
    adapter: llama.cpp-cuda
    modelArtifacts: [sha256:linux-model]
`)

	if len(profile.RuntimeVariants) != 2 {
		t.Fatalf("expected 2 runtimeVariants, got %d", len(profile.RuntimeVariants))
	}
	if profile.RuntimeVariants[0].ID != "metal" {
		t.Fatalf("variant[0].variantId = %q", profile.RuntimeVariants[0].ID)
	}
	if profile.RuntimeVariants[0].Target != "darwin-arm64-metal" {
		t.Fatalf("variant[0].target = %q", profile.RuntimeVariants[0].Target)
	}
	if profile.RuntimeVariants[0].RuntimeAdapter != "llama.cpp-metal" {
		t.Fatalf("variant[0].adapter = %q", profile.RuntimeVariants[0].RuntimeAdapter)
	}
	if strings.Join(profile.RuntimeVariants[0].Artifacts, ",") != "sha256:model,sha256:mmproj" {
		t.Fatalf("variant[0].artifacts = %+v", profile.RuntimeVariants[0].Artifacts)
	}
	if profile.RuntimeVariants[1].ID != "cuda" {
		t.Fatalf("variant[1].variantId = %q", profile.RuntimeVariants[1].ID)
	}
	if profile.RuntimeVariants[1].Target != "linux-amd64-cuda" {
		t.Fatalf("variant[1].target = %q", profile.RuntimeVariants[1].Target)
	}
	if profile.RuntimeVariants[1].RuntimeAdapter != "llama.cpp-cuda" {
		t.Fatalf("variant[1].adapter = %q", profile.RuntimeVariants[1].RuntimeAdapter)
	}
	if strings.Join(profile.RuntimeVariants[1].Artifacts, ",") != "sha256:linux-model" {
		t.Fatalf("variant[1].artifacts = %+v", profile.RuntimeVariants[1].Artifacts)
	}
}

func TestProfileYAMLVariantsInheritParentArtifacts(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	profile := createProfileFromYAML(t, server.URL, `
profileId: llm.chat/inherit-artifacts
model: inherit-artifacts
kind: llm.chat
computeCost: 1
runtime:
  adapter: llama.cpp-metal
  modelArtifacts: [sha256:model]
  contextTokens: 512
requirements:
  target: darwin-arm64-metal
warmable: true
runtimeVariants:
  - variantId: metal
    target: darwin-arm64-metal
    adapter: llama.cpp-metal
`)

	if len(profile.RuntimeVariants) != 1 {
		t.Fatalf("expected 1 runtimeVariant, got %d", len(profile.RuntimeVariants))
	}
	normalized := normalizeVariant(profile, profile.RuntimeVariants[0])
	if strings.Join(normalized.Artifacts, ",") != "sha256:model" {
		t.Fatalf("normalized variant artifacts = %+v, expected inherited from parent", normalized.Artifacts)
	}
}

func TestProfileYAMLVariantsInheritParentRuntime(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	profile := createProfileFromYAML(t, server.URL, `
profileId: llm.chat/inherit-runtime
model: inherit-runtime
kind: llm.chat
computeCost: 1
runtime:
  adapter: llama.cpp-metal
  modelArtifacts: [sha256:model]
  contextTokens: 512
  llamaCpp:
    args: ["-ngl", "42", "--threads", "6"]
requirements:
  target: darwin-arm64-metal
warmable: true
runtimeVariants:
  - variantId: metal
    target: darwin-arm64-metal
    adapter: llama.cpp-metal
`)

	if len(profile.RuntimeVariants) != 1 {
		t.Fatalf("expected 1 runtimeVariant, got %d", len(profile.RuntimeVariants))
	}
	variant := profile.RuntimeVariants[0]
	// normalizeVariant should inherit Runtime from parent when variant has no llamaCpp args
	normalized := normalizeVariant(profile, variant)
	if strings.Join(normalized.Runtime.LlamaCpp.Args, " ") != "-ngl 42 --threads 6" {
		t.Fatalf("variant runtime args = %+v, expected inherited from parent", normalized.Runtime.LlamaCpp.Args)
	}
}

func TestProfileYAMLVariantRequiresTarget(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	body := []byte(`
profileId: llm.chat/no-target-variant
model: no-target-variant
kind: llm.chat
computeCost: 1
runtime:
  adapter: llama.cpp-metal
  modelArtifacts: [sha256:model]
  contextTokens: 512
requirements:
  target: darwin-arm64-metal
warmable: true
runtimeVariants:
  - variantId: bad
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
		t.Fatalf("expected 400 for variant without target, got %d", resp.StatusCode)
	}
}

func TestProfileYAMLRejectsManagedVariantLlamaArgs(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	body := []byte(`
profileId: llm.chat/bad-variant-args
model: bad-variant-args
kind: llm.chat
computeCost: 1
runtime:
  adapter: llama.cpp-metal
  modelArtifacts: [sha256:model]
  contextTokens: 512
requirements:
  target: darwin-arm64-metal
warmable: true
runtimeVariants:
  - variantId: metal
    target: darwin-arm64-metal
    adapter: llama.cpp-metal
    llamaCpp:
      args: ["--host", "0.0.0.0"]
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
		t.Fatalf("expected 400 for variant with managed args, got %d", resp.StatusCode)
	}
}

func TestProfileYAMLVariantRequiresAdapter(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	body := []byte(`
profileId: llm.chat/no-adapter-variant
model: no-adapter-variant
kind: llm.chat
computeCost: 1
runtime:
  adapter: llama.cpp-metal
  modelArtifacts: [sha256:model]
  contextTokens: 512
requirements:
  target: darwin-arm64-metal
warmable: true
runtimeVariants:
  - variantId: cuda
    target: linux-amd64-cuda
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
		t.Fatalf("expected 400 for variant without adapter, got %d", resp.StatusCode)
	}
}

func TestProfileYAMLVariantsRelaxParentRuntime(t *testing.T) {
	manager := newProfileConfigManager(t)
	seedProfileConfigArtifacts(t, manager)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	profile := createProfileFromYAML(t, server.URL, `
profileId: llm.chat/variants-only
model: variants-only
kind: llm.chat
computeCost: 1
runtime:
  adapter: llama.cpp-metal
  contextTokens: 0
requirements:
  target: darwin-arm64-metal
warmable: true
runtimeVariants:
  - variantId: metal
    target: darwin-arm64-metal
    adapter: llama.cpp-metal
    modelArtifacts: [sha256:model]
    contextTokens: 512
  - variantId: cuda
    target: linux-amd64-cuda
    adapter: llama.cpp-cuda
    modelArtifacts: [sha256:linux-model]
    contextTokens: 512
`)

	if len(profile.RuntimeVariants) != 2 {
		t.Fatalf("expected 2 runtimeVariants, got %d", len(profile.RuntimeVariants))
	}
}
