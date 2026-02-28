package rest

// remaining_boost_test.go captures remaining partially-covered handler paths.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scrypster/muninndb/internal/auth"
	"github.com/scrypster/muninndb/internal/engine"
)

// ---------------------------------------------------------------------------
// handleSetVaultConfig — missing: empty vault name defaulting to "default"
// ---------------------------------------------------------------------------

func TestSetVaultConfig_EmptyNameDefaultsToDefault(t *testing.T) {
	store := newTestAuthStore(t)
	srv := newTestServer(t, store)

	// Body with no "name" field — handler should default to "default".
	body, _ := json.Marshal(map[string]interface{}{
		"public": false,
	})
	req := httptest.NewRequest("PUT", "/api/admin/vaults/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["name"] != "default" {
		t.Errorf("expected name='default', got %v", resp["name"])
	}
}

// ---------------------------------------------------------------------------
// handleListAPIKeys — missing: empty vault name defaulting to "default"
// ---------------------------------------------------------------------------

func TestListAPIKeys_DefaultVaultParam(t *testing.T) {
	store := newTestAuthStore(t)
	srv := newTestServer(t, store)

	// No vault param — should default to "default" and succeed.
	req := httptest.NewRequest("GET", "/api/admin/keys", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handleGetVaultPlasticity — missing: empty vault name
// ---------------------------------------------------------------------------

func TestGetVaultPlasticity_EmptyVaultName(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	req := httptest.NewRequest("GET", "/api/admin/vault//plasticity", nil)
	req.SetPathValue("name", "") // explicitly empty
	w := httptest.NewRecorder()
	server.handleGetVaultPlasticity(as)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty vault name, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handlePutVaultPlasticity — missing: empty vault name path
// ---------------------------------------------------------------------------

func TestPutVaultPlasticity_EmptyVaultName(t *testing.T) {
	as := newTestAuthStore(t)
	server := newTestServer(t, as)

	cfg := auth.PlasticityConfig{Preset: "default"}
	body, _ := json.Marshal(cfg)
	req := httptest.NewRequest("PUT", "/api/admin/vault//plasticity", bytes.NewReader(body))
	req.SetPathValue("name", "") // explicitly empty
	w := httptest.NewRecorder()
	server.handlePutVaultPlasticity(as)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty vault name, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handleRenameVault — missing: empty vault name path
// ---------------------------------------------------------------------------

func TestHandleRenameVault_EmptyName(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	body := bytes.NewReader([]byte(`{"new_name":"new-vault"}`))
	req := httptest.NewRequest("POST", "/api/admin/vaults//rename", body)
	req.SetPathValue("name", "")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleRenameVault(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty vault name, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handleReindexFTSVault — missing: empty vault name path
// ---------------------------------------------------------------------------

func TestHandleReindexFTSVault_EmptyName_Boost(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults//reindex-fts", nil)
	req.SetPathValue("name", "")
	w := httptest.NewRecorder()
	server.handleReindexFTSVault(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty vault name, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handleDeleteVault — not-found path (81%)
// ---------------------------------------------------------------------------

func TestHandleDeleteVault_NotFoundVault(t *testing.T) {
	eng := &deleteVaultNotFoundEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("DELETE", "/api/admin/vaults/nonexistent-vault", nil)
	req.Header.Set("X-Allow-Default", "true")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

type deleteVaultNotFoundEngine struct{ MockEngine }

func (e *deleteVaultNotFoundEngine) DeleteVault(_ context.Context, _ string) error {
	return fmt.Errorf("delete: %w", engine.ErrVaultNotFound)
}

// ---------------------------------------------------------------------------
// handleExportVault — reset_metadata path coverage (82.6%)
// ---------------------------------------------------------------------------

func TestHandleExportVault_WithResetMetadata(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("GET", "/api/admin/vaults/test-vault/export?reset_metadata=true", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	// Should succeed (200) since MockEngine.ExportVault writes content.
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
