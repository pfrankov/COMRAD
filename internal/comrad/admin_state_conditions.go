package comrad

import (
	"strings"
	"time"
)

func decorateAdminStateConditions(db *Database, fit []FitResult, cachePlans []CachePlan) {
	decorateProfileConditions(db, fit)
	decoratePolicyConditions(db, cachePlans)
	decorateNodeConditions(db)
	decorateSlotConditions(db)
	decorateArtifactEvictionConditions(db)
}

func decorateProfileConditions(db *Database, fit []FitResult) {
	for id, profile := range db.Profiles {
		profile.Conditions = profileConditions(*db, profile, fit)
		db.Profiles[id] = profile
	}
}

func profileConditions(db Database, profile WorkloadProfile, fit []FitResult) []Condition {
	at := fallbackTime(profile.CreatedAt)
	return []Condition{
		profileReadyCondition(db, profile, at),
		profileSchedulableCondition(profile.ID, fit, at),
		profileArtifactsCondition(db, profile, at),
	}
}

func profileReadyCondition(db Database, profile WorkloadProfile, at time.Time) Condition {
	if profileHasReadyAssignment(db, profile.ID) {
		return newCondition("Ready", "True", "DesiredWarmCopiesReady", "At least one desired warm copy is ready.", at)
	}
	if !profileHasPolicy(db, profile.ID) {
		return newCondition("Ready", "False", "NoCapacityPolicy", "No capacity policy exists for this profile.", at)
	}
	return newCondition("Ready", "False", "NoReadySlot", "No desired warm copy is ready yet.", at)
}

func profileHasReadyAssignment(db Database, profileID string) bool {
	for _, assignment := range db.Assignments {
		if assignment.ProfileID == profileID && assignment.Ready {
			return true
		}
	}
	return false
}

func profileHasPolicy(db Database, profileID string) bool {
	for _, policy := range db.Policies {
		if policy.ProfileID == profileID {
			return true
		}
	}
	return false
}

func profileSchedulableCondition(profileID string, fit []FitResult, at time.Time) Condition {
	reasons := []string{}
	for _, result := range fit {
		if result.ProfileID != profileID {
			continue
		}
		if result.Fits {
			return newCondition("Schedulable", "True", "CompatibleSlotAvailable", "At least one compatible Worker slot exists.", at)
		}
		reasons = append(reasons, result.Reasons...)
	}
	reason := firstSortedReason(reasons, "NoCompatibleWorker")
	return newCondition("Schedulable", "False", reason, "No compatible Worker slot is currently available.", at)
}

func profileArtifactsCondition(db Database, profile WorkloadProfile, at time.Time) Condition {
	for _, id := range profileArtifactIDs(profile) {
		if _, ok := db.Artifacts[id]; !ok {
			return newCondition("ArtifactsAvailable", "False", "MissingArtifact", "One or more profile artifacts are missing.", at)
		}
	}
	return newCondition("ArtifactsAvailable", "True", "AllArtifactsRegistered", "All profile artifacts are registered.", at)
}

func decoratePolicyConditions(db *Database, cachePlans []CachePlan) {
	plans := cachePlansByProfile(cachePlans)
	for id, policy := range db.Policies {
		policy.Conditions = policyConditions(policy, plans[policy.ProfileID])
		db.Policies[id] = policy
	}
}

func policyConditions(policy PlacementPolicy, plan CachePlan) []Condition {
	at := fallbackTime(policy.UpdatedAt)
	return []Condition{
		policyCachedCondition(policy, plan, at),
		policyWarmCondition(policy, plan, at),
		policyPlacementCondition(plan, at),
	}
}

func cachePlansByProfile(plans []CachePlan) map[string]CachePlan {
	out := map[string]CachePlan{}
	for _, plan := range plans {
		out[plan.ProfileRef] = plan
	}
	return out
}

func policyCachedCondition(policy PlacementPolicy, plan CachePlan, at time.Time) Condition {
	if plan.ActualCopies >= desiredCachedCount(policy) {
		return newCondition("Cached", "True", "DesiredCopiesAvailable", "Desired cached copies are available.", at)
	}
	return newCondition("Cached", "False", "DesiredCopiesUnavailable", "Desired cached copies are not available yet.", at)
}

func policyWarmCondition(policy PlacementPolicy, plan CachePlan, at time.Time) Condition {
	actual := actualWarmCopies(plan)
	if actual >= desiredWarmCount(policy) {
		return newCondition("Warm", "True", "DesiredWarmCopiesReady", "Desired warm copies are ready.", at)
	}
	return newCondition("Warm", "False", "DesiredWarmCopiesUnavailable", "Desired warm copies are not ready yet.", at)
}

func actualWarmCopies(plan CachePlan) int {
	count := 0
	for _, worker := range plan.Workers {
		if worker.Warm {
			count++
		}
	}
	return count
}

func policyPlacementCondition(plan CachePlan, at time.Time) Condition {
	if planHasBlockedPlacement(plan) {
		return newCondition("PlacementSatisfied", "False", "PlacementBlocked", "One or more desired placements are blocked.", at)
	}
	return newCondition("PlacementSatisfied", "True", "PlacementApplied", "Desired placement is applied.", at)
}

func planHasBlockedPlacement(plan CachePlan) bool {
	if plan.ActualCopies < plan.DesiredCopies {
		return true
	}
	for _, worker := range plan.Workers {
		if worker.Eviction.Status == ArtifactEvictionFailed {
			return true
		}
	}
	return false
}

func decorateNodeConditions(db *Database) {
	for id, node := range db.Nodes {
		node.Conditions = nodeConditions(node, slotsForNode(*db, id))
		db.Nodes[id] = node
	}
}

