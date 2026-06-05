package comrad

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminPlacementExplainEndpointRequiresAdminAndReturnsDryRun(t *testing.T) {
	manager := newPlacementExplainEndpointManager(t)
	server := httptest.NewServer(manager.Handler())
	defer server.Close()

	assertPlacementExplainUnauthorized(t, server.URL)
	explain := getPlacementExplain(t, server.URL)
	if len(explain.Plan) != 1 || len(explain.Profiles) != 1 {
		t.Fatalf("placement explain response = %+v, want one plan assignment and one profile explanation", explain)
	}
}

func newPlacementExplainEndpointManager(t *testing.T) *Manager {
	t.Helper()
	manager := newTestManager(t, 4, time.Second, 3)
	db := explainEndpointDatabase(time.Now().UTC())
	if err := manager.store.Update(func(existing *Database) error {
		mergePlacementExplainEndpointData(existing, db)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return manager
}

func explainEndpointDatabase(now time.Time) Database {
	db := explainTestDatabase(now)
	addExplainPlacementProfile(&db, "profile", "sha256:profile", 4, 4, now)
	addExplainPlacementPolicy(&db, "profile", PlacementPolicy{ID: "pol-profile", ProfileID: "profile", CachedCount: 1, WarmCount: 1, CreatedAt: now, UpdatedAt: now})
	addExplainPlacementNode(&db, "node-good", 16, 30, 1)
	return db
}

func assertPlacementExplainUnauthorized(t *testing.T, serverURL string) {
	t.Helper()
	resp, err := http.Get(serverURL + "/api/admin/placement/explain")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("placement explain without admin token = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func getPlacementExplain(t *testing.T, serverURL string) PlacementExplainResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/admin/placement/explain", nil)
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
		t.Fatalf("placement explain status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var explain PlacementExplainResponse
	if err := json.NewDecoder(resp.Body).Decode(&explain); err != nil {
		t.Fatal(err)
	}
	return explain
}

func mergePlacementExplainEndpointData(existing *Database, db Database) {
	for id, artifact := range db.Artifacts {
		existing.Artifacts[id] = artifact
	}
	for id, profile := range db.Profiles {
		existing.Profiles[id] = profile
	}
	for id, policy := range db.Policies {
		existing.Policies[id] = policy
	}
	for id, node := range db.Nodes {
		existing.Nodes[id] = node
	}
	for id, slot := range db.Slots {
		existing.Slots[id] = slot
	}
}
