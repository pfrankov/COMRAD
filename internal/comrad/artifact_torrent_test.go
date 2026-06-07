package comrad

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUploadedArtifactGetsStableTorrentMetadata(t *testing.T) {
	_, server := newArtifactAdminServer(t)
	defer server.Close()

	first := uploadNamedTestArtifact(t, server.URL, "alpha.gguf", []byte("same content"))
	second := uploadNamedTestArtifact(t, server.URL, "beta.gguf", []byte("same content"))

	if first.Torrent == nil {
		t.Fatal("first upload missing torrent metadata")
	}
	if second.Torrent == nil {
		t.Fatal("second upload missing torrent metadata")
	}
	if first.Torrent.InfoHash == "" || first.Torrent.MagnetURI == "" || first.Torrent.PieceLength == 0 {
		t.Fatalf("first upload torrent metadata incomplete: %+v", first.Torrent)
	}
	if len(first.Torrent.MetaInfoBytes) == 0 || first.Torrent.MetaInfoPath == "" {
		t.Fatalf("first upload missing metainfo bytes or path: %+v", first.Torrent)
	}
	if first.Torrent.InfoHash != second.Torrent.InfoHash ||
		first.Torrent.MagnetURI != second.Torrent.MagnetURI ||
		first.Torrent.PieceLength != second.Torrent.PieceLength ||
		string(first.Torrent.MetaInfoBytes) != string(second.Torrent.MetaInfoBytes) {
		t.Fatalf("torrent metadata changed across identical uploads:\nfirst=%+v\nsecond=%+v", first.Torrent, second.Torrent)
	}
	if !strings.Contains(first.Torrent.MagnetURI, strings.TrimPrefix(first.Torrent.InfoHash, "sha1:")) {
		t.Fatalf("magnet %q does not include info hash %q", first.Torrent.MagnetURI, first.Torrent.InfoHash)
	}
}

func TestRegisteredArtifactGetsTorrentMetadata(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "tiny.gguf")
	if err := osWriteFile(modelPath, []byte("registered artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	artifact := adminJSON[Artifact](t, server.URL, "admin", "/api/admin/artifacts", CreateArtifactRequest{
		Path: modelPath,
		Kind: "model_gguf",
		Name: "tiny.gguf",
	})
	if artifact.Torrent == nil || artifact.Torrent.InfoHash == "" || len(artifact.Torrent.MetaInfoBytes) == 0 {
		t.Fatalf("registered artifact missing torrent metadata: %+v", artifact)
	}
}

func TestAdminArtifactListOmitsRawMetainfo(t *testing.T) {
	_, server := newArtifactAdminServer(t)
	defer server.Close()

	uploaded := uploadNamedTestArtifact(t, server.URL, "alpha.gguf", []byte("same content"))
	if uploaded.Torrent == nil || len(uploaded.Torrent.MetaInfoBytes) == 0 {
		t.Fatalf("upload missing raw metainfo: %+v", uploaded.Torrent)
	}

	body := adminGET(t, server.URL, "admin", "/api/admin/artifacts")
	if strings.Contains(body, "metainfoBytes") || strings.Contains(body, "metainfoPath") {
		t.Fatalf("artifact list leaked raw metainfo: %s", body)
	}
	var artifacts []Artifact
	if err := json.Unmarshal([]byte(body), &artifacts); err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Torrent == nil || artifacts[0].Torrent.InfoHash == "" {
		t.Fatalf("artifact list missing torrent identity: %+v", artifacts)
	}
}

func TestAdminStateOmitsRawMetainfo(t *testing.T) {
	_, server := newArtifactAdminServer(t)
	defer server.Close()

	uploaded := uploadNamedTestArtifact(t, server.URL, "alpha.gguf", []byte("same content"))
	if uploaded.Torrent == nil || len(uploaded.Torrent.MetaInfoBytes) == 0 {
		t.Fatalf("upload missing raw metainfo: %+v", uploaded.Torrent)
	}

	body := adminGET(t, server.URL, "admin", "/api/admin/state")
	if strings.Contains(body, "metainfoBytes") || strings.Contains(body, "metainfoPath") {
		t.Fatalf("admin state leaked raw metainfo: %s", body)
	}
	var state StateResponse
	if err := json.Unmarshal([]byte(body), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Artifacts) != 1 || state.Artifacts[0].Torrent == nil || state.Artifacts[0].Torrent.InfoHash == "" {
		t.Fatalf("admin state missing torrent identity: %+v", state.Artifacts)
	}
}

