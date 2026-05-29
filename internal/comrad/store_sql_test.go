package comrad

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSQLiteStorePersistsManagerState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comrad.sqlite")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.BackendName() != "sqlite" {
		t.Fatalf("backend = %s, want sqlite", store.BackendName())
	}
	artifact := Artifact{ID: "sha256:sqlite", Kind: "model_gguf", Name: "tiny.gguf", SHA256: "sha256:sqlite", SizeBytes: 4, CreatedAt: time.Now().UTC()}
	if err := store.Update(func(db *Database) error {
		db.Artifacts[artifact.ID] = artifact
		db.Profiles["sqlite-profile"] = WorkloadProfile{ID: "sqlite-profile", Kind: "llm.chat", Alias: "assistant", CreatedAt: time.Now().UTC()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	db := reopened.Snapshot()
	if _, ok := db.Artifacts[artifact.ID]; !ok {
		t.Fatal("artifact did not persist in sqlite")
	}
	if _, ok := db.Profiles["sqlite-profile"]; !ok {
		t.Fatal("profile did not persist in sqlite")
	}
}

func TestSQLStorePersistsCompactStateJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compact.sqlite")
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(db *Database) error {
		db.Tasks["task-a"] = Task{ID: "task-a", Status: TaskStatusCompleted, CreatedAt: time.Now().UTC()}
		db.Reports["report-a"] = ComputeReport{ID: "report-a", TaskID: "task-a", Status: TaskStatusCompleted, CreatedAt: time.Now().UTC()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	backend, ok := store.backend.(*sqlBackend)
	if !ok {
		t.Fatalf("backend = %T, want *sqlBackend", store.backend)
	}
	var raw string
	if err := backend.db.QueryRow("select data from comrad_state where id = 1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(raw)) {
		t.Fatalf("saved state is not valid JSON: %q", raw)
	}
	if strings.Contains(raw, "\n  ") {
		t.Fatalf("saved state is pretty-printed; size=%d", len(raw))
	}
}

func TestManagerStorageAutoFallsBackToSQLiteWhenPostgresUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fallback.sqlite")
	manager, err := NewManager(ManagerConfig{
		DBPath:       path,
		DatabaseURL:  "postgres://127.0.0.1:1/comrad?connect_timeout=1",
		StorageMode:  "auto",
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
	})
	if err != nil {
		t.Fatal(err)
	}
	if manager.Store().BackendName() != "sqlite" {
		t.Fatalf("backend = %s, want sqlite fallback", manager.Store().BackendName())
	}
	if !strings.Contains(manager.Store().Path(), path) {
		t.Fatalf("store path = %s, want %s", manager.Store().Path(), path)
	}
	db := manager.Store().Snapshot()
	if len(db.Audit) < 2 || db.Audit[len(db.Audit)-1].Type != "storage.fallback" {
		t.Fatalf("fallback audit event missing: %+v", db.Audit)
	}
}

func TestManagerStoragePostgresModeFailsWhenPostgresUnavailable(t *testing.T) {
	_, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "fallback.sqlite"),
		DatabaseURL:  "postgres://127.0.0.1:1/comrad?connect_timeout=1",
		StorageMode:  "postgres",
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
		AdminToken:   "admin",
		ClientAPIKey: "client",
		WorkerToken:  "worker",
	})
	if err == nil {
		t.Fatal("expected postgres mode to fail instead of silently falling back")
	}
}
