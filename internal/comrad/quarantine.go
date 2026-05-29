package comrad

import "time"

func recordSlotFailure(db *Database, slot *Slot, reason string, now time.Time, threshold int, duration time.Duration) {
	if reason == "" {
		reason = "unknown"
	}
	slot.FailureCount++
	if slot.FailureCounters == nil {
		slot.FailureCounters = map[string]int{}
	}
	slot.FailureCounters[reason]++
	slot.LastFailure = reason
	slot.LastFailureAt = &now
	if threshold <= 0 {
		threshold = 3
	}
	if duration <= 0 {
		duration = 5 * time.Minute
	}
	if slot.FailureCounters[reason] >= threshold {
		until := now.Add(duration)
		slot.Quarantined = true
		slot.QuarantineReason = reason
		slot.QuarantineUntil = &until
		slot.AcceptsNew = false
		slot.MismatchReason = FailureQuarantined + ": " + reason
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "quarantine.slot_banned", Actor: "manager", Subject: slot.ID, Metadata: map[string]any{"reason": reason}, CreatedAt: now})
	}
	node := db.Nodes[slot.NodeID]
	if node.ID != "" {
		node.LastFailure = reason
		node.LastFailureAt = &now
		db.Nodes[node.ID] = node
	}
}

func quarantineExpired(slot Slot, now time.Time) bool {
	return slot.Quarantined && slot.QuarantineUntil != nil && !slot.QuarantineUntil.After(now)
}

func quarantineExpiredNode(node Node, now time.Time) bool {
	return node.Quarantined && node.QuarantineUntil != nil && !node.QuarantineUntil.After(now)
}

func clearSlotQuarantine(slot *Slot) {
	slot.Quarantined = false
	slot.QuarantineReason = ""
	slot.QuarantineUntil = nil
	if slot.State == SlotStateReady && slot.ActiveTaskID == "" {
		slot.AcceptsNew = true
		slot.MismatchReason = ""
	}
}

func clearNodeQuarantine(node *Node) {
	node.Quarantined = false
	node.QuarantineReason = ""
	node.QuarantineUntil = nil
}

func (m *Manager) clearExpiredQuarantines() {
	now := time.Now().UTC()
	if !m.hasExpiredQuarantines(now) {
		return
	}
	_ = m.store.Update(func(db *Database) error {
		for id, node := range db.Nodes {
			if quarantineExpiredNode(node, now) {
				clearNodeQuarantine(&node)
				db.Nodes[id] = node
				db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "quarantine.node_expired", Actor: "manager", Subject: id, CreatedAt: now})
			}
		}
		for id, slot := range db.Slots {
			if quarantineExpired(slot, now) {
				clearSlotQuarantine(&slot)
				db.Slots[id] = slot
				db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "quarantine.slot_expired", Actor: "manager", Subject: id, CreatedAt: now})
			}
		}
		return nil
	})
}

func (m *Manager) hasExpiredQuarantines(now time.Time) bool {
	db := m.store.Snapshot()
	for _, node := range db.Nodes {
		if quarantineExpiredNode(node, now) {
			return true
		}
	}
	for _, slot := range db.Slots {
		if quarantineExpired(slot, now) {
			return true
		}
	}
	return false
}
