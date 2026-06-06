package comrad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func (w *Worker) ensureArtifact(ctx context.Context, artifact ArtifactSpec) error {
	_, err := w.ensureArtifactDetailed(ctx, artifact, nil)
	return err
}

func (w *Worker) ensureArtifactWithStart(ctx context.Context, artifact ArtifactSpec, onStart func()) error {
	_, err := w.ensureArtifactDetailed(ctx, artifact, onStart)
	return err
}

func (w *Worker) ensureArtifactDetailed(ctx context.Context, artifact ArtifactSpec, onStart func()) (artifactDownloadResult, error) {
	if artifact.ID == "" || artifact.URL == "" {
		return artifactDownloadResult{}, errors.New("artifact spec missing id or url")
	}
	if w.artifactAlreadyCached(artifact) {
		return artifactDownloadResult{}, nil
	}
	w.sendArtifactState(artifact, "download_queued", "")
	release, err := w.acquireDownloadSlot(ctx)
	if err != nil {
		w.sendArtifactState(artifact, "download_failed", err.Error())
		return artifactDownloadResult{}, err
	}
	defer release()
	if w.artifactAlreadyCached(artifact) {
		return artifactDownloadResult{}, nil
	}
	w.sendArtifactState(artifact, "downloading", "")
	if onStart != nil {
		onStart()
	}
	return w.downloadArtifact(ctx, artifact)
}

func (w *Worker) downloadArtifact(ctx context.Context, artifact ArtifactSpec) (artifactDownloadResult, error) {
	target := filepath.Join(w.cfg.CacheDir, safeArtifactFileName(artifact.ID))
	if result, err := w.downloadArtifactWithTorrent(ctx, artifact, target); err == nil {
		if result.Method != "" {
			return result, nil
		}
	} else if result.Method != "" {
		if httpResult, httpErr := w.downloadArtifactWithHTTP(ctx, artifact, target); httpErr == nil {
			return httpResult, nil
		} else {
			return artifactDownloadResult{}, httpErr
		}
	}
	result, err := w.downloadArtifactWithHTTP(ctx, artifact, target)
	if err != nil {
		return artifactDownloadResult{}, err
	}
	return result, nil
}

func (w *Worker) downloadArtifactWithTorrent(ctx context.Context, artifact ArtifactSpec, target string) (artifactDownloadResult, error) {
	if w.p2p == nil || artifact.Torrent == nil {
		return artifactDownloadResult{}, nil
	}
	torrentCtx, cancel := context.WithTimeout(ctx, w.cfg.P2PDownloadTimeout)
	defer cancel()
	if err := w.p2p.Download(torrentCtx, artifact, target); err != nil {
		reason := p2pFailureDownload
		if downloadErr, ok := err.(*p2pDownloadError); ok {
			reason = downloadErr.reason
		}
		w.p2p.RecordFallback(reason, err)
		w.refreshP2PState()
		return artifactDownloadResult{Method: artifactDeliveryHTTPFallback}, err
	}
	if err := VerifyFileSHA256(target, artifact.SHA256); err != nil {
		if w.p2p != nil {
			w.p2p.StopSeeding(artifact.ID)
		}
		_ = os.Remove(target)
		w.p2p.RecordFallback(FailureArtifactDigestMismatch, err)
		w.refreshP2PState()
		return artifactDownloadResult{Method: artifactDeliveryHTTPFallback}, err
	}
	w.mu.Lock()
	w.cache[artifact.ID] = target
	w.cacheState[artifact.ID] = cachedArtifactState{Path: target, Torrent: cloneArtifactTorrent(artifact.Torrent)}
	w.mu.Unlock()
	w.sendArtifactState(artifact, "verified", "")
	w.refreshP2PState()
	if err := w.saveState(); err != nil {
		return artifactDownloadResult{}, err
	}
	return artifactDownloadResult{Method: artifactDeliveryTorrent}, nil
}

