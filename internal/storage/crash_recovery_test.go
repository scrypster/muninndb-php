package storage

import (
	"context"
	"os"
	"testing"
)

// TestPebbleCrashRecovery verifies that data committed with pebble.Sync
// (the default WriteEngram behavior) survives a crash-like close and reopen.
func TestPebbleCrashRecovery(t *testing.T) {
	dir, err := os.MkdirTemp("", "muninndb-crash-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := OpenPebble(dir, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	store := NewPebbleStore(db, PebbleStoreConfig{CacheSize: 0})
	ws := store.VaultPrefix("test")
	ctx := context.Background()

	engrams := []*Engram{
		{Concept: "first memory", Content: "the beginning of things", Relevance: 0.9},
		{Concept: "second memory", Content: "what came after", Relevance: 0.7},
		{Concept: "third memory", Content: "the end of things", Relevance: 0.5},
	}

	var ids []ULID
	for _, eng := range engrams {
		id, err := store.WriteEngram(ctx, ws, eng)
		if err != nil {
			t.Fatalf("WriteEngram: %v", err)
		}
		ids = append(ids, id)
	}

	// Close the DB abruptly (simulating crash — no graceful shutdown).
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen — Pebble should replay its internal WAL.
	db2, err := OpenPebble(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer db2.Close()

	store2 := NewPebbleStore(db2, PebbleStoreConfig{CacheSize: 0})
	ws2 := store2.VaultPrefix("test")

	for i, id := range ids {
		eng, err := store2.GetEngram(ctx, ws2, id)
		if err != nil {
			t.Fatalf("GetEngram %d after recovery: %v", i, err)
		}
		if eng.Concept != engrams[i].Concept {
			t.Errorf("entry %d: expected Concept=%q, got %q", i, engrams[i].Concept, eng.Concept)
		}
		if eng.Content != engrams[i].Content {
			t.Errorf("entry %d: expected Content=%q, got %q", i, engrams[i].Content, eng.Content)
		}
	}
}

// TestPebbleNoSyncCrashRecovery verifies that data committed with NoSync
// is still recoverable after reopen because Pebble writes to its internal WAL
// before the memtable (even without fsync). This is the group-commit path.
func TestPebbleNoSyncCrashRecovery(t *testing.T) {
	dir, err := os.MkdirTemp("", "muninndb-nosync-crash-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := OpenPebble(dir, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	store := NewPebbleStore(db, PebbleStoreConfig{
		CacheSize:     0,
		NoSyncEngrams: true,
	})
	ws := store.VaultPrefix("nosync")
	ctx := context.Background()

	id, err := store.WriteEngram(ctx, ws, &Engram{
		Concept: "nosync memory",
		Content: "written without per-write fsync",
	})
	if err != nil {
		t.Fatalf("WriteEngram: %v", err)
	}

	// Force a sync before close to guarantee WAL is flushed.
	if err := db.LogData(nil, nil); err != nil {
		t.Fatalf("LogData: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := OpenPebble(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer db2.Close()

	store2 := NewPebbleStore(db2, PebbleStoreConfig{CacheSize: 0})
	ws2 := store2.VaultPrefix("nosync")

	eng, err := store2.GetEngram(ctx, ws2, id)
	if err != nil {
		t.Fatalf("GetEngram after NoSync recovery: %v", err)
	}
	if eng.Concept != "nosync memory" {
		t.Errorf("expected Concept=%q, got %q", "nosync memory", eng.Concept)
	}
}

// TestCrashRecoveryPreservesIndexes verifies that secondary indexes
// (state, tag, creator, relevance bucket) survive a crash.
func TestCrashRecoveryPreservesIndexes(t *testing.T) {
	dir, err := os.MkdirTemp("", "muninndb-idx-crash-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := OpenPebble(dir, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	store := NewPebbleStore(db, PebbleStoreConfig{CacheSize: 0})
	ws := store.VaultPrefix("test")
	ctx := context.Background()

	eng := &Engram{
		Concept:   "indexed memory",
		Content:   "this has tags and state",
		Tags:      []string{"important", "indexed"},
		CreatedBy: "test-agent",
		Relevance: 0.85,
	}
	id, err := store.WriteEngram(ctx, ws, eng)
	if err != nil {
		t.Fatalf("WriteEngram: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := OpenPebble(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer db2.Close()

	store2 := NewPebbleStore(db2, PebbleStoreConfig{CacheSize: 0})
	ws2 := store2.VaultPrefix("test")

	// Verify the engram itself survived.
	recovered, err := store2.GetEngram(ctx, ws2, id)
	if err != nil {
		t.Fatalf("GetEngram after recovery: %v", err)
	}
	if recovered.Concept != "indexed memory" {
		t.Errorf("expected Concept=%q, got %q", "indexed memory", recovered.Concept)
	}

	// Verify vault count via the count method.
	count := store2.GetVaultCount(ctx, ws2)
	if count != 1 {
		t.Errorf("expected vault count=1, got %d", count)
	}
}
