package storage

import (
	"context"
	"testing"
	"time"
)

// newTestStore creates a PebbleStore backed by a temp dir.
// openTestPebble already registers Cleanup for the DB; we just wrap it in a store.
func newTestStore(t *testing.T) *PebbleStore {
	t.Helper()
	db := openTestPebble(t)
	return NewPebbleStore(db, PebbleStoreConfig{CacheSize: 100})
}

// TestWriteAssociationGetAssociationsRoundtrip verifies that WriteAssociation persists
// the edge and GetAssociations retrieves it with the correct fields.
func TestWriteAssociationGetAssociationsRoundtrip(t *testing.T) {
	store := newTestStore(t)

	ctx := context.Background()
	ws := store.VaultPrefix("assoc-roundtrip")

	src := NewULID()
	dst := NewULID()

	now := time.Now().Truncate(time.Millisecond)
	assoc := &Association{
		TargetID:      dst,
		RelType:       RelSupports,
		Weight:        0.65,
		Confidence:    0.9,
		CreatedAt:     now,
		LastActivated: int32(now.Unix()),
	}

	if err := store.WriteAssociation(ctx, ws, src, dst, assoc); err != nil {
		t.Fatalf("WriteAssociation: %v", err)
	}

	results, err := store.GetAssociations(ctx, ws, []ULID{src}, 10)
	if err != nil {
		t.Fatalf("GetAssociations: %v", err)
	}

	got, ok := results[src]
	if !ok {
		t.Fatal("no associations returned for src")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 association, got %d", len(got))
	}

	a := got[0]
	if a.TargetID != dst {
		t.Errorf("TargetID: got %v, want %v", a.TargetID, dst)
	}
	if a.RelType != RelSupports {
		t.Errorf("RelType: got %v, want %v", a.RelType, RelSupports)
	}
	if a.Weight < 0.64 || a.Weight > 0.66 {
		t.Errorf("Weight: got %v, want ~0.65", a.Weight)
	}
	if a.Confidence < 0.89 || a.Confidence > 0.91 {
		t.Errorf("Confidence: got %v, want ~0.9", a.Confidence)
	}
}

// TestUpdateAssocWeightPersistsCorrectly verifies that after UpdateAssocWeight the new
// weight is reflected in GetAssociations (not just the index key).
func TestUpdateAssocWeightPersistsCorrectly(t *testing.T) {
	store := newTestStore(t)

	ctx := context.Background()
	ws := store.VaultPrefix("assoc-update")

	src := NewULID()
	dst := NewULID()

	// Write initial association.
	if err := store.WriteAssociation(ctx, ws, src, dst, &Association{
		TargetID: dst,
		Weight:   0.2,
	}); err != nil {
		t.Fatalf("WriteAssociation: %v", err)
	}

	// Verify initial weight via GetAssociations.
	results, err := store.GetAssociations(ctx, ws, []ULID{src}, 10)
	if err != nil {
		t.Fatalf("GetAssociations (initial): %v", err)
	}
	if got := results[src]; len(got) != 1 || got[0].Weight < 0.19 || got[0].Weight > 0.21 {
		t.Fatalf("initial weight unexpected: %+v", results[src])
	}

	// Update weight.
	if err := store.UpdateAssocWeight(ctx, ws, src, dst, 0.85); err != nil {
		t.Fatalf("UpdateAssocWeight: %v", err)
	}

	// Verify updated weight via GetAssocWeight (O(1) index path).
	w, err := store.GetAssocWeight(ctx, ws, src, dst)
	if err != nil {
		t.Fatalf("GetAssocWeight: %v", err)
	}
	if w < 0.84 || w > 0.86 {
		t.Errorf("GetAssocWeight after update: got %v, want ~0.85", w)
	}

	// Force a cache miss by creating a fresh store backed by the same DB.
	store2 := NewPebbleStore(store.db, PebbleStoreConfig{CacheSize: 100})
	results2, err := store2.GetAssociations(ctx, ws, []ULID{src}, 10)
	if err != nil {
		t.Fatalf("GetAssociations (fresh store): %v", err)
	}
	got2 := results2[src]
	if len(got2) != 1 {
		t.Fatalf("expected 1 assoc after update in fresh store, got %d", len(got2))
	}
	if got2[0].Weight < 0.84 || got2[0].Weight > 0.86 {
		t.Errorf("persisted weight wrong: got %v, want ~0.85", got2[0].Weight)
	}
}

