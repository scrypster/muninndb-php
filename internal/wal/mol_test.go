package wal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
)

// TestGroupCommitterStopDrainsQueue verifies that cancelling the context causes
// Run() to drain the pending channel and flush all queued entries before exiting.
func TestGroupCommitterStopDrainsQueue(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer mol.Close()

	db, err := pebble.Open(filepath.Join(dir, "pebble"), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open failed: %v", err)
	}
	defer db.Close()

	// batchSize = 100 so 20 entries won't trigger an auto-flush via the pending
	// drain path inside the regular pw case (maxGroupSize guard).
	gc := &GroupCommitter{
		pending:      make(chan *PendingWrite, 4096),
		mol:          mol,
		db:           db,
		maxGroupSize: 100,
		maxWait:      DefaultMaxWait,
		done:         make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan error, 1)
	go func() {
		runDone <- gc.Run(ctx)
	}()

	// Submit 20 fire-and-forget entries via AppendAsync.
	const numEntries = 20
	for i := 0; i < numEntries; i++ {
		entry := &MOLEntry{
			OpType:  OpEngramWrite,
			VaultID: 1,
			Payload: []byte(fmt.Sprintf("entry %d", i)),
		}
		AppendAsync(gc, entry)
	}

	// Give entries time to land in the pending channel before cancelling.
	time.Sleep(1 * time.Millisecond)

	// Cancel context — Run() should drain pending and return.
	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within timeout after context cancel")
	}

	// mol.nextSeq is incremented by Append on each successful write.
	// After flushing all 20 entries it should equal 20.
	written := mol.nextSeq.Load()
	if written != numEntries {
		t.Errorf("expected %d entries written (mol.nextSeq=%d), got %d", numEntries, written, written)
	}
}

// TestGroupCommitterRunCancels tests that Run(ctx) returns nil when context is canceled
func TestGroupCommitterRunCancels(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer mol.Close()

	db, err := pebble.Open(filepath.Join(dir, "pebble"), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open failed: %v", err)
	}
	defer db.Close()

	gc := NewGroupCommitter(mol, db)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- gc.Run(ctx)
	}()

	// Wait for Run to return
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v, expected nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Run did not return within timeout")
	}
}

// TestAppendAsync_NonBlocking tests that AppendAsync is non-blocking
func TestAppendAsync_NonBlocking(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer mol.Close()

	db, err := pebble.Open(filepath.Join(dir, "pebble"), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open failed: %v", err)
	}
	defer db.Close()

	gc := NewGroupCommitter(mol, db)

	// This AppendAsync should not block even though the fire-and-forget semantic allows drops
	start := time.Now()
	entry := &MOLEntry{OpType: OpEngramUpdate}
	AppendAsync(gc, entry)
	elapsed := time.Since(start)

	// Should complete nearly instantly (well under 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("AppendAsync took too long: %v (should be non-blocking)", elapsed)
	}
}

