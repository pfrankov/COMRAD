package comrad

import (
	"fmt"
	"net/http"
	"time"
)

func (m *Manager) handleAdminProfileComputeCost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req SetProfileComputeCostRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	profile, err := m.setProfileComputeCost(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "profile_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (m *Manager) setProfileComputeCost(req SetProfileComputeCostRequest) (WorkloadProfile, error) {
	var profile WorkloadProfile
	err := m.store.Update(func(db *Database) error {
		var ok bool
		profile, ok = db.Profiles[req.ProfileID]
		if !ok {
			return fmt.Errorf("profile %s not found", req.ProfileID)
		}
		if req.ComputeCost < 0 {
			return fmt.Errorf("computeCost must be non-negative")
		}
		profile.ComputeCost = req.ComputeCost
		profile.Version++
		db.Profiles[profile.ID] = profile
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "profile.compute_cost.updated", Actor: "admin", Subject: profile.ID, CreatedAt: time.Now().UTC()})
		return nil
	})
	return profile, err
}

func (m *Manager) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, SortedUsers(m.store.Snapshot()))
	case http.MethodPost:
		m.handleAdminUserCreate(w, r)
	case http.MethodPut:
		m.handleAdminUserUpdate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *Manager) handleAdminUserCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	user := newUser(req)
	err := m.store.Update(func(db *Database) error {
		if _, ok := db.Users[user.ID]; ok {
			return fmt.Errorf("user %s already exists", user.ID)
		}
		db.Users[user.ID] = user
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "user.created", Actor: "admin", Subject: user.ID, CreatedAt: user.CreatedAt})
		return nil
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (m *Manager) handleAdminUserUpdate(w http.ResponseWriter, r *http.Request) {
	var req UpdateUserRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	user, err := m.updateUser(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (m *Manager) updateUser(req UpdateUserRequest) (User, error) {
	var user User
	err := m.store.Update(func(db *Database) error {
		if req.ID == "" {
			return fmt.Errorf("userId is required")
		}
		var ok bool
		user, ok = db.Users[req.ID]
		if !ok {
			return fmt.Errorf("user %s not found", req.ID)
		}
		if req.Name != "" {
			user.Name = req.Name
		}
		user.Disabled = req.Disabled
		db.Users[user.ID] = user
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "user.updated", Actor: "admin", Subject: user.ID, CreatedAt: time.Now().UTC()})
		return nil
	})
	return user, err
}

func newUser(req CreateUserRequest) User {
	user := User{ID: req.ID, Name: req.Name, CreatedAt: time.Now().UTC()}
	if user.ID == "" {
		user.ID = NewID("usr")
	}
	if user.Name == "" {
		user.Name = user.ID
	}
	return user
}

func (m *Manager) handleAdminAPIKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, SortedAPIKeyViews(m.store.Snapshot()))
	case http.MethodPost:
		m.handleAdminAPIKeyIssue(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *Manager) handleAdminAPIKeyIssue(w http.ResponseWriter, r *http.Request) {
	var req IssueAPIKeyRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	token, key, err := newIssuedAPIKey(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", err.Error())
		return
	}
	if err := m.storeAPIKey(key); err != nil {
		writeError(w, http.StatusBadRequest, "api_key_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, IssueAPIKeyResponse{APIKey: apiKeyView(key), Token: token})
}

func (m *Manager) handleAdminAPIKeyLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req APIKeyLookupRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	out, err := m.lookupAPIKey(req.Token)
	if err != nil {
		writeError(w, http.StatusNotFound, "api_key_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (m *Manager) lookupAPIKey(token string) (APIKeyLookupResponse, error) {
	if token == "" {
		return APIKeyLookupResponse{}, fmt.Errorf("token is required")
	}
	hash := apiTokenHash(token)
	var out APIKeyLookupResponse
	err := m.store.View(func(db Database) error {
		key, ok := activeAPIKeyForHash(db, hash)
		if !ok {
			return fmt.Errorf("api key not found")
		}
		user, ok := db.Users[key.UserID]
		if !ok {
			return fmt.Errorf("user %s not found", key.UserID)
		}
		out = APIKeyLookupResponse{APIKey: apiKeyView(key), User: user}
		return nil
	})
	return out, err
}

func newIssuedAPIKey(req IssueAPIKeyRequest) (string, APIKey, error) {
	token, err := newAPIToken()
	if err != nil {
		return "", APIKey{}, err
	}
	key := APIKey{ID: NewID("key"), UserID: req.UserID, Name: req.Name, TokenHash: apiTokenHash(token), Status: APIKeyStatusActive, CreatedAt: time.Now().UTC()}
	if key.Name == "" {
		key.Name = key.ID
	}
	return token, key, nil
}

func (m *Manager) storeAPIKey(key APIKey) error {
	return m.store.Update(func(db *Database) error {
		if _, ok := db.Users[key.UserID]; !ok {
			return fmt.Errorf("user %s not found", key.UserID)
		}
		db.APIKeys[key.ID] = key
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "api_key.issued", Actor: "admin", Subject: key.ID, CreatedAt: key.CreatedAt})
		return nil
	})
}

func (m *Manager) handleAdminAPIKeyRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req RevokeAPIKeyRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := m.revokeAPIKey(req.ID); err != nil {
		writeError(w, http.StatusBadRequest, "api_key_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (m *Manager) revokeAPIKey(id string) error {
	now := time.Now().UTC()
	return m.store.Update(func(db *Database) error {
		key, ok := db.APIKeys[id]
		if !ok {
			return fmt.Errorf("api key %s not found", id)
		}
		key.Status = APIKeyStatusRevoked
		key.RevokedAt = &now
		db.APIKeys[key.ID] = key
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "api_key.revoked", Actor: "admin", Subject: key.ID, CreatedAt: now})
		return nil
	})
}

func (m *Manager) handleAdminBalanceAdjustment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req AdminBalanceAdjustmentRequest
	if err := readConfig(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	entry, err := adminAdjustmentEntry(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ledger_invalid", err.Error())
		return
	}
	if err := m.storeAdminAdjustment(entry); err != nil {
		writeError(w, http.StatusBadRequest, "ledger_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (m *Manager) storeAdminAdjustment(entry ComputeLedgerEntry) error {
	return m.store.Update(func(db *Database) error {
		if _, ok := db.Users[entry.UserID]; !ok {
			return fmt.Errorf("user %s not found", entry.UserID)
		}
		appendLedgerEntry(db, entry)
		db.Audit = append(db.Audit, AuditEvent{ID: NewID("aud"), Type: "compute.admin_adjustment", Actor: "admin", Subject: entry.UserID, CreatedAt: entry.CreatedAt})
		return nil
	})
}

func adminAdjustmentEntry(req AdminBalanceAdjustmentRequest) (ComputeLedgerEntry, error) {
	if req.UserID == "" {
		return ComputeLedgerEntry{}, fmt.Errorf("userId is required")
	}
	if req.Amount == 0 {
		return ComputeLedgerEntry{}, fmt.Errorf("amount must be non-zero")
	}
	direction, amount := LedgerCredit, req.Amount
	if amount < 0 {
		direction, amount = LedgerDebit, -amount
	}
	reason := req.Reason
	if reason == "" {
		reason = "admin adjustment"
	}
	return ComputeLedgerEntry{Type: LedgerAdminAdjustment, UserID: req.UserID, Amount: amount, Direction: direction, Reason: reason, CreatedAt: time.Now().UTC()}, nil
}
