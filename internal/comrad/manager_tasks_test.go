package comrad

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminStateBoundsTaskHistoryAndSummarizesAllTasks(t *testing.T) {
	m := newTestManager(t, 100, time.Second, 3)
	now := time.Now().UTC()
	for i := 0; i < 75; i++ {
		userID := []string{"user-a", "user-b"}[i%2]
		seedAdminTask(t, m, testTaskSeed{
			id:        taskID(i),
			userID:    userID,
			status:    historyStatus(i),
			createdAt: now.Add(time.Duration(i) * time.Second),
			cost:      int64(i + 1),
		})
	}

	state := m.stateResponse()
	if state.TaskSummary.Total != 75 || state.TaskSummary.Queued != 20 || state.TaskSummary.Running != 10 || state.TaskSummary.Completed != 30 || state.TaskSummary.Failed != 10 || state.TaskSummary.Cancelled != 5 {
		t.Fatalf("summary = %+v", state.TaskSummary)
	}
	if state.TaskSummary.FailuresLastHour != 10 {
		t.Fatalf("failures last hour = %d", state.TaskSummary.FailuresLastHour)
	}
	if !state.TasksTruncated || state.TaskPageLimit != adminStateTaskLimit {
		t.Fatalf("truncation fields: truncated=%v limit=%d", state.TasksTruncated, state.TaskPageLimit)
	}
	if len(state.Tasks) != adminStateTaskLimit {
		t.Fatalf("state tasks = %d", len(state.Tasks))
	}
	if state.Tasks[0].ID != taskID(25) || state.Tasks[len(state.Tasks)-1].ID != taskID(74) {
		t.Fatalf("state task window = %s..%s", state.Tasks[0].ID, state.Tasks[len(state.Tasks)-1].ID)
	}
	if len(state.Attempts) != adminStateTaskLimit || len(state.Reports) != adminStateTaskLimit {
		t.Fatalf("related attempts/reports = %d/%d", len(state.Attempts), len(state.Reports))
	}
}

func TestAdminTasksEndpointPaginatesAndFiltersWithRelatedRecords(t *testing.T) {
	m := newTestManager(t, 100, time.Second, 3)
	now := time.Now().UTC()
	for i := 0; i < 6; i++ {
		seedAdminTask(t, m, testTaskSeed{id: "task-a-completed-" + twoDigits(i), userID: "user-a", status: TaskStatusCompleted, createdAt: now.Add(time.Duration(i) * time.Second), cost: 10})
	}
	for i := 0; i < 4; i++ {
		seedAdminTask(t, m, testTaskSeed{id: "task-a-queued-" + twoDigits(i), userID: "user-a", status: TaskStatusQueued, createdAt: now.Add(time.Duration(20+i) * time.Second), cost: 5})
	}
	for i := 0; i < 5; i++ {
		seedAdminTask(t, m, testTaskSeed{id: "task-b-completed-" + twoDigits(i), userID: "user-b", status: TaskStatusCompleted, createdAt: now.Add(time.Duration(40+i) * time.Second), cost: 7})
	}
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/admin/tasks?status=completed&userId=user-a&limit=2&offset=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out TaskListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 6 || out.Limit != 2 || out.Offset != 1 || !out.HasMore {
		t.Fatalf("page = %+v", out)
	}
	if len(out.Items) != 2 || out.Items[0].ID != "task-a-completed-04" || out.Items[1].ID != "task-a-completed-03" {
		t.Fatalf("items = %+v", out.Items)
	}
	assertRelatedTasks(t, out.Attempts, out.Reports, out.Items)
	if out.Summary.Total != 6 || out.Summary.Completed != 6 {
		t.Fatalf("filtered summary = %+v", out.Summary)
	}
}

type testTaskSeed struct {
	id        string
	userID    string
	status    string
	createdAt time.Time
	cost      int64
}

func seedAdminTask(t *testing.T, m *Manager, seed testTaskSeed) {
	t.Helper()
	attemptID := "attempt-" + seed.id
	reportID := "report-" + seed.id
	reportStatus := seed.status
	if reportStatus == TaskStatusQueued || reportStatus == TaskStatusRunning {
		reportStatus = TaskStatusCompleted
	}
	if err := m.store.Update(func(db *Database) error {
		db.Tasks[seed.id] = Task{
			ID:             seed.id,
			UserID:         seed.userID,
			Kind:           "llm.chat",
			Model:          "gemma-4-e2b",
			ProfileID:      "profile",
			ProfileVersion: 1,
			ComputeCost:    seed.cost,
			Status:         seed.status,
			CreatedAt:      seed.createdAt,
			UpdatedAt:      seed.createdAt,
		}
		db.Attempts[attemptID] = Attempt{
			ID:             attemptID,
			TaskID:         seed.id,
			UserID:         seed.userID,
			NodeID:         "node-a",
			SlotID:         "node-a/slot0",
			ProfileID:      "profile",
			ProfileVersion: 1,
			ComputeCost:    seed.cost,
			Status:         seed.status,
			StartedAt:      seed.createdAt,
		}
		db.Reports[reportID] = ComputeReport{
			ID:             reportID,
			TaskID:         seed.id,
			AttemptID:      attemptID,
			UserID:         seed.userID,
			NodeID:         "node-a",
			SlotID:         "node-a/slot0",
			ProfileID:      "profile",
			ProfileVersion: 1,
			ComputeCost:    seed.cost,
			Status:         reportStatus,
			CreatedAt:      seed.createdAt,
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func assertRelatedTasks(t *testing.T, attempts []Attempt, reports []ComputeReport, tasks []Task) {
	t.Helper()
	ids := map[string]bool{}
	for _, task := range tasks {
		ids[task.ID] = true
	}
	if len(attempts) != len(tasks) || len(reports) != len(tasks) {
		t.Fatalf("related counts attempts/reports/tasks = %d/%d/%d", len(attempts), len(reports), len(tasks))
	}
	for _, attempt := range attempts {
		if !ids[attempt.TaskID] {
			t.Fatalf("attempt for task outside page: %+v", attempt)
		}
	}
	for _, report := range reports {
		if !ids[report.TaskID] {
			t.Fatalf("report for task outside page: %+v", report)
		}
	}
}

func historyStatus(index int) string {
	switch {
	case index < 20:
		return TaskStatusQueued
	case index < 30:
		return TaskStatusRunning
	case index < 60:
		return TaskStatusCompleted
	case index < 70:
		return TaskStatusFailed
	default:
		return TaskStatusCancelled
	}
}

func taskID(index int) string {
	return "task-" + threeDigits(index)
}

func threeDigits(index int) string {
	if index < 10 {
		return "00" + string(rune('0'+index))
	}
	if index < 100 {
		return "0" + twoDigits(index)
	}
	return twoDigits(index / 10)
}

func twoDigits(index int) string {
	tens := index / 10
	ones := index % 10
	return string(rune('0'+tens)) + string(rune('0'+ones))
}
