package rest

// auth_handler_boost_test.go adds tests for auth-store-dependent handlers that
// are partially covered. All tests use newTestAuthStore() and newTestServer()
// helpers defined in admin_handlers_test.go.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scrypster/muninndb/internal/auth"
)

// ---------------------------------------------------------------------------
// handleListAPIKeys — 60%: missing vault error branch
// ---------------------------------------------------------------------------

func TestListAPIKeys_DefaultVault(t *testing.T) {
	store := newTestAuthStore(t)
	srv := newTestServer(t, store)

	// No vault param should default to "default".
	req := httptest.NewRequest("GET", "/api/admin/keys", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["keys"]; !ok {
		t.Error("expected 'keys' field in response")
	}
}

// ---------------------------------------------------------------------------
// handleCreateAPIKey — 71.4%: expiry parsing branches
// ---------------------------------------------------------------------------

func TestCreateAPIKey_WithDayExpiry(t *testing.T) {
	store := newTestAuthStore(t)
	srv := newTestServer(t, store)

	body, _ := json.Marshal(map[string]string{
		"vault":   "default",
		"label":   "expiring-key",
		"mode":    "full",
		"expires": "90d",
	})
	req := httptest.NewRequest("POST", "/api/admin/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 for 90d expiry, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_WithYearExpiry(t *testing.T) {
	store := newTestAuthStore(t)
	srv := newTestServer(t, store)

	body, _ := json.Marshal(map[string]string{
		"vault":   "default",
		"label":   "yearly-key",
		"mode":    "full",
		"expires": "1y",
	})
	req := httptest.NewRequest("POST", "/api/admin/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 for 1y expiry, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_WithRFC3339Expiry(t *testing.T) {
	store := newTestAuthStore(t)
	srv := newTestServer(t, store)

	body, _ := json.Marshal(map[string]string{
		"vault":   "default",
		"label":   "dated-key",
		"mode":    "full",
		"expires": "2099-01-01T00:00:00Z",
	})
	req := httptest.NewRequest("POST", "/api/admin/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 for RFC3339 expiry, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_WithDateOnlyExpiry(t *testing.T) {
	store := newTestAuthStore(t)
	srv := newTestServer(t, store)

	body, _ := json.Marshal(map[string]string{
		"vault":   "default",
		"label":   "date-key",
		"mode":    "full",
		"expires": "2099-06-15",
	})
	req := httptest.NewRequest("POST", "/api/admin/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 for date-only expiry, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_InvalidExpiry(t *testing.T) {
	store := newTestAuthStore(t)
	srv := newTestServer(t, store)

	body, _ := json.Marshal(map[string]string{
		"vault":   "default",
		"label":   "bad-expiry-key",
		"mode":    "full",
		"expires": "not-a-valid-expiry",
	})
	req := httptest.NewRequest("POST", "/api/admin/keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid expiry, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handlePutVaultPlasticity — 63.3%: additional validation branches
// ---------------------------------------------------------------------------

func TestPutVaultPlasticity_InvalidJSON(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	req := httptest.NewRequest("PUT", "/api/admin/vault/myvault/plasticity", bytes.NewReader([]byte("{bad")))
	req.SetPathValue("name", "myvault")
	w := httptest.NewRecorder()
	server.handlePutVaultPlasticity(as)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutVaultPlasticity_InvalidHopDepth(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	hopDepth := 9 // > 8 is invalid
	cfg := auth.PlasticityConfig{HopDepth: &hopDepth}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest("PUT", "/api/admin/vault/myvault/plasticity", bytes.NewReader(body))
	req.SetPathValue("name", "myvault")
	w := httptest.NewRecorder()
	server.handlePutVaultPlasticity(as)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for hop_depth > 8, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutVaultPlasticity_InvalidWeightRange(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	bad := float32(1.5) // > 1 is invalid
	cfg := auth.PlasticityConfig{SemanticWeight: &bad}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest("PUT", "/api/admin/vault/myvault/plasticity", bytes.NewReader(body))
	req.SetPathValue("name", "myvault")
	w := httptest.NewRecorder()
	server.handlePutVaultPlasticity(as)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for weight > 1, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutVaultPlasticity_NegativeTemporalHalflife(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	hl := float32(-1.0) // must be > 0
	cfg := auth.PlasticityConfig{TemporalHalflife: &hl}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest("PUT", "/api/admin/vault/myvault/plasticity", bytes.NewReader(body))
	req.SetPathValue("name", "myvault")
	w := httptest.NewRecorder()
	server.handlePutVaultPlasticity(as)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative temporal_halflife, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutVaultPlasticity_InvalidRecallMode(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	mode := "unknown-mode"
	cfg := auth.PlasticityConfig{
		Preset:     "default",
		RecallMode: &mode,
	}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest("PUT", "/api/admin/vault/myvault/plasticity", bytes.NewReader(body))
	req.SetPathValue("name", "myvault")
	w := httptest.NewRecorder()
	server.handlePutVaultPlasticity(as)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid recall_mode, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutVaultPlasticity_InvalidPreset(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	cfg := auth.PlasticityConfig{
		Preset: "super-aggressive-nonexistent",
	}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest("PUT", "/api/admin/vault/myvault/plasticity", bytes.NewReader(body))
	req.SetPathValue("name", "myvault")
	w := httptest.NewRecorder()
	server.handlePutVaultPlasticity(as)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid preset, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutVaultPlasticity_EmptyTraversalProfileClearedToNil(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	// Empty traversal_profile string should be treated as nil (no override).
	empty := ""
	cfg := auth.PlasticityConfig{
		Preset:           "default",
		TraversalProfile: &empty,
	}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest("PUT", "/api/admin/vault/myvault/plasticity", bytes.NewReader(body))
	req.SetPathValue("name", "myvault")
	w := httptest.NewRecorder()
	server.handlePutVaultPlasticity(as)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for empty traversal_profile, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handleGetVaultPlasticity — 71.4%: missing vault-name validation branch
// ---------------------------------------------------------------------------

func TestGetVaultPlasticity_InvalidVaultName(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	req := httptest.NewRequest("GET", "/api/admin/vault/INVALID!/plasticity", nil)
	req.SetPathValue("name", "INVALID!")
	w := httptest.NewRecorder()
	server.handleGetVaultPlasticity(as)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetVaultPlasticity_ValidVault(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	req := httptest.NewRequest("GET", "/api/admin/vault/default/plasticity", nil)
	req.SetPathValue("name", "default")
	w := httptest.NewRecorder()
	server.handleGetVaultPlasticity(as)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handleChangeAdminPassword — 83.3%: error path when user doesn't exist
// ---------------------------------------------------------------------------

func TestChangeAdminPassword_UserNotFound(t *testing.T) {
	store := newTestAuthStore(t)
	srv := newTestServer(t, store)

	body, _ := json.Marshal(map[string]string{
		"username":     "nonexistent-user",
		"new_password": "newpass123",
	})
	req := httptest.NewRequest("PUT", "/api/admin/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	// ChangeAdminPassword on a non-existent user should return an error.
	if w.Code == http.StatusOK {
		t.Error("expected non-200 response for non-existent user, but got 200")
	}
}
