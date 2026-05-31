package comrad

import (
	"fmt"
	"sort"
	"time"
)

const autoBalanceDemandWindow = 10 * time.Minute

type policyCapacity struct {
	Cached        int
	Warm          int
	MinCached     int
	MinWarm       int
	DemandQueued  int
	DemandRunning int
	DemandRecent  int
}

type policyPlanItem struct {
	profile  WorkloadProfile
	policy   PlacementPolicy
	capacity policyCapacity
	scarcity int
	size     int64
	demand   int
}

type placementPlanner struct {
	db             Database
	now            time.Time
	assignments    []PlacementAssignment
	warmByProfile  map[string]int
	cacheByProfile map[string]int
	slotUsed       map[string]bool
	nodeMemoryUsed map[string]int64
	nodeDiskUsed   map[string]int64
	nodeWarm       map[string]map[string]bool
	nodeCache      map[string]map[string]bool
	missing        map[string]int
}

func PlanPlacement(db Database) []PlacementAssignment {
	now := time.Now().UTC()
	planner := newPlacementPlanner(db, now)
	items := planner.policyItems()
	planner.placeHardPins(items)
	sortPolicyItems(items, false)
	for _, item := range items {
		planner.placeWarmUntil(item, item.capacity.MinWarm)
	}
	for _, item := range items {
		planner.placeCacheUntil(item, item.capacity.MinCached)
	}
	sortPolicyItems(items, true)
	for _, item := range items {
		planner.placeWarmUntil(item, item.capacity.Warm)
	}
	for _, item := range items {
		planner.placeCacheUntil(item, item.capacity.Cached)
	}
	sortAssignments(planner.assignments)
	return planner.assignments
}

func newPlacementPlanner(db Database, now time.Time) *placementPlanner {
	return &placementPlanner{
		db:             db,
		now:            now,
		warmByProfile:  map[string]int{},
		cacheByProfile: map[string]int{},
		slotUsed:       map[string]bool{},
		nodeMemoryUsed: map[string]int64{},
		nodeDiskUsed:   map[string]int64{},
		nodeWarm:       map[string]map[string]bool{},
		nodeCache:      map[string]map[string]bool{},
		missing:        map[string]int{},
	}
}

func (p *placementPlanner) policyItems() []policyPlanItem {
	items := []policyPlanItem{}
	for _, policy := range SortedPolicies(p.db) {
		profile, ok := p.db.Profiles[policy.ProfileID]
		if !ok {
			continue
		}
		capacity := EffectivePolicyCapacity(p.db, policy, p.now)
		item := policyPlanItem{profile: profile, policy: policy, capacity: capacity}
		item.scarcity = p.profileScarcity(item)
		item.size = profilePlacementSize(p.db, profile)
		item.demand = capacity.DemandQueued + capacity.DemandRunning + ceilDiv(capacity.DemandRecent, 4)
		items = append(items, item)
	}
	return items
}

func sortPolicyItems(items []policyPlanItem, extras bool) {
	sort.SliceStable(items, func(i, j int) bool {
		if extras && items[i].demand != items[j].demand {
			return items[i].demand > items[j].demand
		}
		if items[i].scarcity != items[j].scarcity {
			return items[i].scarcity < items[j].scarcity
		}
		if items[i].size != items[j].size {
			return items[i].size > items[j].size
		}
		return items[i].profile.ID < items[j].profile.ID
	})
}

func (p *placementPlanner) placeHardPins(items []policyPlanItem) {
	for _, item := range items {
		for _, pin := range item.policy.HardPinnedSlots {
			p.placeHardPin(item, pin)
		}
	}
}

func (p *placementPlanner) placeHardPin(item policyPlanItem, pin string) {
	slot, ok := p.db.Slots[pin]
	if !ok {
		p.addAssignment(missingPlacementAssignment(item.profile.ID, p.nextMissing(item.profile.ID), true, p.now))
		return
	}
	node := p.db.Nodes[slot.NodeID]
	effective, fit := BestVariantForSlot(item.profile, node, slot)
	a := assignmentFromFit(item.profile.ID, effective, slot, fit, p.now)
	a.DesiredCached = true
	a.DesiredWarm = true
	if !fit.Fits || !p.canUseCandidate(item, placementCandidate{node: node, slot: slot, effective: effective, fit: fit}, true) {
		a.MismatchReason = firstNonEmpty(a.MismatchReason, "resource_exhausted_node_budget")
		p.addAssignment(a)
		return
	}
	p.addAssignment(a)
	p.markWarm(item, placementCandidate{node: node, slot: slot, effective: effective})
}

