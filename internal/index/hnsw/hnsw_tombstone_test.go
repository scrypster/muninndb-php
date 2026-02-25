package hnsw_test

import (
	"context"
	"math/rand"
	"testing"

	"github.com/scrypster/muninndb/internal/index/hnsw"
)

// TestHNSWTombstone inserts 3 vectors, tombstones the middle one, then
// searches for a query nearest to the middle vector and verifies the
// tombstoned ID does not appear in the results.
func TestHNSWTombstone(t *testing.T) {
	db := newTestDB(t)
	idx := hnsw.New(db, testWS())
	t.Cleanup(idx.Close)

	rng := rand.New(rand.NewSource(55))
	ctx := context.Background()

	const dim = 16

	// Insert 3 orthogonal one-hot unit vectors so each is clearly distinguishable.
	// vec[i] has a 1.0 in dimension i and 0 elsewhere — maximally orthogonal.
	var ids [3][16]byte
	var vecs [3][]float32

	for i := 0; i < 3; i++ {
		ids[i] = newID(rng)
		v := make([]float32, dim)
		v[i] = 1.0 // one-hot in dimension i
		vecs[i] = v
		insertVector(t, idx, db, ids[i], vecs[i])
	}

	// Tombstone the middle vector (ids[1]).
	idx.Tombstone(ids[1])

	// Search with the middle vector as the query (k=3 to scan all nodes).
	results, err := idx.Search(ctx, vecs[1], 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// The tombstoned ID must not appear in the results.
	for _, r := range results {
		if r.ID == ids[1] {
			t.Errorf("tombstoned node ids[1] appeared in Search results")
		}
	}

	// The non-tombstoned nodes should still be reachable (at least one result).
	if len(results) == 0 {
		t.Error("expected at least one non-tombstoned result, got none")
	}
}
