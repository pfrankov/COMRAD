package comrad

import (
	"sort"
	"strings"
	"time"
)

const (
	placementPhaseWarm  = "warm"
	placementPhaseCache = "cache"
)

type PlacementExplainResponse struct {
	GeneratedAt time.Time                     `json:"generatedAt"`
	Plan        []PlacementAssignment         `json:"plan"`
	Profiles    []PlacementProfileExplanation `json:"profiles"`
}

type PlacementProfileExplanation struct {
	ProfileID     string                          `json:"profileId"`
	PolicyID      string                          `json:"policyId"`
	LogicalModel  string                          `json:"logicalModel,omitempty"`
	DesiredCached int                             `json:"desiredCached"`
	DesiredWarm   int                             `json:"desiredWarm"`
	Selected      []PlacementCandidateExplanation `json:"selected"`
	Rejected      []PlacementCandidateExplanation `json:"rejected"`
	Missing       []PlacementMissingExplanation   `json:"missing,omitempty"`
}

type PlacementCandidateExplanation struct {
	Phase            string   `json:"phase"`
	NodeID           string   `json:"nodeId,omitempty"`
	SlotID           string   `json:"slotId,omitempty"`
	RuntimeVariantID string   `json:"runtimeVariantId,omitempty"`
	ModelArtifactID  string   `json:"modelArtifactId,omitempty"`
	DesiredCached    bool     `json:"desiredCached"`
	DesiredWarm      bool     `json:"desiredWarm"`
	ActualCached     bool     `json:"actualCached"`
	ActualWarm       bool     `json:"actualWarm"`
	Ready            bool     `json:"ready"`
	Reasons          []string `json:"reasons"`
}

type PlacementMissingExplanation struct {
	Phase         string   `json:"phase"`
	DesiredCached bool     `json:"desiredCached"`
	DesiredWarm   bool     `json:"desiredWarm"`
	Reasons       []string `json:"reasons"`
}

type placementExplainer struct {
	generatedAt time.Time
	profiles    map[string]*PlacementProfileExplanation
	order       []string
	plan        []PlacementAssignment
	seen        map[string]bool
}

func ExplainPlacement(db Database) PlacementExplainResponse {
	now := time.Now().UTC()
	return explainPlacementWithCooldown(db, now, defaultAutoBalanceScaleDownCooldown)
}

func ExplainPlacementWithConfig(db Database, cfg ManagerConfig) PlacementExplainResponse {
	now := time.Now().UTC()
	return explainPlacementWithCooldown(db, now, managerAutoBalanceCooldown(cfg))
}

func explainPlacementWithCooldown(db Database, now time.Time, cooldown time.Duration) PlacementExplainResponse {
	explainer := newPlacementExplainer(now)
	planPlacementWithCooldown(db, now, explainer, cooldown)
	return explainer.response()
}

func newPlacementExplainer(now time.Time) *placementExplainer {
	return &placementExplainer{
		generatedAt: now,
		profiles:    map[string]*PlacementProfileExplanation{},
		seen:        map[string]bool{},
	}
}

func (e *placementExplainer) addPolicyItem(item policyPlanItem) {
	profile := e.profile(item.profile.ID)
	profile.PolicyID = item.policy.ID
	profile.LogicalModel = ProfileLogicalModel(item.profile)
	profile.DesiredCached = item.capacity.Cached
	profile.DesiredWarm = item.capacity.Warm
}

func (e *placementExplainer) setPlan(plan []PlacementAssignment) {
	e.plan = append([]PlacementAssignment(nil), plan...)
}

func (e *placementExplainer) response() PlacementExplainResponse {
	out := PlacementExplainResponse{GeneratedAt: e.generatedAt, Plan: e.plan}
	for _, id := range e.order {
		out.Profiles = append(out.Profiles, *e.profiles[id])
	}
	return out
}

func (e *placementExplainer) profile(profileID string) *PlacementProfileExplanation {
	if profile := e.profiles[profileID]; profile != nil {
		return profile
	}
	profile := &PlacementProfileExplanation{ProfileID: profileID}
	e.profiles[profileID] = profile
	e.order = append(e.order, profileID)
	return profile
}

func (e *placementExplainer) addSelected(profileID, phase string, c placementCandidate, warm bool, reasons []string) {
	profile := e.profile(profileID)
	item := placementCandidateExplanation(phase, c, warm, uniqueSorted(reasons))
	if e.markSeen(profileID, "selected", item) {
		profile.Selected = append(profile.Selected, item)
	}
}

func (e *placementExplainer) addRejected(profileID, phase string, c placementCandidate, warm bool, reasons []string) {
	profile := e.profile(profileID)
	item := placementCandidateExplanation(phase, c, warm, uniqueSorted(reasons))
	if e.markSeen(profileID, "rejected", item) {
		profile.Rejected = append(profile.Rejected, item)
	}
}

func (e *placementExplainer) addMissing(profileID, phase string, warm bool, reasons []string) {
	profile := e.profile(profileID)
	profile.Missing = append(profile.Missing, PlacementMissingExplanation{
		Phase:         phase,
		DesiredCached: true,
		DesiredWarm:   warm,
		Reasons:       uniqueSorted(reasons),
	})
}

