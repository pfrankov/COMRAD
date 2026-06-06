package comrad

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) handleAdminArtifacts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, adminArtifacts(m.store.Snapshot()))
	case http.MethodPost:
		m.handleAdminArtifactCreate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *Manager) handleAdminArtifactCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateArtifactRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	artifact, err := m.buildAdminArtifact(req)
	if err != nil {
		writeAdminArtifactCreateError(w, err)
		return
	}
	if err := m.store.Update(func(db *Database) error {
		db.Artifacts[artifact.ID] = artifact
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "artifact.registered", Actor: "admin", Subject: artifact.ID, CreatedAt: time.Now().UTC()})
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, artifact)
}

func (m *Manager) buildAdminArtifact(req CreateArtifactRequest) (Artifact, error) {
	if req.Path == "" {
		return Artifact{}, artifactCreateError{status: http.StatusBadRequest, code: "bad_request", message: "path is required"}
	}
	sha, size, err := FileSHA256(req.Path)
	if err != nil {
		return Artifact{}, artifactCreateError{status: http.StatusBadRequest, code: "artifact_unreadable", message: err.Error()}
	}
	if req.SHA256 != "" && NormalizeSHA256(req.SHA256) != sha {
		return Artifact{}, artifactCreateError{
			status:  http.StatusBadRequest,
			code:    FailureArtifactDigestMismatch,
			message: fmt.Sprintf("expected %s got %s", NormalizeSHA256(req.SHA256), sha),
		}
	}
	name := req.Name
	if name == "" {
		name = filepath.Base(req.Path)
	}
	kind := req.Kind
	if kind == "" {
		kind = "model_gguf"
	}
	abs, _ := filepath.Abs(req.Path)
	artifact, err := ensureArtifactTorrentMetadata(Artifact{
		ID:        "sha256:" + strings.TrimPrefix(sha, "sha256:"),
		Kind:      kind,
		Name:      name,
		Path:      abs,
		SHA256:    sha,
		SizeBytes: size,
		CreatedAt: time.Now().UTC(),
	}, m.cfg.ArtifactDir)
	if err != nil {
		return Artifact{}, artifactCreateError{status: http.StatusInternalServerError, code: "torrent_metadata_failed", message: err.Error()}
	}
	return artifact, nil
}

type artifactCreateError struct {
	status  int
	code    string
	message string
}

func (e artifactCreateError) Error() string {
	return e.message
}

func writeAdminArtifactCreateError(w http.ResponseWriter, err error) {
	if e, ok := err.(artifactCreateError); ok {
		writeError(w, e.status, e.code, e.message)
		return
	}
	writeError(w, http.StatusInternalServerError, "store_error", err.Error())
}