func (p *placementPlanner) placeWarmUntil(item policyPlanItem, target int) {
	for p.warmByProfile[item.profile.ID] < target {
		candidate, ok := p.nextWarmCandidate(item)
		if !ok {
			p.addMissing(item.profile.ID, true)
			continue
		}
		a := assignmentFromFit(item.profile.ID, candidate.effective, candidate.slot, candidate.fit, p.now)
		a.DesiredCached = true
		a.DesiredWarm = true
		p.addAssignment(a)
		p.markWarm(item, candidate)
	}
}

func (p *placementPlanner) placeCacheUntil(item policyPlanItem, target int) {
	for p.cacheByProfile[item.profile.ID] < target {
		candidate, ok := p.nextCacheCandidate(item)
		if !ok {
			p.addMissing(item.profile.ID, false)
			continue
		}
		a := cacheAssignmentFromCandidate(item.profile.ID, candidate, p.now)
		a.DesiredCached = true
		p.addAssignment(a)
		p.markCache(item, candidate)
	}
}

func (p *placementPlanner) nextWarmCandidate(item policyPlanItem) (placementCandidate, bool) {
	for _, candidate := range placementCandidates(p.db, item.profile, item.policy) {
		if p.slotUsed[candidate.slot.ID] || !slotAvailableForWarm(candidate.slot, item.profile, candidate.effective) {
			continue
		}
		if p.canUseCandidate(item, candidate, true) {
			return candidate, true
		}
	}
	return placementCandidate{}, false
}

func (p *placementPlanner) nextCacheCandidate(item policyPlanItem) (placementCandidate, bool) {
	seen := map[string]bool{}
	for _, candidate := range placementCandidates(p.db, item.profile, item.policy) {
		if seen[candidate.node.ID] || p.nodeHasProfileCache(candidate.node.ID, item.profile.ID) {
			continue
		}
		seen[candidate.node.ID] = true
		if p.canUseCandidate(item, candidate, false) {
			return candidate, true
		}
	}
	return placementCandidate{}, false
}

