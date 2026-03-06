package adjacency_test

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/index/adjacency"
	"github.com/scrypster/muninndb/internal/storage"
	"github.com/scrypster/muninndb/internal/storage/keys"
)

// newTestDB opens a Pebble instance in a temp dir and registers cleanup.
func newTestDB(t *testing.T) *pebble.DB {
	t.Helper()
	db, err := pebble.Open(t.TempDir(), &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// testWS returns a fixed 8-byte workspace prefix for tests.
func testWS() [8]byte {
	return keys.VaultPrefix("test")
}

// newID generates a new random ULID as a [16]byte.
func newID() [16]byte {
	return [16]byte(storage.NewULID())
}

// writeAssoc is a helper that writes a single association via the Graph.
func writeAssoc(t *testing.T, g *adjacency.Graph, ws [8]byte, src, dst [16]byte, weight float32) {
	t.Helper()
	assoc := &storage.Association{
		TargetID:  storage.ULID(dst),
		Weight:    weight,
		CreatedAt: time.Now(),
	}
	if err := g.WriteAssociation(ws, src, assoc); err != nil {
		t.Fatalf("WriteAssociation(%v->%v w=%.2f): %v", src, dst, weight, err)
	}
}

// TestWriteAndGetAssociations verifies that writing A→B and calling GetAssociations(A)
// returns B with the correct weight.
func TestWriteAndGetAssociations(t *testing.T) {
	db := newTestDB(t)
	g := adjacency.New(db)
	ws := testWS()
	ctx := context.Background()

	a := newID()
	b := newID()
	writeAssoc(t, g, ws, a, b, 0.8)

	results, err := g.GetAssociations(ctx, ws, a, 100)
	if err != nil {
		t.Fatalf("GetAssociations: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 association, got %d", len(results))
	}
	if results[0].TargetID != b {
		t.Errorf("expected TargetID=%v, got %v", b, results[0].TargetID)
	}
	// Weight is reconstructed from weight complement; allow small floating-point delta.
	if diff := results[0].Weight - 0.8; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected weight ~0.8, got %v", results[0].Weight)
	}
}

// TestGetAssociationsMaxPerNode writes 25 associations from A and verifies that
// GetAssociations with maxPerNode=10 returns exactly the 10 highest-weight ones.
func TestGetAssociationsMaxPerNode(t *testing.T) {
	db := newTestDB(t)
	g := adjacency.New(db)
	ws := testWS()
	ctx := context.Background()

	a := newID()

	// Write 25 associations with weights 0.01, 0.02, ..., 0.25
	// so that we can clearly identify which are the top 10.
	for i := 1; i <= 25; i++ {
		b := newID()
		writeAssoc(t, g, ws, a, b, float32(i)*0.01)
	}

	results, err := g.GetAssociations(ctx, ws, a, 10)
	if err != nil {
		t.Fatalf("GetAssociations: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("expected 10 results with maxPerNode=10, got %d", len(results))
	}

	// The key schema sorts by weight complement (descending weight), so the
	// 10 results must all have weight >= 0.16 (the 10th highest out of 25 is 0.16).
	for i, r := range results {
		if r.Weight < 0.15 {
			t.Errorf("result[%d] weight %.4f is below expected minimum for top-10", i, r.Weight)
		}
	}
}

// TestWeightSortDescending writes 3 associations with weights 0.3, 0.9, 0.6 and
// verifies GetAssociations returns them in descending weight order (0.9, 0.6, 0.3).
func TestWeightSortDescending(t *testing.T) {
	db := newTestDB(t)
	g := adjacency.New(db)
	ws := testWS()
	ctx := context.Background()

	a := newID()
	b1, b2, b3 := newID(), newID(), newID()

	writeAssoc(t, g, ws, a, b1, 0.3)
	writeAssoc(t, g, ws, a, b2, 0.9)
	writeAssoc(t, g, ws, a, b3, 0.6)

	results, err := g.GetAssociations(ctx, ws, a, 100)
	if err != nil {
		t.Fatalf("GetAssociations: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Expect descending order: 0.9, 0.6, 0.3
	expected := []float32{0.9, 0.6, 0.3}
	for i, want := range expected {
		got := results[i].Weight
		diff := got - want
		if diff < -0.002 || diff > 0.002 {
			t.Errorf("results[%d]: expected weight ~%.1f, got %.4f", i, want, got)
		}
	}
}

// TestBFSTraversal writes a chain A→B→C→D (each with weight 0.9) and verifies
// that Traverse from [A] with maxDepth=3 reaches all 4 nodes at correct hop depths.
func TestBFSTraversal(t *testing.T) {
	db := newTestDB(t)
	g := adjacency.New(db)
	ws := testWS()
	ctx := context.Background()

	a, b, c, d := newID(), newID(), newID(), newID()
	writeAssoc(t, g, ws, a, b, 0.9)
	writeAssoc(t, g, ws, b, c, 0.9)
	writeAssoc(t, g, ws, c, d, 0.9)

	results, err := g.Traverse(ctx, ws, [][16]byte{a}, 0.01, 3)
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}

	// Build a map of ID -> hop depth from results.
	// The seed A itself is not in results (it's the starting point),
	// so we expect B(depth=1), C(depth=2), D(depth=3).
	found := make(map[[16]byte]int)
	for _, r := range results {
		found[r.ID] = r.HopDepth
	}

	expected := map[[16]byte]int{
		b: 1,
		c: 2,
		d: 3,
	}
	for id, wantDepth := range expected {
		gotDepth, ok := found[id]
		if !ok {
			t.Errorf("node not found in traversal results")
			continue
		}
		if gotDepth != wantDepth {
			t.Errorf("expected hop depth %d, got %d", wantDepth, gotDepth)
		}
	}
}

// TestBFSHopPenalty verifies that scores decrease with each hop in a traversal chain.
// The chain uses weight=0.95 and hopPenalty=0.7, so scores decay substantially at each hop.
// We use maxDepth=2 which guarantees both hop-1 and hop-2 nodes are in the results.
func TestBFSHopPenalty(t *testing.T) {
	db := newTestDB(t)
	g := adjacency.New(db)
	ws := testWS()
	ctx := context.Background()

	// Build a chain: A → B → C, high weight to exceed minHopScore over 2 hops.
	// propagated at hop 1: 1.0 * 0.95 * 0.7^1 = 0.665
	// propagated at hop 2: 0.665 * 0.95 * 0.7^2 = 0.311
	// Both are > minHopScore (0.05), so both B and C will be in results.
	a, b, c := newID(), newID(), newID()
	writeAssoc(t, g, ws, a, b, 0.95)
	writeAssoc(t, g, ws, b, c, 0.95)

	results, err := g.Traverse(ctx, ws, [][16]byte{a}, 0.001, 2)
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}

	scoreByID := make(map[[16]byte]float64)
	for _, r := range results {
		scoreByID[r.ID] = r.Score
	}

	scoreB, okB := scoreByID[b]
	scoreC, okC := scoreByID[c]
	if !okB {
		t.Fatal("node B (hop 1) not found in traversal results")
	}
	if !okC {
		t.Fatal("node C (hop 2) not found in traversal results")
	}

	// hop-1 score must be strictly greater than hop-2 score due to penalty accumulation.
	if scoreB <= scoreC {
		t.Errorf("score at hop 1 (%.4f) should be greater than score at hop 2 (%.4f)", scoreB, scoreC)
	}
}

// TestBFSCycleNoDuplicates writes A→B and B→A (a cycle) and verifies that each
// node appears at most once in the traversal results.
func TestBFSCycleNoDuplicates(t *testing.T) {
	db := newTestDB(t)
	g := adjacency.New(db)
	ws := testWS()
	ctx := context.Background()

	a := newID()
	b := newID()
	writeAssoc(t, g, ws, a, b, 0.9)
	writeAssoc(t, g, ws, b, a, 0.9)

	results, err := g.Traverse(ctx, ws, [][16]byte{a}, 0.001, 5)
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}

	seen := make(map[[16]byte]int)
	for _, r := range results {
		seen[r.ID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("node %v appeared %d times in results (expected at most 1)", id, count)
		}
	}
}

// TestBFSThresholdCutsOff writes a chain with weight 0.5 at each hop and sets
// threshold=0.3. The propagated score follows score * 0.5 * 0.7^depth.
// After enough hops, the propagated score will drop below minHopScore (0.05),
// which is the effective internal cutoff.
// We verify the traversal doesn't produce an unbounded number of results.
func TestBFSThresholdCutsOff(t *testing.T) {
	db := newTestDB(t)
	g := adjacency.New(db)
	ws := testWS()
	ctx := context.Background()

	// Build a long chain of 20 nodes with weight 0.5 per hop.
	const chainLen = 20
	ids := make([][16]byte, chainLen)
	for i := range ids {
		ids[i] = newID()
	}
	for i := 0; i < chainLen-1; i++ {
		writeAssoc(t, g, ws, ids[i], ids[i+1], 0.5)
	}

	results, err := g.Traverse(ctx, ws, [][16]byte{ids[0]}, 0.3, chainLen)
	if err != nil {
		t.Fatalf("Traverse: %v", err)
	}

	// With weight=0.5 and hopPenalty=0.7, propagated score after hop h is:
	// 1.0 * 0.5 * 0.7^h
	// This drops below minHopScore (0.05) quickly:
	//   hop 1: 0.5 * 0.7 = 0.35
	//   hop 2: 0.5 * 0.49 = 0.245 — wait, score accumulates: prev_score * weight * 0.7^depth
	// The traversal should cut off well before 20 hops.
	if len(results) >= chainLen-1 {
		t.Errorf("traversal should have been cut off by score threshold, but got %d results (chain has %d hops)", len(results), chainLen-1)
	}
}

// TestGetAssociations_NodeIDEndsIn0xFF is a regression test for the prefix upper-bound
// overflow bug. When a node ID's last byte is 0xFF, the naive "+1" calculation wraps to
// 0x00, producing upperBound < lowerBound and causing the Pebble iterator to return
// nothing. prefixSuccessor carries the increment correctly, so associations are found.
func TestGetAssociations_NodeIDEndsIn0xFF(t *testing.T) {
	db := newTestDB(t)
	g := adjacency.New(db)
	ws := testWS()
	ctx := context.Background()

	// Construct a node ID whose last byte is 0xFF to trigger the overflow.
	var srcID [16]byte
	for i := range srcID {
		srcID[i] = 0xAA
	}
	srcID[15] = 0xFF

	dstID := newID()
	writeAssoc(t, g, ws, srcID, dstID, 0.9)

	results, err := g.GetAssociations(ctx, ws, srcID, 100)
	if err != nil {
		t.Fatalf("GetAssociations: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 association for node with 0xFF last byte, got %d (prefix overflow bug)", len(results))
	}
	if results[0].TargetID != dstID {
		t.Errorf("expected TargetID=%v, got %v", dstID, results[0].TargetID)
	}
}

// TestGetAssociationsEmptyNode calls GetAssociations on a node with no associations
// and verifies it returns an empty slice without error.
func TestGetAssociationsEmptyNode(t *testing.T) {
	db := newTestDB(t)
	g := adjacency.New(db)
	ws := testWS()
	ctx := context.Background()

	lonely := newID()
	results, err := g.GetAssociations(ctx, ws, lonely, 100)
	if err != nil {
		t.Fatalf("GetAssociations on empty node returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for node with no associations, got %d", len(results))
	}
}
