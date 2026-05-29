package comrad

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

const defaultClientUserID = "user_configured_client"

type userContextKey struct{}

func (m *Manager) ensureConfiguredClientKey() error {
	if m.cfg.ClientAPIKey == "" {
		return nil
	}
	hash := apiTokenHash(m.cfg.ClientAPIKey)
	return m.store.Update(func(db *Database) error {
		if apiKeyUserID(*db, hash) != "" {
			return nil
		}
		now := time.Now().UTC()
		if _, ok := db.Users[defaultClientUserID]; !ok {
			db.Users[defaultClientUserID] = User{ID: defaultClientUserID, Name: "Configured API client", CreatedAt: now}
		}
		db.APIKeys["key_configured_client"] = APIKey{ID: "key_configured_client", UserID: defaultClientUserID, Name: "Configured API key", TokenHash: hash, Status: APIKeyStatusActive, CreatedAt: now}
		return nil
	})
}

func (m *Manager) clientOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := m.authenticateClient(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid client API key required")
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey{}, user)
		next(w, r.WithContext(ctx))
	}
}

func (m *Manager) authenticateClient(r *http.Request) (User, bool) {
	token := bearerToken(r)
	if token == "" {
		return User{}, false
	}
	var out User
	hash := apiTokenHash(token)
	err := m.store.Update(func(db *Database) error {
		key, ok := activeAPIKeyForHash(*db, hash)
		if !ok {
			return fmt.Errorf("unauthorized")
		}
		user, ok := db.Users[key.UserID]
		if !ok || user.Disabled {
			return fmt.Errorf("unauthorized")
		}
		now := time.Now().UTC()
		key.LastUsedAt = &now
		db.APIKeys[key.ID] = key
		out = user
		return nil
	})
	return out, err == nil
}

func requestUser(r *http.Request) User {
	user, _ := r.Context().Value(userContextKey{}).(User)
	return user
}

func activeAPIKeyForHash(db Database, hash string) (APIKey, bool) {
	var found APIKey
	for _, key := range db.APIKeys {
		if key.TokenHash != hash || key.Status != APIKeyStatusActive || key.RevokedAt != nil {
			continue
		}
		if found.ID != "" {
			return APIKey{}, false
		}
		found = key
	}
	return found, found.ID != ""
}

func apiKeyUserID(db Database, hash string) string {
	key, ok := activeAPIKeyForHash(db, hash)
	if !ok {
		return ""
	}
	return key.UserID
}

func apiTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func newAPIToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "rgk_" + hex.EncodeToString(b[:]), nil
}

func apiKeyView(key APIKey) APIKeyView {
	return APIKeyView{ID: key.ID, UserID: key.UserID, Name: key.Name, Status: key.Status, CreatedAt: key.CreatedAt, RevokedAt: key.RevokedAt, LastUsedAt: key.LastUsedAt}
}
