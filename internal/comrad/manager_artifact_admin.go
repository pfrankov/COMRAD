package comrad

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) handleAdminArtifactUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	artifact, err := m.receiveUploadedArtifact(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload_failed", err.Error())
		return
	}
	if err := m.storeUploadedArtifact(artifact); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, artifact)
}

func (m *Manager) handleAdminArtifactByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/admin/artifacts/"))
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "artifact id required")
		return
	}
	artifact, err := m.deleteArtifactRecord(id)
	if err != nil {
		writeArtifactDeleteError(w, err)
		return
	}
	if err := m.removeManagedArtifactFile(artifact); err != nil {
		writeError(w, http.StatusInternalServerError, "artifact_file_delete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (m *Manager) receiveUploadedArtifact(r *http.Request) (Artifact, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return Artifact{}, err
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return Artifact{}, fmt.Errorf("file is required")
	}
	defer file.Close()
	name := uploadArtifactName(r.FormValue("name"), header.Filename)
	tmp, err := copyUploadToArtifactDir(file, m.cfg.ArtifactDir)
	if err != nil {
		return Artifact{}, err
	}
	return finalizeUploadedArtifact(tmp, name, r.FormValue("kind"), r.FormValue("sha256"), m.cfg.ArtifactDir)
}

func (m *Manager) storeUploadedArtifact(artifact Artifact) error {
	return m.store.Update(func(db *Database) error {
		db.Artifacts[artifact.ID] = artifact
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "artifact.uploaded", Actor: "admin", Subject: artifact.ID, CreatedAt: time.Now().UTC()})
		return nil
	})
}

func (m *Manager) deleteArtifactRecord(id string) (Artifact, error) {
	var artifact Artifact
	err := m.store.Update(func(db *Database) error {
		found, ok := db.Artifacts[id]
		if !ok {
			return artifactDeleteError{code: "not_found", message: "artifact not found"}
		}
		if reason := artifactDeleteBlocker(db, id); reason != "" {
			return artifactDeleteError{code: "artifact_in_use", message: reason}
		}
		artifact = found
		delete(db.Artifacts, id)
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "artifact.deleted", Actor: "admin", Subject: id, CreatedAt: time.Now().UTC()})
		return nil
	})
	return artifact, err
}

type artifactDeleteError struct {
	code    string
	message string
}

func (e artifactDeleteError) Error() string {
	return e.message
}

func writeArtifactDeleteError(w http.ResponseWriter, err error) {
	if e, ok := err.(artifactDeleteError); ok {
		if e.code == "artifact_in_use" {
			writeError(w, http.StatusConflict, e.code, e.message)
			return
		}
		writeError(w, http.StatusNotFound, e.code, e.message)
		return
	}
	writeError(w, http.StatusInternalServerError, "store_error", err.Error())
}

func artifactDeleteBlocker(db *Database, id string) string {
	for _, profile := range db.Profiles {
		if Contains(profileArtifactIDs(profile), id) {
			return "artifact is used by profile " + profile.ID
		}
	}
	for _, update := range db.Updates {
		if update.ArtifactID == id {
			return "artifact is used by update " + update.ID
		}
	}
	return ""
}

func (m *Manager) removeManagedArtifactFile(artifact Artifact) error {
	if err := removeManagedMetainfoFile(artifact, m.cfg.ArtifactDir); err != nil {
		return err
	}
	if !pathInsideDir(artifact.Path, m.cfg.ArtifactDir) {
		return nil
	}
	err := os.Remove(artifact.Path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func removeManagedMetainfoFile(artifact Artifact, artifactDir string) error {
	if artifact.Torrent == nil || artifact.Torrent.MetaInfoPath == "" {
		return nil
	}
	if !pathInsideDir(artifact.Torrent.MetaInfoPath, artifactDir) {
		return nil
	}
	err := os.Remove(artifact.Torrent.MetaInfoPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func pathInsideDir(path, dir string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}

func copyUploadToArtifactDir(file io.Reader, dir string) (string, error) {
	if err := EnsureDir(dir); err != nil {
		return "", err
	}
	tmp := filepath.Join(dir, NewID("upload")+".tmp")
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", closeErr
	}
	return tmp, nil
}

func finalizeUploadedArtifact(tmp, name, kind, expected, artifactDir string) (Artifact, error) {
	sha, size, err := FileSHA256(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return Artifact{}, err
	}
	if expected != "" && NormalizeSHA256(expected) != sha {
		_ = os.Remove(tmp)
		return Artifact{}, fmt.Errorf("expected %s got %s", NormalizeSHA256(expected), sha)
	}
	final := filepath.Join(filepath.Dir(tmp), artifactStorageName(sha, name))
	if err := moveUploadIntoPlace(tmp, final); err != nil {
		return Artifact{}, err
	}
	if kind == "" {
		kind = inferArtifactKind(name)
	}
	return ensureArtifactTorrentMetadata(Artifact{
		ID:        "sha256:" + strings.TrimPrefix(sha, "sha256:"),
		Kind:      kind,
		Name:      name,
		Path:      final,
		SHA256:    sha,
		SizeBytes: size,
		CreatedAt: time.Now().UTC(),
	}, artifactDir)
}

func moveUploadIntoPlace(tmp, final string) error {
	if _, err := os.Stat(final); err == nil {
		_ = os.Remove(tmp)
		return nil
	}
	return os.Rename(tmp, final)
}

func uploadArtifactName(explicit, filename string) string {
	if explicit != "" {
		return safeUploadFileName(explicit)
	}
	return safeUploadFileName(filename)
}

func artifactStorageName(sha, name string) string {
	return strings.TrimPrefix(sha, "sha256:") + "-" + safeUploadFileName(name)
}

func safeUploadFileName(name string) string {
	base := filepath.Base(name)
	if base == "." || base == "/" || base == "" {
		base = "artifact"
	}
	clean := strings.Map(safeUploadRune, base)
	clean = strings.Trim(clean, "._-")
	if clean == "" {
		return "artifact"
	}
	return clean
}

func safeUploadRune(r rune) rune {
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
		return r
	}
	if r == '.' || r == '_' || r == '-' {
		return r
	}
	return '_'
}

func inferArtifactKind(name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "mmproj") {
		return "model_mmproj"
	}
	if strings.HasSuffix(lower, ".gguf") {
		return "model_gguf"
	}
	return "model_support"
}
