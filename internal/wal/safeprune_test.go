package wal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// createSealedSegment writes a sealed segment file with a single entry at the given seqNum.
func createSealedSegment(t *testing.T, dir string, seqNum uint64) {
	t.Helper()
	entry := &MOLEntry{
		SeqNum:  seqNum,
		OpType:  OpEngramWrite,
		VaultID: 1,
		Payload: []byte(fmt.Sprintf("entry-%d", seqNum)),
	}
	data, err := marshalEntry(entry)
	if err != nil {
		t.Fatalf("marshalEntry(%d): %v", seqNum, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("mol-%d.log", seqNum))
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestSafePrune_PrunesSealedSegments(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer mol.Close()

	createSealedSegment(t, dir, 10)
	createSealedSegment(t, dir, 20)
	createSealedSegment(t, dir, 30)

	pruned, err := mol.SafePrune(20)
	if err != nil {
		t.Fatalf("SafePrune: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}

	// mol-10.log and mol-20.log should be gone
	for _, seq := range []uint64{10, 20} {
		path := filepath.Join(dir, fmt.Sprintf("mol-%d.log", seq))
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("mol-%d.log should have been pruned", seq)
		}
	}

	// mol-30.log should remain
	path := filepath.Join(dir, "mol-30.log")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("mol-30.log should still exist: %v", err)
	}
}

func TestSafePrune_PreservesActiveSegment(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer mol.Close()

	// Write an entry to the active segment so it has content.
	if err := mol.Append(&MOLEntry{OpType: OpEngramWrite, VaultID: 1, Payload: []byte("x")}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	createSealedSegment(t, dir, 5)

	pruned, err := mol.SafePrune(999)
	if err != nil {
		t.Fatalf("SafePrune: %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want 1", pruned)
	}

	// Active segment must still exist.
	activePath := filepath.Join(dir, "mol-active.log")
	if _, err := os.Stat(activePath); err != nil {
		t.Errorf("mol-active.log should still exist: %v", err)
	}
}

func TestSafePrune_NoSegmentsToPrune(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer mol.Close()

	createSealedSegment(t, dir, 50)
	createSealedSegment(t, dir, 60)

	pruned, err := mol.SafePrune(10)
	if err != nil {
		t.Fatalf("SafePrune: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}

	// Both segments should still exist.
	for _, seq := range []uint64{50, 60} {
		path := filepath.Join(dir, fmt.Sprintf("mol-%d.log", seq))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("mol-%d.log should still exist: %v", seq, err)
		}
	}
}

func TestSafePrune_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer mol.Close()

	pruned, err := mol.SafePrune(100)
	if err != nil {
		t.Fatalf("SafePrune: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
}

func TestSafePrune_ZeroMinSeq(t *testing.T) {
	dir := t.TempDir()
	mol, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer mol.Close()

	createSealedSegment(t, dir, 10)
	createSealedSegment(t, dir, 20)

	pruned, err := mol.SafePrune(0)
	if err != nil {
		t.Fatalf("SafePrune: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}

	// Both segments should still exist.
	for _, seq := range []uint64{10, 20} {
		path := filepath.Join(dir, fmt.Sprintf("mol-%d.log", seq))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("mol-%d.log should still exist: %v", seq, err)
		}
	}
}
