package comrad

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

const (
	defaultWorkerP2PPort            = 6881
	defaultWorkerP2PMaxUploads      = 8
	defaultWorkerP2PDownloadTimeout = 2 * time.Minute
	defaultWorkerP2PNoPeerWait      = 2 * time.Second

	artifactDeliveryTorrent      = "torrent"
	artifactDeliveryHTTPFallback = "http_fallback"
	artifactDeliveryHTTPOnly     = "http_only"

	p2pFailureUnavailable = "runtime_unavailable"
	p2pFailureNoPeers     = "no_peers"
	p2pFailureTimeout     = "timeout"
	p2pFailureDownload    = "download_failed"
)

type artifactDownloadResult struct {
	Method string
}

type p2pDownloadError struct {
	reason string
	err    error
}

func (e *p2pDownloadError) Error() string {
	if e.err == nil {
		return e.reason
	}
	return fmt.Sprintf("%s: %v", e.reason, e.err)
}

func (e *p2pDownloadError) Unwrap() error {
	return e.err
}

type workerP2PRuntime interface {
	Available() bool
	Snapshot() *WorkerP2PStatus
	Seed(artifact ArtifactSpec, path string) error
	StopSeeding(artifactID string)
	Download(ctx context.Context, artifact ArtifactSpec, target string) error
	AddPeers(artifactID string, addrs []string)
	RecordFallback(reason string, err error)
	Close()
}

type anacrolixWorkerP2P struct {
	client      *torrent.Client
	status      WorkerP2PStatus
	torrents    map[string]*torrent.Torrent
	downloading map[string]bool
	mu          sync.Mutex
}

func newAnacrolixWorkerP2P(cfg WorkerConfig) (workerP2PRuntime, *WorkerP2PStatus, error) {
	status := WorkerP2PStatus{
		Port:                   cfg.P2PPort,
		MaxUploads:             cfg.P2PMaxUploads,
		DownloadTimeoutSeconds: int64(cfg.P2PDownloadTimeout.Seconds()),
	}
	clientCfg := torrent.NewDefaultClientConfig()
	clientCfg.DataDir = cfg.CacheDir
	clientCfg.ListenPort = cfg.P2PPort
	clientCfg.Seed = true
	clientCfg.DisableTrackers = true
	clientCfg.DisableWebtorrent = true
	clientCfg.DisableWebseeds = true
	clientCfg.EstablishedConnsPerTorrent = maxInt(1, cfg.P2PMaxUploads)
	clientCfg.TorrentPeersHighWater = maxInt(8, cfg.P2PMaxUploads)
	clientCfg.TorrentPeersLowWater = maxInt(4, cfg.P2PMaxUploads/2)
	if cfg.p2pClientConfigHook != nil {
		cfg.p2pClientConfigHook(clientCfg)
	}
	client, err := torrent.NewClient(clientCfg)
	if err != nil {
		now := time.Now().UTC()
		status.LastFailure = err.Error()
		status.LastFailureAt = &now
		return &anacrolixWorkerP2P{status: status, torrents: map[string]*torrent.Torrent{}, downloading: map[string]bool{}}, &status, nil
	}
	status.Available = true
	return &anacrolixWorkerP2P{
		client:      client,
		status:      status,
		torrents:    map[string]*torrent.Torrent{},
		downloading: map[string]bool{},
	}, &status, nil
}

func (p *anacrolixWorkerP2P) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client != nil && p.status.Available
}

func (p *anacrolixWorkerP2P) Snapshot() *WorkerP2PStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.status
	out.SeedingCount = 0
	out.DownloadingCount = len(p.downloading)
	out.PeerCount = 0
	for _, tor := range p.torrents {
		if tor == nil {
			continue
		}
		if tor.Complete().Bool() {
			out.SeedingCount++
		}
		stats := tor.Stats()
		out.PeerCount += stats.ActivePeers + stats.PendingPeers
	}
	return cloneWorkerP2PStatus(&out)
}

