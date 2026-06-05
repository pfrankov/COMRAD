package comrad

const placementDrainingReason = "placement_draining"

func warmTargetsByProfile(items []policyPlanItem) map[string]int {
	out := map[string]int{}
	for _, item := range items {
		out[item.profile.ID] = item.capacity.Warm
	}
	return out
}

func desiredWarmCounts(assignments []PlacementAssignment) map[string]int {
	out := map[string]int{}
	for _, assignment := range assignments {
		if assignment.DesiredWarm && assignment.MismatchReason == "" {
			out[assignment.ProfileID]++
		}
	}
	return out
}

func applyPlacementDrainState(db *Database) {
	desired, draining := warmSlotIntent(*db)
	for id, slot := range db.Slots {
		if !slotDrainable(slot) {
			continue
		}
		if !profileHasPolicy(*db, slot.ProfileID) {
			continue
		}
		if desired[id] && !draining[id] {
			slot.AcceptsNew = true
			if slot.MismatchReason == placementDrainingReason {
				slot.MismatchReason = ""
			}
			db.Slots[id] = slot
			continue
		}
		slot.AcceptsNew = false
		if slot.MismatchReason == "" {
			slot.MismatchReason = placementDrainingReason
		}
		db.Slots[id] = slot
	}
}

func warmSlotIntent(db Database) (map[string]bool, map[string]bool) {
	desired := map[string]bool{}
	draining := map[string]bool{}
	for _, assignment := range db.Assignments {
		if assignment.SlotID == "" || !assignment.DesiredWarm || assignment.MismatchReason != "" {
			continue
		}
		desired[assignment.SlotID] = true
		if assignment.Draining {
			draining[assignment.SlotID] = true
		}
	}
	return desired, draining
}

func slotDrainable(slot Slot) bool {
	return slot.ProfileID != "" &&
		slot.State == SlotStateReady &&
		slot.ActiveTaskID == "" &&
		!slot.Quarantined
}
