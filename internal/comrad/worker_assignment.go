package comrad

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

func (w *Worker) handleAssignment(ctx context.Context, payload AssignmentPayload) error {
	if err := validateAssignmentPayload(payload); err != nil {
		return err
	}
	if w.assignmentAlreadySatisfied(payload) {
		return nil
	}
	if !w.beginAssignment(payload) {
		return nil
	}
	defer w.finishAssignment(payload)
	return w.runAssignment(ctx, payload)
}

func (w *Worker) queueAssignment(ctx context.Context, payload AssignmentPayload) error {
	if err := validateAssignmentPayload(payload); err != nil {
		return err
	}
	if w.assignmentAlreadySatisfied(payload) || !w.beginAssignment(payload) {
		return nil
	}
	go func() {
		defer w.finishAssignment(payload)
		if err := w.runAssignment(ctx, payload); err != nil {
			log.Printf("assignment %s failed: %v", payload.Profile.ID, err)
		}
	}()
	return nil
}

func validateAssignmentPayload(payload AssignmentPayload) error {
	if payload.Profile.ID == "" {
		return errors.New("assignment missing profile")
	}
	return nil
}

func (w *Worker) beginAssignment(payload AssignmentPayload) bool {
	key := assignmentKey(payload.Profile)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.activeAssignments == nil {
		w.activeAssignments = map[string]bool{}
	}
	if w.assigns == nil {
		w.assigns = map[string]AssignmentPayload{}
	}
	if w.activeAssignments[key] {
		return false
	}
	w.assigns[key] = payload
	w.activeAssignments[key] = true
	return true
}

func (w *Worker) finishAssignment(payload AssignmentPayload) {
	w.mu.Lock()
	delete(w.activeAssignments, assignmentKey(payload.Profile))
	w.mu.Unlock()
}

func (w *Worker) runAssignment(ctx context.Context, payload AssignmentPayload) error {
	if payload.Cached && !payload.Warm {
		return w.ensureCachedAssignment(ctx, payload)
	}
	slotID, fit := w.localFit(payload.Profile, payload.SlotID)
	if !fit.Fits {
		w.setAnySlotState(SlotStateIdle, payload.Profile.ID, strings.Join(fit.Reasons, ","))
		return fmt.Errorf("assignment rejected: %s", strings.Join(fit.Reasons, ","))
	}
	if err := w.ensureSlotCached(ctx, slotID, payload); err != nil {
		return err
	}
	if err := w.ensureSlotWarm(ctx, slotID, payload); err != nil {
		return err
	}
	w.saveState()
	return nil
}

func (w *Worker) ensureCachedAssignment(ctx context.Context, payload AssignmentPayload) error {
	for _, artifact := range payload.Artifacts {
		if err := w.ensureArtifact(ctx, artifact); err != nil {
			return err
		}
	}
	w.saveState()
	return nil
}

func (w *Worker) ensureSlotCached(ctx context.Context, slotID string, payload AssignmentPayload) error {
	if !payload.Cached {
		return nil
	}
	if !w.artifactsCached(payload.Artifacts) {
		w.setSlotState(slotID, SlotStateDownloadQueued, payload.Profile.ID, "")
	}
	for _, artifact := range payload.Artifacts {
		onStart := func() { w.setSlotState(slotID, SlotStateDownloading, payload.Profile.ID, "") }
		if err := w.ensureArtifactWithStart(ctx, artifact, onStart); err != nil {
			w.setSlotState(slotID, SlotStateError, payload.Profile.ID, err.Error())
			return err
		}
	}
	w.setSlotState(slotID, SlotStateCached, payload.Profile.ID, "")
	return nil
}

func (w *Worker) ensureSlotWarm(ctx context.Context, slotID string, payload AssignmentPayload) error {
	if !payload.Warm {
		return nil
	}
	w.setSlotState(slotID, SlotStateLoading, payload.Profile.ID, "")
	if err := w.verifyProfileArtifacts(payload.Profile); err != nil {
		w.setSlotState(slotID, SlotStateError, payload.Profile.ID, err.Error())
		return err
	}
	w.setSlotState(slotID, SlotStateWarming, payload.Profile.ID, "")
	if err := w.ensureRuntimeServer(ctx, slotID, payload.Profile); err != nil {
		w.setSlotState(slotID, SlotStateError, payload.Profile.ID, err.Error())
		return err
	}
	w.setSlotState(slotID, SlotStateReady, payload.Profile.ID, "")
	return nil
}

func (w *Worker) assignmentAlreadySatisfied(payload AssignmentPayload) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := assignmentKey(payload.Profile)
	if w.activeAssignments[key] {
		return true
	}
	if payload.Cached && !payload.Warm {
		return workerHasArtifacts(w.cache, payload.Artifacts)
	}
	for _, slot := range w.slots {
		if !slotProfileCurrent(slot, payload.Profile.ID, payload.Profile) {
			continue
		}
		switch slot.State {
		case SlotStateReady, SlotStateServing:
			proc := w.runtimes[slot.ID]
			return proc != nil && proc.profileKey == key && proc.alive()
		case SlotStateDownloadQueued, SlotStateDownloading, SlotStateCached, SlotStateLoading, SlotStateWarming:
			return true
		}
	}
	return false
}

func (w *Worker) artifactsCached(artifacts []ArtifactSpec) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return workerHasArtifacts(w.cache, artifacts)
}

func workerHasArtifacts(cache map[string]string, artifacts []ArtifactSpec) bool {
	for _, artifact := range artifacts {
		if cache[artifact.ID] == "" {
			return false
		}
	}
	return len(artifacts) > 0
}

func (w *Worker) localFit(profile WorkloadProfile, preferredSlot string) (string, FitResult) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if preferredSlot != "" {
		slot := w.slots[preferredSlot]
		fit := FitProfileToSlot(profile, w.node, slot)
		return slot.ID, fit
	}
	for _, slot := range w.slots {
		fit := FitProfileToSlot(profile, w.node, slot)
		if fit.Fits {
			return slot.ID, fit
		}
		return slot.ID, fit
	}
	return "", FitResult{ProfileID: profile.ID, Fits: false, Reasons: []string{"no_slots"}}
}
