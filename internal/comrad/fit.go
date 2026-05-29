package comrad

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func FitProfileToSlot(profile WorkloadProfile, node Node, slot Slot) FitResult {
	if profile.RuntimeVariantID == "" && len(profile.RuntimeVariants) > 0 {
		effective, fit := BestVariantForSlot(profile, node, slot)
		if fit.Fits {
			return FitProfileToSlot(effective, node, slot)
		}
		return fit
	}
	result := newFitResult(profile, node, slot)
	if profile.Requirements == nil {
		markFit(&result, FailureUnknownRequirements)
		return result
	}
	req := profile.Requirements
	checkNodeAdmission(&result, node, slot, req)
	checkRuntimeAdapter(&result, profile, node, slot, req)
	checkResources(&result, slot.Resources, req)
	checkLLMRequirements(&result, profile)
	if len(result.Reasons) > 0 {
		sort.Strings(result.Reasons)
	}
	return result
}

func newFitResult(profile WorkloadProfile, node Node, slot Slot) FitResult {
	return FitResult{ProfileID: profile.ID, LogicalModel: ProfileLogicalModel(profile), RuntimeVariantID: profile.RuntimeVariantID, SlotID: slot.ID, NodeID: node.ID, Fits: true}
}

func markFit(result *FitResult, reason string) {
	result.Fits = false
	result.Reasons = append(result.Reasons, reason)
}

func checkNodeAdmission(result *FitResult, node Node, slot Slot, req *Requirements) {
	if node.State == NodeStateDisabled || node.Mode == "disabled" {
		markFit(result, "node_disabled")
	}
	if node.Quarantined {
		markFit(result, FailureQuarantined)
	}
	if slot.Quarantined {
		markFit(result, FailureQuarantined)
	}
	if !node.Approved {
		markFit(result, "node_not_approved")
	}
	if req.Target != "" && slot.Target != req.Target {
		markFit(result, FailureTargetUnsupported)
	}
	if !HasAll(node.Tags, req.RequireTags) {
		markFit(result, "missing_required_tags")
	}
}

func checkRuntimeAdapter(result *FitResult, profile WorkloadProfile, node Node, slot Slot, req *Requirements) {
	adapter := req.RuntimeAdapter
	if adapter == "" {
		adapter = profile.RuntimeAdapter
	}
	if adapter == "" {
		return
	}
	if slot.RuntimeAdapter != "" && slot.RuntimeAdapter != adapter {
		markFit(result, FailureRuntimeAdapterMissing)
		return
	}
	if !Contains(node.RuntimeAdapters, adapter) {
		markFit(result, FailureRuntimeAdapterMissing)
	}
}

func checkResources(result *FitResult, resources ResourceBudget, req *Requirements) {
	if req.RAMBytes > 0 && resources.RAMBytes > 0 && resources.RAMBytes < req.RAMBytes {
		markFit(result, FailureResourceExhaustedRAM)
	}
	if req.VRAMBytes > 0 && resources.VRAMBytes > 0 && resources.VRAMBytes < req.VRAMBytes {
		markFit(result, FailureResourceExhaustedVRAM)
	}
	if req.UnifiedMemoryBytes > 0 && resources.UnifiedMemoryBytes > 0 && resources.UnifiedMemoryBytes < req.UnifiedMemoryBytes {
		markFit(result, "resource_exhausted_unified_memory")
	}
	if req.DiskBytes > 0 && resources.DiskBytes > 0 && resources.DiskBytes < req.DiskBytes {
		markFit(result, FailureResourceExhaustedDisk)
	}
}

func checkLLMRequirements(result *FitResult, profile WorkloadProfile) {
	if profile.Kind != "llm.chat" {
		return
	}
	if profile.LLM == nil {
		markFit(result, "missing_llm_section")
		return
	}
	if ConcreteModelArtifactID(profile) == "" {
		markFit(result, "missing_model_artifact")
	}
}

func isLlamaCppAdapter(adapter string) bool {
	return strings.Contains(strings.ToLower(adapter), "llama.cpp")
}

func BuildFitMatrix(db Database) []FitResult {
	profiles := SortedProfiles(db)
	slots := SortedSlots(db)
	out := make([]FitResult, 0, len(profiles)*len(slots))
	for _, profile := range profiles {
		variants := ProfileRuntimeVariants(profile)
		for _, slot := range slots {
			node, ok := db.Nodes[slot.NodeID]
			if !ok {
				out = append(out, FitResult{
					ProfileID:    profile.ID,
					LogicalModel: ProfileLogicalModel(profile),
					SlotID:       slot.ID,
					NodeID:       slot.NodeID,
					Fits:         false,
					Reasons:      []string{"node_missing"},
				})
				continue
			}
			for _, variant := range variants {
				out = append(out, FitProfileToSlot(EffectiveProfileForVariant(profile, variant), node, slot))
			}
		}
	}
	return out
}

