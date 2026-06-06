package comrad

import "time"

func normalizeWorkerNode(node Node, sessionID string, cached []string, warm []string, now time.Time) Node {
	if node.ID == "" {
		node.ID = NewID("node")
	}
	node.LastSeen = now
	node.State = NodeStateOnline
	node.ConnectedSession = sessionID
	if node.Mode == "" {
		node.Mode = "reserved_always"
	}
	if node.Target == "" && node.OS == "darwin" && node.Arch == "arm64" {
		node.Target = TargetDarwinArm64Metal
	}
	if cached != nil {
		node.CachedArtifacts = cached
	}
	if warm != nil {
		node.WarmProfiles = warm
	}
	return node
}

func mergeExistingNode(node Node, existing Node, autoApprove bool, now time.Time) Node {
	if existing.ID != "" {
		node.Approved = existing.Approved
		mergeExistingNodeConfig(&node, existing)
		copyNodeFailureState(&node, existing)
	}
	if quarantineExpiredNode(node, now) {
		clearNodeQuarantine(&node)
	}
	clearExpiredWorkerSuppression(&node, now)
	if autoApprove {
		node.Approved = true
	}
	return node
}

func mergeExistingNodeConfig(node *Node, existing Node) {
	if len(existing.Tags) > 0 && len(node.Tags) == 0 {
		node.Tags = existing.Tags
	}
	if existing.Mode != "" && node.Mode == "" {
		node.Mode = existing.Mode
	}
}

func copyNodeFailureState(node *Node, existing Node) {
	node.LastFailure = existing.LastFailure
	node.LastFailureAt = existing.LastFailureAt
	node.P2P = cloneWorkerP2PStatus(existing.P2P)
	node.Quarantined = existing.Quarantined
	node.QuarantineReason = existing.QuarantineReason
	node.QuarantineUntil = existing.QuarantineUntil
	node.RecentFlapEvents = existing.RecentFlapEvents
	node.WarmPlacementSuppressed = existing.WarmPlacementSuppressed
	node.WarmPlacementSuppressionReason = existing.WarmPlacementSuppressionReason
	node.WarmPlacementSuppressionUntil = existing.WarmPlacementSuppressionUntil
}

func upsertWorkerSlots(db *Database, node Node, slots []Slot, now time.Time) {
	for _, slot := range slots {
		slot = normalizeWorkerSlot(slot, node)
		slot = mergeExistingSlot(slot, db.Slots[slot.ID], now)
		db.Slots[slot.ID] = slot
	}
}

func normalizeWorkerSlot(slot Slot, node Node) Slot {
	if slot.ID == "" {
		slot.ID = node.ID + "/slot0"
	}
	slot.NodeID = node.ID
	if slot.Target == "" {
		slot.Target = node.Target
	}
	if slot.RuntimeAdapter == "" && len(node.RuntimeAdapters) > 0 {
		slot.RuntimeAdapter = node.RuntimeAdapters[0]
	}
	if slot.State == "" {
		slot.State = SlotStateIdle
	}
	slot.AcceptsNew = slot.State == SlotStateReady
	return slot
}

func mergeExistingSlot(slot Slot, old Slot, now time.Time) Slot {
	mergeSlotProfileState(&slot, old)
	copySlotFailureState(&slot, old)
	if quarantineExpired(slot, now) {
		clearSlotQuarantine(&slot)
	}
	if slot.Quarantined {
		slot.AcceptsNew = false
		slot.MismatchReason = FailureQuarantined + ": " + slot.QuarantineReason
	}
	return slot
}

func mergeSlotProfileState(slot *Slot, old Slot) {
	if old.ProfileID != "" && slot.ProfileID == "" {
		slot.ProfileID = old.ProfileID
	}
	if old.ProfileVersion != 0 && slot.ProfileVersion == 0 {
		slot.ProfileVersion = old.ProfileVersion
	}
	if old.RuntimeVariantID != "" && slot.RuntimeVariantID == "" {
		slot.RuntimeVariantID = old.RuntimeVariantID
	}
	if old.LogicalModel != "" && slot.LogicalModel == "" {
		slot.LogicalModel = old.LogicalModel
	}
	if old.ModelArtifactID != "" && slot.ModelArtifactID == "" {
		slot.ModelArtifactID = old.ModelArtifactID
	}
	if old.ModelSHA256 != "" && slot.ModelSHA256 == "" {
		slot.ModelSHA256 = old.ModelSHA256
	}
}

func copySlotFailureState(slot *Slot, old Slot) {
	slot.FailureCount = old.FailureCount
	slot.FailureCounters = old.FailureCounters
	slot.LastFailure = old.LastFailure
	slot.LastFailureAt = old.LastFailureAt
	slot.Quarantined = old.Quarantined
	slot.QuarantineReason = old.QuarantineReason
	slot.QuarantineUntil = old.QuarantineUntil
}
