package comrad

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

func TestWorkerFallsBackToHTTPWhenTorrentHasNoPeers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("http fallback artifact"))
	}))
	defer server.Close()

	worker := newP2PWorkerForTest(t, WorkerConfig{
		NodeID:                 "node-a",
		StatePath:              filepath.Join(t.TempDir(), "state.json"),
		CacheDir:               filepath.Join(t.TempDir(), "cache"),
		SlotCount:              1,
		P2PPort:                39011,
		P2PDownloadTimeout:     200 * time.Millisecond,
		MaxConcurrentDownloads: 1,
	})
	artifact := makeTorrentArtifactSpecForTest(t, worker.cfg.CacheDir, "model.gguf", []byte("http fallback artifact"), server.URL+"/artifact")

	result, err := worker.ensureArtifactDetailed(context.Background(), artifact, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != artifactDeliveryHTTPFallback {
		t.Fatalf("delivery method = %q, want %q", result.Method, artifactDeliveryHTTPFallback)
	}
	if worker.node.P2P == nil || worker.node.P2P.FallbackCount != 1 {
		t.Fatalf("fallback count = %+v, want 1", worker.node.P2P)
	}
	if !worker.artifactAlreadyCached(artifact) {
		t.Fatal("artifact was not cached after HTTP fallback")
	}
}

func TestWorkerStartupP2PFailureLeavesHTTPDownloadOperational(t *testing.T) {
	tcpLn, err := net.Listen("tcp", "127.0.0.1:39012")
	if err != nil {
		t.Fatal(err)
	}
	defer tcpLn.Close()
	udpLn, err := net.ListenPacket("udp", "127.0.0.1:39012")
	if err != nil {
		t.Fatal(err)
	}
	defer udpLn.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("startup failure fallback"))
	}))
	defer server.Close()

	worker := newP2PWorkerForTest(t, WorkerConfig{
		NodeID:                 "node-a",
		StatePath:              filepath.Join(t.TempDir(), "state.json"),
		CacheDir:               filepath.Join(t.TempDir(), "cache"),
		SlotCount:              1,
		P2PPort:                39012,
		P2PDownloadTimeout:     200 * time.Millisecond,
		MaxConcurrentDownloads: 1,
	})
	if worker.node.P2P == nil || worker.node.P2P.Available {
		t.Fatalf("worker p2p state = %+v, want unavailable", worker.node.P2P)
	}
	if worker.node.P2P.LastFailure == "" {
		t.Fatalf("worker p2p state missing startup failure: %+v", worker.node.P2P)
	}
	artifact := makeTorrentArtifactSpecForTest(t, worker.cfg.CacheDir, "model.gguf", []byte("startup failure fallback"), server.URL+"/artifact")

	result, err := worker.ensureArtifactDetailed(context.Background(), artifact, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != artifactDeliveryHTTPFallback {
		t.Fatalf("delivery method = %q, want %q", result.Method, artifactDeliveryHTTPFallback)
	}
}

func TestWorkerDownloadsArtifactOverTorrentAndStopsSeedingOnEviction(t *testing.T) {
	seeder := newP2PWorkerForTest(t, WorkerConfig{
		NodeID:                 "node-seeder",
		StatePath:              filepath.Join(t.TempDir(), "state.json"),
		CacheDir:               filepath.Join(t.TempDir(), "cache"),
		SlotCount:              1,
		P2PPort:                39013,
		P2PDownloadTimeout:     2 * time.Second,
		MaxConcurrentDownloads: 1,
	})
	leecher := newP2PWorkerForTest(t, WorkerConfig{
		NodeID:                 "node-leecher",
		StatePath:              filepath.Join(t.TempDir(), "state.json"),
		CacheDir:               filepath.Join(t.TempDir(), "cache"),
		SlotCount:              1,
		P2PPort:                39014,
		P2PDownloadTimeout:     2 * time.Second,
		MaxConcurrentDownloads: 1,
	})
	artifact := makeTorrentArtifactSpecForTest(t, seeder.cfg.CacheDir, "model.gguf", []byte("torrent transfer"), "http://unused.invalid/artifact")
	target := filepath.Join(seeder.cfg.CacheDir, safeArtifactFileName(artifact.ID))
	if err := osWriteFile(target, []byte("torrent transfer"), 0o644); err != nil {
		t.Fatal(err)
	}
	seeder.cache[artifact.ID] = target
	seeder.cacheState[artifact.ID] = cachedArtifactState{Path: target, Torrent: artifact.Torrent}
	if err := seeder.seedCachedArtifact(artifact, target); err != nil {
		t.Fatal(err)
	}
	waitForTorrentComplete(t, seeder, artifact.ID)
	done := make(chan struct{})
	var (
		result artifactDownloadResult
		err    error
	)
	go func() {
		result, err = leecher.ensureArtifactDetailed(context.Background(), artifact, nil)
		close(done)
	}()
	waitForWorkerTorrent(t, leecher, artifact.ID)
	linkTorrentPeersForTest(t, seeder, leecher, artifact.ID)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != artifactDeliveryTorrent {
		t.Fatalf("delivery method = %q, want %q", result.Method, artifactDeliveryTorrent)
	}
	if leecher.node.P2P == nil || leecher.node.P2P.SeedingCount < 1 {
		t.Fatalf("leecher p2p state = %+v, want seeding count >= 1", leecher.node.P2P)
	}
	if err := leecher.evictArtifact(EvictArtifactPayload{ArtifactID: artifact.ID}); err != nil {
		t.Fatal(err)
	}
	if leecher.node.P2P == nil || leecher.node.P2P.SeedingCount != 0 {
		t.Fatalf("leecher p2p state after eviction = %+v, want seeding count 0", leecher.node.P2P)
	}
}

