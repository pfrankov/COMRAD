package comrad

import "sort"

func sortNodes(v []Node) {
	sort.Slice(v, func(i, j int) bool { return v[i].ID < v[j].ID })
}

func sortSlots(v []Slot) {
	sort.Slice(v, func(i, j int) bool { return v[i].ID < v[j].ID })
}

func sortProfiles(v []WorkloadProfile) {
	sort.Slice(v, func(i, j int) bool { return v[i].ID < v[j].ID })
}

func sortArtifacts(v []Artifact) {
	sort.Slice(v, func(i, j int) bool { return v[i].ID < v[j].ID })
}

func sortPolicies(v []PlacementPolicy) {
	sort.Slice(v, func(i, j int) bool { return v[i].ID < v[j].ID })
}

func sortAssignments(v []PlacementAssignment) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].ProfileID == v[j].ProfileID {
			return v[i].SlotID < v[j].SlotID
		}
		return v[i].ProfileID < v[j].ProfileID
	})
}

func sortAttempts(v []Attempt) {
	sort.Slice(v, func(i, j int) bool { return v[i].StartedAt.Before(v[j].StartedAt) })
}

func sortReports(v []ComputeReport) {
	sort.Slice(v, func(i, j int) bool { return v[i].CreatedAt.Before(v[j].CreatedAt) })
}

func sortUsers(v []User) {
	sort.Slice(v, func(i, j int) bool { return v[i].ID < v[j].ID })
}

func sortAPIKeyViews(v []APIKeyView) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].UserID == v[j].UserID {
			return v[i].ID < v[j].ID
		}
		return v[i].UserID < v[j].UserID
	})
}

func sortComputeLedger(v []ComputeLedgerEntry) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].CreatedAt.Equal(v[j].CreatedAt) {
			return v[i].ID < v[j].ID
		}
		return v[i].CreatedAt.Before(v[j].CreatedAt)
	})
}
