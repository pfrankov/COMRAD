package comrad

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) handleWorkerArtifact(w http.ResponseWriter, r *http.Request) {
	if !m.workerAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "worker token required")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/worker/artifacts/")
	id = strings.TrimSpace(id)
	if id == "" {
		writeError(w, http.StatusNotFound, "not_found", "artifact id required")
		return
	}
	db := m.store.Snapshot()
	artifact, ok := db.Artifacts[id]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "artifact not found")
		return
	}
	nodeID := r.Header.Get("X-COMRAD-Node-ID")
	if nodeID == "" {
		writeError(w, http.StatusForbidden, "forbidden", "worker node id is required")
		return
	}
	if !workerNodeTokenAuthorized(db, nodeID, r.Header.Get("X-COMRAD-Node-Token")) {
		writeError(w, http.StatusForbidden, "forbidden", "worker node token is required")
		return
	}
	if !artifactAllowedForNode(db, nodeID, id) {
		writeError(w, http.StatusForbidden, "forbidden", "artifact is not assigned to this worker")
		return
	}
	f, err := os.Open(artifact.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "artifact_unreadable", err.Error())
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "artifact_unreadable", err.Error())
		return
	}
	w.Header().Set("X-COMRAD-SHA256", artifact.SHA256)
	w.Header().Set("Content-Disposition", "attachment; filename="+strconvQuote(filepath.Base(artifact.Name)))
	_ = m.store.Audit("artifact.accessed", "worker", artifact.ID, map[string]any{"nodeId": nodeID})
	http.ServeContent(w, r, filepath.Base(artifact.Path), info.ModTime(), f)
}

func artifactAllowedForNode(db Database, nodeID, artifactID string) bool {
	for _, a := range db.Assignments {
		if a.NodeID != nodeID || !a.DesiredCached {
			continue
		}
		profile := effectiveProfileForAssignment(db.Profiles[a.ProfileID], a)
		if Contains(profile.Artifacts, artifactID) {
			return true
		}
	}
	for _, update := range db.Updates {
		if update.ArtifactID != artifactID {
			continue
		}
		if len(update.TargetNodes) == 0 || Contains(update.TargetNodes, nodeID) {
			return true
		}
	}
	return false
}

func strconvQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "\"", "_")
	return "\"" + s + "\""
}
