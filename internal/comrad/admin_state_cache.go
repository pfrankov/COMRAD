package comrad

import (
	"sort"
	"time"
)

func BuildCachePlans(db Database) []CachePlan {
	return buildCachePlans(db, defaultAutoBalanceScaleDownCooldown)
}

func BuildCachePlansWithConfig(db Database, cfg ManagerConfig) []CachePlan {
	return buildCachePlans(db, managerAutoBalanceCooldown(cfg))
}

func buildCachePlans(db Database, cooldown time.Duration) []CachePlan {
	policies := SortedPolicies(db)
	out := make([]CachePlan, 0, len(policies))
	for _, policy := range policies {
		profile, ok := db.Profiles[policy.ProfileID]
		if !ok {
			continue
		}
		out = append(out, buildCachePlanWithCooldown(db, profile, policy, cooldown))
	}
	return out
}

func buildCachePlan(db Database, profile WorkloadProfile, policy PlacementPolicy) CachePlan {
	return buildCachePlanWithCooldown(db, profile, policy, defaultAutoBalanceScaleDownCooldown)
}

func buildCachePlanWithCooldown(db Database, profile WorkloadProfile, policy PlacementPolicy, cooldown time.Duration) CachePlan {
	artifacts := profileArtifactIDs(profile)
	desiredNodes := desiredCacheNodes(db, profile.ID)
	workers := cacheWorkerStatuses(db, profile, artifacts, desiredNodes)
	capacity := effectivePolicyCapacity(db, policy, time.Now().UTC(), cooldown)
	plan := CachePlan{
		ProfileRef:       profile.ID,
		Artifacts:        artifacts,
		RequireTags:      cachePlanRequireTags(profile, policy),
		DesiredCopies:    capacity.Cached,
		EvictionsPending: pendingEvictionsForArtifacts(db, artifacts),
		Workers:          workers,
	}
	plan.ActualCopies, plan.StaleCopies = cachePlanCopyCounts(workers, desiredNodes)
	plan.Conditions = cachePlanConditions(plan, policy)
	return plan
}

func desiredCacheNodes(db Database, profileID string) map[string]bool {
	out := map[string]bool{}
	for _, assignment := range db.Assignments {
		if assignment.ProfileID == profileID && assignment.DesiredCached && assignment.NodeID != "" {
			out[assignment.NodeID] = true
		}
	}
	return out
}

func cachePlanRequireTags(profile WorkloadProfile, policy PlacementPolicy) []string {
	tags := append([]string{}, policy.Constraints.RequireTags...)
	if profile.Requirements != nil {
		tags = append(tags, profile.Requirements.RequireTags...)
	}
	return uniqueSorted(tags)
}

func pendingEvictionsForArtifacts(db Database, artifacts []string) int {
	count := 0
	for _, record := range db.ArtifactEvictions {
		if artifactInSet(record.ArtifactID, artifacts) && evictionPending(record.Status) {
			count++
		}
	}
	return count
}

func evictionPending(status string) bool {
	return status == ArtifactEvictionQueued || status == ArtifactEvictionBlocked
}

func cacheWorkerStatuses(db Database, profile WorkloadProfile, artifacts []string, desiredNodes map[string]bool) []CacheWorkerStatus {
	out := []CacheWorkerStatus{}
	for _, node := range SortedNodes(db) {
		status := cacheWorkerStatusForNode(db, profile, artifacts, node)
		if includeCacheWorker(status, desiredNodes[node.ID]) {
			out = append(out, status)
		}
	}
	return out
}

func cacheWorkerStatusForNode(db Database, profile WorkloadProfile, artifacts []string, node Node) CacheWorkerStatus {
	return CacheWorkerStatus{
		NodeID:   node.ID,
		Cached:   nodeHasArtifacts(node, artifacts),
		Warm:     nodeWarmForProfile(db, node.ID, profile),
		Active:   nodeActiveForProfile(db, node.ID, profile.ID),
		Eviction: latestEvictionStatus(db, node.ID, artifacts),
		Intent:   latestCacheIntentStatus(db, node.ID, artifacts),
	}
}

func includeCacheWorker(status CacheWorkerStatus, desired bool) bool {
	return desired || status.Cached || status.Warm || status.Active || status.Eviction.Status != "none"
}

func cachePlanCopyCounts(workers []CacheWorkerStatus, desiredNodes map[string]bool) (int, int) {
	actual := 0
	stale := 0
	for _, worker := range workers {
		if !worker.Cached {
			continue
		}
		actual++
		if !desiredNodes[worker.NodeID] && !worker.Active {
			stale++
		}
	}
	return actual, stale
}

