package comrad

func placementExplainResponseSchema() map[string]any {
	return object(map[string]any{
		"generatedAt": dateTimeSchema(),
		"plan":        arrayOf("PlacementAssignment"),
		"profiles":    arrayOf("PlacementProfileExplanation"),
	})
}

func placementProfileExplanationSchema() map[string]any {
	return object(map[string]any{
		"profileId":     stringSchema(),
		"policyId":      stringSchema(),
		"logicalModel":  stringSchema(),
		"desiredCached": integerSchema(),
		"desiredWarm":   integerSchema(),
		"selected":      arrayOf("PlacementCandidateExplanation"),
		"rejected":      arrayOf("PlacementCandidateExplanation"),
		"missing":       arrayOf("PlacementMissingExplanation"),
	})
}

func placementCandidateExplanationSchema() map[string]any {
	return object(map[string]any{
		"phase":            stringSchema(),
		"nodeId":           stringSchema(),
		"slotId":           stringSchema(),
		"runtimeVariantId": stringSchema(),
		"modelArtifactId":  stringSchema(),
		"desiredCached":    booleanSchema(),
		"desiredWarm":      booleanSchema(),
		"actualCached":     booleanSchema(),
		"actualWarm":       booleanSchema(),
		"ready":            booleanSchema(),
		"reasons":          arrayOfPrimitive("string"),
	})
}

func placementMissingExplanationSchema() map[string]any {
	return object(map[string]any{
		"phase":         stringSchema(),
		"desiredCached": booleanSchema(),
		"desiredWarm":   booleanSchema(),
		"reasons":       arrayOfPrimitive("string"),
	})
}
