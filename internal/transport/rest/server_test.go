package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/cognitive"
	"github.com/scrypster/muninndb/internal/config"
	"github.com/scrypster/muninndb/internal/engine/trigger"
	"github.com/scrypster/muninndb/internal/engine/vaultjob"
	"github.com/scrypster/muninndb/internal/replication"
	"github.com/scrypster/muninndb/internal/storage"
	mbp "github.com/scrypster/muninndb/internal/transport/mbp"
)

// MockEngine is a mock implementation of EngineAPI for testing.
type MockEngine struct{}

func (m *MockEngine) Hello(ctx context.Context, req *HelloRequest) (*HelloResponse, error) {
	return &HelloResponse{
		ServerVersion: "1.0.0",
		SessionID:     "test-session",
		VaultID:       "test-vault",
		Capabilities:  []string{"compression"},
	}, nil
}

func (m *MockEngine) Write(ctx context.Context, req *WriteRequest) (*WriteResponse, error) {
	return &WriteResponse{
		ID:        "test-id",
		CreatedAt: 1234567890,
	}, nil
}

func (m *MockEngine) WriteBatch(ctx context.Context, reqs []*WriteRequest) ([]*WriteResponse, []error) {
	responses := make([]*WriteResponse, len(reqs))
	errs := make([]error, len(reqs))
	for i := range reqs {
		responses[i] = &WriteResponse{
			ID:        fmt.Sprintf("batch-id-%d", i),
			CreatedAt: 1234567890,
		}
	}
	return responses, errs
}

func (m *MockEngine) Read(ctx context.Context, req *ReadRequest) (*ReadResponse, error) {
	return &ReadResponse{
		ID:         "test-id",
		Concept:    "test",
		Content:    "test content",
		Confidence: 0.9,
	}, nil
}

func (m *MockEngine) Activate(ctx context.Context, req *ActivateRequest) (*ActivateResponse, error) {
	return &ActivateResponse{
		QueryID:    "query-1",
		TotalFound: 1,
		Activations: []ActivationItem{
			{
				ID:      "test-id",
				Concept: "test",
				Score:   0.8,
			},
		},
	}, nil
}

func (m *MockEngine) Link(ctx context.Context, req *mbp.LinkRequest) (*LinkResponse, error) {
	return &LinkResponse{OK: true}, nil
}

func (m *MockEngine) Forget(ctx context.Context, req *ForgetRequest) (*ForgetResponse, error) {
	return &ForgetResponse{OK: true}, nil
}

func (m *MockEngine) Stat(ctx context.Context, req *StatRequest) (*StatResponse, error) {
	return &StatResponse{
		EngramCount:  100,
		VaultCount:   1,
		StorageBytes: 1024000,
	}, nil
}

func (m *MockEngine) ListEngrams(ctx context.Context, req *ListEngramsRequest) (*ListEngramsResponse, error) {
	return &ListEngramsResponse{
		Engrams: []EngramItem{
			{
				ID:         "test-id",
				Concept:    "test concept",
				Content:    "test content",
				Confidence: 0.9,
				Vault:      req.Vault,
			},
		},
		Total:  1,
		Limit:  req.Limit,
		Offset: req.Offset,
	}, nil
}

func (m *MockEngine) GetEngramLinks(ctx context.Context, req *GetEngramLinksRequest) (*GetEngramLinksResponse, error) {
	return &GetEngramLinksResponse{Links: []AssociationItem{}}, nil
}

func (m *MockEngine) ListVaults(ctx context.Context) ([]string, error) {
	return []string{"default"}, nil
}

func (m *MockEngine) GetSession(ctx context.Context, req *GetSessionRequest) (*GetSessionResponse, error) {
	return &GetSessionResponse{
		Entries: []SessionItem{
			{
				ID:        "test-id",
				Concept:   "test concept",
				Content:   "test content",
				CreatedAt: 1234567890,
			},
		},
	}, nil
}

func (m *MockEngine) WorkerStats() cognitive.EngineWorkerStats {
	return cognitive.EngineWorkerStats{}
}

func (m *MockEngine) SubscribeWithDeliver(ctx context.Context, req *mbp.SubscribeRequest, deliver trigger.DeliverFunc) (string, error) {
	return "mock-sub", nil
}

func (m *MockEngine) Unsubscribe(ctx context.Context, subID string) error {
	return nil
}

