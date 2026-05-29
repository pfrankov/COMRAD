package comrad

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUserRegistrationAPIKeysAndClientAuth(t *testing.T) {
	m := newTestManager(t, 2, time.Second, 3)
	seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	user := adminJSON[User](t, server.URL, "admin", "/api/admin/users", CreateUserRequest{Name: "Research user"})
	issued := adminJSON[IssueAPIKeyResponse](t, server.URL, "admin", "/api/admin/api-keys", IssueAPIKeyRequest{UserID: user.ID, Name: "cli"})
	if user.ID == "" || issued.Token == "" || issued.APIKey.UserID != user.ID || issued.APIKey.Status != APIKeyStatusActive {
		t.Fatalf("unexpected registration response user=%+v key=%+v", user, issued)
	}
	body := mustJSON(t, issued)
	if bytes.Contains(body, []byte("tokenHash")) {
		t.Fatalf("API key response leaked token hash: %s", body)
	}

	assertClientStatus(t, server.URL, "", http.StatusUnauthorized)
	assertClientStatus(t, server.URL, "invalid", http.StatusUnauthorized)
	assertClientStatus(t, server.URL, issued.Token, http.StatusOK)
}

func TestAdminCanEditAPIClientAndFindByRawKey(t *testing.T) {
	m := newTestManager(t, 2, time.Second, 3)
	seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()

	user := adminJSON[User](t, server.URL, "admin", "/api/admin/users", CreateUserRequest{Name: "Initial client"})
	issued := adminJSON[IssueAPIKeyResponse](t, server.URL, "admin", "/api/admin/api-keys", IssueAPIKeyRequest{UserID: user.ID, Name: "production"})
	lookup := adminJSON[APIKeyLookupResponse](t, server.URL, "admin", "/api/admin/api-keys/lookup", APIKeyLookupRequest{Token: issued.Token})
	if lookup.User.ID != user.ID || lookup.APIKey.ID != issued.APIKey.ID {
		t.Fatalf("lookup=%+v want user=%s key=%s", lookup, user.ID, issued.APIKey.ID)
	}
	body := mustJSON(t, lookup)
	if bytes.Contains(body, []byte("tokenHash")) || bytes.Contains(body, []byte(issued.Token)) {
		t.Fatalf("lookup leaked sensitive key material: %s", body)
	}

	edited := adminMethodJSON[User](t, http.MethodPut, server.URL, "admin", "/api/admin/users", UpdateUserRequest{ID: user.ID, Name: "Production client", Disabled: true})
	if edited.Name != "Production client" || !edited.Disabled {
		t.Fatalf("edited user=%+v", edited)
	}
	assertClientStatus(t, server.URL, issued.Token, http.StatusUnauthorized)

	enabled := adminMethodJSON[User](t, http.MethodPut, server.URL, "admin", "/api/admin/users", UpdateUserRequest{ID: user.ID, Name: "Production client", Disabled: false})
	if enabled.Disabled {
		t.Fatalf("enabled user still disabled: %+v", enabled)
	}
	assertClientStatus(t, server.URL, issued.Token, http.StatusOK)
}

func TestTaskAttemptReportAndLedgerUseRequestingUser(t *testing.T) {
	m := newTestManager(t, 4, 2*time.Second, 3)
	profile := seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	consumer, consumerToken := createUserWithKey(t, server.URL, "consumer")
	producer, _ := createUserWithKey(t, server.URL, "producer")
	setProfileCost(t, server.URL, profile.ID, 7)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	setNodeOwner(t, m, "node-a", producer.ID)

	result := startStreamingChat(t, server.URL, consumerToken, "assistant", nilContext())
	payload := nextExecute(t, session)
	completePayload(m, payload, "paid")
	out := <-result
	if out.status != http.StatusOK {
		t.Fatalf("stream status=%d body=%s", out.status, out.body)
	}

	state := getState(t, server.URL, "admin")
	task, attempt, report := state.Tasks[0], state.Attempts[0], state.Reports[0]
	if task.UserID != consumer.ID || attempt.UserID != consumer.ID || report.UserID != consumer.ID {
		t.Fatalf("user ids task=%q attempt=%q report=%q want %q", task.UserID, attempt.UserID, report.UserID, consumer.ID)
	}
	if attempt.ComputeCost != 7 || report.ComputeCost != 7 || task.ComputeCost != 7 {
		t.Fatalf("costs task=%d attempt=%d report=%d", task.ComputeCost, attempt.ComputeCost, report.ComputeCost)
	}
	assertLedgerPair(t, state.ComputeLedger, consumer.ID, producer.ID, task.ID, attempt.ID, report.ID, 7)
	assertUserBalance(t, state.Users, consumer.ID, -7)
	assertUserBalance(t, state.Users, producer.ID, 7)
}

