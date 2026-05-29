package comrad

import "time"

const (
	ArtifactEvictionQueued  = "queued"
	ArtifactEvictionBlocked = "blocked"
	ArtifactEvictionEvicted = "evicted"
	ArtifactEvictionFailed  = "failed"
)

func (m *Manager) recordArtifactEviction(nodeID, artifactID, reason, status, failure string, now time.Time) {
	_ = m.store.Update(func(db *Database) error {
		upsertArtifactEvictionRecord(db, nodeID, artifactID, reason, status, failure, now)
		return nil
	})
}

func upsertArtifactEvictionRecord(db *Database, nodeID, artifactID, reason, status, failure string, now time.Time) {
	ensureMaps(db)
	artifactID = NormalizeSHA256(artifactID)
	if nodeID == "" || artifactID == "" {
		return
	}
	id := latestArtifactEvictionID(*db, nodeID, artifactID)
	record := db.ArtifactEvictions[id]
	if record.ID == "" {
		record.ID = NewID("evict")
		record.NodeID = nodeID
		record.ArtifactID = artifactID
		record.RequestedAt = now
	}
	if reason != "" {
		record.Reason = reason
	}
	record.Status = status
	record.Failure = failure
	record.UpdatedAt = now
	db.ArtifactEvictions[record.ID] = record
}

func latestArtifactEvictionID(db Database, nodeID, artifactID string) string {
	var id string
	var updatedAt time.Time
	for _, record := range db.ArtifactEvictions {
		if record.NodeID != nodeID || NormalizeSHA256(record.ArtifactID) != artifactID {
			continue
		}
		if id == "" || record.UpdatedAt.After(updatedAt) {
			id = record.ID
			updatedAt = record.UpdatedAt
		}
	}
	return id
}

func evictionStatusForArtifactState(state string) string {
	switch state {
	case "evicted":
		return ArtifactEvictionEvicted
	case "evict_failed":
		return ArtifactEvictionFailed
	default:
		return ""
	}
}