func (p *anacrolixWorkerP2P) Seed(artifact ArtifactSpec, path string) error {
	if !p.Available() || artifact.Torrent == nil {
		return nil
	}
	_, err := p.ensureTorrent(artifact)
	if err != nil {
		p.recordFailure(p2pFailureDownload, err)
	}
	return err
}

func (p *anacrolixWorkerP2P) StopSeeding(artifactID string) {
	p.dropTorrent(artifactID)
}

func (p *anacrolixWorkerP2P) Download(ctx context.Context, artifact ArtifactSpec, target string) error {
	if !p.Available() {
		return &p2pDownloadError{reason: p2pFailureUnavailable, err: errors.New("torrent runtime unavailable")}
	}
	tor, err := p.ensureDownloadTorrent(artifact)
	if err != nil {
		return err
	}
	if err := waitTorrentInfo(ctx, tor); err != nil {
		p.dropTorrent(artifact.ID)
		return &p2pDownloadError{reason: p2pFailureTimeout, err: err}
	}
	p.addPeersToTorrent(tor, artifact.P2PPeers)
	p.setDownloading(artifact.ID, true)
	defer p.setDownloading(artifact.ID, false)
	tor.DownloadAll()
	return p.waitForTorrentCompletion(ctx, artifact.ID, tor)
}

func (p *anacrolixWorkerP2P) AddPeers(artifactID string, addrs []string) {
	if !p.Available() || len(addrs) == 0 {
		return
	}
	p.mu.Lock()
	tor := p.torrents[artifactID]
	p.mu.Unlock()
	if tor == nil {
		return
	}
	p.addPeersToTorrent(tor, addrs)
}

func (p *anacrolixWorkerP2P) addPeersToTorrent(tor *torrent.Torrent, addrs []string) {
	var peers []torrent.PeerInfo
	for _, addr := range addrs {
		peers = append(peers, torrent.PeerInfo{
			Addr:   peerAddrAddr(addr),
			Source: torrent.PeerSourceDirect,
		})
	}
	if len(peers) > 0 {
		tor.AddPeers(peers)
	}
}

func (p *anacrolixWorkerP2P) ensureDownloadTorrent(artifact ArtifactSpec) (*torrent.Torrent, error) {
	tor, err := p.ensureTorrent(artifact)
	if err != nil {
		p.recordFailure(p2pFailureDownload, err)
		return nil, &p2pDownloadError{reason: p2pFailureDownload, err: err}
	}
	return tor, nil
}

func (p *anacrolixWorkerP2P) waitForTorrentCompletion(ctx context.Context, artifactID string, tor *torrent.Torrent) error {
	noPeerDeadline := time.Now().Add(defaultWorkerP2PNoPeerWait)
	sawPeers := false
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-tor.Complete().On():
			return nil
		case <-ticker.C:
			stats := tor.Stats()
			if stats.ActivePeers > 0 || stats.PendingPeers > 0 {
				sawPeers = true
				continue
			}
			if !sawPeers && time.Now().After(noPeerDeadline) {
				err := errors.New("torrent has no peers")
				p.recordFailure(p2pFailureNoPeers, err)
				p.dropTorrent(artifactID)
				return &p2pDownloadError{reason: p2pFailureNoPeers, err: err}
			}
		case <-ctx.Done():
			reason := p2pFailureTimeout
			stats := tor.Stats()
			if !sawPeers && stats.ActivePeers == 0 && stats.PendingPeers == 0 {
				reason = p2pFailureNoPeers
			}
			p.recordFailure(reason, ctx.Err())
			p.dropTorrent(artifactID)
			return &p2pDownloadError{reason: reason, err: ctx.Err()}
		}
	}
}

func (p *anacrolixWorkerP2P) RecordFallback(reason string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.FallbackCount++
	recordP2PFailure(&p.status, reason, err)
}

func (p *anacrolixWorkerP2P) Close() {
	p.mu.Lock()
	client := p.client
	p.client = nil
	p.torrents = map[string]*torrent.Torrent{}
	p.downloading = map[string]bool{}
	p.mu.Unlock()
	if client != nil {
		client.Close()
	}
}

