package comrad

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

type RuntimeMetrics struct {
	AdminStateWSClients                  int
	AdminStateWSConnectsTotal            int64
	AdminStateWSBroadcastsTotal          int64
	AdminStateWSDroppedUpdatesTotal      int64
	AdminStateWSWriteFailuresTotal       int64
	AdminStateWSLastSnapshotBytes        int
	AdminStateWSLastBroadcastSubscribers int
}

func (m *Manager) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	writePrometheusMetrics(w, m.store.Snapshot(), len(m.queue), cap(m.queue), m.store.BackendName(), m.runtimeMetricsSnapshot())
}

func (m *Manager) runtimeMetricsSnapshot() RuntimeMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.runtimeMetrics
	out.AdminStateWSClients = len(m.adminStateSubscribers)
	return out
}

func writePrometheusMetrics(w io.Writer, db Database, queueInUse, queueLimit int, backend string, runtime RuntimeMetrics) {
	fmt.Fprintln(w, "# HELP comrad_queue_limit Maximum number of queued or active task leases accepted by the Manager.")
	fmt.Fprintln(w, "# TYPE comrad_queue_limit gauge")
	fmt.Fprintf(w, "comrad_queue_limit %d\n", queueLimit)
	fmt.Fprintln(w, "# HELP comrad_queue_in_use Queue lease slots currently in use.")
	fmt.Fprintln(w, "# TYPE comrad_queue_in_use gauge")
	fmt.Fprintf(w, "comrad_queue_in_use %d\n", queueInUse)
	fmt.Fprintln(w, "# HELP comrad_storage_backend_info Storage backend selected by the Manager.")
	fmt.Fprintln(w, "# TYPE comrad_storage_backend_info gauge")
	fmt.Fprintf(w, "comrad_storage_backend_info{backend=%q} 1\n", backend)
	writeCountByLabel(w, "comrad_nodes_total", "Nodes by lifecycle state.", "state", nodeStateCounts(db))
	writeCountByLabel(w, "comrad_slots_total", "Slots by lifecycle state.", "state", slotStateCounts(db))
	writeCountByLabel(w, "comrad_tasks_total", "Tasks by lifecycle status.", "status", taskStatusCounts(db))
	writeCountByLabel(w, "comrad_reports_total", "Compute reports by status.", "status", reportStatusCounts(db))
	fmt.Fprintln(w, "# HELP comrad_profiles_total Workload profiles stored by the Manager.")
	fmt.Fprintln(w, "# TYPE comrad_profiles_total gauge")
	fmt.Fprintf(w, "comrad_profiles_total %d\n", len(db.Profiles))
	fmt.Fprintln(w, "# HELP comrad_artifacts_total Artifacts stored by the Manager.")
	fmt.Fprintln(w, "# TYPE comrad_artifacts_total gauge")
	fmt.Fprintf(w, "comrad_artifacts_total %d\n", len(db.Artifacts))
	writeCapacityMetrics(w, db)
	writeRuntimeMetrics(w, runtime)
}

type capacityMetricState struct {
	ProfileID     string
	LogicalModel  string
	DesiredCached int
	ActualCached  int
	DesiredWarm   int
	ActualWarm    int
	Warming       int
	Failed        int
	Blocked       int
}

func writeCapacityMetrics(w io.Writer, db Database) {
	states := capacityMetricStates(db)
	writeCapacityMetric(w, "comrad_capacity_desired_cached", "Desired cached copies by logical model and profile.", states, func(s capacityMetricState) int { return s.DesiredCached })
	writeCapacityMetric(w, "comrad_capacity_actual_cached", "Actual cached copies by logical model and profile.", states, func(s capacityMetricState) int { return s.ActualCached })
	writeCapacityMetric(w, "comrad_capacity_desired_warm", "Desired warm copies by logical model and profile.", states, func(s capacityMetricState) int { return s.DesiredWarm })
	writeCapacityMetric(w, "comrad_capacity_actual_warm", "Actual warm copies by logical model and profile.", states, func(s capacityMetricState) int { return s.ActualWarm })
	writeCapacityMetric(w, "comrad_capacity_warming", "Desired warm copies currently downloading, loading, or warming.", states, func(s capacityMetricState) int { return s.Warming })
	writeCapacityMetric(w, "comrad_capacity_failed", "Desired capacity copies currently failed.", states, func(s capacityMetricState) int { return s.Failed })
	writeCapacityMetric(w, "comrad_capacity_blocked", "Desired capacity copies blocked by placement.", states, func(s capacityMetricState) int { return s.Blocked })
}

func writeCapacityMetric(w io.Writer, metric, help string, states []capacityMetricState, value func(capacityMetricState) int) {
	fmt.Fprintf(w, "# HELP %s %s\n", metric, help)
	fmt.Fprintf(w, "# TYPE %s gauge\n", metric)
	for _, state := range states {
		fmt.Fprintf(w, "%s{model=%q,profile=%q} %d\n", metric, state.LogicalModel, state.ProfileID, value(state))
	}
}

func capacityMetricStates(db Database) []capacityMetricState {
	plans := cachePlansByProfile(BuildCachePlans(db))
	now := time.Now().UTC()
	out := []capacityMetricState{}
	for _, policy := range SortedPolicies(db) {
		profile, ok := db.Profiles[policy.ProfileID]
		if !ok {
			continue
		}
		capacity := EffectivePolicyCapacity(db, policy, now)
		plan := plans[profile.ID]
		warming, failed, blocked := capacityProgressCounts(db, profile.ID)
		out = append(out, capacityMetricState{
			ProfileID:     profile.ID,
			LogicalModel:  ProfileLogicalModel(profile),
			DesiredCached: capacity.Cached,
			ActualCached:  plan.ActualCopies,
			DesiredWarm:   capacity.Warm,
			ActualWarm:    actualWarmCopies(plan),
			Warming:       warming,
			Failed:        failed,
			Blocked:       blocked,
		})
	}
	return out
}