func TestFailedAttemptDoesNotChargeByDefault(t *testing.T) {
	m := newTestManager(t, 4, 2*time.Second, 3)
	profile := seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	consumer, token := createUserWithKey(t, server.URL, "consumer")
	setProfileCost(t, server.URL, profile.ID, 5)
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)

	result := startStreamingChat(t, server.URL, token, "assistant", nilContext())
	payload := nextExecute(t, session)
	if err := m.handleComputeReport("node-a", ComputeReport{TaskID: payload.TaskID, AttemptID: payload.AttemptID, NodeID: "node-a", SlotID: payload.SlotID, ProfileID: payload.Profile.ID, Status: TaskStatusFailed, Phase: "runtime", FailureReason: FailureRuntimeError, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	<-result

	state := getState(t, server.URL, "admin")
	if len(state.ComputeLedger) != 0 {
		t.Fatalf("failed attempt charged ledger entries: %+v", state.ComputeLedger)
	}
	assertUserBalance(t, state.Users, consumer.ID, 0)
}

func TestManualAdjustmentAndBalanceEnforcement(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	m.cfg.EnforceBalance = true
	profile := seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	user, token := createUserWithKey(t, server.URL, "buyer")
	setProfileCost(t, server.URL, profile.ID, 3)

	blocked := doChatOnce(t, server.URL, token, "assistant")
	if blocked.status != http.StatusPaymentRequired || !bytes.Contains([]byte(blocked.body), []byte("insufficient_balance")) {
		t.Fatalf("blocked status=%d body=%s", blocked.status, blocked.body)
	}
	adminJSON[ComputeLedgerEntry](t, server.URL, "admin", "/api/admin/users/adjust-balance", AdminBalanceAdjustmentRequest{UserID: user.ID, Amount: 3, Reason: "test top-up"})
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)
	result := startStreamingChat(t, server.URL, token, "assistant", nilContext())
	payload := nextExecute(t, session)
	completePayload(m, payload, "allowed")
	if out := <-result; out.status != http.StatusOK {
		t.Fatalf("allowed status=%d body=%s", out.status, out.body)
	}

	state := getState(t, server.URL, "admin")
	assertUserBalance(t, state.Users, user.ID, 0)
	if !hasLedgerType(state.ComputeLedger, LedgerAdminAdjustment) {
		t.Fatalf("missing admin adjustment ledger entry: %+v", state.ComputeLedger)
	}
}

func TestBalanceEnforcementCountsPendingTaskCost(t *testing.T) {
	m := newTestManager(t, 4, 200*time.Millisecond, 3)
	m.cfg.EnforceBalance = true
	profile := seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	user, token := createUserWithKey(t, server.URL, "buyer")
	setProfileCost(t, server.URL, profile.ID, 3)
	adminJSON[ComputeLedgerEntry](t, server.URL, "admin", "/api/admin/users/adjust-balance", AdminBalanceAdjustmentRequest{UserID: user.ID, Amount: 3, Reason: "test top-up"})
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)

	first := startStreamingChat(t, server.URL, token, "assistant", nilContext())
	payload := nextExecute(t, session)
	blocked := doChatOnce(t, server.URL, token, "assistant")
	if blocked.status != http.StatusPaymentRequired || !bytes.Contains([]byte(blocked.body), []byte("insufficient_balance")) {
		t.Fatalf("blocked status=%d body=%s", blocked.status, blocked.body)
	}
	completePayload(m, payload, "allowed")
	if out := <-first; out.status != http.StatusOK {
		t.Fatalf("first status=%d body=%s", out.status, out.body)
	}
}

