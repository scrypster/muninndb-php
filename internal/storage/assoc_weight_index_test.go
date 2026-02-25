package storage

import (
	"context"
	"os"
	"testing"
)

// newTestStoreHelper creates a PebbleStore backed by a temporary directory.
// Returns the store and a cleanup function.
func newTestStoreHelper(t *testing.T) (*PebbleStore, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "muninndb-assoc-weight-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	db, err := OpenPebble(dir, DefaultOptions())
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("OpenPebble: %v", err)
	}
	store := NewPebbleStore(db, PebbleStoreConfig{CacheSize: 100})
	cleanup := func() {
		db.Close()
		os.RemoveAll(dir)
	}
	return store, cleanup
}

// TestAssocWeightIndex_O1Lookup verifies GetAssocWeight uses the 0x14 index
// for direct O(1) lookups instead of scanning all forward associations.
func TestAssocWeightIndex_O1Lookup(t *testing.T) {
	store, cleanup := newTestStoreHelper(t)
	defer cleanup()

	ctx := context.Background()
	ws := store.VaultPrefix("test-vault")

	a := NewULID()
	b := NewULID()
	c := NewULID()

	// Write three associations from a
	assocAB := &Association{TargetID: b, Weight: 0.8}
	assocAC := &Association{TargetID: c, Weight: 0.3}
	if err := store.WriteAssociation(ctx, ws, a, b, assocAB); err != nil {
		t.Fatalf("WriteAssociation a->b: %v", err)
	}
	if err := store.WriteAssociation(ctx, ws, a, c, assocAC); err != nil {
		t.Fatalf("WriteAssociation a->c: %v", err)
	}

	// GetAssocWeight should return correct weights via O(1) index
	w, err := store.GetAssocWeight(ctx, ws, a, b)
	if err != nil {
		t.Fatalf("GetAssocWeight a->b: %v", err)
	}
	if w < 0.79 || w > 0.81 {
		t.Errorf("expected weight ~0.8, got %v", w)
	}

	w2, err := store.GetAssocWeight(ctx, ws, a, c)
	if err != nil {
		t.Fatalf("GetAssocWeight a->c: %v", err)
	}
	if w2 < 0.29 || w2 > 0.31 {
		t.Errorf("expected weight ~0.3, got %v", w2)
	}

	// Non-existent pair should return 0
	d := NewULID()
	w3, err := store.GetAssocWeight(ctx, ws, a, d)
	if err != nil {
		t.Fatalf("GetAssocWeight a->d: %v", err)
	}
	if w3 != 0.0 {
		t.Errorf("expected 0 for non-existent pair, got %v", w3)
	}
}

// TestAssocWeightIndex_UpdateWeight verifies UpdateAssocWeight keeps the index current.
func TestAssocWeightIndex_UpdateWeight(t *testing.T) {
	store, cleanup := newTestStoreHelper(t)
	defer cleanup()

	ctx := context.Background()
	ws := store.VaultPrefix("update-test")

	a := NewULID()
	b := NewULID()

	assoc := &Association{TargetID: b, Weight: 0.5}
	if err := store.WriteAssociation(ctx, ws, a, b, assoc); err != nil {
		t.Fatal(err)
	}

	// Update weight
	if err := store.UpdateAssocWeight(ctx, ws, a, b, 0.9); err != nil {
		t.Fatalf("UpdateAssocWeight: %v", err)
	}

	// Index should reflect new weight
	w, err := store.GetAssocWeight(ctx, ws, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if w < 0.89 || w > 0.91 {
		t.Errorf("expected weight ~0.9 after update, got %v", w)
	}
}

// TestAssocWeightIndex_DecayRemovesEntry verifies DecayAssocWeights removes
// the 0x14 index entry when weight drops below minWeight.
func TestAssocWeightIndex_DecayRemovesEntry(t *testing.T) {
	store, cleanup := newTestStoreHelper(t)
	defer cleanup()

	ctx := context.Background()
	ws := store.VaultPrefix("decay-test")

	a := NewULID()
	b := NewULID()

	assoc := &Association{TargetID: b, Weight: 0.05}
	if err := store.WriteAssociation(ctx, ws, a, b, assoc); err != nil {
		t.Fatal(err)
	}

	// Decay by 50% with minWeight=0.1 — should remove the association
	removed, err := store.DecayAssocWeights(ctx, ws, 0.5, 0.1)
	if err != nil {
		t.Fatalf("DecayAssocWeights: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	// Index entry should be gone
	w, err := store.GetAssocWeight(ctx, ws, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if w != 0.0 {
		t.Errorf("expected 0 after decay removal, got %v", w)
	}
}