func (w *Worker) downloadArtifactWithHTTP(ctx context.Context, artifact ArtifactSpec, target string) (artifactDownloadResult, error) {
	resp, err := w.openArtifactResponse(ctx, artifact)
	if err != nil {
		return artifactDownloadResult{}, err
	}
	defer resp.Body.Close()
	if err := validArtifactResponse(resp); err != nil {
		w.sendArtifactState(artifact, "download_failed", err.Error())
		return artifactDownloadResult{}, err
	}
	if err := w.writeArtifactFile(resp.Body, target, artifact); err != nil {
		return artifactDownloadResult{}, err
	}
	w.mu.Lock()
	w.cache[artifact.ID] = target
	w.cacheState[artifact.ID] = cachedArtifactState{Path: target, Torrent: cloneArtifactTorrent(artifact.Torrent)}
	w.mu.Unlock()
	if err := w.seedCachedArtifact(artifact, target); err != nil {
		return artifactDownloadResult{}, err
	}
	w.sendArtifactState(artifact, "verified", "")
	w.refreshP2PState()
	if err := w.saveState(); err != nil {
		return artifactDownloadResult{}, err
	}
	if artifact.Torrent != nil {
		return artifactDownloadResult{Method: artifactDeliveryHTTPFallback}, nil
	}
	return artifactDownloadResult{Method: artifactDeliveryHTTPOnly}, nil
}

func (w *Worker) artifactAlreadyCached(artifact ArtifactSpec) bool {
	w.mu.Lock()
	existing := w.cache[artifact.ID]
	w.mu.Unlock()
	if existing == "" {
		return false
	}
	if err := VerifyFileSHA256(existing, artifact.SHA256); err == nil {
		w.mu.Lock()
		w.cacheState[artifact.ID] = cachedArtifactState{Path: existing, Torrent: cloneArtifactTorrent(artifact.Torrent)}
		w.mu.Unlock()
		_ = w.seedCachedArtifact(artifact, existing)
		w.sendArtifactState(artifact, "verified", "")
		return true
	}
	if w.p2p != nil {
		w.p2p.StopSeeding(artifact.ID)
		w.refreshP2PState()
	}
	_ = os.Remove(existing)
	w.mu.Lock()
	delete(w.cache, artifact.ID)
	delete(w.cacheState, artifact.ID)
	w.mu.Unlock()
	return false
}

func (w *Worker) openArtifactResponse(ctx context.Context, artifact ArtifactSpec) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	nodeID := w.node.ID
	nodeToken := w.nodeToken
	w.mu.Unlock()
	req.Header.Set("Authorization", "Bearer "+w.cfg.Token)
	req.Header.Set("X-COMRAD-Node-ID", nodeID)
	req.Header.Set("X-COMRAD-Node-Token", nodeToken)
	resp, err := w.client.Do(req)
	if err != nil {
		w.sendArtifactState(artifact, "download_failed", err.Error())
		return nil, err
	}
	return resp, nil
}

func validArtifactResponse(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("artifact download failed: %s %s", resp.Status, string(body))
	}
	return nil
}

func (w *Worker) writeArtifactFile(body io.Reader, target string, artifact ArtifactSpec) error {
	tmp := target + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		w.sendArtifactState(artifact, "download_failed", copyErr.Error())
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := VerifyFileSHA256(tmp, artifact.SHA256); err != nil {
		_ = os.Remove(tmp)
		w.sendArtifactState(artifact, "digest_mismatch", err.Error())
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	return nil
}

func (w *Worker) verifyProfileArtifacts(profile WorkloadProfile) error {
	w.mu.Lock()
	assignment := w.assigns[assignmentKey(profile)]
	w.mu.Unlock()
	if len(assignment.Artifacts) == 0 {
		return errors.New("profile has no assigned artifacts")
	}
	for _, artifact := range assignment.Artifacts {
		w.mu.Lock()
		path := w.cache[artifact.ID]
		w.mu.Unlock()
		if path == "" {
			return fmt.Errorf("artifact %s not cached", artifact.ID)
		}
		if err := VerifyFileSHA256(path, artifact.SHA256); err != nil {
			return err
		}
	}
	return nil
}