func PlanPlacement(db Database) []PlacementAssignment {
	now := time.Now().UTC()
	assignments := []PlacementAssignment{}
	occupiedWarmSlots := warmOccupiedSlots(db)
	for _, policy := range SortedPolicies(db) {
		profile, ok := db.Profiles[policy.ProfileID]
		if !ok {
			continue
		}
		assignments = append(assignments, planPolicyPlacement(db, profile, policy, occupiedWarmSlots, now)...)
	}
	sortAssignments(assignments)
	return assignments
}

func warmOccupiedSlots(db Database) map[string]bool {
	out := map[string]bool{}
	for _, existing := range db.Assignments {
		if existing.ActualWarm && existing.SlotID != "" {
			out[existing.SlotID] = true
		}
	}
	return out
}

func planPolicyPlacement(db Database, profile WorkloadProfile, policy PlacementPolicy, occupiedWarmSlots map[string]bool, now time.Time) []PlacementAssignment {
	selected := map[string]PlacementAssignment{}
	assignments := planHardPins(db, profile, policy, selected, occupiedWarmSlots, now)
	desiredCached := desiredCachedCount(policy)
	remainingCached := max(0, desiredCached-len(selected))
	remainingWarm := max(0, policy.WarmCount-len(selected))
	assignments = append(assignments, planCandidatePlacements(db, profile, policy, selected, occupiedWarmSlots, remainingCached, remainingWarm, now)...)
	if desiredCached > 0 && len(selected) < desiredCached {
		assignments = append(assignments, missingPlacementAssignment(profile.ID, len(selected), len(selected) < policy.WarmCount, now))
	}
	return assignments
}

func desiredCachedCount(policy PlacementPolicy) int {
	if policy.CachedCount < policy.WarmCount {
		return policy.WarmCount
	}
	return policy.CachedCount
}

func planHardPins(db Database, profile WorkloadProfile, policy PlacementPolicy, selected map[string]PlacementAssignment, occupiedWarmSlots map[string]bool, now time.Time) []PlacementAssignment {
	assignments := []PlacementAssignment{}
	for _, pin := range policy.HardPinnedSlots {
		a := hardPinAssignment(db, profile, pin, now)
		assignments = append(assignments, a)
		if a.SlotID != "" && a.MismatchReason != "hard_pin_unavailable" {
			selected[a.SlotID] = a
			occupiedWarmSlots[a.SlotID] = true
		}
	}
	return assignments
}

func hardPinAssignment(db Database, profile WorkloadProfile, pin string, now time.Time) PlacementAssignment {
	slot, ok := db.Slots[pin]
	if !ok {
		return PlacementAssignment{ID: assignmentID(profile.ID, "", pin), ProfileID: profile.ID, SlotID: pin, DesiredCached: true, DesiredWarm: true, MismatchReason: "hard_pin_unavailable", UpdatedAt: now}
	}
	node := db.Nodes[slot.NodeID]
	effective, fit := BestVariantForSlot(profile, node, slot)
	a := assignmentFromFit(profile.ID, effective, slot, fit, now)
	a.DesiredCached = true
	a.DesiredWarm = true
	if !fit.Fits {
		a.MismatchReason = strings.Join(fit.Reasons, ",")
	}
	return a
}

func planCandidatePlacements(db Database, profile WorkloadProfile, policy PlacementPolicy, selected map[string]PlacementAssignment, occupiedWarmSlots map[string]bool, remainingCached, remainingWarm int, now time.Time) []PlacementAssignment {
	assignments := []PlacementAssignment{}
	for _, c := range placementCandidates(db, profile, policy) {
		if remainingCached <= 0 {
			break
		}
		if _, ok := selected[c.slot.ID]; ok {
			continue
		}
		a, warmed := candidateAssignment(db, profile, c, occupiedWarmSlots, remainingWarm, now)
		if warmed {
			remainingWarm--
		}
		assignments = append(assignments, a)
		selected[c.slot.ID] = a
		remainingCached--
	}
	return assignments
}

func candidateAssignment(db Database, profile WorkloadProfile, c placementCandidate, occupiedWarmSlots map[string]bool, remainingWarm int, now time.Time) (PlacementAssignment, bool) {
	a := assignmentFromFit(profile.ID, c.effective, c.slot, c.fit, now)
	a.DesiredCached = true
	if remainingWarm <= 0 || !profile.Warmable {
		return a, false
	}
	if occupiedWarmSlots[c.slot.ID] && db.Slots[c.slot.ID].ProfileID != profile.ID {
		return a, false
	}
	a.DesiredWarm = true
	occupiedWarmSlots[c.slot.ID] = true
	return a, true
}

