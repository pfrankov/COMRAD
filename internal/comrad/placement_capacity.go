package comrad

import (
	"fmt"
	"time"
)

func validatePlacementPolicy(policy PlacementPolicy) error {
	for name, value := range map[string]int{
		"cachedCount":              policy.CachedCount,
		"warmCount":                policy.WarmCount,
		"minCachedCount":           policy.MinCachedCount,
		"maxCachedCount":           policy.MaxCachedCount,
		"minWarmCount":             policy.MinWarmCount,
		"maxWarmCount":             policy.MaxWarmCount,
		"maxCachedProfilesPerNode": policy.MaxCachedProfilesPerNode,
		"maxWarmProfilesPerNode":   policy.MaxWarmProfilesPerNode,
	} {
		if value < 0 {
			return fmt.Errorf("%s must be non-negative", name)
		}
	}
	if policy.MaxWarmCount > 0 && policy.MinWarmCount > policy.MaxWarmCount {
		return fmt.Errorf("minWarmCount must be less than or equal to maxWarmCount")
	}
	if policy.MaxCachedCount > 0 && policy.MinCachedCount > policy.MaxCachedCount {
		return fmt.Errorf("minCachedCount must be less than or equal to maxCachedCount")
	}
	return nil
}

func EffectivePolicyCapacity(db Database, policy PlacementPolicy, now time.Time) policyCapacity {
	return effectivePolicyCapacity(db, policy, now, defaultAutoBalanceScaleDownCooldown)
}

func effectivePolicyCapacity(db Database, policy PlacementPolicy, now time.Time, cooldown time.Duration) policyCapacity {
	if !policy.AutoBalance {
		warm := max(0, policy.WarmCount)
		cached := max(warm, policy.CachedCount)
		return policyCapacity{Cached: cached, Warm: warm, MinCached: cached, MinWarm: warm}
	}
	queued, running, recent, smoothed := profileDemand(db, policy.ProfileID, now)
	minWarm := policy.MinWarmCount
	if minWarm == 0 && policy.WarmCount > 0 {
		minWarm = policy.WarmCount
	}
	minCached := max(policy.MinCachedCount, minWarm)
	if minCached == 0 && policy.CachedCount > 0 {
		minCached = policy.CachedCount
	}
	demand := queued + running + smoothed
	demand = max(demand, autoBalanceCooldownDemand(db, policy.ProfileID, now, cooldown))
	warm := clampCount(demand, minWarm, policy.MaxWarmCount)
	cached := clampCount(max(warm, minCached), minCached, policy.MaxCachedCount)
	cached = max(cached, warm)
	return policyCapacity{
		Cached:         cached,
		Warm:           warm,
		MinCached:      minCached,
		MinWarm:        minWarm,
		DemandQueued:   queued,
		DemandRunning:  running,
		DemandRecent:   recent,
		DemandSmoothed: smoothed,
	}
}

func profileDemand(db Database, profileID string, now time.Time) (int, int, int, int) {
	queued, running, recent := 0, 0, 0
	buckets := [autoBalanceDemandBuckets]int{}
	for _, task := range db.Tasks {
		if task.ProfileID != profileID {
			continue
		}
		if task.Status == TaskStatusQueued {
			queued++
		}
		if task.Status == TaskStatusRunning {
			running++
		}
		if bucket, ok := demandBucket(task.CreatedAt, now); ok {
			recent++
			buckets[bucket]++
		}
	}
	return queued, running, recent, smoothedDemand(recent, buckets[:])
}

func decoratePolicyEffectiveCapacity(db *Database, now time.Time) {
	decoratePolicyEffectiveCapacityWithCooldown(db, now, defaultAutoBalanceScaleDownCooldown)
}

func decoratePolicyEffectiveCapacityWithCooldown(db *Database, now time.Time, cooldown time.Duration) {
	for id, policy := range db.Policies {
		capacity := effectivePolicyCapacity(*db, policy, now, cooldown)
		policy.EffectiveCachedCount = capacity.Cached
		policy.EffectiveWarmCount = capacity.Warm
		policy.DemandQueued = capacity.DemandQueued
		policy.DemandRunning = capacity.DemandRunning
		policy.DemandRecent = capacity.DemandRecent
		policy.DemandSmoothed = capacity.DemandSmoothed
		db.Policies[id] = policy
	}
}

func demandBucket(createdAt, now time.Time) (int, bool) {
	if createdAt.IsZero() || createdAt.After(now) {
		return 0, false
	}
	age := now.Sub(createdAt)
	if age >= autoBalanceDemandWindow {
		return 0, false
	}
	bucketSize := autoBalanceDemandWindow / autoBalanceDemandBuckets
	bucket := int(age / bucketSize)
	if bucket >= autoBalanceDemandBuckets {
		return 0, false
	}
	return bucket, true
}

func smoothedDemand(recent int, buckets []int) int {
	if recent == 0 {
		return 0
	}
	activeBuckets := 0
	for _, count := range buckets {
		if count > 0 {
			activeBuckets++
		}
	}
	activeBuckets = max(1, activeBuckets)
	return ceilDiv(recent*activeBuckets, 4*autoBalanceDemandBuckets)
}

func desiredCachedCount(policy PlacementPolicy) int {
	if policy.EffectiveCachedCount > 0 || policy.AutoBalance {
		return policy.EffectiveCachedCount
	}
	return max(policy.CachedCount, policy.WarmCount)
}

func desiredWarmCount(policy PlacementPolicy) int {
	if policy.EffectiveWarmCount > 0 || policy.AutoBalance {
		return policy.EffectiveWarmCount
	}
	return policy.WarmCount
}

func profilePlacementSize(db Database, profile WorkloadProfile) int64 {
	size := int64(0)
	for _, variant := range ProfileRuntimeVariants(profile) {
		effective := EffectiveProfileForVariant(profile, variant)
		size = max64(size, profileMemoryBytes(effective)+profileDiskBytes(db, effective))
	}
	return size
}

func (p *placementPlanner) profileScarcity(item policyPlanItem) int {
	nodes := map[string]bool{}
	for _, candidate := range placementCandidates(p.db, item.profile, item.policy) {
		nodes[candidate.node.ID] = true
	}
	if len(nodes) == 0 {
		return 1 << 30
	}
	return len(nodes)
}

func profileMemoryBytes(profile WorkloadProfile) int64 {
	if profile.Requirements == nil {
		return 0
	}
	if profile.Requirements.UnifiedMemoryBytes > 0 {
		return profile.Requirements.UnifiedMemoryBytes
	}
	return profile.Requirements.RAMBytes
}

func profileDiskBytes(db Database, profile WorkloadProfile) int64 {
	if profile.Requirements != nil && profile.Requirements.DiskBytes > 0 {
		return profile.Requirements.DiskBytes
	}
	total := int64(0)
	for _, id := range profile.Artifacts {
		total += db.Artifacts[id].SizeBytes
	}
	return total
}

func nodeMemoryBudget(node Node) int64 {
	if node.Budgets.UnifiedMemoryBytes > 0 {
		return node.Budgets.UnifiedMemoryBytes
	}
	return node.Budgets.RAMBytes
}

func nodePlacementBudget(node Node) int64 {
	return max64(nodeMemoryBudget(node), node.Budgets.DiskBytes)
}

func clampCount(value, minValue, maxValue int) int {
	if value < minValue {
		value = minValue
	}
	if maxValue > 0 && value > maxValue {
		value = maxValue
	}
	return value
}

func ceilDiv(value, divisor int) int {
	if value <= 0 || divisor <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
