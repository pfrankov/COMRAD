package comrad

import "time"

func (m *Manager) runWorkerHeartbeatMonitor(ctxDone <-chan struct{}) {
	interval := m.cfg.WorkerHeartbeatTimeout / 3
	if interval <= 0 || interval > 10*time.Second {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctxDone:
			return
		case <-ticker.C:
			m.expireWorkerHeartbeats(time.Now().UTC())
		}
	}
}

func (m *Manager) expireWorkerHeartbeats(now time.Time) {
	timeout := m.cfg.WorkerHeartbeatTimeout
	if timeout <= 0 {
		return
	}
	cutoff := now.Add(-timeout)
	sessions, offline := m.staleWorkerSessions(cutoff)
	for _, session := range sessions {
		session.close()
	}
	if len(offline) > 0 {
		m.markOfflineWorkers(offline)
	}
	if len(sessions) > 0 || len(offline) > 0 {
		m.replanAndDispatch()
	}
}

func (m *Manager) staleWorkerSessions(cutoff time.Time) ([]*workerSession, []string) {
	db := m.store.Snapshot()
	sessions := []*workerSession{}
	offline := []string{}
	for _, node := range db.Nodes {
		if node.State != NodeStateOnline || node.LastSeen.After(cutoff) {
			continue
		}
		m.mu.Lock()
		session := m.sessions[node.ID]
		m.mu.Unlock()
		if session != nil {
			sessions = append(sessions, session)
		} else {
			offline = append(offline, node.ID)
		}
	}
	return sessions, offline
}

func (m *Manager) markOfflineWorkers(nodeIDs []string) {
	_ = m.store.Update(func(db *Database) error {
		for _, nodeID := range nodeIDs {
			node := db.Nodes[nodeID]
			if node.ID != "" {
				markDisconnectedNode(db, node)
			}
		}
		return nil
	})
}
