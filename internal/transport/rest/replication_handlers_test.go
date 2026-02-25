package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleReplicationStatus_NoCoordinator verifies that when no coordinator is set
// the handler returns HTTP 200 with enabled=false.
func TestHandleReplicationStatus_NoCoordinator(t *testing.T) {
	srv := newTestServer(t, nil)

	req := httptest.NewRequest("GET", "/v1/replication/status", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp replicationStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Enabled {
		t.Error("expected enabled=false when no coordinator is configured")
	}
}

// TestHandleReplicationStatus_WithCoordinator verifies that when a coordinator is set
// the handler returns HTTP 200 with enabled=true and the expected response fields.
func TestHandleReplicationStatus_WithCoordinator(t *testing.T) {
	const testSecret = "replication-handler-test-secret"
	srv := newTestServer(t, nil)
	coord := newTestCoordinatorWithSecret(t, testSecret)
	srv.SetCoordinator(coord)

	req := httptest.NewRequest("GET", "/v1/replication/status", nil)
	req.Header.Set("Authorization", "Bearer "+testSecret)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp replicationStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Enabled {
		t.Error("expected enabled=true when coordinator is configured")
	}
	if resp.Role == "" {
		t.Error("expected non-empty role field")
	}
}

// TestHandleReplicationLag_Unavailable verifies that when no coordinator is configured
// the lag endpoint returns HTTP 503 Service Unavailable.
func TestHandleReplicationLag_Unavailable(t *testing.T) {
	srv := newTestServer(t, nil)

	req := httptest.NewRequest("GET", "/v1/replication/lag", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when cluster is disabled, got %d: %s", w.Code, w.Body.String())
	}
}
