package comrad

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsAndMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comrad.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	artifact := Artifact{ID: "sha256:abc", Kind: "model_gguf", Name: "tiny.gguf", SHA256: "sha256:abc", SizeBytes: 3, CreatedAt: time.Now().UTC()}
	if err := store.Update(func(db *Database) error {
		db.Artifacts[artifact.ID] = artifact
		db.Profiles["p1"] = WorkloadProfile{ID: "p1", Kind: "llm.chat", Alias: "assistant", CreatedAt: time.Now().UTC()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	db := reopened.Snapshot()
	if db.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema version = %d", db.SchemaVersion)
	}
	if _, ok := db.Artifacts[artifact.ID]; !ok {
		t.Fatal("artifact did not persist")
	}
	if _, ok := db.Profiles["p1"]; !ok {
		t.Fatal("profile did not persist")
	}
}

func TestFitRejectsProfileWithoutRequirements(t *testing.T) {
	node := Node{ID: "node1", Approved: true, State: NodeStateOnline, RuntimeAdapters: []string{"llama.cpp-metal"}}
	slot := Slot{ID: "node1/metal0", NodeID: "node1", Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal"}
	profile := WorkloadProfile{ID: "p1", Kind: "llm.chat", RuntimeAdapter: "llama.cpp-metal", Artifacts: []string{"sha256:abc"}, LLM: &LLMProfile{ContextTokens: 4096}}
	fit := FitProfileToSlot(profile, node, slot)
	if fit.Fits {
		t.Fatal("profile without requirements should not fit")
	}
	if !Contains(fit.Reasons, FailureUnknownRequirements) {
		t.Fatalf("expected unknown_requirements, got %v", fit.Reasons)
	}
}

func TestFitAllowsLlamaCppProfileWithWorkerInstalledRuntime(t *testing.T) {
	node := Node{ID: "node1", Approved: true, State: NodeStateOnline, RuntimeAdapters: []string{"llama.cpp-metal"}}
	slot := Slot{ID: "node1/metal0", NodeID: "node1", Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal"}
	profile := WorkloadProfile{
		ID:             "p1",
		Kind:           "llm.chat",
		RuntimeAdapter: "llama.cpp-metal",
		Artifacts:      []string{"sha256:model"},
		Requirements:   &Requirements{Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal"},
		LLM:            &LLMProfile{ContextTokens: 4096},
	}
	fit := FitProfileToSlot(profile, node, slot)
	if !fit.Fits {
		t.Fatalf("llama.cpp profile should fit with model artifacts only, got %v", fit.Reasons)
	}
}

func TestResolveLLMProfileChoosesMinimumSufficientContext(t *testing.T) {
	db := emptyDatabase()
	req := &Requirements{Target: TargetDarwinArm64Metal, RuntimeAdapter: "llama.cpp-metal"}
	db.Profiles["p4k"] = WorkloadProfile{ID: "p4k", Name: "p4k", Alias: "assistant", Kind: "llm.chat", Artifacts: []string{"sha256:a"}, Requirements: req, LLM: &LLMProfile{ContextTokens: 4096}}
	db.Profiles["p8k"] = WorkloadProfile{ID: "p8k", Name: "p8k", Alias: "assistant", Kind: "llm.chat", Artifacts: []string{"sha256:b"}, Requirements: req, LLM: &LLMProfile{ContextTokens: 8192}}
	db.Profiles["p32k"] = WorkloadProfile{ID: "p32k", Name: "p32k", Alias: "assistant", Kind: "llm.chat", Artifacts: []string{"sha256:c"}, Requirements: req, LLM: &LLMProfile{ContextTokens: 32768}}
	profile, err := ResolveLLMProfile(db, "assistant", 6000)
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "p8k" {
		t.Fatalf("selected %s, want p8k", profile.ID)
	}
	if _, err := ResolveLLMProfile(db, "assistant", 40000); err == nil {
		t.Fatal("expected insufficient context error")
	}
}

func TestVerifyFileSHA256RejectsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.gguf")
	if err := os.WriteFile(path, []byte("GGUF-ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, _, err := FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFileSHA256(path, sha); err == nil {
		t.Fatal("expected sha256 verification to reject corrupted artifact")
	}
}
