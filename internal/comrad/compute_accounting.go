package comrad

import (
	"errors"
	"time"
)

var errInsufficientBalance = errors.New("insufficient_balance")

func appendLedgerEntry(db *Database, entry ComputeLedgerEntry) {
	if entry.ID == "" {
		entry.ID = NewID("led")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	db.ComputeLedger = append(db.ComputeLedger, entry)
	applyLedgerBalance(db, entry)
}

func applyLedgerBalance(db *Database, entry ComputeLedgerEntry) {
	user := db.Users[entry.UserID]
	if user.ID == "" {
		return
	}
	if entry.Direction == LedgerDebit {
		user.ComputeBalance -= entry.Amount
	} else if entry.Direction == LedgerCredit {
		user.ComputeBalance += entry.Amount
	}
	db.Users[user.ID] = user
}

func applyReportAccounting(db *Database, report *ComputeReport, now time.Time) {
	attempt := db.Attempts[report.AttemptID]
	task := db.Tasks[report.TaskID]
	report.UserID = firstNonEmpty(attempt.UserID, task.UserID, report.UserID)
	report.ProfileVersion = firstNonZero(attempt.ProfileVersion, task.ProfileVersion, report.ProfileVersion)
	report.ComputeCost = firstNonZero64(attempt.ComputeCost, task.ComputeCost, report.ComputeCost)
	if report.Status == TaskStatusCompleted && report.ComputeCost > 0 && !ledgerExistsForReport(db, report.ID) {
		appendConsumptionLedger(db, *report, now)
		appendProductionLedger(db, *report, now)
	}
}

func appendConsumptionLedger(db *Database, report ComputeReport, now time.Time) {
	if _, ok := db.Users[report.UserID]; !ok {
		return
	}
	appendLedgerEntry(db, ComputeLedgerEntry{
		Type:           LedgerConsumeCompute,
		UserID:         report.UserID,
		TaskID:         report.TaskID,
		AttemptID:      report.AttemptID,
		ReportID:       report.ID,
		NodeID:         report.NodeID,
		SlotID:         report.SlotID,
		ProfileID:      report.ProfileID,
		ProfileVersion: report.ProfileVersion,
		ComputeCost:    report.ComputeCost,
		Amount:         report.ComputeCost,
		Direction:      LedgerDebit,
		Reason:         "completed task",
		CreatedAt:      now,
	})
}

func appendProductionLedger(db *Database, report ComputeReport, now time.Time) {
	node := db.Nodes[report.NodeID]
	if node.OwnerUserID == "" {
		return
	}
	if _, ok := db.Users[node.OwnerUserID]; !ok {
		return
	}
	appendLedgerEntry(db, ComputeLedgerEntry{
		Type:           LedgerProduceCompute,
		UserID:         node.OwnerUserID,
		TaskID:         report.TaskID,
		AttemptID:      report.AttemptID,
		ReportID:       report.ID,
		NodeID:         report.NodeID,
		SlotID:         report.SlotID,
		ProfileID:      report.ProfileID,
		ProfileVersion: report.ProfileVersion,
		ComputeCost:    report.ComputeCost,
		Amount:         report.ComputeCost,
		Direction:      LedgerCredit,
		Reason:         "completed task on owned node",
		CreatedAt:      now,
	})
}

func ledgerExistsForReport(db *Database, reportID string) bool {
	for _, entry := range db.ComputeLedger {
		if entry.ReportID == reportID {
			return true
		}
	}
	return false
}

func ensureSufficientBalance(db Database, user User, profile WorkloadProfile, enforce bool) error {
	return ensureSufficientAvailableBalance(db, user.ID, profile.ComputeCost, enforce)
}

func ensureSufficientAvailableBalance(db Database, userID string, computeCost int64, enforce bool) error {
	if !enforce || computeCost <= 0 {
		return nil
	}
	current := db.Users[userID]
	if current.ComputeBalance-pendingComputeCost(db, userID) < computeCost {
		return errInsufficientBalance
	}
	return nil
}

func pendingComputeCost(db Database, userID string) int64 {
	var total int64
	for _, task := range db.Tasks {
		if task.UserID != userID {
			continue
		}
		if task.Status == TaskStatusQueued || task.Status == TaskStatusRunning {
			total += task.ComputeCost
		}
	}
	return total
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonZero64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func profileVersion(profile WorkloadProfile) int {
	if profile.Version > 0 {
		return profile.Version
	}
	return 1
}