func newP2PWorkerForTest(t *testing.T, cfg WorkerConfig) *Worker {
	t.Helper()
	cfg.p2pClientConfigHook = func(clientCfg *torrent.ClientConfig) {
		clientCfg.NoDHT = true
	}
	worker, err := NewWorker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	worker.send = make(chan Envelope, 64)
	return worker
}

func makeTorrentArtifactSpecForTest(t *testing.T, dir, name string, content []byte, url string) ArtifactSpec {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := osWriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sha, size, err := FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := ensureArtifactTorrentMetadata(Artifact{
		ID:        "sha256:" + strings.TrimPrefix(sha, "sha256:"),
		Kind:      "model_gguf",
		Name:      name,
		Path:      path,
		SHA256:    sha,
		SizeBytes: size,
		CreatedAt: time.Now().UTC(),
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	return ArtifactSpec{
		ID:        artifact.ID,
		Kind:      artifact.Kind,
		Name:      artifact.Name,
		SHA256:    artifact.SHA256,
		SizeBytes: artifact.SizeBytes,
		URL:       url,
		Torrent:   artifact.Torrent,
	}
}

func linkTorrentPeersForTest(t *testing.T, seeder, leecher *Worker, artifactID string) {
	t.Helper()
	seederRuntime, ok := seeder.p2p.(*anacrolixWorkerP2P)
	if !ok {
		t.Fatalf("unexpected seeder runtime type %T", seeder.p2p)
	}
	leecherRuntime, ok := leecher.p2p.(*anacrolixWorkerP2P)
	if !ok {
		t.Fatalf("unexpected leecher runtime type %T", leecher.p2p)
	}
	leecherRuntime.mu.Lock()
	tor := leecherRuntime.torrents[artifactID]
	leecherRuntime.mu.Unlock()
	if tor == nil {
		t.Fatalf("leecher torrent %s not found", artifactID)
	}
	if added := tor.AddClientPeer(seederRuntime.client); added == 0 {
		t.Fatal("expected at least one peer to be added")
	}
}

func waitForTorrentComplete(t *testing.T, worker *Worker, artifactID string) {
	t.Helper()
	runtime, ok := worker.p2p.(*anacrolixWorkerP2P)
	if !ok {
		t.Fatalf("unexpected runtime type %T", worker.p2p)
	}
	deadline := time.After(5 * time.Second)
	for {
		runtime.mu.Lock()
		tor := runtime.torrents[artifactID]
		runtime.mu.Unlock()
		if tor != nil && tor.Complete().Bool() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("torrent did not complete")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func waitForWorkerTorrent(t *testing.T, worker *Worker, artifactID string) {
	t.Helper()
	runtime, ok := worker.p2p.(*anacrolixWorkerP2P)
	if !ok {
		t.Fatalf("unexpected runtime type %T", worker.p2p)
	}
	deadline := time.After(5 * time.Second)
	for {
		runtime.mu.Lock()
		tor := runtime.torrents[artifactID]
		runtime.mu.Unlock()
		if tor != nil {
			return
		}
		select {
		case <-deadline:
			t.Fatal("torrent did not appear on worker")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func drainWorkerSend(worker *Worker) []Envelope {
	var out []Envelope
	for {
		select {
		case msg := <-worker.send:
			out = append(out, msg)
		default:
			return out
		}
	}
}
