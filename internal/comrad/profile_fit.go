package comrad

import (
	"fmt"
	"sort"
)

func ResolveLLMProfile(db Database, model string, minContext int) (WorkloadProfile, error) {
	type candidate struct {
		profile WorkloadProfile
		context int
	}
	var candidates []candidate
	for _, profile := range db.Profiles {
		if profile.Kind != "llm.chat" {
			continue
		}
		matchedProfile := profile.ID == model || profile.Name == model || profile.Alias == model || ProfileLogicalModel(profile) == model
		if matchedProfile {
			ctx := minimumSufficientContext(profile, minContext)
			if ctx > 0 {
				candidates = append(candidates, candidate{profile: profile, context: ctx})
			}
			continue
		}
		for _, variant := range ProfileRuntimeVariants(profile) {
			if variant.ID != model && variant.Name != model {
				continue
			}
			effective := profile
			effective.RuntimeVariants = []RuntimeModelVariant{variant}
			ctx := minimumSufficientContext(effective, minContext)
			if ctx > 0 {
				candidates = append(candidates, candidate{profile: effective, context: ctx})
			}
		}
	}
	if len(candidates) == 0 {
		return WorkloadProfile{}, fmt.Errorf("model_not_found")
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].context < candidates[j].context
	})
	for _, c := range candidates {
		if c.context >= minContext {
			return c.profile, nil
		}
	}
	return WorkloadProfile{}, fmt.Errorf("insufficient_context")
}

func ProfileLogicalModel(profile WorkloadProfile) string {
	if profile.LogicalModel != "" {
		return profile.LogicalModel
	}
	if profile.Alias != "" {
		return profile.Alias
	}
	if profile.Name != "" {
		return profile.Name
	}
	return profile.ID
}

func ProfileRuntimeVariants(profile WorkloadProfile) []RuntimeModelVariant {
	if len(profile.RuntimeVariants) > 0 {
		out := make([]RuntimeModelVariant, 0, len(profile.RuntimeVariants))
		for _, variant := range profile.RuntimeVariants {
			out = append(out, normalizeVariant(profile, variant))
		}
		return out
	}
	return []RuntimeModelVariant{normalizeVariant(profile, RuntimeModelVariant{
		ID:             profile.RuntimeVariantID,
		Target:         "",
		RuntimeAdapter: profile.RuntimeAdapter,
		Artifacts:      profile.Artifacts,
		Requirements:   profile.Requirements,
		LLM:            profile.LLM,
		Runtime:        profile.Runtime,
	})}
}

func normalizeVariant(profile WorkloadProfile, variant RuntimeModelVariant) RuntimeModelVariant {
	if variant.ID == "" {
		variant.ID = profile.RuntimeVariantID
	}
	if variant.ID == "" {
		variant.ID = profile.ID
	}
	if variant.RuntimeAdapter == "" {
		variant.RuntimeAdapter = profile.RuntimeAdapter
	}
	if len(variant.Artifacts) == 0 {
		variant.Artifacts = profile.Artifacts
	}
	if variant.Requirements == nil {
		variant.Requirements = profile.Requirements
	}
	if variant.LLM == nil {
		variant.LLM = profile.LLM
	}
	if variant.Target == "" && variant.Requirements != nil {
		variant.Target = variant.Requirements.Target
	}
	return variant
}

func EffectiveProfileForVariant(profile WorkloadProfile, variant RuntimeModelVariant) WorkloadProfile {
	variant = normalizeVariant(profile, variant)
	out := profile
	out.LogicalModel = ProfileLogicalModel(profile)
	out.RuntimeVariantID = variant.ID
	out.RuntimeAdapter = variant.RuntimeAdapter
	out.Artifacts = variant.Artifacts
	out.Requirements = variant.Requirements
	out.LLM = variant.LLM
	out.Runtime = variant.Runtime
	out.RuntimeVariants = nil
	return out
}

func BestVariantForSlot(profile WorkloadProfile, node Node, slot Slot) (WorkloadProfile, FitResult) {
	var best WorkloadProfile
	var bestFit FitResult
	var miss []string
	for _, variant := range ProfileRuntimeVariants(profile) {
		effective := EffectiveProfileForVariant(profile, variant)
		fit := FitProfileToSlot(effective, node, slot)
		if fit.Fits {
			return effective, fit
		}
		miss = append(miss, fit.Reasons...)
		best = effective
		bestFit = fit
	}
	if best.ID == "" {
		best = profile
	}
	if bestFit.ProfileID == "" {
		bestFit = FitResult{ProfileID: profile.ID, LogicalModel: ProfileLogicalModel(profile), SlotID: slot.ID, NodeID: node.ID, Fits: false, Reasons: []string{FailureUnknownRequirements}}
	}
	if len(miss) > 0 {
		bestFit.Reasons = uniqueSorted(miss)
		bestFit.Fits = false
	}
	return best, bestFit
}

func ConcreteModelArtifactID(profile WorkloadProfile) string {
	if len(profile.Artifacts) > 0 {
		return profile.Artifacts[0]
	}
	return ""
}

func ConcreteModelSHA256(profile WorkloadProfile) string {
	return NormalizeSHA256(ConcreteModelArtifactID(profile))
}

func profileArtifactIDs(profile WorkloadProfile) []string {
	ids := []string{}
	for _, id := range profile.Artifacts {
		ids = appendUnique(ids, id)
	}
	for _, variant := range ProfileRuntimeVariants(profile) {
		for _, id := range variant.Artifacts {
			ids = appendUnique(ids, id)
		}
	}
	return ids
}

func effectiveProfileForAssignment(profile WorkloadProfile, assignment PlacementAssignment) WorkloadProfile {
	if assignment.RuntimeVariantID == "" {
		return profile
	}
	for _, variant := range ProfileRuntimeVariants(profile) {
		if variant.ID == assignment.RuntimeVariantID {
			return EffectiveProfileForVariant(profile, variant)
		}
	}
	return profile
}

func minimumSufficientContext(profile WorkloadProfile, minContext int) int {
	best := 0
	for _, variant := range ProfileRuntimeVariants(profile) {
		if variant.Requirements == nil || variant.LLM == nil {
			continue
		}
		ctx := variant.LLM.ContextTokens
		if ctx < minContext {
			continue
		}
		if best == 0 || ctx < best {
			best = ctx
		}
	}
	return best
}

func appendUnique(items []string, item string) []string {
	if item == "" || Contains(items, item) {
		return items
	}
	return append(items, item)
}

func uniqueSorted(items []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
