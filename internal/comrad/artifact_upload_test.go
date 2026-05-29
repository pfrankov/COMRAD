package comrad

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAdminCanUploadModelArtifact(t *testing.T) {
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
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	body, contentType := multipartArtifactBody(t, "gemma.gguf", "model_gguf", []byte("GGUF upload"))
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/admin/artifacts/upload", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload status = %d", resp.StatusCode)
	}
	var artifact Artifact
	if err := json.NewDecoder(resp.Body).Decode(&artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Kind != "model_gguf" || artifact.Name != "gemma.gguf" {
		t.Fatalf("artifact metadata = %+v", artifact)
	}
	if err := VerifyFileSHA256(artifact.Path, artifact.SHA256); err != nil {
		t.Fatalf("uploaded artifact not stored with matching sha: %v", err)
	}
}

func TestAdminCanDeleteUnusedUploadedArtifact(t *testing.T) {
	manager, server := newArtifactAdminServer(t)
	defer server.Close()
	artifact := uploadTestArtifact(t, server.URL, []byte("GGUF delete"))

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/api/admin/artifacts/"+artifact.ID, nil)
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
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	if _, err := os.Stat(artifact.Path); !os.IsNotExist(err) {
		t.Fatalf("uploaded file still exists or stat failed unexpectedly: %v", err)
	}
	if _, ok := manager.store.Snapshot().Artifacts[artifact.ID]; ok {
		t.Fatal("artifact still exists in manager state")
	}
}

func TestAdminCannotDeleteArtifactUsedByProfile(t *testing.T) {
	manager, server := newArtifactAdminServer(t)
	defer server.Close()
	artifact := uploadTestArtifact(t, server.URL, []byte("GGUF in use"))
	if err := manager.store.Update(func(db *Database) error {
		db.Profiles["profile"] = WorkloadProfile{ID: "profile", Kind: "llm.chat", Artifacts: []string{artifact.ID}, CreatedAt: time.Now().UTC()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/api/admin/artifacts/"+artifact.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	if _, ok := manager.store.Snapshot().Artifacts[artifact.ID]; !ok {
		t.Fatal("artifact was deleted while still used by a profile")
	}
	if err := VerifyFileSHA256(artifact.Path, artifact.SHA256); err != nil {
		t.Fatalf("artifact file was modified or removed: %v", err)
	}
}

func newArtifactAdminServer(t *testing.T) (*Manager, *httptest.Server) {
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

func uploadTestArtifact(t *testing.T, baseURL string, content []byte) Artifact {
	t.Helper()
	body, contentType := multipartArtifactBody(t, "gemma.gguf", "model_gguf", content)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/admin/artifacts/upload", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload status = %d", resp.StatusCode)
	}
	var artifact Artifact
	if err := json.NewDecoder(resp.Body).Decode(&artifact); err != nil {
		t.Fatal(err)
	}
	return artifact
}

func multipartArtifactBody(t *testing.T, name, kind string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := bytes.NewBuffer(nil)
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("kind", kind)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}
