package comrad

import (
	"net/url"
	"sort"
	"strconv"
	"time"
)

const (
	adminStateTaskLimit   = 50
	adminTaskDefaultLimit = 50
	adminTaskMaxLimit     = 500
)

type taskListQuery struct {
	Limit     int
	Offset    int
	Status    string
	UserID    string
	ProfileID string
	TaskID    string
}

func parseTaskListQuery(values url.Values) taskListQuery {
	return taskListQuery{
		Limit:     boundedLimit(values.Get("limit")),
		Offset:    nonNegativeInt(values.Get("offset")),
		Status:    values.Get("status"),
		UserID:    values.Get("userId"),
		ProfileID: values.Get("profileId"),
		TaskID:    values.Get("taskId"),
	}
}

func taskListResponse(db Database, query taskListQuery) TaskListResponse {
	tasks := filterTasks(sortedTasksNewest(db), query)
	page := taskPage(tasks, query.Offset, query.Limit)
	ids := taskIDs(page)
	return TaskListResponse{
		Items:    page,
		Attempts: relatedAttempts(db, ids),
		Reports:  relatedReports(db, ids),
		Total:    len(tasks),
		Limit:    query.Limit,
		Offset:   query.Offset,
		HasMore:  query.Offset+len(page) < len(tasks),
		Summary:  summarizeTasks(tasks, db.Reports),
	}
}

func stateTaskWindow(db Database) ([]Task, []Attempt, []ComputeReport, bool) {
	tasks := sortedTasks(db)
	truncated := len(tasks) > adminStateTaskLimit
	if truncated {
		tasks = tasks[len(tasks)-adminStateTaskLimit:]
	}
	ids := taskIDs(tasks)
	return tasks, relatedAttempts(db, ids), relatedReports(db, ids), truncated
}

func summarizeDatabaseTasks(db Database) TaskSummary {
	return summarizeTasks(sortedTasks(db), db.Reports)
}

func filterTasks(tasks []Task, query taskListQuery) []Task {
	out := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if taskMatchesQuery(task, query) {
			out = append(out, task)
		}
	}
	return out
}

func taskMatchesQuery(task Task, query taskListQuery) bool {
	return stringMatches(query.TaskID, task.ID) &&
		stringMatches(query.Status, task.Status) &&
		stringMatches(query.UserID, task.UserID) &&
		stringMatches(query.ProfileID, task.ProfileID)
}

func taskPage(tasks []Task, offset int, limit int) []Task {
	if offset >= len(tasks) {
		return []Task{}
	}
	end := offset + limit
	if end > len(tasks) {
		end = len(tasks)
	}
	return tasks[offset:end]
}

func summarizeTasks(tasks []Task, reports map[string]ComputeReport) TaskSummary {
	summary := TaskSummary{Total: len(tasks)}
	byUser := map[string]*TaskUserSummary{}
	for _, task := range tasks {
		addTaskToSummary(&summary, byUser, task)
	}
	summary.FailuresLastHour = failuresLastHour(tasks, reports)
	summary.ByUser = sortedUserTaskSummaries(byUser)
	return summary
}

func addTaskToSummary(summary *TaskSummary, byUser map[string]*TaskUserSummary, task Task) {
	user := taskUserSummary(byUser, task.UserID)
	user.Total++
	user.ComputeCost += task.ComputeCost
	countTaskStatus(task.Status, &summary.Queued, &summary.Running, &summary.Completed, &summary.Failed, &summary.Cancelled)
	countTaskStatus(task.Status, &user.Queued, &user.Running, &user.Completed, &user.Failed, &user.Cancelled)
}

func countTaskStatus(status string, queued *int, running *int, completed *int, failed *int, cancelled *int) {
	switch status {
	case TaskStatusQueued:
		(*queued)++
	case TaskStatusRunning:
		(*running)++
	case TaskStatusCompleted:
		(*completed)++
	case TaskStatusFailed:
		(*failed)++
	case TaskStatusCancelled:
		(*cancelled)++
	}
}

func failuresLastHour(tasks []Task, reports map[string]ComputeReport) int {
	taskSet := taskIDs(tasks)
	cutoff := time.Now().UTC().Add(-time.Hour)
	count := 0
	for _, report := range reports {
		if taskSet[report.TaskID] && report.Status == TaskStatusFailed && report.CreatedAt.After(cutoff) {
			count++
		}
	}
	return count
}

func taskUserSummary(byUser map[string]*TaskUserSummary, userID string) *TaskUserSummary {
	if userID == "" {
		userID = "unknown"
	}
	if byUser[userID] == nil {
		byUser[userID] = &TaskUserSummary{UserID: userID}
	}
	return byUser[userID]
}

func sortedUserTaskSummaries(byUser map[string]*TaskUserSummary) []TaskUserSummary {
	out := make([]TaskUserSummary, 0, len(byUser))
	for _, item := range byUser {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out
}

func relatedAttempts(db Database, ids map[string]bool) []Attempt {
	out := make([]Attempt, 0, len(ids))
	for _, attempt := range db.Attempts {
		if ids[attempt.TaskID] {
			out = append(out, attempt)
		}
	}
	sortAttempts(out)
	return out
}

func relatedReports(db Database, ids map[string]bool) []ComputeReport {
	out := make([]ComputeReport, 0, len(ids))
	for _, report := range db.Reports {
		if ids[report.TaskID] {
			out = append(out, report)
		}
	}
	sortReports(out)
	return out
}

func taskIDs(tasks []Task) map[string]bool {
	out := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		out[task.ID] = true
	}
	return out
}

func sortedTasksNewest(db Database) []Task {
	out := sortedTasks(db)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[j].CreatedAt.Before(out[i].CreatedAt)
	})
	return out
}

func stringMatches(filter string, value string) bool {
	return filter == "" || filter == value
}

func boundedLimit(value string) int {
	limit := parseIntOr(value, adminTaskDefaultLimit)
	if limit < 1 {
		return adminTaskDefaultLimit
	}
	if limit > adminTaskMaxLimit {
		return adminTaskMaxLimit
	}
	return limit
}

func nonNegativeInt(value string) int {
	offset := parseIntOr(value, 0)
	if offset < 0 {
		return 0
	}
	return offset
}

func parseIntOr(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
