package comrad

import "fmt"

func validateWorkerEnvelopeSession(s *workerSession, msg Envelope) error {
	if msg.Type == MsgHello || msg.Type == MsgFullState {
		return nil
	}
	if s.nodeID == "" {
		return fmt.Errorf("worker session is not registered")
	}
	if msg.NodeID != "" && msg.NodeID != s.nodeID {
		return fmt.Errorf("worker message node %s does not match session node %s", msg.NodeID, s.nodeID)
	}
	return nil
}

func authorizeWorkerNode(db *Database, s *workerSession, nodeID, providedToken string) (string, error) {
	if nodeID == "" {
		return "", fmt.Errorf("worker node id is required")
	}
	hash := db.NodeTokenHashes[nodeID]
	if hash == "" {
		if _, exists := db.Nodes[nodeID]; exists {
			return "", fmt.Errorf("worker node token missing for existing node %s", nodeID)
		}
		token := NewID("wnt")
		db.NodeTokenHashes[nodeID] = apiTokenHash(token)
		return token, nil
	}
	if s.nodeID == nodeID {
		return "", nil
	}
	if apiTokenHash(providedToken) != hash {
		return "", fmt.Errorf("worker node token required for %s", nodeID)
	}
	return "", nil
}

func requireWorkerAttempt(db Database, nodeID, attemptID, taskID string) (Attempt, error) {
	if attemptID == "" {
		return Attempt{}, fmt.Errorf("attempt id is required")
	}
	attempt := db.Attempts[attemptID]
	if attempt.ID == "" {
		return Attempt{}, fmt.Errorf("attempt %s not found", attemptID)
	}
	if attempt.NodeID != nodeID {
		return Attempt{}, fmt.Errorf("attempt %s does not belong to worker %s", attemptID, nodeID)
	}
	if taskID != "" && attempt.TaskID != taskID {
		return Attempt{}, fmt.Errorf("attempt %s does not belong to task %s", attemptID, taskID)
	}
	return attempt, nil
}

func applyAuthoritativeReportFields(report *ComputeReport, attempt Attempt, task Task) {
	report.TaskID = attempt.TaskID
	report.UserID = firstNonEmpty(attempt.UserID, task.UserID)
	report.NodeID = attempt.NodeID
	report.SlotID = attempt.SlotID
	report.ProfileID = firstNonEmpty(attempt.ProfileID, task.ProfileID)
	report.ProfileVersion = firstNonZero(attempt.ProfileVersion, task.ProfileVersion)
	report.LogicalModel = firstNonEmpty(attempt.LogicalModel, task.LogicalModel)
	report.RuntimeVariantID = firstNonEmpty(attempt.RuntimeVariantID, task.RuntimeVariantID)
	report.RuntimeAdapter = attempt.RuntimeAdapter
	report.ComputeCost = firstNonZero64(attempt.ComputeCost, task.ComputeCost)
}

func workerNodeTokenAuthorized(db Database, nodeID, token string) bool {
	if nodeID == "" || token == "" {
		return false
	}
	return apiTokenHash(token) == db.NodeTokenHashes[nodeID]
}
