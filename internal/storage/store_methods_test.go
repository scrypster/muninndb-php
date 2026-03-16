package storage

import (
	"os"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/storage/keys"
)

// TestPebbleStore_Checkpoint verifies that Checkpoint creates a readable
// Pebble database at the destination directory.
func TestPebbleStore_Checkpoint(t *testing.T) {
	store := openTestStore(t)

	// Write something so the checkpoint is non-trivial.
	if err := store.db.Set([]byte("chk-key"), []byte("chk-val"), pebble.Sync); err != nil {
		t.Fatalf("setup write: %v", err)
	}

	// Pebble requires the destination directory to not exist.
	destDir := t.TempDir() + "/checkpoint"
	t.Cleanup(func() { os.RemoveAll(destDir) })

	if err := store.Checkpoint(destDir); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	// The destination must be a valid Pebble database.
	chkDB, err := OpenPebble(destDir, DefaultOptions())
	if err != nil {
		t.Fatalf("open checkpoint db: %v", err)
	}
	defer chkDB.Close()

	val, closer, err := chkDB.Get([]byte("chk-key"))
	if err != nil {
		t.Fatalf("get from checkpoint: %v", err)
	}
	defer closer.Close()
	if string(val) != "chk-val" {
		t.Errorf("checkpoint value: got %q, want %q", val, "chk-val")
	}
}

// TestPebbleStore_PebbleMetrics verifies that PebbleMetrics returns a non-nil
// metrics struct and that DiskSpaceUsage is consistent with DiskSize.
func TestPebbleStore_PebbleMetrics(t *testing.T) {
	store := openTestStore(t)

	m := store.PebbleMetrics()
	if m == nil {
		t.Fatal("PebbleMetrics returned nil")
	}
	// DiskSize() uses Metrics() internally; values should agree.
	if got, want := store.DiskSize(), int64(m.DiskSpaceUsage()); got != want {
		t.Errorf("DiskSize()=%d != PebbleMetrics().DiskSpaceUsage()=%d", got, want)
	}
}

// TestPebbleStore_ScoringStore verifies that ScoringStore returns the same
// non-nil instance on repeated calls (shared, not re-created).
func TestPebbleStore_ScoringStore(t *testing.T) {
	store := openTestStore(t)

	s1 := store.ScoringStore()
	if s1 == nil {
		t.Fatal("ScoringStore returned nil")
	}
	s2 := store.ScoringStore()
	if s1 != s2 {
		t.Error("ScoringStore returned different instances on repeated calls — must be shared")
	}
}

// TestPebbleStore_ProvenanceStore verifies that ProvenanceStore returns the same
// non-nil instance on repeated calls (shared, not re-created).
func TestPebbleStore_ProvenanceStore(t *testing.T) {
	store := openTestStore(t)

	p1 := store.ProvenanceStore()
	if p1 == nil {
		t.Fatal("ProvenanceStore returned nil")
	}
	p2 := store.ProvenanceStore()
	if p1 != p2 {
		t.Error("ProvenanceStore returned different instances on repeated calls — must be shared")
	}
}

// TestPebbleStore_ProvenanceStore_SameAsInternal verifies that ProvenanceStore()
// returns the same instance held internally by PebbleStore (used by provWork),
// confirming there is no duplicate store.
func TestPebbleStore_ProvenanceStore_SameAsInternal(t *testing.T) {
	store := openTestStore(t)
	if store.ProvenanceStore() != store.provenance {
		t.Error("ProvenanceStore() is not the same instance as store.provenance — duplicate store detected")
	}
}

// TestPebbleStore_ClearFTSKeys verifies that ClearFTSKeys deletes all keys
// under the four FTS prefixes for the given vault workspace.
func TestPebbleStore_ClearFTSKeys(t *testing.T) {
	store := openTestStore(t)

	ws := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	wsPlus := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x09}

	// Write one key under each FTS prefix.
	ftsPrefixes := []byte{0x05, 0x06, 0x08, 0x09}
	for _, p := range ftsPrefixes {
		k := make([]byte, 9)
		k[0] = p
		copy(k[1:], ws[:])
		if err := store.db.Set(k, []byte{p}, pebble.Sync); err != nil {
			t.Fatalf("setup key 0x%02X: %v", p, err)
		}
	}

	// Also write a key with a different prefix that must NOT be deleted.
	safeKey := []byte{0x01, 0x01, 0x02, 0x03}
	if err := store.db.Set(safeKey, []byte("keep"), pebble.Sync); err != nil {
		t.Fatalf("setup safe key: %v", err)
	}

	if err := store.ClearFTSKeys(ws, wsPlus); err != nil {
		t.Fatalf("ClearFTSKeys: %v", err)
	}

	// All FTS keys must be gone.
	for _, p := range ftsPrefixes {
		k := make([]byte, 9)
		k[0] = p
		copy(k[1:], ws[:])
		_, closer, err := store.db.Get(k)
		if err == nil {
			closer.Close()
			t.Errorf("FTS key 0x%02X still present after ClearFTSKeys", p)
		}
	}

	// Non-FTS key must still exist.
	val, closer, err := store.db.Get(safeKey)
	if err != nil {
		t.Fatalf("safe key missing after ClearFTSKeys: %v", err)
	}
	defer closer.Close()
	if string(val) != "keep" {
		t.Errorf("safe key value corrupted: got %q", val)
	}
}

// TestPebbleStore_SetFTSVersionMarker verifies that SetFTSVersionMarker writes
// the correct version byte at the FTS version key for the vault.
func TestPebbleStore_SetFTSVersionMarker(t *testing.T) {
	store := openTestStore(t)

	ws := [8]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22}

	if err := store.SetFTSVersionMarker(ws, 0x01); err != nil {
		t.Fatalf("SetFTSVersionMarker: %v", err)
	}

	versionKey := keys.FTSVersionKey(ws)
	val, closer, err := store.db.Get(versionKey)
	if err != nil {
		t.Fatalf("get version marker: %v", err)
	}
	defer closer.Close()

	if len(val) != 1 || val[0] != 0x01 {
		t.Errorf("version marker: got %v, want [0x01]", val)
	}
}

// TestPebbleStore_SetFTSVersionMarker_Overwrite verifies that the version marker
// can be updated (e.g. from 0x00 to 0x01).
func TestPebbleStore_SetFTSVersionMarker_Overwrite(t *testing.T) {
	store := openTestStore(t)
	ws := [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}

	for _, ver := range []byte{0x00, 0x01, 0x02} {
		if err := store.SetFTSVersionMarker(ws, ver); err != nil {
			t.Fatalf("SetFTSVersionMarker(%d): %v", ver, err)
		}
		versionKey := keys.FTSVersionKey(ws)
		val, closer, err := store.db.Get(versionKey)
		if err != nil {
			t.Fatalf("get version marker after write(%d): %v", ver, err)
		}
		if len(val) != 1 || val[0] != ver {
			closer.Close()
			t.Errorf("version %d: got %v, want [0x%02X]", ver, val, ver)
		}
		closer.Close()
	}
}