func (m *MockEngine) ClearVault(ctx context.Context, vaultName string) error { return nil }
func (m *MockEngine) DeleteVault(ctx context.Context, vaultName string) error { return nil }
func (m *MockEngine) StartClone(ctx context.Context, sourceVault, newName string) (*vaultjob.Job, error) {
	return &vaultjob.Job{ID: "mock-clone-job", Operation: "clone", Source: sourceVault, Target: newName}, nil
}
func (m *MockEngine) StartMerge(ctx context.Context, sourceVault, targetVault string, deleteSource bool) (*vaultjob.Job, error) {
	return &vaultjob.Job{ID: "mock-merge-job", Operation: "merge", Source: sourceVault, Target: targetVault}, nil
}
func (m *MockEngine) GetVaultJob(jobID string) (*vaultjob.Job, bool) { return nil, false }

func (m *MockEngine) ExportVault(ctx context.Context, vaultName, embedderModel string, dimension int, resetMeta bool, w io.Writer) (*storage.ExportResult, error) {
	// Write a minimal non-empty response so callers can tell export ran.
	w.Write([]byte("mock-export"))
	return &storage.ExportResult{EngramCount: 0, TotalKeys: 0}, nil
}
func (m *MockEngine) StartImport(ctx context.Context, vaultName, embedderModel string, dimension int, resetMeta bool, r io.Reader) (*vaultjob.Job, error) {
	return &vaultjob.Job{ID: "mock-import-job", Operation: "import", Target: vaultName}, nil
}

func (m *MockEngine) ReindexFTSVault(ctx context.Context, vaultName string) (int64, error) {
	return 0, nil
}

func (m *MockEngine) Checkpoint(destDir string) error {
	return nil
}

// backupMockEngine embeds MockEngine but creates a real Pebble checkpoint so
// the verification step has something to open.
type backupMockEngine struct {
	MockEngine
	pebbleDir string // set in test to a temp pebble DB dir
}

func (b *backupMockEngine) Checkpoint(destDir string) error {
	db, err := pebble.Open(b.pebbleDir, &pebble.Options{})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Checkpoint(destDir)
}

// Ensure bytes import is used.
var _ = bytes.NewReader

func TestHealthEndpoint(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	// Create a test request
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	// Call the handler directly
	server.mux.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp.Status)
	}
}

func TestReadyEndpoint(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("GET", "/api/ready", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ReadyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ready" {
		t.Errorf("expected status 'ready', got '%s'", resp.Status)
	}
}

func TestListEngrams(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("GET", "/api/engrams?vault=default", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ListEngramsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Engrams == nil {
		t.Error("expected engrams key in response, got nil")
	}
}

func TestListVaults(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("GET", "/api/vaults", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp []string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp) == 0 {
		t.Error("expected at least one vault in response")
	}
}

func TestGetSession(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("GET", "/api/session?vault=default&since=2020-01-01T00:00:00Z", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp GetSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestCORSHeaders(t *testing.T) {
	engine := &MockEngine{}
	const testOrigin = "http://example.com"
	server := NewServer("localhost:8080", engine, nil, nil, []string{testOrigin}, EmbedInfo{}, nil, "")

	// Test OPTIONS preflight with matching origin.
	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	req.Header.Set("Origin", testOrigin)
	w := httptest.NewRecorder()

	// CORS middleware wraps the http.Server handler, not the mux directly.
	server.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d for OPTIONS, got %d", http.StatusNoContent, w.Code)
	}

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
		t.Errorf("expected Access-Control-Allow-Origin: %s, got %q", testOrigin, got)
	}

	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods header to be set")
	}

	// Test GET request also gets CORS headers when origin matches allowlist.
	req2 := httptest.NewRequest("GET", "/api/health", nil)
	req2.Header.Set("Origin", testOrigin)
	w2 := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(w2, req2)

	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
		t.Errorf("expected Access-Control-Allow-Origin: %s on GET, got %q", testOrigin, got)
	}
}