func (e *placementExplainer) markSeen(profileID, decision string, item PlacementCandidateExplanation) bool {
	key := strings.Join([]string{profileID, decision, item.Phase, item.NodeID, item.SlotID, item.RuntimeVariantID, strings.Join(item.Reasons, ",")}, "|")
	if e.seen[key] {
		return false
	}
	e.seen[key] = true
	return true
}

func placementCandidateExplanation(phase string, c placementCandidate, warm bool, reasons []string) PlacementCandidateExplanation {
	return PlacementCandidateExplanation{
		Phase:            phase,
		NodeID:           c.node.ID,
		SlotID:           c.slot.ID,
		RuntimeVariantID: c.effective.RuntimeVariantID,
		ModelArtifactID:  ConcreteModelArtifactID(c.effective),
		DesiredCached:    true,
		DesiredWarm:      warm,
		ActualCached:     nodeHasArtifacts(c.node, c.effective.Artifacts),
		ActualWarm:       slotProfileCurrent(c.slot, c.profile.ID, c.effective) && (c.slot.State == SlotStateReady || c.slot.State == SlotStateServing),
		Ready:            slotProfileCurrent(c.slot, c.profile.ID, c.effective) && c.slot.State == SlotStateReady && c.slot.ActiveTaskID == "" && !c.slot.Quarantined,
		Reasons:          reasons,
	}
}

func (p *placementPlanner) nextWarmCandidateExplain(item policyPlanItem) (placementCandidate, bool) {
	var selected placementCandidate
	found := false
	for _, candidate := range explainPlacementCandidates(p.db, item.profile, item.policy) {
		if reasons := p.warmCandidateRejectionReasons(item, candidate); len(reasons) > 0 {
			p.explain.addRejected(item.profile.ID, placementPhaseWarm, candidate, true, reasons)
			continue
		}
		if found {
			continue
		}
		p.explain.addSelected(item.profile.ID, placementPhaseWarm, candidate, true, []string{"selected_for_warm_copy"})
		selected = candidate
		found = true
	}
	return selected, found
}

func (p *placementPlanner) nextCacheCandidateExplain(item policyPlanItem) (placementCandidate, bool) {
	seen := map[string]bool{}
	var selected placementCandidate
	found := false
	for _, candidate := range explainPlacementCandidates(p.db, item.profile, item.policy) {
		if base := candidateBaseRejectionReasons(item, candidate); len(base) > 0 {
			p.explain.addRejected(item.profile.ID, placementPhaseCache, candidate, false, base)
			continue
		}
		if seen[candidate.node.ID] || p.nodeHasProfileCache(candidate.node.ID, item.profile.ID) {
			continue
		}
		seen[candidate.node.ID] = true
		if reasons := p.cacheCandidateRejectionReasons(item, candidate); len(reasons) > 0 {
			p.explain.addRejected(item.profile.ID, placementPhaseCache, candidate, false, reasons)
			continue
		}
		if found {
			continue
		}
		p.explain.addSelected(item.profile.ID, placementPhaseCache, candidate, false, []string{"selected_for_cached_copy"})
		selected = candidate
		found = true
	}
	return selected, found
}

func explainPlacementCandidates(db Database, profile WorkloadProfile, policy PlacementPolicy) []placementCandidate {
	out := []placementCandidate{}
	for _, slot := range SortedSlots(db) {
		node, ok := db.Nodes[slot.NodeID]
		for _, variant := range ProfileRuntimeVariants(profile) {
			effective := EffectiveProfileForVariant(profile, variant)
			fit := explainFitResult(effective, node, slot, ok)
			out = append(out, placementCandidate{slot: slot, node: node, profile: profile, effective: effective, fit: fit})
		}
	}
	sortExplainPlacementCandidates(out, profile.ID, policy)
	return out
}

func explainFitResult(profile WorkloadProfile, node Node, slot Slot, nodeOK bool) FitResult {
	if !nodeOK {
		return FitResult{ProfileID: profile.ID, LogicalModel: ProfileLogicalModel(profile), SlotID: slot.ID, NodeID: slot.NodeID, Fits: false, Reasons: []string{"node_missing"}}
	}
	return FitProfileToSlot(profile, node, slot)
}

func sortExplainPlacementCandidates(out []placementCandidate, profileID string, policy PlacementPolicy) {
	sort.SliceStable(out, func(i, j int) bool {
		scoreI := placementScore(profileID, policy, out[i])
		scoreJ := placementScore(profileID, policy, out[j])
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		if out[i].slot.ID != out[j].slot.ID {
			return out[i].slot.ID < out[j].slot.ID
		}
		return out[i].effective.RuntimeVariantID < out[j].effective.RuntimeVariantID
	})
}

func (p *placementPlanner) hardPinRejectionReasons(item policyPlanItem, candidate placementCandidate) []string {
	reasons := candidateBaseRejectionReasons(item, candidate)
	reasons = append(reasons, p.nodeBudgetRejectionReasons(item, candidate, true)...)
	return uniqueSorted(reasons)
}

