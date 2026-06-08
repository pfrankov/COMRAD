package comrad

import (
	"context"
	"encoding/json"
)

func workerAck(w *Worker, ctx context.Context, msg Envelope) error {
	if len(msg.Payload) == 0 {
		return nil
	}
	var ack WorkerRegistrationAck
	if err := json.Unmarshal(msg.Payload, &ack); err != nil {
		return err
	}
	if ack.NodeToken == "" {
		return nil
	}
	w.mu.Lock()
	w.nodeToken = ack.NodeToken
	w.mu.Unlock()
	return w.saveState()
}

func workerAssignProfile(w *Worker, ctx context.Context, msg Envelope) error {
	var payload AssignmentPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	if err := w.queueAssignment(ctx, payload); err != nil {
		return err
	}
	w.enqueue(Envelope{ID: msg.ID, Type: MsgAck, NodeID: w.node.ID})
	return nil
}

func workerExecuteTask(w *Worker, ctx context.Context, msg Envelope) error {
	var payload ExecuteTaskPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	go w.executeTask(payload)
	w.enqueue(Envelope{ID: msg.ID, Type: MsgAck, NodeID: w.node.ID, TaskID: payload.TaskID, Attempt: payload.AttemptID})
	return nil
}

func workerCancelTask(w *Worker, ctx context.Context, msg Envelope) error {
	var payload CancelTaskPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	w.cancelAttempt(payload.AttemptID)
	w.enqueue(Envelope{ID: msg.ID, Type: MsgAck, NodeID: w.node.ID, TaskID: payload.TaskID, Attempt: payload.AttemptID})
	return nil
}

func workerP2PConfig(w *Worker, ctx context.Context, msg Envelope) error {
	var payload P2PConfigPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	w.applyP2PConfig(payload.Enabled)
	w.enqueue(Envelope{ID: msg.ID, Type: MsgAck, NodeID: w.node.ID})
	return nil
}

func workerUpdate(w *Worker, ctx context.Context, msg Envelope) error {
	var payload UpdatePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}
	go w.handleUpdate(payload)
	w.enqueue(Envelope{ID: msg.ID, Type: MsgAck, NodeID: w.node.ID})
	return nil
}
