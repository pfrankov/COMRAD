package comrad

import (
	"maps"
	"slices"
	"time"
)

func cloneDatabase(db Database) Database {
	return Database{
		SchemaVersion:     db.SchemaVersion,
		Migrations:        slices.Clone(db.Migrations),
		Settings:          db.Settings,
		Nodes:             cloneMapValues(db.Nodes, cloneNode),
		Slots:             cloneMapValues(db.Slots, cloneSlot),
		Artifacts:         cloneMapValues(db.Artifacts, cloneArtifact),
		ArtifactEvictions: cloneMapValues(db.ArtifactEvictions, cloneArtifactEvictionRecord),
		CacheIntents:      maps.Clone(db.CacheIntents),
		Profiles:          cloneMapValues(db.Profiles, cloneWorkloadProfile),
		Policies:          cloneMapValues(db.Policies, clonePlacementPolicy),
		Assignments:       maps.Clone(db.Assignments),
		Tasks:             cloneMapValues(db.Tasks, cloneTask),
		Attempts:          cloneMapValues(db.Attempts, cloneAttempt),
		Reports:           maps.Clone(db.Reports),
		Updates:           cloneMapValues(db.Updates, cloneUpdateRecord),
		Users:             maps.Clone(db.Users),
		APIKeys:           cloneMapValues(db.APIKeys, cloneAPIKey),
		NodeTokenHashes:   maps.Clone(db.NodeTokenHashes),
		ComputeLedger:     slices.Clone(db.ComputeLedger),
		Audit:             cloneAuditEvents(db.Audit),
	}
}

func cloneMapValues[V any](in map[string]V, clone func(V) V) map[string]V {
	if in == nil {
		return nil
	}
	out := make(map[string]V, len(in))
	for k, v := range in {
		out[k] = clone(v)
	}
	return out
}

func cloneNode(in Node) Node {
	out := in
	out.Tags = slices.Clone(in.Tags)
	out.RuntimeAdapters = slices.Clone(in.RuntimeAdapters)
	out.P2P = cloneWorkerP2PStatus(in.P2P)
	out.CachedArtifacts = slices.Clone(in.CachedArtifacts)
	out.WarmProfiles = slices.Clone(in.WarmProfiles)
	out.LastFailureAt = cloneTimePtr(in.LastFailureAt)
	out.QuarantineUntil = cloneTimePtr(in.QuarantineUntil)
	out.RecentFlapEvents = slices.Clone(in.RecentFlapEvents)
	out.WarmPlacementSuppressionUntil = cloneTimePtr(in.WarmPlacementSuppressionUntil)
	out.Conditions = cloneConditions(in.Conditions)
	out.ConnectedSession = ""
	return out
}

func cloneArtifact(in Artifact) Artifact {
	out := in
	out.Torrent = cloneArtifactTorrent(in.Torrent)
	return out
}

func cloneArtifactTorrent(in *ArtifactTorrent) *ArtifactTorrent {
	if in == nil {
		return nil
	}
	out := *in
	out.MetaInfoBytes = slices.Clone(in.MetaInfoBytes)
	return &out
}

func cloneWorkerP2PStatus(in *WorkerP2PStatus) *WorkerP2PStatus {
	if in == nil {
		return nil
	}
	out := *in
	out.LastFailureAt = cloneTimePtr(in.LastFailureAt)
	return &out
}

func cloneSlot(in Slot) Slot {
	out := in
	out.FailureCounters = maps.Clone(in.FailureCounters)
	out.LastFailureAt = cloneTimePtr(in.LastFailureAt)
	out.QuarantineUntil = cloneTimePtr(in.QuarantineUntil)
	out.Conditions = cloneConditions(in.Conditions)
	return out
}

func cloneArtifactEvictionRecord(in ArtifactEvictionRecord) ArtifactEvictionRecord {
	out := in
	out.Conditions = cloneConditions(in.Conditions)
	return out
}

func cloneWorkloadProfile(in WorkloadProfile) WorkloadProfile {
	out := in
	out.Artifacts = slices.Clone(in.Artifacts)
	out.Requirements = cloneRequirements(in.Requirements)
	out.LLM = cloneLLMProfile(in.LLM)
	out.Runtime = cloneRuntimeParameters(in.Runtime)
	out.RuntimeVariants = cloneRuntimeModelVariants(in.RuntimeVariants)
	out.Conditions = cloneConditions(in.Conditions)
	return out
}

func cloneRuntimeModelVariants(in []RuntimeModelVariant) []RuntimeModelVariant {
	if in == nil {
		return nil
	}
	out := make([]RuntimeModelVariant, len(in))
	for i, v := range in {
		out[i] = v
		out[i].Artifacts = slices.Clone(v.Artifacts)
		out[i].Requirements = cloneRequirements(v.Requirements)
		out[i].LLM = cloneLLMProfile(v.LLM)
		out[i].Runtime = cloneRuntimeParameters(v.Runtime)
		out[i].Metadata = maps.Clone(v.Metadata)
	}
	return out
}

func cloneRequirements(in *Requirements) *Requirements {
	if in == nil {
		return nil
	}
	out := *in
	out.RequireTags = slices.Clone(in.RequireTags)
	return &out
}

func cloneLLMProfile(in *LLMProfile) *LLMProfile {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneRuntimeParameters(in RuntimeParameters) RuntimeParameters {
	out := in
	out.LlamaCpp.Args = slices.Clone(in.LlamaCpp.Args)
	return out
}

func clonePlacementPolicy(in PlacementPolicy) PlacementPolicy {
	out := in
	out.Constraints.RequireTags = slices.Clone(in.Constraints.RequireTags)
	out.Constraints.PreferNodes = slices.Clone(in.Constraints.PreferNodes)
	out.Constraints.DenyNodes = slices.Clone(in.Constraints.DenyNodes)
	out.HardPinnedSlots = slices.Clone(in.HardPinnedSlots)
	out.Conditions = cloneConditions(in.Conditions)
	return out
}

func cloneConditions(in []Condition) []Condition {
	return slices.Clone(in)
}

func cloneTask(in Task) Task {
	out := in
	out.FailedSlots = slices.Clone(in.FailedSlots)
	out.CompletedAt = cloneTimePtr(in.CompletedAt)
	out.Metadata = maps.Clone(in.Metadata)
	return out
}

func cloneAttempt(in Attempt) Attempt {
	out := in
	out.FirstOutputAt = cloneTimePtr(in.FirstOutputAt)
	out.CompletedAt = cloneTimePtr(in.CompletedAt)
	return out
}

func cloneUpdateRecord(in UpdateRecord) UpdateRecord {
	out := in
	out.TargetNodes = slices.Clone(in.TargetNodes)
	return out
}

func cloneAPIKey(in APIKey) APIKey {
	out := in
	out.RevokedAt = cloneTimePtr(in.RevokedAt)
	out.LastUsedAt = cloneTimePtr(in.LastUsedAt)
	return out
}

func cloneAuditEvents(in []AuditEvent) []AuditEvent {
	if in == nil {
		return nil
	}
	out := make([]AuditEvent, len(in))
	for i, event := range in {
		out[i] = event
		out[i].Metadata = maps.Clone(event.Metadata)
	}
	return out
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
