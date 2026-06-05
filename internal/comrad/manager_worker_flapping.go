package comrad

import "time"

const (
	workerFlapEventDisconnect       = "disconnect"
	workerFlapEventReconnect        = "reconnect"
	workerFlapEventHeartbeatExpired = "heartbeat_expired"
)

func recordWorkerFlapEvent(node Node, eventType string, now time.Time, cfg ManagerConfig) Node {
	if eventType == "" {
		return node
	}
	node.RecentFlapEvents = append(pruneWorkerFlapEvents(node.RecentFlapEvents, now, workerFlapWindow(cfg)), WorkerFlapEvent{Type: eventType, At: now})
	if len(node.RecentFlapEvents) >= workerFlapThreshold(cfg) {
		until := now.Add(workerFlapCooldown(cfg))
		node.WarmPlacementSuppressed = true
		node.WarmPlacementSuppressionReason = FailureWorkerFlapping
		node.WarmPlacementSuppressionUntil = &until
	}
	return node
}

func pruneWorkerFlapEvents(events []WorkerFlapEvent, now time.Time, window time.Duration) []WorkerFlapEvent {
	cutoff := now.Add(-window)
	out := events[:0]
	for _, event := range events {
		if !event.At.Before(cutoff) {
			out = append(out, event)
		}
	}
	return out
}

func workerReconnectEventApplies(existing Node, sessionID string) bool {
	if existing.ID == "" {
		return false
	}
	if existing.State != NodeStateOnline {
		return true
	}
	return existing.ConnectedSession != sessionID
}

func workerFlapThreshold(cfg ManagerConfig) int {
	if cfg.WorkerFlapThreshold <= 0 {
		return 4
	}
	return cfg.WorkerFlapThreshold
}

func workerFlapWindow(cfg ManagerConfig) time.Duration {
	if cfg.WorkerFlapWindow <= 0 {
		return 5 * time.Minute
	}
	return cfg.WorkerFlapWindow
}

func workerFlapCooldown(cfg ManagerConfig) time.Duration {
	if cfg.WorkerFlapCooldown <= 0 {
		return 5 * time.Minute
	}
	return cfg.WorkerFlapCooldown
}

func nodeWarmPlacementSuppressed(node Node, now time.Time) bool {
	return node.WarmPlacementSuppressed && node.WarmPlacementSuppressionUntil != nil && node.WarmPlacementSuppressionUntil.After(now)
}

func clearExpiredWorkerSuppression(node *Node, now time.Time) bool {
	if !node.WarmPlacementSuppressed {
		return false
	}
	if node.WarmPlacementSuppressionUntil != nil && node.WarmPlacementSuppressionUntil.After(now) {
		return false
	}
	node.WarmPlacementSuppressed = false
	node.WarmPlacementSuppressionReason = ""
	node.WarmPlacementSuppressionUntil = nil
	return true
}

func (m *Manager) clearExpiredWorkerSuppressions(now time.Time) {
	if !m.needsWorkerFlapMaintenance(now) {
		return
	}
	_ = m.store.Update(func(db *Database) error {
		clearExpiredWorkerSuppressions(db, now, m.cfg)
		return nil
	})
}

func (m *Manager) needsWorkerFlapMaintenance(now time.Time) bool {
	db := m.store.Snapshot()
	cutoff := now.Add(-workerFlapWindow(m.cfg))
	for _, node := range db.Nodes {
		if node.WarmPlacementSuppressed && !nodeWarmPlacementSuppressed(node, now) {
			return true
		}
		for _, event := range node.RecentFlapEvents {
			if event.At.Before(cutoff) {
				return true
			}
		}
	}
	return false
}

func clearExpiredWorkerSuppressions(db *Database, now time.Time, cfg ManagerConfig) {
	for id, node := range db.Nodes {
		node.RecentFlapEvents = pruneWorkerFlapEvents(node.RecentFlapEvents, now, workerFlapWindow(cfg))
		clearExpiredWorkerSuppression(&node, now)
		db.Nodes[id] = node
	}
}