func (p *anacrolixWorkerP2P) ensureTorrent(artifact ArtifactSpec) (*torrent.Torrent, error) {
	p.mu.Lock()
	if tor := p.torrents[artifact.ID]; tor != nil {
		p.mu.Unlock()
		return tor, nil
	}
	client := p.client
	p.mu.Unlock()
	if client == nil {
		return nil, errors.New("torrent client is not available")
	}
	mi, err := loadTorrentMetaInfo(artifact)
	if err != nil {
		return nil, err
	}
	tor, _, err := client.AddTorrentSpec(torrent.TorrentSpecFromMetaInfo(mi))
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.torrents[artifact.ID] = tor
	p.mu.Unlock()
	return tor, nil
}

func (p *anacrolixWorkerP2P) setDownloading(artifactID string, active bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if active {
		p.downloading[artifactID] = true
		return
	}
	delete(p.downloading, artifactID)
}

func (p *anacrolixWorkerP2P) dropTorrent(artifactID string) {
	p.mu.Lock()
	tor := p.torrents[artifactID]
	delete(p.torrents, artifactID)
	delete(p.downloading, artifactID)
	p.mu.Unlock()
	if tor != nil {
		tor.Drop()
	}
}

func (p *anacrolixWorkerP2P) recordFailure(reason string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	recordP2PFailure(&p.status, reason, err)
}

func recordP2PFailure(status *WorkerP2PStatus, reason string, err error) {
	now := time.Now().UTC()
	status.LastFailureAt = &now
	switch {
	case err == nil:
		status.LastFailure = reason
	case reason == "":
		status.LastFailure = err.Error()
	default:
		status.LastFailure = reason + ": " + err.Error()
	}
}

func loadTorrentMetaInfo(artifact ArtifactSpec) (*metainfo.MetaInfo, error) {
	if artifact.Torrent == nil || len(artifact.Torrent.MetaInfoBytes) == 0 {
		return nil, errors.New("artifact torrent metadata is missing")
	}
	return metainfo.Load(bytes.NewReader(artifact.Torrent.MetaInfoBytes))
}

func waitTorrentInfo(ctx context.Context, tor *torrent.Torrent) error {
	select {
	case <-tor.GotInfo():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type peerAddrAddr string

func (a peerAddrAddr) String() string { return string(a) }
func (a peerAddrAddr) Network() string { return "tcp" }

func (w *Worker) initP2P() error {
	factory := w.cfg.p2pFactory
	if factory == nil {
		factory = newAnacrolixWorkerP2P
	}
	runtime, status, err := factory(w.cfg)
	if err != nil {
		return err
	}
	w.p2p = runtime
	w.node.P2P = cloneWorkerP2PStatus(status)
	for artifactID, entry := range w.cacheState {
		if entry.Path == "" {
			continue
		}
		w.cache[artifactID] = entry.Path
		if entry.Torrent != nil {
			_ = w.seedCachedArtifact(ArtifactSpec{
				ID:      artifactID,
				SHA256:  artifactID,
				Torrent: cloneArtifactTorrent(entry.Torrent),
			}, entry.Path)
		}
	}
	w.refreshP2PState()
	return nil
}

func (w *Worker) closeP2P() {
	if w.p2p != nil {
		w.p2p.Close()
	}
}

func (w *Worker) refreshP2PState() {
	if w.p2p == nil {
		return
	}
	w.mu.Lock()
	w.node.P2P = w.p2p.Snapshot()
	w.mu.Unlock()
}

func (w *Worker) seedCachedArtifact(artifact ArtifactSpec, path string) error {
	if w.p2p == nil || !w.p2p.Available() || artifact.Torrent == nil || path == "" {
		return nil
	}
	if err := w.p2p.Seed(artifact, path); err != nil {
		w.refreshP2PState()
		return err
	}
	w.refreshP2PState()
	return nil
}
