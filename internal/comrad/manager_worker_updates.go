package comrad

import (
	"strings"
	"time"
)

func (m *Manager) recordWorkerUpdateTelemetry(nodeID, updateID, status, detail string) error {
	return m.store.Update(func(db *Database) error {
		update, ok := db.Updates[updateID]
		if !ok {
			return nil
		}
		if status != "" {
			update.Status = status
		}
		if detail != "" && status == "update_failed" {
			update.Failure = detail
		}
		if status == "update_downloaded" {
			update.Delivery = mergeUpdateDelivery(update.Delivery, detail)
			update.DeliveryDetail = detail
		}
		db.Updates[updateID] = update
		node := db.Nodes[nodeID]
		node.UpdateStatus = status
		if status == "update_failed" && detail != "" {
			node.LastFailure = detail
			now := time.Now().UTC()
			node.LastFailureAt = &now
		}
		db.Nodes[nodeID] = node
		return nil
	})
}

func mergeUpdateDelivery(current, next string) string {
	next = strings.TrimSpace(next)
	if next == "" || next == current {
		return current
	}
	if current == "" {
		return next
	}
	return "mixed"
}

func (m *Manager) dispatchUpdate(update UpdateRecord) {
	db := m.store.Snapshot()
	targets := update.TargetNodes
	if len(targets) == 0 {
		for id := range db.Nodes {
			targets = append(targets, id)
		}
	}
	for _, nodeID := range targets {
		m.mu.Lock()
		sess := m.sessions[nodeID]
		m.mu.Unlock()
		if sess == nil {
			continue
		}
		payload := UpdatePayload{Update: update}
		if art, ok := db.Artifacts[update.ArtifactID]; ok {
			payload.Artifact = ArtifactSpec{
				ID:        art.ID,
				Kind:      art.Kind,
				Name:      art.Name,
				SHA256:    art.SHA256,
				SizeBytes: art.SizeBytes,
				URL:       strings.TrimRight(sess.baseURL, "/") + "/api/worker/artifacts/" + art.ID,
				Torrent:   cloneArtifactTorrent(art.Torrent),
			}
		}
		sess.enqueue(Envelope{ID: NewID("msg"), Type: MsgUpdateWorker, NodeID: nodeID, Payload: MarshalPayload(payload)})
	}
}
