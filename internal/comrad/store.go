package comrad

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const CurrentSchemaVersion = 3

type Database struct {
	SchemaVersion     int                               `json:"schemaVersion"`
	Migrations        []string                          `json:"migrations"`
	Settings          GlobalSettings                    `json:"settings"`
	Nodes             map[string]Node                   `json:"nodes"`
	Slots             map[string]Slot                   `json:"slots"`
	Artifacts         map[string]Artifact               `json:"artifacts"`
	ArtifactEvictions map[string]ArtifactEvictionRecord `json:"artifactEvictions,omitempty"`
	CacheIntents      map[string]CacheIntentRecord      `json:"cacheIntents,omitempty"`
	Profiles          map[string]WorkloadProfile        `json:"profiles"`
	Policies          map[string]PlacementPolicy        `json:"policies"`
	Assignments       map[string]PlacementAssignment    `json:"assignments"`
	Tasks             map[string]Task                   `json:"tasks"`
	Attempts          map[string]Attempt                `json:"attempts"`
	Reports           map[string]ComputeReport          `json:"reports"`
	Updates           map[string]UpdateRecord           `json:"updates"`
	Users             map[string]User                   `json:"users"`
	APIKeys           map[string]APIKey                 `json:"apiKeys"`
	NodeTokenHashes   map[string]string                 `json:"nodeTokenHashes,omitempty"`
	ComputeLedger     []ComputeLedgerEntry              `json:"computeLedger"`
	Audit             []AuditEvent                      `json:"audit"`
}

type GlobalSettings struct {
	P2PEnabled bool `json:"p2pEnabled"`
}

type Store struct {
	mu          sync.RWMutex
	backend     storeBackend
	db          Database
	afterUpdate func()
}

func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store path is required")
	}
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		return openStoreWithBackend(newJSONFileBackend(path))
	}
	return OpenSQLiteStore(path)
}