func missingPlacementAssignment(profileID string, selected int, warm bool, now time.Time) PlacementAssignment {
	return PlacementAssignment{ID: assignmentID(profileID, "", fmt.Sprintf("missing-%d", selected)), ProfileID: profileID, DesiredCached: true, DesiredWarm: warm, MismatchReason: "insufficient_compatible_slots", UpdatedAt: now}
}

type placementCandidate struct {
	slot      Slot
	node      Node
	profile   WorkloadProfile
	effective WorkloadProfile
	fit       FitResult
}

func placementCandidates(db Database, profile WorkloadProfile, policy PlacementPolicy) []placementCandidate {
	out := []placementCandidate{}
	for _, slot := range SortedSlots(db) {
		node, ok := db.Nodes[slot.NodeID]
		if !ok {
			continue
		}
		if Contains(policy.Constraints.DenyNodes, node.ID) || Contains(policy.Constraints.DenyNodes, node.Name) {
			continue
		}
		if !HasAll(node.Tags, policy.Constraints.RequireTags) {
			continue
		}
		for _, variant := range ProfileRuntimeVariants(profile) {
			effective := EffectiveProfileForVariant(profile, variant)
			fit := FitProfileToSlot(effective, node, slot)
			if !fit.Fits {
				continue
			}
			out = append(out, placementCandidate{slot: slot, node: node, profile: profile, effective: effective, fit: fit})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		scoreI := placementScore(db, profile.ID, policy, out[i])
		scoreJ := placementScore(db, profile.ID, policy, out[j])
		if scoreI == scoreJ {
			return out[i].slot.ID < out[j].slot.ID
		}
		return scoreI > scoreJ
	})
	return out
}

func placementScore(db Database, profileID string, policy PlacementPolicy, c placementCandidate) int {
	score := 0
	if Contains(policy.Constraints.PreferNodes, c.node.ID) || Contains(policy.Constraints.PreferNodes, c.node.Name) {
		score += 100
	}
	if c.slot.ProfileID == profileID {
		score += 30
	}
	if c.slot.State == SlotStateReady || c.slot.State == SlotStateCached {
		score += 20
	}
	score -= c.slot.FailureCount * 5
	if !c.slot.LastTaskAt.IsZero() {
		score -= int(time.Since(c.slot.LastTaskAt).Seconds() / 60)
	}
	return score
}

func assignmentFromFit(profileID string, profile WorkloadProfile, slot Slot, fit FitResult, now time.Time) PlacementAssignment {
	current := slotProfileCurrent(slot, profileID, profile)
	a := PlacementAssignment{
		ID:               assignmentID(profileID, slot.NodeID, slot.ID),
		ProfileID:        profileID,
		LogicalModel:     ProfileLogicalModel(profile),
		RuntimeVariantID: profile.RuntimeVariantID,
		ModelArtifactID:  ConcreteModelArtifactID(profile),
		ModelSHA256:      ConcreteModelSHA256(profile),
		NodeID:           slot.NodeID,
		SlotID:           slot.ID,
		ActualCached:     current && (slot.State == SlotStateCached || slot.State == SlotStateWarming || slot.State == SlotStateReady || slot.State == SlotStateServing),
		ActualWarm:       current && (slot.State == SlotStateReady || slot.State == SlotStateServing),
		Ready:            current && slot.State == SlotStateReady && slot.ActiveTaskID == "" && !slot.Quarantined,
		MismatchReason:   "",
		UpdatedAt:        now,
	}
	if !fit.Fits {
		a.MismatchReason = strings.Join(fit.Reasons, ",")
	}
	if a.MismatchReason == "" && a.DesiredWarm && !a.ActualWarm {
		a.MismatchReason = slot.MismatchReason
	}
	return a
}

func slotProfileCurrent(slot Slot, profileID string, profile WorkloadProfile) bool {
	return slot.ProfileID == profileID &&
		slot.ProfileVersion == profileVersion(profile) &&
		slot.LogicalModel == ProfileLogicalModel(profile) &&
		slot.RuntimeVariantID == profile.RuntimeVariantID &&
		slot.ModelArtifactID == ConcreteModelArtifactID(profile) &&
		slot.ModelSHA256 == ConcreteModelSHA256(profile)
}

func assignmentID(profileID, nodeID, slotID string) string {
	return "asg_" + strings.NewReplacer("/", "_", ":", "_").Replace(profileID+"_"+nodeID+"_"+slotID)
}
