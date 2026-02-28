package rest

// backup_boost_test.go adds targeted tests for the backup handler and its
// helper functions (backupCopyFile, backupCopyDir, backupVerifyCheckpoint).

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// backupCopyFile — 0% coverage, pure file-copy function
// ---------------------------------------------------------------------------

func TestBackupCopyFile_CopiesContent(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	dst := filepath.Join(t.TempDir(), "dst.txt")

	if err := os.WriteFile(src, []byte("hello backup"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := backupCopyFile(src, dst); err != nil {
		t.Fatalf("backupCopyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "hello backup" {
		t.Errorf("expected 'hello backup', got %q", string(got))
	}
}

func TestBackupCopyFile_MissingSrc(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "dst.txt")
	err := backupCopyFile("/nonexistent/path/src.txt", dst)
	if err == nil {
		t.Error("expected error for missing src, got nil")
	}
}

// ---------------------------------------------------------------------------
// backupCopyDir — 0% coverage, recursive dir-copy function
// ---------------------------------------------------------------------------

func TestBackupCopyDir_CopiesFiles(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst-dir")

	// Create a nested file structure.
	subdir := filepath.Join(src, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "file1.txt"), []byte("file1"), 0644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "file2.txt"), []byte("file2"), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	if err := backupCopyDir(src, dst); err != nil {
		t.Fatalf("backupCopyDir: %v", err)
	}

	// Verify copied files exist with correct content.
	got1, err := os.ReadFile(filepath.Join(dst, "file1.txt"))
	if err != nil || string(got1) != "file1" {
		t.Errorf("file1 mismatch: err=%v content=%q", err, got1)
	}
	got2, err := os.ReadFile(filepath.Join(dst, "subdir", "file2.txt"))
	if err != nil || string(got2) != "file2" {
		t.Errorf("file2 mismatch: err=%v content=%q", err, got2)
	}
}

// ---------------------------------------------------------------------------
// handleBackup — checkpoint failure path
// ---------------------------------------------------------------------------

func TestHandleBackup_CheckpointFailure(t *testing.T) {
	eng := &checkpointErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	outputDir := filepath.Join(t.TempDir(), "backup-fail-out")
	body := `{"output_dir":"` + outputDir + `"}`
	req := httptest.NewRequest("POST", "/api/admin/backup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on checkpoint failure, got %d: %s", w.Code, w.Body.String())
	}
	// Output dir should be cleaned up after checkpoint failure.
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Error("expected output dir to be removed after checkpoint failure")
	}
}

// checkpointErrorEngine returns an error from Checkpoint.
type checkpointErrorEngine struct{ MockEngine }

func (e *checkpointErrorEngine) Checkpoint(_ string) error {
	return errors.New("simulated checkpoint failure")
}

// ---------------------------------------------------------------------------
// handleBackup — with dataDir set, exercises the WAL and secret copy paths
// ---------------------------------------------------------------------------

func TestHandleBackup_WithDataDir_CopiesWalAndSecret(t *testing.T) {
	// Build a fake dataDir with a wal/ subdirectory and auth_secret file.
	dataDir := t.TempDir()
	walDir := filepath.Join(dataDir, "wal")
	if err := os.MkdirAll(walDir, 0755); err != nil {
		t.Fatalf("mkdir wal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(walDir, "segment.log"), []byte("wal data"), 0644); err != nil {
		t.Fatalf("write wal segment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "auth_secret"), []byte("secret!"), 0600); err != nil {
		t.Fatalf("write auth_secret: %v", err)
	}

	// Use a real Pebble checkpoint engine.
	pebbleDir := filepath.Join(dataDir, "pebble")
	eng := &backupMockEngine{pebbleDir: pebbleDir}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, dataDir, nil)

	outputDir := filepath.Join(t.TempDir(), "backup-with-wal")
	body := `{"output_dir":"` + outputDir + `"}`
	req := httptest.NewRequest("POST", "/api/admin/backup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify wal was copied.
	if _, err := os.Stat(filepath.Join(outputDir, "wal")); os.IsNotExist(err) {
		t.Error("expected wal dir to be copied to backup output")
	}
	// Verify auth_secret was copied.
	if _, err := os.Stat(filepath.Join(outputDir, "auth_secret")); os.IsNotExist(err) {
		t.Error("expected auth_secret to be copied to backup output")
	}
}

// ---------------------------------------------------------------------------
// handleReady — subsystems not ready path (0% — it's the non-ready branch)
// ---------------------------------------------------------------------------

func TestReadyEndpoint_NotReady_Boost(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)
	// Force subsystemsReady to false (it defaults to false, but subsystemsReady
	// is set to true somewhere in the initialization. Let's check.)
	server.subsystemsReady.Store(false)

	req := httptest.NewRequest("GET", "/api/ready", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when not ready, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// handleHealth — with non-empty version string
// ---------------------------------------------------------------------------

func TestHealthEndpoint_WithVersion(t *testing.T) {
	eng := &MockEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)
	server.version = "1.2.3"

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// withClusterAuth — token mismatch path (withClusterAuth at 60%)
// ---------------------------------------------------------------------------

func TestWithClusterAuth_TokenMismatch(t *testing.T) {
	handler := withClusterAuth("correct-secret", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 on token mismatch, got %d", w.Code)
	}
}

func TestWithClusterAuth_MissingBearer(t *testing.T) {
	handler := withClusterAuth("correct-secret", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	// No Authorization header at all.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when Authorization header is missing, got %d", w.Code)
	}
}

func TestWithClusterAuth_ValidToken(t *testing.T) {
	handler := withClusterAuth("correct-secret", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer correct-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with correct token, got %d", w.Code)
	}
}

func TestWithClusterAuth_InactiveCluster_PassesThrough(t *testing.T) {
	handler := withClusterAuth("", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when cluster is inactive, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleContradictions — error path
// ---------------------------------------------------------------------------

func TestContradictionsEndpoint_EngineError(t *testing.T) {
	eng := &contradictionsErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("GET", "/api/contradictions?vault=default", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type contradictionsErrorEngine struct{ MockEngine }

func (e *contradictionsErrorEngine) GetContradictions(_ context.Context, _ string) (*ContradictionsResponse, error) {
	return nil, errors.New("contradictions failed")
}

// ---------------------------------------------------------------------------
// handleGuide — error path
// ---------------------------------------------------------------------------

func TestGuideEndpoint_EngineError(t *testing.T) {
	eng := &guideErrorEngine{}
	server := NewServer("localhost:8080", eng, nil, nil, nil, EmbedInfo{}, nil, "", nil)

	req := httptest.NewRequest("GET", "/api/guide?vault=default", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

type guideErrorEngine struct{ MockEngine }

func (e *guideErrorEngine) GetGuide(_ context.Context, _ string) (string, error) {
	return "", errors.New("guide failed")
}
