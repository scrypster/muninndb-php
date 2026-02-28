package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// stubCheckpointer is a test double for Checkpointer.
// It writes a marker file inside destDir to confirm the call happened.
type stubCheckpointer struct {
	called    int
	markerErr error // if non-nil, Checkpoint returns this error
}

func (s *stubCheckpointer) Checkpoint(destDir string) error {
	s.called++
	if s.markerErr != nil {
		return s.markerErr
	}
	if err := os.MkdirAll(destDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destDir, "checkpoint.marker"), []byte("ok"), 0600)
}

// TestNew_DisabledWhenNoInterval verifies that New returns nil when Interval is zero.
func TestNew_DisabledWhenNoInterval(t *testing.T) {
	stub := &stubCheckpointer{}
	sched := New(Config{Interval: 0, BackupDir: "/tmp/backups", Retain: 5}, stub)
	if sched != nil {
		t.Fatalf("expected nil scheduler when Interval is zero, got non-nil")
	}
}

// TestRunOnce_CreatesCheckpoint verifies that runOnce creates a backup directory
// with the expected timestamp-based name and calls Checkpoint inside it.
func TestRunOnce_CreatesCheckpoint(t *testing.T) {
	dir := t.TempDir()
	stub := &stubCheckpointer{}

	sched := New(Config{
		Interval:  time.Hour, // won't tick in test
		BackupDir: dir,
		Retain:    5,
		DataDir:   "", // no aux files
	}, stub)
	if sched == nil {
		t.Fatal("expected non-nil scheduler")
	}

	before := time.Now().UTC()
	sched.runOnce()
	after := time.Now().UTC()

	if stub.called != 1 {
		t.Fatalf("expected Checkpoint called once, got %d", stub.called)
	}

	// Verify a backup directory was created.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup dir, got %d", len(entries))
	}

	name := entries[0].Name()

	// Name must begin with "backup-" and contain a valid timestamp.
	const prefix = "backup-"
	if len(name) <= len(prefix) {
		t.Fatalf("unexpected backup dir name: %q", name)
	}
	tsStr := name[len(prefix):]
	ts, err := time.Parse("20060102-150405", tsStr)
	if err != nil {
		t.Fatalf("backup dir timestamp parse error for %q: %v", tsStr, err)
	}

	// The parsed timestamp should fall within the run window (allow 1s slack).
	if ts.Before(before.Add(-time.Second)) || ts.After(after.Add(time.Second)) {
		t.Fatalf("timestamp %v not in expected range [%v, %v]", ts, before, after)
	}

	// Verify the pebble sub-directory and marker exist.
	markerPath := filepath.Join(dir, name, "pebble", "checkpoint.marker")
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("marker file missing at %s: %v", markerPath, err)
	}
}

// TestPruneOldBackups_KeepsRetainCount creates 7 backup directories, sets
// retain=3, runs runOnce (which adds an 8th), and verifies only 3 remain.
func TestPruneOldBackups_KeepsRetainCount(t *testing.T) {
	dir := t.TempDir()
	stub := &stubCheckpointer{}

	const retain = 3

	// Pre-create 7 backup dirs with ascending timestamps so pruning removes
	// the oldest ones first.
	for i := 0; i < 7; i++ {
		ts := time.Date(2024, 1, i+1, 12, 0, 0, 0, time.UTC)
		name := fmt.Sprintf("backup-%s", ts.Format("20060102-150405"))
		if err := os.MkdirAll(filepath.Join(dir, name), 0700); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}

	sched := New(Config{
		Interval:  time.Hour,
		BackupDir: dir,
		Retain:    retain,
		DataDir:   "",
	}, stub)

	sched.runOnce()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	if len(names) != retain {
		t.Fatalf("expected %d backup dirs after prune, got %d: %v", retain, len(names), names)
	}
}

// TestGetStatus_ReflectsLastRun verifies that after runOnce completes the
// status fields are populated correctly.
func TestGetStatus_ReflectsLastRun(t *testing.T) {
	dir := t.TempDir()
	stub := &stubCheckpointer{}

	cfg := Config{
		Interval:  5 * time.Minute,
		BackupDir: dir,
		Retain:    5,
		DataDir:   "",
	}
	sched := New(cfg, stub)

	// Status before any run: enabled but no last-run info.
	before := sched.GetStatus()
	if !before.Enabled {
		t.Fatal("expected Enabled=true before first run")
	}
	if !before.LastRunAt.IsZero() {
		t.Fatalf("expected zero LastRunAt before first run, got %v", before.LastRunAt)
	}

	runStart := time.Now()
	sched.runOnce()
	runEnd := time.Now()

	st := sched.GetStatus()

	if !st.Enabled {
		t.Error("expected Enabled=true")
	}
	if st.Interval != cfg.Interval.String() {
		t.Errorf("expected Interval=%q, got %q", cfg.Interval.String(), st.Interval)
	}
	if st.BackupDir != cfg.BackupDir {
		t.Errorf("expected BackupDir=%q, got %q", cfg.BackupDir, st.BackupDir)
	}
	if st.Retain != cfg.Retain {
		t.Errorf("expected Retain=%d, got %d", cfg.Retain, st.Retain)
	}
	if !st.LastRunOK {
		t.Errorf("expected LastRunOK=true, got false (error: %s)", st.LastError)
	}
	if st.LastRunAt.Before(runStart) || st.LastRunAt.After(runEnd) {
		t.Errorf("LastRunAt %v outside run window [%v, %v]", st.LastRunAt, runStart, runEnd)
	}
	if st.LastElapsed == "" {
		t.Error("expected non-empty LastElapsed")
	}
	if st.LastError != "" {
		t.Errorf("expected empty LastError, got %q", st.LastError)
	}
}

// TestScheduler_Start verifies the scheduler fires through its goroutine via
// a very short interval and that the engine Checkpoint is called at least once.
func TestScheduler_Start(t *testing.T) {
	dir := t.TempDir()
	stub := &stubCheckpointer{}

	sched := New(Config{
		Interval:  50 * time.Millisecond,
		BackupDir: dir,
		Retain:    10,
		DataDir:   "",
	}, stub)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	sched.Start(ctx)
	<-ctx.Done()

	if stub.called == 0 {
		t.Fatal("expected Checkpoint to be called at least once by the scheduler goroutine")
	}
}