func capacityProgressCounts(db Database, profileID string) (int, int, int) {
	warming, failed, blocked := 0, 0, 0
	desiredWarmSlots := desiredWarmSlotIDs(db, profileID)
	for _, slot := range db.Slots {
		if !desiredWarmSlots[slot.ID] || slot.ProfileID != profileID {
			continue
		}
		switch slot.State {
		case SlotStateDownloading, SlotStateCached, SlotStateLoading, SlotStateWarming:
			warming++
		case SlotStateError:
			failed++
		}
	}
	for _, assignment := range db.Assignments {
		if assignment.ProfileID != profileID || !assignment.DesiredCached || assignment.MismatchReason == "" {
			continue
		}
		if slot, ok := db.Slots[assignment.SlotID]; ok && slot.State == SlotStateError {
			continue
		}
		blocked++
	}
	return warming, failed, blocked
}

func desiredWarmSlotIDs(db Database, profileID string) map[string]bool {
	out := map[string]bool{}
	for _, assignment := range db.Assignments {
		if assignment.ProfileID == profileID && assignment.DesiredWarm && assignment.SlotID != "" {
			out[assignment.SlotID] = true
		}
	}
	return out
}

func writeRuntimeMetrics(w io.Writer, runtime RuntimeMetrics) {
	fmt.Fprintln(w, "# HELP comrad_admin_state_ws_clients Active dashboard state WebSocket clients.")
	fmt.Fprintln(w, "# TYPE comrad_admin_state_ws_clients gauge")
	fmt.Fprintf(w, "comrad_admin_state_ws_clients %d\n", runtime.AdminStateWSClients)
	fmt.Fprintln(w, "# HELP comrad_admin_state_ws_connects_total Dashboard state WebSocket connections accepted.")
	fmt.Fprintln(w, "# TYPE comrad_admin_state_ws_connects_total counter")
	fmt.Fprintf(w, "comrad_admin_state_ws_connects_total %d\n", runtime.AdminStateWSConnectsTotal)
	fmt.Fprintln(w, "# HELP comrad_admin_state_ws_broadcasts_total Dashboard state broadcasts attempted.")
	fmt.Fprintln(w, "# TYPE comrad_admin_state_ws_broadcasts_total counter")
	fmt.Fprintf(w, "comrad_admin_state_ws_broadcasts_total %d\n", runtime.AdminStateWSBroadcastsTotal)
	fmt.Fprintln(w, "# HELP comrad_admin_state_ws_dropped_updates_total Dashboard state updates replaced in full subscriber buffers.")
	fmt.Fprintln(w, "# TYPE comrad_admin_state_ws_dropped_updates_total counter")
	fmt.Fprintf(w, "comrad_admin_state_ws_dropped_updates_total %d\n", runtime.AdminStateWSDroppedUpdatesTotal)
	fmt.Fprintln(w, "# HELP comrad_admin_state_ws_write_failures_total Dashboard state WebSocket write failures.")
	fmt.Fprintln(w, "# TYPE comrad_admin_state_ws_write_failures_total counter")
	fmt.Fprintf(w, "comrad_admin_state_ws_write_failures_total %d\n", runtime.AdminStateWSWriteFailuresTotal)
	fmt.Fprintln(w, "# HELP comrad_admin_state_ws_last_snapshot_bytes Last dashboard state snapshot size in bytes.")
	fmt.Fprintln(w, "# TYPE comrad_admin_state_ws_last_snapshot_bytes gauge")
	fmt.Fprintf(w, "comrad_admin_state_ws_last_snapshot_bytes %d\n", runtime.AdminStateWSLastSnapshotBytes)
	fmt.Fprintln(w, "# HELP comrad_admin_state_ws_last_broadcast_subscribers Subscribers targeted by the last dashboard state broadcast.")
	fmt.Fprintln(w, "# TYPE comrad_admin_state_ws_last_broadcast_subscribers gauge")
	fmt.Fprintf(w, "comrad_admin_state_ws_last_broadcast_subscribers %d\n", runtime.AdminStateWSLastBroadcastSubscribers)
}

func writeCountByLabel(w io.Writer, metric, help, label string, counts map[string]int) {
	fmt.Fprintf(w, "# HELP %s %s\n", metric, help)
	fmt.Fprintf(w, "# TYPE %s gauge\n", metric)
	for _, key := range sortedMetricKeys(counts) {
		fmt.Fprintf(w, "%s{%s=%q} %d\n", metric, label, key, counts[key])
	}
}

func sortedMetricKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nodeStateCounts(db Database) map[string]int {
	counts := map[string]int{}
	for _, node := range db.Nodes {
		counts[node.State]++
	}
	return counts
}

func slotStateCounts(db Database) map[string]int {
	counts := map[string]int{}
	for _, slot := range db.Slots {
		counts[slot.State]++
	}
	return counts
}

func taskStatusCounts(db Database) map[string]int {
	counts := map[string]int{}
	for _, task := range db.Tasks {
		counts[task.Status]++
	}
	return counts
}

func reportStatusCounts(db Database) map[string]int {
	counts := map[string]int{}
	for _, report := range db.Reports {
		counts[report.Status]++
	}
	return counts
}