func nodeHasArtifacts(node Node, artifacts []string) bool {
	if len(artifacts) == 0 {
		return false
	}
	for _, artifact := range artifacts {
		if !containsArtifactID(node.CachedArtifacts, artifact) {
			return false
		}
	}
	return true
}

func nodeWarmForProfile(db Database, nodeID string, profile WorkloadProfile) bool {
	if nodeHasWarmAssignment(db, nodeID, profile.ID) {
		return true
	}
	for _, slot := range db.Slots {
		if slot.NodeID == nodeID && slotWarmForProfile(slot, profile) {
			return true
		}
	}
	return false
}

func nodeHasWarmAssignment(db Database, nodeID, profileID string) bool {
	for _, assignment := range db.Assignments {
		if assignment.NodeID == nodeID && assignment.ProfileID == profileID && assignment.ActualWarm {
			return true
		}
	}
	return false
}

func slotWarmForProfile(slot Slot, profile WorkloadProfile) bool {
	return slot.ProfileID == profile.ID && (slot.State == SlotStateReady || slot.State == SlotStateServing)
}

func nodeActiveForProfile(db Database, nodeID, profileID string) bool {
	return nodeHasRunningAttempt(db, nodeID, profileID) || nodeHasServingSlot(db, nodeID, profileID)
}

func nodeHasRunningAttempt(db Database, nodeID, profileID string) bool {
	for _, attempt := range db.Attempts {
		if attempt.NodeID == nodeID && attempt.ProfileID == profileID && attempt.Status == TaskStatusRunning {
			return true
		}
	}
	return false
}

func nodeHasServingSlot(db Database, nodeID, profileID string) bool {
	for _, slot := range db.Slots {
		if slot.NodeID == nodeID && slot.ProfileID == profileID && slot.State == SlotStateServing {
			return true
		}
	}
	return false
}

func latestEvictionStatus(db Database, nodeID string, artifacts []string) CacheEvictionStatus {
	var latest ArtifactEvictionRecord
	for _, record := range db.ArtifactEvictions {
		if record.NodeID == nodeID && artifactInSet(record.ArtifactID, artifacts) && record.UpdatedAt.After(latest.UpdatedAt) {
			latest = record
		}
	}
	if latest.ID == "" {
		return CacheEvictionStatus{Status: "none"}
	}
	return CacheEvictionStatus{
		Status:    latest.Status,
		Reason:    latest.Reason,
		Failure:   latest.Failure,
		UpdatedAt: latest.UpdatedAt,
	}
}

func latestCacheIntentStatus(db Database, nodeID string, artifacts []string) CacheIntentStatus {
	var latest CacheIntentRecord
	for _, record := range db.CacheIntents {
		if record.NodeID == nodeID && artifactInSet(record.ArtifactID, artifacts) && record.UpdatedAt.After(latest.UpdatedAt) {
			latest = record
		}
	}
	if latest.ID == "" {
		return CacheIntentStatus{}
	}
	return CacheIntentStatus{Action: latest.Action, UpdatedAt: latest.UpdatedAt}
}

func artifactInSet(artifactID string, artifacts []string) bool {
	artifactID = NormalizeSHA256(artifactID)
	for _, artifact := range artifacts {
		if NormalizeSHA256(artifact) == artifactID {
			return true
		}
	}
	return false
}

func cachePlanConditions(plan CachePlan, policy PlacementPolicy) []Condition {
	at := policy.UpdatedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return []Condition{
		cacheCopiesCondition("Cached", plan.ActualCopies, plan.DesiredCopies, at),
		cacheCopiesCondition("Stale", 0, plan.StaleCopies, at),
		cacheEvictionsCondition(plan.EvictionsPending, at),
	}
}

func cacheCopiesCondition(typ string, actual, desired int, at time.Time) Condition {
	if actual >= desired {
		return newCondition(typ, "True", "DesiredCopiesAvailable", "Desired cached copies are available.", at)
	}
	return newCondition(typ, "False", "DesiredCopiesUnavailable", "Desired cached copies are not available yet.", at)
}

func cacheEvictionsCondition(pending int, at time.Time) Condition {
	if pending == 0 {
		return newCondition("EvictionsClear", "True", "NoPendingEvictions", "No cache evictions are pending.", at)
	}
	return newCondition("EvictionsClear", "False", "EvictionsPending", "Cache evictions are pending or blocked.", at)
}

func sortCachePlans(plans []CachePlan) {
	sort.Slice(plans, func(i, j int) bool { return plans[i].ProfileRef < plans[j].ProfileRef })
}