func placementCandidates(db Database, profile WorkloadProfile, policy PlacementPolicy) []placementCandidate {
	out := []placementCandidate{}
	for _, slot := range SortedSlots(db) {
		node, ok := db.Nodes[slot.NodeID]
		if !ok || !nodeUsableForPlacement(node, policy) {
			continue
		}
		for _, variant := range ProfileRuntimeVariants(profile) {
			effective := EffectiveProfileForVariant(profile, variant)
			fit := FitProfileToSlot(effective, node, slot)
			if fit.Fits {
				out = append(out, placementCandidate{slot: slot, node: node, profile: profile, effective: effective, fit: fit})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		scoreI := placementScore(profile.ID, policy, out[i])
		scoreJ := placementScore(profile.ID, policy, out[j])
		if scoreI == scoreJ {
			return out[i].slot.ID < out[j].slot.ID
		}
		return scoreI > scoreJ
	})
	return out
}

func nodeUsableForPlacement(node Node, policy PlacementPolicy) bool {
	if node.State != NodeStateOnline || !node.Approved || node.Quarantined {
		return false
	}
	if Contains(policy.Constraints.DenyNodes, node.ID) || Contains(policy.Constraints.DenyNodes, node.Name) {
		return false
	}
	return HasAll(node.Tags, policy.Constraints.RequireTags)
}

func placementScore(profileID string, policy PlacementPolicy, c placementCandidate) int64 {
	score := int64(0)
	if Contains(policy.Constraints.PreferNodes, c.node.ID) || Contains(policy.Constraints.PreferNodes, c.node.Name) {
		score += 1_000_000
	}
	if slotProfileCurrent(c.slot, profileID, c.effective) {
		score += 100_000
	}
	score += nodePlacementBudget(c.node) / (1 << 30)
	score -= int64(c.slot.FailureCount * 100)
	return score
}

func (p *placementPlanner) canUseCandidate(item policyPlanItem, candidate placementCandidate, warm bool) bool {
	if !p.withinProfileLimit(item, candidate.node.ID, warm) {
		return false
	}
	if !p.withinDiskBudget(candidate.node, candidate.effective) {
		return false
	}
	return !warm || p.withinMemoryBudget(candidate.node, candidate.effective)
}

func (p *placementPlanner) withinProfileLimit(item policyPlanItem, nodeID string, warm bool) bool {
	if warm && item.policy.MaxWarmProfilesPerNode > 0 && !p.nodeHasProfileWarm(nodeID, item.profile.ID) {
		if len(p.nodeWarm[nodeID]) >= item.policy.MaxWarmProfilesPerNode {
			return false
		}
	}
	if !p.nodeHasProfileCache(nodeID, item.profile.ID) && item.policy.MaxCachedProfilesPerNode > 0 {
		return len(p.nodeCache[nodeID]) < item.policy.MaxCachedProfilesPerNode
	}
	return true
}

func (p *placementPlanner) withinDiskBudget(node Node, profile WorkloadProfile) bool {
	budget := node.Budgets.DiskBytes
	if budget <= 0 {
		return true
	}
	return p.nodeDiskUsed[node.ID]+profileDiskBytes(p.db, profile) <= budget
}

func (p *placementPlanner) withinMemoryBudget(node Node, profile WorkloadProfile) bool {
	budget := nodeMemoryBudget(node)
	need := profileMemoryBytes(profile)
	if budget <= 0 || need <= 0 {
		return true
	}
	return p.nodeMemoryUsed[node.ID]+need <= budget
}

func (p *placementPlanner) markWarm(item policyPlanItem, candidate placementCandidate) {
	p.slotUsed[candidate.slot.ID] = true
	p.nodeMemoryUsed[candidate.node.ID] += profileMemoryBytes(candidate.effective)
	if p.nodeWarm[candidate.node.ID] == nil {
		p.nodeWarm[candidate.node.ID] = map[string]bool{}
	}
	p.nodeWarm[candidate.node.ID][item.profile.ID] = true
	p.warmByProfile[item.profile.ID]++
	p.markCache(item, candidate)
}

func (p *placementPlanner) markCache(item policyPlanItem, candidate placementCandidate) {
	if p.nodeHasProfileCache(candidate.node.ID, item.profile.ID) {
		return
	}
	p.nodeDiskUsed[candidate.node.ID] += profileDiskBytes(p.db, candidate.effective)
	if p.nodeCache[candidate.node.ID] == nil {
		p.nodeCache[candidate.node.ID] = map[string]bool{}
	}
	p.nodeCache[candidate.node.ID][item.profile.ID] = true
	p.cacheByProfile[item.profile.ID]++
}

func (p *placementPlanner) nodeHasProfileWarm(nodeID, profileID string) bool {
	return p.nodeWarm[nodeID] != nil && p.nodeWarm[nodeID][profileID]
}

func (p *placementPlanner) nodeHasProfileCache(nodeID, profileID string) bool {
	return p.nodeCache[nodeID] != nil && p.nodeCache[nodeID][profileID]
}

func (p *placementPlanner) addMissing(profileID string, warm bool) {
	a := missingPlacementAssignment(profileID, p.nextMissing(profileID), warm, p.now)
	p.addAssignment(a)
	p.cacheByProfile[profileID]++
	if warm {
		p.warmByProfile[profileID]++
	}
}

func (p *placementPlanner) nextMissing(profileID string) int {
	n := p.missing[profileID]
	p.missing[profileID] = n + 1
	return n
}

func (p *placementPlanner) addAssignment(a PlacementAssignment) {
	p.assignments = append(p.assignments, a)
}

func cacheAssignmentFromCandidate(profileID string, c placementCandidate, now time.Time) PlacementAssignment {
	current := nodeHasArtifacts(c.node, c.effective.Artifacts)
	return PlacementAssignment{
		ID:               assignmentID(profileID, c.node.ID, "cache"),
		ProfileID:        profileID,
		LogicalModel:     ProfileLogicalModel(c.effective),
		RuntimeVariantID: c.effective.RuntimeVariantID,
		ModelArtifactID:  ConcreteModelArtifactID(c.effective),
		ModelSHA256:      ConcreteModelSHA256(c.effective),
		NodeID:           c.node.ID,
		DesiredCached:    true,
		ActualCached:     current,
		UpdatedAt:        now,
	}
}

func missingPlacementAssignment(profileID string, selected int, warm bool, now time.Time) PlacementAssignment {
	return PlacementAssignment{ID: assignmentID(profileID, "", fmt.Sprintf("missing-%d", selected)), ProfileID: profileID, DesiredCached: true, DesiredWarm: warm, MismatchReason: "insufficient_compatible_slots", UpdatedAt: now}
}

func slotAvailableForWarm(slot Slot, profile WorkloadProfile, effective WorkloadProfile) bool {
	if slot.ActiveTaskID != "" {
		return false
	}
	if slot.State == SlotStateServing {
		return slotProfileCurrent(slot, profile.ID, effective)
	}
	return slot.State != SlotStateUnavailable && slot.State != SlotStateError
}