func openStoreWithBackend(backend storeBackend) (*Store, error) {
	s := &Store{backend: backend}
	db, ok, err := backend.Load()
	if err != nil {
		return nil, err
	}
	if !ok {
		s.db = emptyDatabase()
		s.auditLocked("migration.applied", "system", "schema", map[string]any{"version": CurrentSchemaVersion})
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		return s, nil
	}
	s.db = db
	if err := s.migrateLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

func emptyDatabase() Database {
	db := Database{SchemaVersion: CurrentSchemaVersion, Settings: GlobalSettings{P2PEnabled: true}}
	ensureMaps(&db)
	db.Migrations = append(db.Migrations, fmt.Sprintf("schema_%d", CurrentSchemaVersion))
	return db
}

func ensureMaps(db *Database) {
	ensureInventoryMaps(db)
	ensureExecutionMaps(db)
	ensureAccountingMaps(db)
}

func ensureInventoryMaps(db *Database) {
	if db.Nodes == nil {
		db.Nodes = map[string]Node{}
	}
	if db.Slots == nil {
		db.Slots = map[string]Slot{}
	}
	if db.Artifacts == nil {
		db.Artifacts = map[string]Artifact{}
	}
	if db.ArtifactEvictions == nil {
		db.ArtifactEvictions = map[string]ArtifactEvictionRecord{}
	}
	if db.CacheIntents == nil {
		db.CacheIntents = map[string]CacheIntentRecord{}
	}
	if db.Profiles == nil {
		db.Profiles = map[string]WorkloadProfile{}
	}
	if db.Policies == nil {
		db.Policies = map[string]PlacementPolicy{}
	}
	if db.Assignments == nil {
		db.Assignments = map[string]PlacementAssignment{}
	}
	if db.NodeTokenHashes == nil {
		db.NodeTokenHashes = map[string]string{}
	}
}

func ensureExecutionMaps(db *Database) {
	if db.Tasks == nil {
		db.Tasks = map[string]Task{}
	}
	if db.Attempts == nil {
		db.Attempts = map[string]Attempt{}
	}
	if db.Reports == nil {
		db.Reports = map[string]ComputeReport{}
	}
	if db.Updates == nil {
		db.Updates = map[string]UpdateRecord{}
	}
}

func ensureAccountingMaps(db *Database) {
	if db.Users == nil {
		db.Users = map[string]User{}
	}
	if db.APIKeys == nil {
		db.APIKeys = map[string]APIKey{}
	}
	if db.ComputeLedger == nil {
		db.ComputeLedger = []ComputeLedgerEntry{}
	}
}

func (s *Store) migrateLocked() error {
	ensureMaps(&s.db)
	if s.db.SchemaVersion > CurrentSchemaVersion {
		return fmt.Errorf("database schema %d is newer than binary schema %d", s.db.SchemaVersion, CurrentSchemaVersion)
	}
	if s.db.SchemaVersion == CurrentSchemaVersion {
		return nil
	}
	if s.db.SchemaVersion < 3 {
		s.db.Settings = GlobalSettings{P2PEnabled: true}
	}
	s.db.SchemaVersion = CurrentSchemaVersion
	s.db.Migrations = append(s.db.Migrations, fmt.Sprintf("schema_%d", CurrentSchemaVersion))
	s.auditLocked("migration.applied", "system", "schema", map[string]any{"version": CurrentSchemaVersion})
	return s.saveLocked()
}

func (s *Store) Path() string {
	return s.backend.Path()
}

func (s *Store) BackendName() string {
	return s.backend.Name()
}

func (s *Store) Snapshot() Database {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneDatabase(s.db)
}

func (s *Store) View(fn func(Database) error) error {
	s.mu.RLock()
	db := cloneDatabase(s.db)
	s.mu.RUnlock()
	return fn(db)
}

func (s *Store) SetAfterUpdate(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.afterUpdate = fn
}

func (s *Store) Update(fn func(*Database) error) error {
	s.mu.Lock()
	ensureMaps(&s.db)
	if err := fn(&s.db); err != nil {
		s.mu.Unlock()
		return err
	}
	err := s.saveLocked()
	afterUpdate := s.afterUpdate
	s.mu.Unlock()
	if err == nil && afterUpdate != nil {
		afterUpdate()
	}
	return err
}

func (s *Store) Audit(eventType, actor, subject string, metadata map[string]any) error {
	return s.Update(func(db *Database) error {
		db.Audit = append(db.Audit, AuditEvent{
			ID:        NewID("aud"),
			Type:      eventType,
			Actor:     actor,
			Subject:   subject,
			Metadata:  metadata,
			CreatedAt: time.Now().UTC(),
		})
		return nil
	})
}

func (s *Store) auditLocked(eventType, actor, subject string, metadata map[string]any) {
	s.db.Audit = append(s.db.Audit, AuditEvent{
		ID:        NewID("aud"),
		Type:      eventType,
		Actor:     actor,
		Subject:   subject,
		Metadata:  metadata,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Store) saveLocked() error {
	return s.backend.Save(s.db)
}

func SortedNodes(db Database) []Node {
	out := make([]Node, 0, len(db.Nodes))
	for _, v := range db.Nodes {
		out = append(out, v)
	}
	sortNodes(out)
	return out
}

func SortedSlots(db Database) []Slot {
	out := make([]Slot, 0, len(db.Slots))
	for _, v := range db.Slots {
		out = append(out, v)
	}
	sortSlots(out)
	return out
}

func SortedProfiles(db Database) []WorkloadProfile {
	out := make([]WorkloadProfile, 0, len(db.Profiles))
	for _, v := range db.Profiles {
		out = append(out, v)
	}
	sortProfiles(out)
	return out
}

func SortedArtifacts(db Database) []Artifact {
	out := make([]Artifact, 0, len(db.Artifacts))
	for _, v := range db.Artifacts {
		out = append(out, v)
	}
	sortArtifacts(out)
	return out
}

func SortedPolicies(db Database) []PlacementPolicy {
	out := make([]PlacementPolicy, 0, len(db.Policies))
	for _, v := range db.Policies {
		out = append(out, v)
	}
	sortPolicies(out)
	return out
}

func SortedAssignments(db Database) []PlacementAssignment {
	out := make([]PlacementAssignment, 0, len(db.Assignments))
	for _, v := range db.Assignments {
		out = append(out, v)
	}
	sortAssignments(out)
	return out
}

func SortedAttempts(db Database) []Attempt {
	out := make([]Attempt, 0, len(db.Attempts))
	for _, v := range db.Attempts {
		out = append(out, v)
	}
	sortAttempts(out)
	return out
}

func SortedReports(db Database) []ComputeReport {
	out := make([]ComputeReport, 0, len(db.Reports))
	for _, v := range db.Reports {
		out = append(out, v)
	}
	sortReports(out)
	return out
}

func SortedUsers(db Database) []User {
	out := make([]User, 0, len(db.Users))
	for _, v := range db.Users {
		out = append(out, v)
	}
	sortUsers(out)
	return out
}

func SortedAPIKeyViews(db Database) []APIKeyView {
	out := make([]APIKeyView, 0, len(db.APIKeys))
	for _, v := range db.APIKeys {
		out = append(out, apiKeyView(v))
	}
	sortAPIKeyViews(out)
	return out
}

func SortedComputeLedger(db Database) []ComputeLedgerEntry {
	out := append([]ComputeLedgerEntry(nil), db.ComputeLedger...)
	sortComputeLedger(out)
	return out
}
