package comrad

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func workerEvictArtifact(w *Worker, ctx context.Context, msg Envelope) error {
	var payload EvictArtifactPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	if err := w.evictArtifact(payload); err != nil {
		return err
	}
	w.enqueue(Envelope{ID: msg.ID, Type: MsgAck, NodeID: w.node.ID})
	return nil
}

func (w *Worker) evictArtifact(payload EvictArtifactPayload) error {
	artifactID := NormalizeSHA256(payload.ArtifactID)
	if artifactID == "" {
		return fmt.Errorf("artifactId is required")
	}
	plan, err := w.planArtifactEviction(artifactID)
	if err != nil {
		w.sendArtifactState(ArtifactSpec{ID: artifactID}, "evict_failed", err.Error())
		return err
	}
	for _, proc := range plan.procs {
		proc.stop()
	}
	if p2p := w.p2pRuntime(); p2p != nil {
		p2p.StopSeeding(artifactID)
		w.refreshP2PState()
	}
	if plan.path != "" {
		if err := os.Remove(plan.path); err != nil && !os.IsNotExist(err) {
			w.sendArtifactState(ArtifactSpec{ID: artifactID}, "evict_failed", err.Error())
			return err
		}
	}
	updates := w.applyArtifactEviction(artifactID, plan)
	if err := w.saveState(); err != nil {
		w.sendArtifactState(ArtifactSpec{ID: artifactID}, "evict_failed", err.Error())
		return err
	}
	for _, slot := range updates {
		w.sendSlotState(slot)
	}
	w.sendArtifactState(ArtifactSpec{ID: artifactID}, "evicted", "")
	return nil
}

type artifactEvictionPlan struct {
	path     string
	procs    []*llamaServerProcess
	slotIDs  []string
	profiles []string
}

func (w *Worker) planArtifactEviction(artifactID string) (artifactEvictionPlan, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	plan := artifactEvictionPlan{path: w.cache[artifactID]}
	for slotID, slot := range w.slots {
		if !w.slotUsesArtifactLocked(slot, artifactID) {
			continue
		}
		if slot.State == SlotStateServing || slot.ActiveTaskID != "" {
			return artifactEvictionPlan{}, fmt.Errorf("artifact_in_use: %s is serving on %s", artifactID, slotID)
		}
		plan.slotIDs = append(plan.slotIDs, slotID)
		if proc := w.runtimes[slotID]; proc != nil {
			plan.procs = append(plan.procs, proc)
		}
	}
	for key, assignment := range w.assigns {
		if assignmentUsesArtifact(assignment, artifactID) {
			plan.profiles = append(plan.profiles, key)
		}
	}
	for key, profile := range w.warm {
		if profileUsesArtifact(profile, artifactID) && !Contains(plan.profiles, key) {
			plan.profiles = append(plan.profiles, key)
		}
	}
	return plan, nil
}

func (w *Worker) applyArtifactEviction(artifactID string, plan artifactEvictionPlan) []Slot {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.cache, artifactID)
	delete(w.cacheState, artifactID)
	for _, key := range plan.profiles {
		delete(w.assigns, key)
		delete(w.warm, key)
	}
	updates := make([]Slot, 0, len(plan.slotIDs))
	for _, slotID := range plan.slotIDs {
		delete(w.runtimes, slotID)
		delete(w.runtimeRestarts, slotID)
		slot := w.slots[slotID]
		clearEvictedSlot(&slot)
		w.slots[slotID] = slot
		updates = append(updates, slot)
	}
	return updates
}

func clearEvictedSlot(slot *Slot) {
	slot.State = SlotStateIdle
	slot.ProfileID = ""
	slot.ProfileVersion = 0
	slot.LogicalModel = ""
	slot.RuntimeVariantID = ""
	slot.ModelArtifactID = ""
	slot.ModelSHA256 = ""
	slot.ActiveTaskID = ""
	slot.AcceptsNew = false
	slot.MismatchReason = "artifact_evicted"
	slot.LastReady = time.Time{}
}

func (w *Worker) slotUsesArtifactLocked(slot Slot, artifactID string) bool {
	if NormalizeSHA256(slot.ModelArtifactID) == artifactID || NormalizeSHA256(slot.ModelSHA256) == artifactID {
		return true
	}
	for _, assignment := range w.assigns {
		if assignment.Profile.ID == slot.ProfileID && assignmentUsesArtifact(assignment, artifactID) {
			return true
		}
	}
	for _, profile := range w.warm {
		if profile.ID == slot.ProfileID && profileUsesArtifact(profile, artifactID) {
			return true
		}
	}
	return false
}

func assignmentUsesArtifact(assignment AssignmentPayload, artifactID string) bool {
	if profileUsesArtifact(assignment.Profile, artifactID) {
		return true
	}
	for _, artifact := range assignment.Artifacts {
		if NormalizeSHA256(artifact.ID) == artifactID {
			return true
		}
	}
	return false
}

func profileUsesArtifact(profile WorkloadProfile, artifactID string) bool {
	for _, id := range profileArtifactIDs(profile) {
		if NormalizeSHA256(id) == artifactID {
			return true
		}
	}
	return false
}

func (w *Worker) sendSlotState(slot Slot) {
	w.enqueue(Envelope{ID: NewID("msg"), Type: MsgSlotState, NodeID: w.node.ID, Payload: MarshalPayload(SlotStatePayload{
		SlotID:         slot.ID,
		State:          slot.State,
		ProfileID:      slot.ProfileID,
		ActiveTaskID:   slot.ActiveTaskID,
		MismatchReason: slot.MismatchReason,
	})})
}
