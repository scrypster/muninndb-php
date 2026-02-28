package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	"github.com/scrypster/muninndb/internal/auth"
)

func newTestStore(t *testing.T) *auth.Store {
	t.Helper()
	db, err := pebble.Open("", &pebble.Options{FS: vfs.NewMem()})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return auth.NewStore(db)
}

func TestAuthMiddleware_PublicVaultNoKey(t *testing.T) {
	s := newTestStore(t)
	// Explicitly configure vault as public to allow unauthenticated access.
	// Unconfigured vaults now default to locked (fail-closed).
	s.SetVaultConfig(auth.VaultConfig{Name: "default", Public: true})

	handler := s.VaultAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/engrams?vault=default", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("public vault no key: expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_UnconfiguredVaultNoKey(t *testing.T) {
	s := newTestStore(t)
	// No config stored — unconfigured vaults default to locked (fail-closed).

	handler := s.VaultAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/engrams?vault=default", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("unconfigured vault no key: expected 401 (fail-closed), got %d", w.Code)
	}
}

func TestAuthMiddleware_LockedVaultNoKey(t *testing.T) {
	s := newTestStore(t)
	s.SetVaultConfig(auth.VaultConfig{Name: "secret", Public: false})

	handler := s.VaultAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/engrams?vault=secret", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("locked vault no key: expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	s := newTestStore(t)
	s.SetVaultConfig(auth.VaultConfig{Name: "myv", Public: false})
	token, _, _ := s.GenerateAPIKey("myv", "agent", "observe", nil)

	var capturedMode string
	handler := s.VaultAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		capturedMode, _ = r.Context().Value(auth.ContextMode).(string)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/engrams?vault=myv", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("valid key: expected 200, got %d", w.Code)
	}
	if capturedMode != "observe" {
		t.Errorf("expected mode 'observe' in context, got %q", capturedMode)
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	s := newTestStore(t)

	handler := s.VaultAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/engrams?vault=default", nil)
	req.Header.Set("Authorization", "Bearer mk_thisisnotavalidtoken")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid key: expected 401, got %d", w.Code)
	}
}

func TestObserveFromContext_Defaults(t *testing.T) {
	// Without any context value, ObserveFromContext returns false
	req := httptest.NewRequest("GET", "/", nil)
	if auth.ObserveFromContext(req.Context()) {
		t.Error("expected ObserveFromContext to return false with no context value")
	}
}