func TestZeroCostRequestWorksWithZeroBalanceWhenEnforcementEnabled(t *testing.T) {
	m := newTestManager(t, 4, time.Second, 3)
	m.cfg.EnforceBalance = true
	profile := seedBasicProfile(t, m)
	server := httptest.NewServer(m.Handler())
	defer server.Close()
	user, token := createUserWithKey(t, server.URL, "free-user")
	session := addReadySession(t, m, "node-a", "node-a/slot0", profile)

	result := startStreamingChat(t, server.URL, token, "assistant", nilContext())
	payload := nextExecute(t, session)
	completePayload(m, payload, "free")
	if out := <-result; out.status != http.StatusOK {
		t.Fatalf("free status=%d body=%s", out.status, out.body)
	}
	state := getState(t, server.URL, "admin")
	assertUserBalance(t, state.Users, user.ID, 0)
	if len(state.ComputeLedger) != 0 {
		t.Fatalf("zero-cost task created ledger entries: %+v", state.ComputeLedger)
	}
}

func TestProfileComputeCostDefaultsToZero(t *testing.T) {
	m := newTestManager(t, 2, time.Second, 3)
	profile := seedBasicProfile(t, m)
	if profile.ComputeCost != 0 {
		t.Fatalf("compute cost default = %d", profile.ComputeCost)
	}
}

func assertClientStatus(t *testing.T, baseURL, token string, want int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("client status = %d, want %d", resp.StatusCode, want)
	}
}

func createUserWithKey(t *testing.T, baseURL, name string) (User, string) {
	t.Helper()
	user := adminJSON[User](t, baseURL, "admin", "/api/admin/users", CreateUserRequest{Name: name})
	key := adminJSON[IssueAPIKeyResponse](t, baseURL, "admin", "/api/admin/api-keys", IssueAPIKeyRequest{UserID: user.ID, Name: "test"})
	return user, key.Token
}

func setProfileCost(t *testing.T, baseURL, profileID string, cost int64) {
	t.Helper()
	adminJSON[WorkloadProfile](t, baseURL, "admin", "/api/admin/profiles/compute-cost", SetProfileComputeCostRequest{ProfileID: profileID, ComputeCost: cost})
}

func setNodeOwner(t *testing.T, m *Manager, nodeID, userID string) {
	t.Helper()
	if err := m.store.Update(func(db *Database) error {
		node := db.Nodes[nodeID]
		node.OwnerUserID = userID
		db.Nodes[nodeID] = node
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func nilContext() context.Context {
	return context.Background()
}

func assertLedgerPair(t *testing.T, ledger []ComputeLedgerEntry, consumer, producer, taskID, attemptID, reportID string, amount int64) {
	t.Helper()
	var debit, credit bool
	for _, entry := range ledger {
		matches := entry.TaskID == taskID && entry.AttemptID == attemptID && entry.ReportID == reportID && entry.Amount == amount
		debit = debit || matches && entry.UserID == consumer && entry.Type == LedgerConsumeCompute && entry.Direction == LedgerDebit
		credit = credit || matches && entry.UserID == producer && entry.Type == LedgerProduceCompute && entry.Direction == LedgerCredit
	}
	if !debit || !credit {
		t.Fatalf("missing ledger pair debit=%t credit=%t entries=%+v", debit, credit, ledger)
	}
}

func assertUserBalance(t *testing.T, users []User, userID string, want int64) {
	t.Helper()
	for _, user := range users {
		if user.ID == userID {
			if user.ComputeBalance != want {
				t.Fatalf("balance for %s = %d, want %d", userID, user.ComputeBalance, want)
			}
			return
		}
	}
	t.Fatalf("user %s not found in %+v", userID, users)
}

func hasLedgerType(ledger []ComputeLedgerEntry, kind string) bool {
	for _, entry := range ledger {
		if entry.Type == kind {
			return true
		}
	}
	return false
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
