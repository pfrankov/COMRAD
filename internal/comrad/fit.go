package comrad

import (
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

type placementCandidate struct {
	slot      Slot
	node      Node
	profile   WorkloadProfile
	effective WorkloadProfile
	fit       FitResult
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