// TestMOLMaybeSealSegment tests that MaybeSealSegment rotates files when threshold exceeded
func TestMOLMaybeSealSegment(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer mol.Close()

	// Set a very low threshold for testing (e.g., 50 bytes)
	mol.SealThreshold = 50

	// Write data to exceed threshold
	entry1 := &MOLEntry{
		OpType:  OpEngramWrite,
		VaultID: 1,
		Payload: make([]byte, 40),
	}
	if err := mol.Append(entry1); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Check size after first append
	mol.mu.Lock()
	sizeBefore := mol.active.size
	mol.mu.Unlock()
	t.Logf("Size after entry1: %d bytes", sizeBefore)

	entry2 := &MOLEntry{
		OpType:  OpEngramWrite,
		VaultID: 2,
		Payload: make([]byte, 40),
	}
	if err := mol.Append(entry2); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Get the active segment before sealing
	mol.mu.Lock()
	oldSizeBeforeSeal := mol.active.size
	oldSeqBefore := mol.nextSeq.Load()
	mol.mu.Unlock()
	t.Logf("Size after entry2: %d bytes (threshold: %d), nextSeq: %d", oldSizeBeforeSeal, mol.SealThreshold, oldSeqBefore)

	// Call MaybeSealSegment
	if err := mol.MaybeSealSegment(); err != nil {
		t.Fatalf("MaybeSealSegment failed: %v", err)
	}

	// Verify a new active file was created with size 0
	mol.mu.Lock()
	newActivePath := mol.active.path
	newSize := mol.active.size
	mol.mu.Unlock()

	if newActivePath != filepath.Join(dir, "mol-active.log") {
		t.Errorf("Expected new active path to be mol-active.log, got %s", newActivePath)
	}

	if newSize != 0 {
		t.Errorf("New active file should have size 0, got %d", newSize)
	}

	if _, err := os.Stat(newActivePath); err != nil {
		t.Errorf("New active file should exist: %v", err)
	}

	// Verify sealed file matches the pattern mol-{seqNum}.log
	sealedFiles, err := filepath.Glob(filepath.Join(dir, "mol-*.log"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}

	if len(sealedFiles) == 0 {
		t.Error("Expected at least one sealed file (mol-*.log)")
	}

	t.Logf("Sealed files: %v", sealedFiles)
}

// TestMOLRecover tests that Recover reads and replays all sealed segment entries
func TestMOLRecover(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer mol.Close()

	// Set a low threshold for testing
	mol.SealThreshold = 100

	// Write and seal entries
	entry1 := &MOLEntry{
		OpType:  OpEngramWrite,
		VaultID: 1,
		Payload: make([]byte, 60),
	}
	if err := mol.Append(entry1); err != nil {
		t.Fatalf("Append 1 failed: %v", err)
	}

	entry2 := &MOLEntry{
		OpType:  OpEngramWrite,
		VaultID: 2,
		Payload: make([]byte, 60),
	}
	if err := mol.Append(entry2); err != nil {
		t.Fatalf("Append 2 failed: %v", err)
	}

	// Seal the segment
	if err := mol.MaybeSealSegment(); err != nil {
		t.Fatalf("MaybeSealSegment failed: %v", err)
	}

	// Create pebble DB for recovery
	db, err := pebble.Open(filepath.Join(dir, "pebble"), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open failed: %v", err)
	}
	defer db.Close()

	// Track recovered entries
	var recovered []*MOLEntry
	var mu sync.Mutex

	replayFn := func(entry *MOLEntry) error {
		mu.Lock()
		recovered = append(recovered, entry)
		mu.Unlock()
		return nil
	}

	// Call Recover
	if err := mol.Recover(db, replayFn); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	// Verify both entries were recovered
	if len(recovered) != 2 {
		t.Errorf("Expected 2 recovered entries, got %d", len(recovered))
	}

	// Verify entry details
	for i, entry := range recovered {
		if entry.OpType != OpEngramWrite {
			t.Errorf("Entry %d: expected OpType OpEngramWrite, got %d", i, entry.OpType)
		}
	}
}

// TestMOLRecover_EmptyWAL tests that Recover handles empty WAL gracefully
func TestMOLRecover_EmptyWAL(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer mol.Close()

	db, err := pebble.Open(filepath.Join(dir, "pebble"), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open failed: %v", err)
	}
	defer db.Close()

	callCount := atomic.Int32{}
	replayFn := func(entry *MOLEntry) error {
		callCount.Add(1)
		return nil
	}

	// Recover on empty WAL should not error
	if err := mol.Recover(db, replayFn); err != nil {
		t.Fatalf("Recover failed on empty WAL: %v", err)
	}

	// Should not call replay function
	if callCount.Load() != 0 {
		t.Errorf("Expected 0 replay calls on empty WAL, got %d", callCount.Load())
	}
}

// TestMOLRecover_ReplayError tests that Recover propagates replay function errors
func TestMOLRecover_ReplayError(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer mol.Close()

	// Set low threshold and seal a segment with an entry
	mol.SealThreshold = 100

	entry := &MOLEntry{
		OpType:  OpEngramWrite,
		VaultID: 1,
		Payload: make([]byte, 60),
	}
	if err := mol.Append(entry); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if err := mol.MaybeSealSegment(); err != nil {
		t.Fatalf("MaybeSealSegment failed: %v", err)
	}

	db, err := pebble.Open(filepath.Join(dir, "pebble"), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open failed: %v", err)
	}
	defer db.Close()

	replayFn := func(entry *MOLEntry) error {
		return fmt.Errorf("test error")
	}

	// Recover should propagate the error
	err = mol.Recover(db, replayFn)
	if err == nil {
		t.Error("Expected Recover to return error from replayFn")
	}
}

// TestWAL_ReplayAfterRestart verifies that WAL entries survive a restart and are
// correctly replayed. This test writes entries to sealed segments, then reopens
// the WAL and confirms Recover() replays them correctly.
func TestWAL_ReplayAfterRestart(t *testing.T) {
	dir := t.TempDir()

	// Phase 1: Write and seal entries
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Set low threshold to force a seal
	mol.SealThreshold = 100

	// Write first entry
	entry1 := &MOLEntry{
		OpType:  OpEngramWrite,
		VaultID: 1,
		Payload: make([]byte, 50),
	}
	if err := mol.Append(entry1); err != nil {
		t.Fatalf("Append 1 failed: %v", err)
	}

	// Write second entry
	entry2 := &MOLEntry{
		OpType:  OpEngramWrite,
		VaultID: 2,
		Payload: make([]byte, 50),
	}
	if err := mol.Append(entry2); err != nil {
		t.Fatalf("Append 2 failed: %v", err)
	}

	// Seal the segment (this renames mol-active.log to mol-{seqNum}.log)
	if err := mol.MaybeSealSegment(); err != nil {
		t.Fatalf("MaybeSealSegment failed: %v", err)
	}

	// Sync to ensure sealed entries are durable
	if err := mol.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if err := mol.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Phase 2: Restart and recover
	mol2, err := Open(dir)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer mol2.Close()

	db, err := pebble.Open(filepath.Join(dir, "pebble"), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open failed: %v", err)
	}
	defer db.Close()

	// Track recovered entries
	var recovered []*MOLEntry
	var mu sync.Mutex

	replayFn := func(entry *MOLEntry) error {
		mu.Lock()
		recovered = append(recovered, entry)
		mu.Unlock()
		return nil
	}

	if err := mol2.Recover(db, replayFn); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	// Should recover exactly 2 entries (from sealed segment)
	if len(recovered) != 2 {
		t.Errorf("Expected 2 recovered entries, got %d", len(recovered))
	}

	// Verify vault IDs and types
	if len(recovered) >= 1 {
		if recovered[0].VaultID != 1 {
			t.Errorf("Entry 0: expected VaultID=1, got %d", recovered[0].VaultID)
		}
		if recovered[0].OpType != OpEngramWrite {
			t.Errorf("Entry 0: expected OpType=OpEngramWrite, got %d", recovered[0].OpType)
		}
	}
	if len(recovered) >= 2 {
		if recovered[1].VaultID != 2 {
			t.Errorf("Entry 1: expected VaultID=2, got %d", recovered[1].VaultID)
		}
		if recovered[1].OpType != OpEngramWrite {
			t.Errorf("Entry 1: expected OpType=OpEngramWrite, got %d", recovered[1].OpType)
		}
	}
}
