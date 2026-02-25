package wal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/pebble"
)

func TestMOL_NextSeqRecoveredOnReopen(t *testing.T) {
	dir := t.TempDir()

	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		if err := mol.Append(&MOLEntry{
			OpType:  OpEngramWrite,
			VaultID: 1,
			Payload: []byte(fmt.Sprintf("entry-%d", i)),
		}); err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}
	if err := mol.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	seqBefore := mol.nextSeq.Load()
	if seqBefore != 5 {
		t.Fatalf("expected nextSeq=5 before close, got %d", seqBefore)
	}
	mol.Close()

	mol2, err := Open(dir)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer mol2.Close()

	seqAfter := mol2.nextSeq.Load()
	if seqAfter != 5 {
		t.Fatalf("expected nextSeq=5 after reopen, got %d", seqAfter)
	}

	// New appends should not collide with existing seq numbers.
	if err := mol2.Append(&MOLEntry{
		OpType:  OpEngramWrite,
		VaultID: 1,
		Payload: []byte("entry-5"),
	}); err != nil {
		t.Fatalf("Append after reopen failed: %v", err)
	}

	finalSeq := mol2.nextSeq.Load()
	if finalSeq != 6 {
		t.Fatalf("expected nextSeq=6 after new append, got %d", finalSeq)
	}
}

func TestMOL_NextSeqRecoveredFromSealedAndActive(t *testing.T) {
	dir := t.TempDir()

	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	mol.SealThreshold = 100

	// Write enough to seal a segment.
	for i := 0; i < 3; i++ {
		if err := mol.Append(&MOLEntry{
			OpType:  OpEngramWrite,
			VaultID: 1,
			Payload: make([]byte, 50),
		}); err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}
	if err := mol.MaybeSealSegment(); err != nil {
		t.Fatalf("MaybeSealSegment failed: %v", err)
	}

	// Write more entries into the new active segment.
	for i := 0; i < 2; i++ {
		if err := mol.Append(&MOLEntry{
			OpType:  OpEngramWrite,
			VaultID: 1,
			Payload: []byte("post-seal"),
		}); err != nil {
			t.Fatalf("Append post-seal %d failed: %v", i, err)
		}
	}
	if err := mol.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	expectedSeq := mol.nextSeq.Load() // should be 5
	mol.Close()

	mol2, err := Open(dir)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer mol2.Close()

	recovered := mol2.nextSeq.Load()
	if recovered != expectedSeq {
		t.Fatalf("expected nextSeq=%d after reopen, got %d", expectedSeq, recovered)
	}
}

func TestMOL_EmptyDirOpensCleanly(t *testing.T) {
	dir := t.TempDir()

	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer mol.Close()

	if mol.nextSeq.Load() != 0 {
		t.Fatalf("expected nextSeq=0 for empty dir, got %d", mol.nextSeq.Load())
	}
}

func TestMOL_RecoverActiveSegmentEntries(t *testing.T) {
	dir := t.TempDir()

	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Write entries to active segment only (no sealing).
	for i := 0; i < 3; i++ {
		if err := mol.Append(&MOLEntry{
			OpType:  OpEngramWrite,
			VaultID: uint32(i + 1),
			Payload: []byte(fmt.Sprintf("active-entry-%d", i)),
		}); err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}
	if err := mol.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	mol.Close()

	// Reopen and Recover — active segment entries should be readable.
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

	var recovered []*MOLEntry
	err = mol2.Recover(db, func(e *MOLEntry) error {
		recovered = append(recovered, e)
		return nil
	})
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	if len(recovered) != 3 {
		t.Fatalf("expected 3 recovered entries from active segment, got %d", len(recovered))
	}

	for i, e := range recovered {
		if e.VaultID != uint32(i+1) {
			t.Errorf("entry %d: expected VaultID=%d, got %d", i, i+1, e.VaultID)
		}
	}
}

func TestSaveLoadLastSeq(t *testing.T) {
	dir := t.TempDir()

	db, err := pebble.Open(filepath.Join(dir, "pebble"), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open failed: %v", err)
	}
	defer db.Close()

	// Fresh DB should return 0.
	if seq := LoadLastSeq(db); seq != 0 {
		t.Fatalf("expected 0 on fresh DB, got %d", seq)
	}

	if err := SaveLastSeq(db, 42); err != nil {
		t.Fatalf("SaveLastSeq failed: %v", err)
	}

	if seq := LoadLastSeq(db); seq != 42 {
		t.Fatalf("expected 42 after save, got %d", seq)
	}

	if err := SaveLastSeq(db, 100); err != nil {
		t.Fatalf("SaveLastSeq (update) failed: %v", err)
	}
	if seq := LoadLastSeq(db); seq != 100 {
		t.Fatalf("expected 100 after update, got %d", seq)
	}
}

func TestMOL_TruncatedActiveSegment(t *testing.T) {
	dir := t.TempDir()

	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Write 3 entries
	for i := 0; i < 3; i++ {
		if err := mol.Append(&MOLEntry{
			OpType:  OpEngramWrite,
			VaultID: 1,
			Payload: []byte(fmt.Sprintf("entry-%d", i)),
		}); err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}
	if err := mol.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	mol.Close()

	// Simulate crash by truncating the last few bytes of the active segment.
	activePath := filepath.Join(dir, "mol-active.log")
	info, err := os.Stat(activePath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	// Truncate to remove the last entry's CRC (partial write).
	truncatedSize := info.Size() - 4
	if err := os.Truncate(activePath, truncatedSize); err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Reopen — should recover the first 2 entries and stop at the truncated one.
	mol2, err := Open(dir)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer mol2.Close()

	// Seq should be recovered from the readable entries.
	// The 3rd entry is truncated, so maxSeq should be from entry 1 (seq=1, 0-indexed).
	if mol2.nextSeq.Load() < 2 {
		t.Fatalf("expected nextSeq >= 2 after truncated recovery, got %d", mol2.nextSeq.Load())
	}
}
