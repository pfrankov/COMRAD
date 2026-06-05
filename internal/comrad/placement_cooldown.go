package comrad

import "time"

const defaultAutoBalanceScaleDownCooldown = 5 * time.Minute

func managerAutoBalanceCooldown(cfg ManagerConfig) time.Duration {
	if cfg.AutoBalanceScaleDownCooldown > 0 {
		return cfg.AutoBalanceScaleDownCooldown
	}
	return defaultAutoBalanceScaleDownCooldown
}

func autoBalanceCooldownDemand(db Database, profileID string, now time.Time, cooldown time.Duration) int {
	if cooldown <= 0 {
		return 0
	}
	_, _, _, smoothed := profileDemand(db, profileID, now.Add(-cooldown))
	return max(smoothed, recentlyFinishedDemand(db, profileID, now, cooldown))
}

func recentlyFinishedDemand(db Database, profileID string, now time.Time, cooldown time.Duration) int {
	cutoff := now.Add(-cooldown)
	recentCutoff := now.Add(-autoBalanceDemandWindow)
	count := 0
	for _, task := range db.Tasks {
		if task.ProfileID != profileID || !taskTerminal(task.Status) {
			continue
		}
		if task.CreatedAt.After(recentCutoff) {
			continue
		}
		finishedAt := taskFinishedAt(task)
		if finishedAt.After(cutoff) && !finishedAt.After(now) {
			count++
		}
	}
	return count
}

func taskTerminal(status string) bool {
	return status == TaskStatusCompleted || status == TaskStatusFailed || status == TaskStatusCancelled
}

func taskFinishedAt(task Task) time.Time {
	if task.CompletedAt != nil {
		return *task.CompletedAt
	}
	return task.UpdatedAt
}
