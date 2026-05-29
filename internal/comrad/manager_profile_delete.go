package comrad

import (
	"fmt"
	"net/http"
	"time"
)

func (m *Manager) handleAdminProfileDelete(w http.ResponseWriter, r *http.Request) {
	profileID := r.URL.Query().Get("profileId")
	if profileID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "profileId is required")
		return
	}
	if err := m.deleteProfile(profileID); err != nil {
		writeProfileDeleteError(w, err)
		return
	}
	m.replanAndDispatch()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (m *Manager) deleteProfile(profileID string) error {
	return m.store.Update(func(db *Database) error {
		if _, ok := db.Profiles[profileID]; !ok {
			return artifactDeleteError{code: "not_found", message: "profile not found"}
		}
		if profileHasLiveWork(*db, profileID) {
			return artifactDeleteError{code: "profile_in_use", message: "profile has queued or running work"}
		}
		delete(db.Profiles, profileID)
		for id, policy := range db.Policies {
			if policy.ProfileID == profileID {
				delete(db.Policies, id)
			}
		}
		for id, assignment := range db.Assignments {
			if assignment.ProfileID == profileID {
				delete(db.Assignments, id)
			}
		}
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "profile.deleted", Actor: "admin", Subject: profileID, CreatedAt: time.Now().UTC()})
		return nil
	})
}

func profileHasLiveWork(db Database, profileID string) bool {
	for _, task := range db.Tasks {
		if task.ProfileID == profileID && (task.Status == TaskStatusQueued || task.Status == TaskStatusRunning) {
			return true
		}
	}
	for _, attempt := range db.Attempts {
		if attempt.ProfileID == profileID && attempt.Status == TaskStatusRunning {
			return true
		}
	}
	return false
}

func writeProfileDeleteError(w http.ResponseWriter, err error) {
	if e, ok := err.(artifactDeleteError); ok {
		switch e.code {
		case "not_found":
			writeError(w, http.StatusNotFound, e.code, e.message)
		case "profile_in_use":
			writeError(w, http.StatusConflict, e.code, e.message)
		default:
			writeError(w, http.StatusBadRequest, e.code, e.message)
		}
		return
	}
	writeError(w, http.StatusInternalServerError, "profile_delete_failed", fmt.Sprint(err))
}