func (p *placementPlanner) warmCandidateRejectionReasons(item policyPlanItem, candidate placementCandidate) []string {
	reasons := candidateBaseRejectionReasons(item, candidate)
	if p.slotUsed[candidate.slot.ID] {
		reasons = append(reasons, "slot_already_reserved")
	}
	if !slotAvailableForWarm(candidate.slot, item.profile, candidate.effective) {
		reasons = append(reasons, warmSlotUnavailableReasons(candidate.slot, item.profile, candidate.effective)...)
	}
	reasons = append(reasons, p.nodeBudgetRejectionReasons(item, candidate, true)...)
	return uniqueSorted(reasons)
}

func (p *placementPlanner) cacheCandidateRejectionReasons(item policyPlanItem, candidate placementCandidate) []string {
	reasons := candidateBaseRejectionReasons(item, candidate)
	reasons = append(reasons, p.nodeBudgetRejectionReasons(item, candidate, false)...)
	return uniqueSorted(reasons)
}

func candidateBaseRejectionReasons(item policyPlanItem, candidate placementCandidate) []string {
	reasons := nodePlacementRejectionReasons(candidate.node, item.policy)
	if !candidate.fit.Fits {
		reasons = append(reasons, candidate.fit.Reasons...)
	}
	return uniqueSorted(reasons)
}

func nodePlacementRejectionReasons(node Node, policy PlacementPolicy) []string {
	if node.ID == "" {
		return []string{"node_missing"}
	}
	reasons := []string{}
	if node.State == NodeStateDisabled || node.Mode == "disabled" {
		reasons = append(reasons, "node_disabled")
	} else if node.State == NodeStateOffline {
		reasons = append(reasons, "node_offline")
	} else if node.State != NodeStateOnline {
		reasons = append(reasons, "node_not_online")
	}
	if !node.Approved {
		reasons = append(reasons, "node_not_approved")
	}
	if node.Quarantined {
		reasons = append(reasons, FailureQuarantined)
	}
	if Contains(policy.Constraints.DenyNodes, node.ID) || Contains(policy.Constraints.DenyNodes, node.Name) {
		reasons = append(reasons, "node_denied")
	}
	if !HasAll(node.Tags, policy.Constraints.RequireTags) {
		reasons = append(reasons, "missing_required_tags")
	}
	return reasons
}

func warmSlotUnavailableReasons(slot Slot, profile WorkloadProfile, effective WorkloadProfile) []string {
	reasons := []string{}
	if slot.ActiveTaskID != "" {
		reasons = append(reasons, "slot_active_task")
	}
	if slot.State == SlotStateServing && !slotProfileCurrent(slot, profile.ID, effective) {
		reasons = append(reasons, "slot_serving_other_profile")
	}
	if slot.State == SlotStateUnavailable {
		reasons = append(reasons, "slot_unavailable")
	}
	if slot.State == SlotStateError {
		reasons = append(reasons, "slot_error")
	}
	if slot.MismatchReason != "" {
		reasons = append(reasons, slot.MismatchReason)
	}
	return reasons
}

func (p *placementPlanner) nodeBudgetRejectionReasons(item policyPlanItem, candidate placementCandidate, warm bool) []string {
	reasons := []string{}
	if warm && nodeWarmPlacementSuppressed(candidate.node, p.now) {
		reasons = append(reasons, FailureWorkerFlapping)
	}
	if reason := p.profileLimitRejectionReason(item, candidate.node.ID, warm); reason != "" {
		reasons = append(reasons, reason)
	}
	if !p.withinDiskBudget(candidate.node, candidate.effective) {
		reasons = append(reasons, FailureResourceExhaustedDisk)
	}
	if warm && !p.withinMemoryBudget(candidate.node, candidate.effective) {
		reasons = append(reasons, placementMemoryReason(candidate.effective))
	}
	return reasons
}

func (p *placementPlanner) profileLimitRejectionReason(item policyPlanItem, nodeID string, warm bool) string {
	if warm && item.policy.MaxWarmProfilesPerNode > 0 && !p.nodeHasProfileWarm(nodeID, item.profile.ID) {
		if len(p.nodeWarm[nodeID]) >= item.policy.MaxWarmProfilesPerNode {
			return "per_node_warm_profile_limit"
		}
	}
	if !p.nodeHasProfileCache(nodeID, item.profile.ID) && item.policy.MaxCachedProfilesPerNode > 0 {
		if len(p.nodeCache[nodeID]) >= item.policy.MaxCachedProfilesPerNode {
			return "per_node_cached_profile_limit"
		}
	}
	return ""
}

func placementMemoryReason(profile WorkloadProfile) string {
	if profile.Requirements != nil && profile.Requirements.UnifiedMemoryBytes > 0 {
		return "resource_exhausted_unified_memory"
	}
	return FailureResourceExhaustedRAM
}

func placementPhase(warm bool) string {
	if warm {
		return placementPhaseWarm
	}
	return placementPhaseCache
}
