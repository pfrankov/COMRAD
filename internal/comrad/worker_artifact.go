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
	if artifact.ID == "" || artifact.URL == "" {
		return errors.New("artifact spec missing id or url")
	}
	if w.artifactAlreadyCached(artifact) {
		return nil
	}
	target := filepath.Join(w.cfg.CacheDir, safeArtifactFileName(artifact.ID))
	resp, err := w.openArtifactResponse(ctx, artifact)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := validArtifactResponse(resp); err != nil {
		w.sendArtifactState(artifact, "download_failed", err.Error())
		return err
	}
	if err := w.writeArtifactFile(resp.Body, target, artifact); err != nil {
		return err
	}
	w.mu.Lock()
	w.cache[artifact.ID] = target
	w.mu.Unlock()
	w.sendArtifactState(artifact, "verified", "")
	return nil
}

func (w *Worker) artifactAlreadyCached(artifact ArtifactSpec) bool {
	w.mu.Lock()
	existing := w.cache[artifact.ID]
	w.mu.Unlock()
	if existing == "" {
		return false
	}
	if err := VerifyFileSHA256(existing, artifact.SHA256); err == nil {
		w.sendArtifactState(artifact, "verified", "")
		return true
	}
	_ = os.Remove(existing)
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