func TestListEngramsDefaultVault(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	// No vault param — should default to "default"
	req := httptest.NewRequest("GET", "/api/engrams", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ListEngramsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Engrams == nil {
		t.Error("expected engrams in response")
	}
}

func TestListEngramsLimitClamping(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	// Overlarge limit should be clamped to 100
	req := httptest.NewRequest("GET", "/api/engrams?vault=default&limit=500", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ListEngramsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Limit > 100 {
		t.Errorf("expected limit clamped to 100, got %d", resp.Limit)
	}
}

func TestGetEngramLinks(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("GET", "/api/engrams/test-id/links", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp GetEngramLinksResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Links == nil {
		t.Error("expected links array (may be empty)")
	}
}

func TestGetSessionDefaultSince(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	// No since param — should default to last 24h
	req := httptest.NewRequest("GET", "/api/session?vault=default", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGetSessionMalformedSince(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	// Malformed since — should fall back to 24h, not error
	req := httptest.NewRequest("GET", "/api/session?vault=default&since=not-a-date", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d (graceful fallback), got %d", http.StatusOK, w.Code)
	}
}

func TestGetSessionLimitClamping(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("GET", "/api/session?vault=default&limit=9999", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestCORSHeadersOnNewRoutes(t *testing.T) {
	engine := &MockEngine{}
	const testOrigin = "http://example.com"
	server := NewServer("localhost:8080", engine, nil, nil, []string{testOrigin}, EmbedInfo{}, nil, "")

	routes := []string{
		"/api/engrams",
		"/api/vaults",
		"/api/session?vault=default",
	}
	for _, path := range routes {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Origin", testOrigin)
		w := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
			t.Errorf("route %s: expected Access-Control-Allow-Origin: %s, got %q", path, testOrigin, got)
		}
	}
}

func TestPreflightOnNewRoutes(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	routes := []string{"/api/engrams", "/api/vaults", "/api/session"}
	for _, path := range routes {
		req := httptest.NewRequest("OPTIONS", path, nil)
		w := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("OPTIONS %s: expected 204, got %d", path, w.Code)
		}
	}
}

type errorEngine struct{ MockEngine }

func (e *errorEngine) ListEngrams(ctx context.Context, req *ListEngramsRequest) (*ListEngramsResponse, error) {
	return nil, fmt.Errorf("storage error")
}
func (e *errorEngine) ListVaults(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("storage error")
}
func (e *errorEngine) GetSession(ctx context.Context, req *GetSessionRequest) (*GetSessionResponse, error) {
	return nil, fmt.Errorf("storage error")
}
func (e *errorEngine) GetEngramLinks(ctx context.Context, req *GetEngramLinksRequest) (*GetEngramLinksResponse, error) {
	return nil, fmt.Errorf("storage error")
}

func TestListEngramsEngineError(t *testing.T) {
	server := NewServer("localhost:8080", &errorEngine{}, nil, nil, nil, EmbedInfo{}, nil, "")
	req := httptest.NewRequest("GET", "/api/engrams?vault=default", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on engine error, got %d", w.Code)
	}
}

func TestListVaultsEngineError(t *testing.T) {
	server := NewServer("localhost:8080", &errorEngine{}, nil, nil, nil, EmbedInfo{}, nil, "")
	req := httptest.NewRequest("GET", "/api/vaults", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on engine error, got %d", w.Code)
	}
}

func TestGetSessionEngineError(t *testing.T) {
	server := NewServer("localhost:8080", &errorEngine{}, nil, nil, nil, EmbedInfo{}, nil, "")
	req := httptest.NewRequest("GET", "/api/session?vault=default", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on engine error, got %d", w.Code)
	}
}

func TestGetEngramLinksEngineError(t *testing.T) {
	server := NewServer("localhost:8080", &errorEngine{}, nil, nil, nil, EmbedInfo{}, nil, "")
	req := httptest.NewRequest("GET", "/api/engrams/test-id/links", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on engine error, got %d", w.Code)
	}
}

func TestShutdown(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:0", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start server in a goroutine
	done := make(chan error)
	go func() {
		done <- server.Serve(ctx)
	}()

	// Give the server time to start
	time.Sleep(10 * time.Millisecond)

	// Trigger shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)

	// Wait for server to shut down
	select {
	case <-done:
		// Server shut down successfully
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server shutdown")
	}
}

// TestContextPropagation verifies that request context is passed to the engine.
// A cancelled context should cause the engine to receive the cancellation.
func TestContextPropagation(t *testing.T) {
	var gotCtx context.Context
	eng := &ctxCapturingEngine{captureCtx: func(ctx context.Context) { gotCtx = ctx }}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("GET", "/api/stats", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if gotCtx == nil {
		t.Fatal("engine never received a context")
	}
	if gotCtx.Err() == nil {
		t.Error("expected cancelled context to be propagated to engine")
	}
}

type ctxCapturingEngine struct {
	MockEngine
	captureCtx func(context.Context)
}

func (e *ctxCapturingEngine) Stat(ctx context.Context, req *StatRequest) (*StatResponse, error) {
	e.captureCtx(ctx)
	return &StatResponse{}, nil
}

func TestServer_DisableCluster(t *testing.T) {
	s := &Server{}
	s.DisableCluster()
	if s.coordinator != nil {
		t.Fatal("expected nil coordinator after disable")
	}
}

func TestServer_ActiveCoordinator_Nil(t *testing.T) {
	s := &Server{}
	if s.ActiveCoordinator() != nil {
		t.Fatal("expected nil for new server")
	}
}

func TestServer_PersistClusterDisabled_NoDataDir(t *testing.T) {
	s := &Server{}
	if err := s.persistClusterDisabled(); err != nil {
		t.Fatalf("unexpected error with empty dataDir: %v", err)
	}
}

// TestCreateEngram tests POST /api/engrams → 201
func TestCreateEngram(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	body := `{"concept":"test concept","content":"test content","vault":"default"}`
	req := httptest.NewRequest("POST", "/api/engrams", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	// Verify response has ID field (struct field names, not msgpack tags)
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ID"] == "" || resp["ID"] == nil {
		t.Error("expected non-empty ID in response")
	}
}

// TestCreateEngram_InvalidJSON tests POST /api/engrams with bad body → 400
func TestCreateEngram_InvalidJSON(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("POST", "/api/engrams", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestActivate tests POST /api/activate → 200
func TestActivate(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	body := `{"context":["memory","learning"],"vault":"default","max_results":10}`
	req := httptest.NewRequest("POST", "/api/activate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["Activations"] == nil {
		t.Error("expected Activations field in response")
	}
}

// TestActivate_InvalidJSON tests POST /api/activate with bad body → 400
func TestActivate_InvalidJSON(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("POST", "/api/activate", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestDeleteEngram tests DELETE /api/engrams/{id} → 200
func TestDeleteEngram(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	req := httptest.NewRequest("DELETE", "/api/engrams/01ARZ3NDEKTSV4RRFFQ69G5FAV", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ok, _ := resp["OK"].(bool); !ok {
		t.Error("expected OK:true in response")
	}
}

// TestDeleteEngram_MissingID tests DELETE /api/engrams/ without ID → 400
func TestDeleteEngram_MissingID(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	// The mux will not match /api/engrams/ (missing {id}), so request will 404
	req := httptest.NewRequest("DELETE", "/api/engrams/", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	// Without a valid path parameter, the handler is not invoked at all by Go's mux.
	// We expect a 404 in this case.
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing path parameter, got %d", w.Code)
	}
}

// TestActivate_EmptyContext tests POST /api/activate with empty context array
func TestActivate_EmptyContext(t *testing.T) {
	engine := &MockEngine{}
	server := NewServer("localhost:8080", engine, nil, nil, nil, EmbedInfo{}, nil, "")

	body := `{"context":[],"vault":"default","max_results":10}`
	req := httptest.NewRequest("POST", "/api/activate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	// Should return 200 and not panic, even with empty context
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// Should have Activations field (may be empty list)
	if resp["Activations"] == nil {
		t.Error("expected Activations field in response")
	}
}

func TestOnlineBackupEndpoint(t *testing.T) {
	// Create a real Pebble DB so the checkpoint and verification are exercised.
	pebbleDir := filepath.Join(t.TempDir(), "pebble")
	db, err := pebble.Open(pebbleDir, &pebble.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Set([]byte("backup-key"), []byte("backup-val"), pebble.Sync); err != nil {
		t.Fatal(err)
	}
	db.Close()

	eng := &backupMockEngine{pebbleDir: pebbleDir}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "")

	outputDir := filepath.Join(t.TempDir(), "online-backup-out")
	body := fmt.Sprintf(`{"output_dir":%q}`, outputDir)
	req := httptest.NewRequest("POST", "/api/admin/backup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp BackupResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.OutputDir != outputDir {
		t.Errorf("expected output_dir=%q, got %q", outputDir, resp.OutputDir)
	}
	if resp.SizeBytes <= 0 {
		t.Errorf("expected positive size_bytes, got %d", resp.SizeBytes)
	}

	// Verify the checkpoint directory exists and is readable.
	checkpointDir := filepath.Join(outputDir, "pebble")
	if _, err := os.Stat(checkpointDir); os.IsNotExist(err) {
		t.Fatal("checkpoint directory does not exist")
	}
	verifyDB, err := pebble.Open(checkpointDir, &pebble.Options{ReadOnly: true})
	if err != nil {
		t.Fatalf("failed to open checkpoint for verification: %v", err)
	}
	defer verifyDB.Close()

	val, closer, err := verifyDB.Get([]byte("backup-key"))
	if err != nil {
		t.Fatalf("key not found in checkpoint: %v", err)
	}
	if string(val) != "backup-val" {
		t.Fatalf("expected backup-val, got %q", string(val))
	}
	closer.Close()
}

func TestOnlineBackupEndpoint_ConflictExistingDir(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "")

	existingDir := t.TempDir()
	body := fmt.Sprintf(`{"output_dir":%q}`, existingDir)
	req := httptest.NewRequest("POST", "/api/admin/backup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOnlineBackupEndpoint_MissingOutputDir(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "")

	body := `{}`
	req := httptest.NewRequest("POST", "/api/admin/backup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRemoveNode_SelfRemoval_Returns400(t *testing.T) {
	dir := t.TempDir()
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repLog := replication.NewReplicationLog(db)
	applier := replication.NewApplier(db)
	epochStore, err := replication.NewEpochStore(db)
	if err != nil {
		t.Fatalf("NewEpochStore: %v", err)
	}

	cfg := &config.ClusterConfig{
		Enabled:  true,
		NodeID:   "cortex-1",
		BindAddr: "127.0.0.1:0",
		Role:     "primary",
	}
	coord := replication.NewClusterCoordinator(cfg, repLog, applier, epochStore)

	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "")
	server.SetCoordinator(coord)

	req := httptest.NewRequest("DELETE", "/api/admin/cluster/nodes/cortex-1", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for self-removal, got %d: %s", w.Code, w.Body.String())
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Message == "" {
		t.Error("expected non-empty error message")
	}
}

func TestRemoveNode_OtherNode_Returns200(t *testing.T) {
	dir := t.TempDir()
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repLog := replication.NewReplicationLog(db)
	applier := replication.NewApplier(db)
	epochStore, err := replication.NewEpochStore(db)
	if err != nil {
		t.Fatalf("NewEpochStore: %v", err)
	}

	cfg := &config.ClusterConfig{
		Enabled:  true,
		NodeID:   "cortex-1",
		BindAddr: "127.0.0.1:0",
		Role:     "primary",
	}
	coord := replication.NewClusterCoordinator(cfg, repLog, applier, epochStore)

	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "")
	server.SetCoordinator(coord)

	req := httptest.NewRequest("DELETE", "/api/admin/cluster/nodes/lobe-2", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for removing other node, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBatchCreateEngrams(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "")

	body := `{"engrams":[{"content":"one","concept":"c1"},{"content":"two","concept":"c2"},{"content":"three"}]}`
	req := httptest.NewRequest("POST", "/api/engrams/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Results []struct {
			Index  int    `json:"index"`
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(resp.Results))
	}
	for i, r := range resp.Results {
		if r.Status != "ok" {
			t.Errorf("result[%d]: status = %q, want 'ok'", i, r.Status)
		}
		if r.ID == "" {
			t.Errorf("result[%d]: ID is empty", i)
		}
	}
}

func TestBatchCreateEngramsExceedsLimit(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "")

	engrams := make([]map[string]string, 51)
	for i := range engrams {
		engrams[i] = map[string]string{"content": fmt.Sprintf("item %d", i)}
	}
	bodyBytes, _ := json.Marshal(map[string]any{"engrams": engrams})
	req := httptest.NewRequest("POST", "/api/engrams/batch", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for >50 items, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBatchCreateEngramsEmptyArray(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "")

	body := `{"engrams":[]}`
	req := httptest.NewRequest("POST", "/api/engrams/batch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty array, got %d: %s", w.Code, w.Body.String())
	}
}