func nodeConditions(node Node, slots []Slot) []Condition {
	at := fallbackTime(node.LastSeen)
	return []Condition{
		nodeConnectedCondition(node, at),
		boolCondition("Approved", node.Approved, "WorkerApproved", "WorkerNotApproved", "Worker is approved.", "Worker is not approved.", at),
		nodeCompatibleCondition(node, slots, at),
		boolCondition("Quarantined", node.Quarantined, "WorkerQuarantined", "WorkerNotQuarantined", "Worker is quarantined.", "Worker is not quarantined.", at),
	}
}

func slotsForNode(db Database, nodeID string) []Slot {
	out := []Slot{}
	for _, slot := range db.Slots {
		if slot.NodeID == nodeID {
			out = append(out, slot)
		}
	}
	return out
}

func nodeConnectedCondition(node Node, at time.Time) Condition {
	connected := node.State == NodeStateOnline
	return boolCondition("Connected", connected, "WorkerOnline", "WorkerOffline", "Worker is connected.", "Worker is offline.", at)
}

func nodeCompatibleCondition(node Node, slots []Slot, at time.Time) Condition {
	compatible := len(node.RuntimeAdapters) > 0 && len(slots) > 0
	return boolCondition("Compatible", compatible, "RuntimeAdapterAvailable", "NoRuntimeAdapter", "Worker reports a runtime adapter and slots.", "Worker has no runtime adapter or slots.", at)
}

func decorateSlotConditions(db *Database) {
	for id, slot := range db.Slots {
		slot.Conditions = slotConditions(slot)
		db.Slots[id] = slot
	}
}

func slotConditions(slot Slot) []Condition {
	at := fallbackTime(slot.LastReady)
	return []Condition{
		slotReadyCondition(slot, at),
		boolCondition("Assigned", slot.ProfileID != "", "ProfileAssigned", "NoProfileAssigned", "Slot has an assigned profile.", "Slot has no assigned profile.", at),
		boolCondition("Serving", slot.State == SlotStateServing, "TaskServing", "NotServing", "Slot is serving a task.", "Slot is not serving a task.", at),
		boolCondition("Quarantined", slot.Quarantined, "SlotQuarantined", "SlotNotQuarantined", "Slot is quarantined.", "Slot is not quarantined.", at),
	}
}

func slotReadyCondition(slot Slot, at time.Time) Condition {
	ready := slot.State == SlotStateReady && slot.AcceptsNew && !slot.Quarantined
	return boolCondition("Ready", ready, "SlotReady", "SlotNotReady", "Slot is ready for new tasks.", "Slot is not ready for new tasks.", at)
}

func decorateArtifactEvictionConditions(db *Database) {
	for id, record := range db.ArtifactEvictions {
		record.Conditions = artifactEvictionConditions(record)
		db.ArtifactEvictions[id] = record
	}
}

func artifactEvictionConditions(record ArtifactEvictionRecord) []Condition {
	at := fallbackTime(record.UpdatedAt)
	return []Condition{
		evictionCondition(record, "Queued", ArtifactEvictionQueued, "EvictionQueued", at),
		evictionCondition(record, "Blocked", ArtifactEvictionBlocked, blockedEvictionReason(record), at),
		evictionCondition(record, "Evicted", ArtifactEvictionEvicted, "EvictionCompleted", at),
		evictionCondition(record, "Failed", ArtifactEvictionFailed, "EvictionFailed", at),
	}
}

func evictionCondition(record ArtifactEvictionRecord, typ, status, reason string, at time.Time) Condition {
	if record.Status == status {
		return newCondition(typ, "True", reason, evictionMessage(record), at)
	}
	return newCondition(typ, "False", "NotCurrentStatus", "This is not the current eviction status.", at)
}

func blockedEvictionReason(record ArtifactEvictionRecord) string {
	if record.Failure == "worker_offline" {
		return "WorkerOffline"
	}
	if record.Failure == "worker_send_queue_full" {
		return "WorkerSendQueueFull"
	}
	return "EvictionBlocked"
}

func evictionMessage(record ArtifactEvictionRecord) string {
	if record.Failure != "" {
		return "Cache eviction " + safeConditionText(record.Failure) + "."
	}
	if record.Reason != "" {
		return "Cache eviction requested because " + safeConditionText(record.Reason) + "."
	}
	return "Cache eviction status is " + record.Status + "."
}

func boolCondition(typ string, ok bool, trueReason, falseReason, trueMessage, falseMessage string, at time.Time) Condition {
	if ok {
		return newCondition(typ, "True", trueReason, trueMessage, at)
	}
	return newCondition(typ, "False", falseReason, falseMessage, at)
}

func newCondition(typ, status, reason, message string, at time.Time) Condition {
	return Condition{Type: typ, Status: status, Reason: reason, Message: safeConditionText(message), LastTransitionTime: fallbackTime(at)}
}

func fallbackTime(at time.Time) time.Time {
	if at.IsZero() {
		return time.Now().UTC()
	}
	return at
}

func firstSortedReason(reasons []string, fallback string) string {
	reasons = uniqueSorted(reasons)
	if len(reasons) == 0 {
		return fallback
	}
	return conditionReason(reasons[0])
}

func conditionReason(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '_' || r == '-' || r == ':' || r == ',' })
	out := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		out += strings.ToUpper(part[:1]) + part[1:]
	}
	if out == "" {
		return "Unknown"
	}
	return out
}

func safeConditionText(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if len(value) > 240 {
		return value[:240]
	}
	return value
}
