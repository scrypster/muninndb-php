package rest

// admin_handler_boost_test.go adds tests for partially-covered admin handler
// paths that are not already exercised by admin_coverage_test.go or
// admin_vault_handlers_test.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scrypster/muninndb/internal/engine"
	"github.com/scrypster/muninndb/internal/engine/vaultjob"
	"github.com/scrypster/muninndb/internal/storage"
)

// ---------------------------------------------------------------------------
// handleReindexFTSVault — 58.8%: missing vault-not-found and generic-error paths
// ---------------------------------------------------------------------------

func TestHandleReindexFTSVault_VaultNotFound(t *testing.T) {
	eng := &reindexNotFoundEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults/missing-vault/reindex-fts", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReindexFTSVault_GenericError(t *testing.T) {
	eng := &reindexErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults/some-vault/reindex-fts", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type reindexNotFoundEngine struct{ MockEngine }

func (e *reindexNotFoundEngine) ReindexFTSVault(_ context.Context, _ string) (int64, error) {
	return 0, fmt.Errorf("reindex: %w", engine.ErrVaultNotFound)
}

type reindexErrorEngine struct{ MockEngine }

func (e *reindexErrorEngine) ReindexFTSVault(_ context.Context, _ string) (int64, error) {
	return 0, errors.New("reindex failed")
}

// ---------------------------------------------------------------------------
// handleReembedVault — 50%: missing vault-not-found and generic-error paths,
// and model override via request body
// ---------------------------------------------------------------------------

func TestHandleReembedVault_InvalidVaultName(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults/INVALID!NAME/reembed", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReembedVault_VaultNotFound(t *testing.T) {
	eng := &reembedNotFoundEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults/missing-vault/reembed", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReembedVault_GenericError(t *testing.T) {
	eng := &reembedErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults/some-vault/reembed", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReembedVault_WithModelOverride(t *testing.T) {
	eng := &reembedCapturingEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	body := `{"model":"bge-small-en-v1.5"}`
	req := httptest.NewRequest("POST", "/api/admin/vaults/test-vault/reembed", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if eng.capturedModel != "bge-small-en-v1.5" {
		t.Errorf("expected model 'bge-small-en-v1.5', got %q", eng.capturedModel)
	}
}

type reembedNotFoundEngine struct{ MockEngine }

func (e *reembedNotFoundEngine) StartReembedVault(_ context.Context, _, _ string) (*vaultjob.Job, error) {
	return nil, fmt.Errorf("reembed: %w", engine.ErrVaultNotFound)
}

type reembedErrorEngine struct{ MockEngine }

func (e *reembedErrorEngine) StartReembedVault(_ context.Context, _, _ string) (*vaultjob.Job, error) {
	return nil, errors.New("reembed failed")
}

type reembedCapturingEngine struct {
	MockEngine
	capturedModel string
}

func (e *reembedCapturingEngine) StartReembedVault(_ context.Context, _, model string) (*vaultjob.Job, error) {
	e.capturedModel = model
	return &vaultjob.Job{ID: "model-override-job"}, nil
}

// ---------------------------------------------------------------------------
// handleExportVault — 52.2%: ErrVaultNotFound path
// ---------------------------------------------------------------------------

func TestHandleExportVault_VaultNotFound(t *testing.T) {
	eng := &exportNotFoundEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("GET", "/api/admin/vaults/missing-vault/export", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleExportVault_PreStreamError(t *testing.T) {
	eng := &exportPreStreamErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("GET", "/api/admin/vaults/some-vault/export", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for pre-stream error, got %d: %s", w.Code, w.Body.String())
	}
}

type exportNotFoundEngine struct{ MockEngine }

func (e *exportNotFoundEngine) ExportVault(_ context.Context, _, _ string, _ int, _ bool, _ io.Writer) (*storage.ExportResult, error) {
	return nil, fmt.Errorf("export: %w", engine.ErrVaultNotFound)
}

type exportPreStreamErrorEngine struct{ MockEngine }

func (e *exportPreStreamErrorEngine) ExportVault(_ context.Context, _, _ string, _ int, _ bool, _ io.Writer) (*storage.ExportResult, error) {
	// Return an error without writing any bytes — this is the pre-stream case.
	return nil, errors.New("pre-stream error")
}

// ---------------------------------------------------------------------------
// handleImportVault — 61.9%: ErrVaultNotFound and ErrVaultNameCollision paths
// ---------------------------------------------------------------------------

func TestHandleImportVault_VaultNotFound(t *testing.T) {
	eng := &importNotFoundEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults/import?vault=missing-vault", strings.NewReader("data"))
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleImportVault_Collision(t *testing.T) {
	eng := &importCollisionEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults/import?vault=existing-vault", strings.NewReader("data"))
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleImportVault_GenericError(t *testing.T) {
	eng := &importErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults/import?vault=some-vault", strings.NewReader("data"))
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type importNotFoundEngine struct{ MockEngine }

func (e *importNotFoundEngine) StartImport(_ context.Context, _, _ string, _ int, _ bool, _ io.Reader) (*vaultjob.Job, error) {
	return nil, fmt.Errorf("import: %w", engine.ErrVaultNotFound)
}

type importCollisionEngine struct{ MockEngine }

func (e *importCollisionEngine) StartImport(_ context.Context, _, _ string, _ int, _ bool, _ io.Reader) (*vaultjob.Job, error) {
	return nil, fmt.Errorf("import: %w", engine.ErrVaultNameCollision)
}

type importErrorEngine struct{ MockEngine }

func (e *importErrorEngine) StartImport(_ context.Context, _, _ string, _ int, _ bool, _ io.Reader) (*vaultjob.Job, error) {
	return nil, errors.New("import failed")
}

// ---------------------------------------------------------------------------
// handlePlugins — 58.3%: nil plugin registry already covered by existing tests.
// Test the populated-registry path (for branch completeness).
// ---------------------------------------------------------------------------

func TestHandleGetPluginConfig_NoDataDirBoost(t *testing.T) {
	eng := &MockEngine{}
	// No data dir → returns empty config.
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("GET", "/api/admin/plugin-config", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleRenameVault — missing branch: ErrVaultNameCollision (86.7% → closer to 100%)
// ---------------------------------------------------------------------------

func TestHandleRenameVault_NameCollision(t *testing.T) {
	eng := &renameCollisionEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	body := strings.NewReader(`{"new_name":"existing-vault"}`)
	req := httptest.NewRequest("POST", "/api/admin/vaults/old-vault/rename", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 on name collision, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRenameVault_GenericError(t *testing.T) {
	eng := &renameGenericErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	body := strings.NewReader(`{"new_name":"new-vault"}`)
	req := httptest.NewRequest("POST", "/api/admin/vaults/old-vault/rename", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on generic error, got %d: %s", w.Code, w.Body.String())
	}
}

type renameCollisionEngine struct{ MockEngine }

func (e *renameCollisionEngine) RenameVault(_ context.Context, _, _ string) error {
	return fmt.Errorf("rename: %w", engine.ErrVaultNameCollision)
}

type renameGenericErrorEngine struct{ MockEngine }

func (e *renameGenericErrorEngine) RenameVault(_ context.Context, _, _ string) error {
	return errors.New("unexpected rename error")
}

// ---------------------------------------------------------------------------
// handleDeleteVault — missing branch: handleDeleteVault with store error (81%)
// (the empty-name branch is tested by TestHandleDeleteVault_Default_NoHeader)
// ---------------------------------------------------------------------------

func TestHandleDeleteVault_StoreError(t *testing.T) {
	eng := &deleteVaultErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("DELETE", "/api/admin/vaults/problem-vault", nil)
	req.Header.Set("X-Allow-Default", "true")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type deleteVaultErrorEngine struct{ MockEngine }

func (e *deleteVaultErrorEngine) DeleteVault(_ context.Context, _ string) error {
	return errors.New("delete vault error")
}

// ---------------------------------------------------------------------------
// handleClearVault — missing branch: store error (77.8%)
// ---------------------------------------------------------------------------

func TestHandleClearVault_StoreError(t *testing.T) {
	eng := &clearVaultErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("POST", "/api/admin/vaults/problem-vault/clear", nil)
	req.Header.Set("X-Allow-Default", "true")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type clearVaultErrorEngine struct{ MockEngine }

func (e *clearVaultErrorEngine) ClearVault(_ context.Context, _ string) error {
	return errors.New("clear vault error")
}

// ---------------------------------------------------------------------------
// handleCloneVault — missing branch: generic error (85.2%)
// ---------------------------------------------------------------------------

func TestHandleCloneVault_GenericError(t *testing.T) {
	eng := &cloneGenericErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	body := `{"new_name":"target-vault"}`
	req := httptest.NewRequest("POST", "/api/admin/vaults/source-vault/clone", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type cloneGenericErrorEngine struct{ MockEngine }

func (e *cloneGenericErrorEngine) StartClone(_ context.Context, _, _ string) (*vaultjob.Job, error) {
	return nil, errors.New("clone failed")
}

// ---------------------------------------------------------------------------
// handleMergeVault — missing branch: generic error (83.3%)
// ---------------------------------------------------------------------------

func TestHandleMergeVault_GenericError(t *testing.T) {
	eng := &mergeGenericErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	body := `{"target":"target-vault"}`
	req := httptest.NewRequest("POST", "/api/admin/vaults/source-vault/merge-into", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type mergeGenericErrorEngine struct{ MockEngine }

func (e *mergeGenericErrorEngine) StartMerge(_ context.Context, _, _ string, _ bool) (*vaultjob.Job, error) {
	return nil, errors.New("merge failed")
}

// ---------------------------------------------------------------------------
// handleStats — with vault param (exercises the vault-scoped code path)
// ---------------------------------------------------------------------------

func TestHandleStats_WithVault(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("GET", "/api/stats?vault=my-vault", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp StatResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.EngramCount != 100 {
		t.Errorf("expected 100 engrams, got %d", resp.EngramCount)
	}
}
