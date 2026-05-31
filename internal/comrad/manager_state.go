package comrad

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func (m *Manager) stateResponse() StateResponse {
	m.clearExpiredQuarantines()
	db := m.store.Snapshot()
	decoratePolicyEffectiveCapacity(&db, time.Now().UTC())
	fitMatrix := BuildFitMatrix(db)
	cachePlans := BuildCachePlans(db)
	sortCachePlans(cachePlans)
	runtimeSummary := BuildRuntimeSummary(db)
	decorateAdminStateConditions(&db, fitMatrix, cachePlans)
	audit := db.Audit
	if len(audit) > 100 {
		audit = audit[len(audit)-100:]
	}
	tasks, attempts, reports, truncated := stateTaskWindow(db)
	return StateResponse{
		Version:           Version,
		SchemaVersion:     db.SchemaVersion,
		Nodes:             SortedNodes(db),
		Slots:             SortedSlots(db),
		Artifacts:         SortedArtifacts(db),
		ArtifactEvictions: sortedArtifactEvictions(db),
		Profiles:          SortedProfiles(db),
		Policies:          SortedPolicies(db),
		Assignments:       SortedAssignments(db),
		FitMatrix:         fitMatrix,
		RuntimeSummary:    runtimeSummary,
		CachePlans:        cachePlans,
		Tasks:             tasks,
		Attempts:          attempts,
		Reports:           reports,
		TaskSummary:       summarizeDatabaseTasks(db),
		TaskPageLimit:     adminStateTaskLimit,
		TasksTruncated:    truncated,
		Updates:           sortedUpdates(db),
		Users:             SortedUsers(db),
		APIKeys:           SortedAPIKeyViews(db),
		ComputeLedger:     SortedComputeLedger(db),
		Queue:             QueueState{Limit: cap(m.queue), InUse: len(m.queue), Queued: queuedTaskCount(db)},
		AuditTail:         audit,
	}
}

func queuedTaskCount(db Database) int {
	n := 0
	for _, task := range db.Tasks {
		if task.Status == TaskStatusQueued {
			n++
		}
	}
	return n
}

func taskHasFirstOutput(db Database, taskID string) bool {
	for _, attempt := range db.Attempts {
		if attempt.TaskID == taskID && attempt.FirstOutputSent {
			return true
		}
	}
	return false
}

func sortedTasks(db Database) []Task {
	out := make([]Task, 0, len(db.Tasks))
	for _, task := range db.Tasks {
		out = append(out, task)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func sortedUpdates(db Database) []UpdateRecord {
	out := make([]UpdateRecord, 0, len(db.Updates))
	for _, u := range db.Updates {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func sortedArtifactEvictions(db Database) []ArtifactEvictionRecord {
	out := make([]ArtifactEvictionRecord, 0, len(db.ArtifactEvictions))
	for _, record := range db.ArtifactEvictions {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[j].UpdatedAt.Before(out[i].UpdatedAt)
	})
	return out
}

func (m *Manager) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !bearerMatches(r, m.cfg.AdminToken) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required")
			return
		}
		next(w, r)
	}
}

func (m *Manager) adminBearerOnly(next http.HandlerFunc) http.HandlerFunc {
	return m.adminOnly(next)
}

func (m *Manager) adminTicketOrBearer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !bearerMatches(r, m.cfg.AdminToken) && !m.consumeAdminStateWSTicket(r.URL.Query().Get("ticket"), time.Now().UTC()) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "admin token required")
			return
		}
		next(w, r)
	}
}

func (m *Manager) workerAuthorized(r *http.Request) bool {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = bearerToken(r)
	}
	return token != "" && token == m.cfg.WorkerToken
}

func bearerMatches(r *http.Request, expected string) bool {
	return bearerToken(r) == expected
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 10<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func readConfig(r *http.Request, out any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		return err
	}
	if isYAMLRequest(r) {
		dec := yaml.NewDecoder(bytes.NewReader(body))
		dec.KnownFields(true)
		return dec.Decode(out)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func isYAMLRequest(r *http.Request) bool {
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	return strings.Contains(ct, "yaml") || strings.Contains(ct, "yml")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func writeSSE(w io.Writer, event string, v any) {
	b, _ := json.Marshal(v)
	if event != "" && event != "message" {
		fmt.Fprintf(w, "event: %s\n", event)
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
}

func errorCode(err error) string {
	if err == nil {
		return "unknown"
	}
	s := err.Error()
	if s == "" {
		return "unknown"
	}
	if strings.Contains(s, FailureNoCapacity) {
		return FailureNoCapacity
	}
	return strings.Fields(s)[0]
}