func TestArtifactSpecsIncludeTorrentMetadata(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.json"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
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
	artifact := mustManagerArtifactWithTorrent(t, manager, "model.gguf", []byte("torrent payload"))
	profile := WorkloadProfile{ID: "profile-a", Artifacts: []string{artifact.ID}}

	specs := manager.artifactSpecs(profile, "http://manager.test")
	if len(specs) != 1 {
		t.Fatalf("artifact specs len = %d, want 1", len(specs))
	}
	if specs[0].Torrent == nil || specs[0].Torrent.InfoHash != artifact.Torrent.InfoHash {
		t.Fatalf("artifact spec missing torrent metadata: %+v", specs[0])
	}
}

func TestArtifactSpecsIncludeP2PPeers(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.json"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
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
	artifact := mustManagerArtifactWithTorrent(t, manager, "model.gguf", []byte("peer relay payload"))
	profile := WorkloadProfile{ID: "profile-b", Artifacts: []string{artifact.ID}}

	seeder := &workerSession{
		id:         "ses-seeder",
		nodeID:     "node-seeder",
		baseURL:    "http://manager.test",
		remoteHost: "192.168.1.50",
		manager:    manager,
		send:       make(chan Envelope, 1),
		done:       make(chan struct{}),
	}
	manager.mu.Lock()
	manager.sessions["node-seeder"] = seeder
	manager.mu.Unlock()
	if err := manager.store.Update(func(db *Database) error {
		db.Nodes["node-seeder"] = Node{
			ID:              "node-seeder",
			State:           NodeStateOnline,
			CachedArtifacts: []string{artifact.ID},
			P2P:             &WorkerP2PStatus{Available: true, Port: 6881},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	specs := manager.artifactSpecs(profile, "http://manager.test")
	if len(specs) != 1 {
		t.Fatalf("artifact specs len = %d, want 1", len(specs))
	}
	if len(specs[0].P2PPeers) != 1 || specs[0].P2PPeers[0] != "192.168.1.50:6881" {
		t.Fatalf("expected peer 192.168.1.50:6881, got %v", specs[0].P2PPeers)
	}
}

func TestArtifactSpecsOmitPeersForOfflineNode(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.json"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
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
	artifact := mustManagerArtifactWithTorrent(t, manager, "model.gguf", []byte("offline peer"))
	profile := WorkloadProfile{ID: "profile-c", Artifacts: []string{artifact.ID}}

	if err := manager.store.Update(func(db *Database) error {
		db.Nodes["node-offline"] = Node{
			ID:              "node-offline",
			State:           NodeStateOffline,
			CachedArtifacts: []string{artifact.ID},
			P2P:             &WorkerP2PStatus{Available: true, Port: 6881},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	specs := manager.artifactSpecs(profile, "http://manager.test")
	if len(specs) != 1 {
		t.Fatalf("artifact specs len = %d, want 1", len(specs))
	}
	if len(specs[0].P2PPeers) != 0 {
		t.Fatalf("expected no peers for offline node, got %v", specs[0].P2PPeers)
	}
}

func TestDispatchUpdateIncludesArtifactTorrentMetadata(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.json"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
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
	artifact := mustManagerArtifactWithTorrent(t, manager, "worker.tar.gz", []byte("worker update"))
	session := &workerSession{
		id:      "ses-a",
		nodeID:  "node-a",
		baseURL: "http://manager.test",
		manager: manager,
		send:    make(chan Envelope, 1),
		done:    make(chan struct{}),
	}
	manager.mu.Lock()
	manager.sessions["node-a"] = session
	manager.mu.Unlock()
	update := UpdateRecord{
		ID:         "upd-a",
		Kind:       "worker",
		Version:    "v1.2.3",
		ArtifactID: artifact.ID,
		SHA256:     artifact.SHA256,
		Status:     "pending",
		CreatedAt:  time.Now().UTC(),
	}
	if err := manager.store.Update(func(db *Database) error {
		db.Nodes["node-a"] = Node{ID: "node-a", State: NodeStateOnline, Approved: true}
		db.Updates[update.ID] = update
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	manager.dispatchUpdate(update)

	select {
	case msg := <-session.send:
		var payload UpdatePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Artifact.Torrent == nil || payload.Artifact.Torrent.InfoHash != artifact.Torrent.InfoHash {
			t.Fatalf("update payload missing torrent metadata: %+v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("update payload was not enqueued")
	}
}

func mustManagerArtifactWithTorrent(t *testing.T, manager *Manager, name string, content []byte) Artifact {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := osWriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sha, size, err := FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := ensureArtifactTorrentMetadata(Artifact{
		ID:        "sha256:" + strings.TrimPrefix(sha, "sha256:"),
		Kind:      inferArtifactKind(name),
		Name:      name,
		Path:      path,
		SHA256:    sha,
		SizeBytes: size,
		CreatedAt: time.Now().UTC(),
	}, manager.cfg.ArtifactDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.store.Update(func(db *Database) error {
		db.Artifacts[artifact.ID] = artifact
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return artifact
}

func uploadNamedTestArtifact(t *testing.T, baseURL, name string, content []byte) Artifact {
	t.Helper()
	body, contentType := multipartArtifactBody(t, name, inferArtifactKind(name), content)
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

func adminGET(t *testing.T, baseURL, token, path string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func osWriteFile(path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}

func TestMigrateArtifactTorrentMetadata(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.json"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
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
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "tiny.gguf")
	if err := osWriteFile(modelPath, []byte("migrate me"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, size, err := FileSHA256(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	artifact := Artifact{
		ID:        "sha256:" + strings.TrimPrefix(sha, "sha256:"),
		Kind:      "model_gguf",
		Name:      "tiny.gguf",
		Path:      modelPath,
		SHA256:    sha,
		SizeBytes: size,
		CreatedAt: time.Now().UTC(),
	}
	if err := manager.store.Update(func(db *Database) error {
		db.Artifacts[artifact.ID] = artifact
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.migrateArtifactTorrentMetadata(); err != nil {
		t.Fatal(err)
	}
	db := manager.store.Snapshot()
	updated, ok := db.Artifacts[artifact.ID]
	if !ok {
		t.Fatal("artifact missing after migration")
	}
	if updated.Torrent == nil || updated.Torrent.InfoHash == "" || len(updated.Torrent.MetaInfoBytes) == 0 {
		t.Fatalf("artifact missing torrent metadata after migration: %+v", updated.Torrent)
	}
}

func TestArtifactSpecsOmitTorrentWhenP2PDisabled(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		DBPath:       filepath.Join(t.TempDir(), "comrad.json"),
		ArtifactDir:  filepath.Join(t.TempDir(), "artifacts"),
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
	artifact := mustManagerArtifactWithTorrent(t, manager, "model.gguf", []byte("p2p disabled test"))
	profile := WorkloadProfile{ID: "profile-p2p-off", Artifacts: []string{artifact.ID}}

	if err := manager.store.Update(func(db *Database) error {
		db.Settings.P2PEnabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	specs := manager.artifactSpecs(profile, "http://manager.test")
	if len(specs) != 1 {
		t.Fatalf("artifact specs len = %d, want 1", len(specs))
	}
	if specs[0].Torrent != nil {
		t.Fatal("torrent metadata should be nil when P2P disabled")
	}
	if len(specs[0].P2PPeers) != 0 {
		t.Fatalf("p2p peers should be empty when P2P disabled, got %v", specs[0].P2PPeers)
	}
	if specs[0].URL == "" {
		t.Fatal("HTTP URL should still be populated")
	}
}

func TestMigrateSkippedWhenP2PDisabled(t *testing.T) {
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

	modelPath := filepath.Join(dir, "tiny.gguf")
	if err := osWriteFile(modelPath, []byte("no torrent"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, size, err := FileSHA256(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	artifact := Artifact{
		ID:        "sha256:" + strings.TrimPrefix(sha, "sha256:"),
		Kind:      "model_gguf",
		Name:      "tiny.gguf",
		Path:      modelPath,
		SHA256:    sha,
		SizeBytes: size,
		CreatedAt: time.Now().UTC(),
	}
	if err := manager.store.Update(func(db *Database) error {
		db.Artifacts[artifact.ID] = artifact
		db.Settings.P2PEnabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := manager.migrateArtifactTorrentMetadata(); err != nil {
		t.Fatal(err)
	}

	db := manager.store.Snapshot()
	updated := db.Artifacts[artifact.ID]
	if updated.Torrent != nil {
		t.Fatal("torrent metadata should not be generated when P2P disabled")
	}
}