// TestDecayAssocWeightsReducesBelowThreshold verifies that DecayAssocWeights
// removes associations whose weight falls below the minWeight threshold.
func TestDecayAssocWeightsReducesBelowThreshold(t *testing.T) {
	store := newTestStore(t)

	ctx := context.Background()
	ws := store.VaultPrefix("decay-roundtrip")

	// Write three associations with different weights.
	pairs := [][2]ULID{
		{NewULID(), NewULID()}, // weight 0.8 — stays after 50% decay (0.4 > 0.3)
		{NewULID(), NewULID()}, // weight 0.5 — stays after 50% decay (0.25 < 0.3, removed)
		{NewULID(), NewULID()}, // weight 0.1 — removed after 50% decay (0.05 < 0.3)
	}
	weights := []float32{0.8, 0.5, 0.1}

	for i, p := range pairs {
		if err := store.WriteAssociation(ctx, ws, p[0], p[1], &Association{
			TargetID: p[1],
			Weight:   weights[i],
		}); err != nil {
			t.Fatalf("WriteAssociation[%d]: %v", i, err)
		}
	}

	// Decay by 50% with minWeight=0.3 — should remove pairs[1] and pairs[2].
	removed, err := store.DecayAssocWeights(ctx, ws, 0.5, 0.3)
	if err != nil {
		t.Fatalf("DecayAssocWeights: %v", err)
	}
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	// pairs[0] should survive with weight ~0.4.
	w0, err := store.GetAssocWeight(ctx, ws, pairs[0][0], pairs[0][1])
	if err != nil {
		t.Fatalf("GetAssocWeight[0]: %v", err)
	}
	if w0 < 0.35 || w0 > 0.45 {
		t.Errorf("surviving weight: got %v, want ~0.4", w0)
	}

	// pairs[1] should be gone.
	w1, err := store.GetAssocWeight(ctx, ws, pairs[1][0], pairs[1][1])
	if err != nil {
		t.Fatalf("GetAssocWeight[1]: %v", err)
	}
	if w1 != 0.0 {
		t.Errorf("decayed-below-min weight should be 0, got %v", w1)
	}

	// pairs[2] should be gone.
	w2, err := store.GetAssocWeight(ctx, ws, pairs[2][0], pairs[2][1])
	if err != nil {
		t.Fatalf("GetAssocWeight[2]: %v", err)
	}
	if w2 != 0.0 {
		t.Errorf("decayed-below-min weight should be 0, got %v", w2)
	}
}

// TestGetAssociationsMultipleSourceIDs verifies batch retrieval works correctly
// for multiple source IDs in a single call.
func TestGetAssociationsMultipleSourceIDs(t *testing.T) {
	store := newTestStore(t)

	ctx := context.Background()
	ws := store.VaultPrefix("assoc-batch")

	srcA := NewULID()
	srcB := NewULID()
	dst1 := NewULID()
	dst2 := NewULID()
	dst3 := NewULID()

	_ = store.WriteAssociation(ctx, ws, srcA, dst1, &Association{TargetID: dst1, Weight: 0.7})
	_ = store.WriteAssociation(ctx, ws, srcA, dst2, &Association{TargetID: dst2, Weight: 0.5})
	_ = store.WriteAssociation(ctx, ws, srcB, dst3, &Association{TargetID: dst3, Weight: 0.9})

	results, err := store.GetAssociations(ctx, ws, []ULID{srcA, srcB}, 10)
	if err != nil {
		t.Fatalf("GetAssociations: %v", err)
	}

	if len(results[srcA]) != 2 {
		t.Errorf("srcA: expected 2 associations, got %d", len(results[srcA]))
	}
	if len(results[srcB]) != 1 {
		t.Errorf("srcB: expected 1 association, got %d", len(results[srcB]))
	}
	if results[srcB][0].TargetID != dst3 {
		t.Errorf("srcB target: got %v, want %v", results[srcB][0].TargetID, dst3)
	}
}
