package consolidation

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/storage"
)

func TestTransitiveInference_BasicTriangle(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	wsPrefix := store.ResolveVaultPrefix("transitive_test")

	// Create 3 engrams: A, B, C
	a := &storage.Engram{Concept: "a", Content: "content a", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	b := &storage.Engram{Concept: "b", Content: "content b", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	c := &storage.Engram{Concept: "c", Content: "content c", Confidence: 1.0, Relevance: 0.8, Stability: 30}

	idA, _ := store.WriteEngram(ctx, wsPrefix, a)
	idB, _ := store.WriteEngram(ctx, wsPrefix, b)
	idC, _ := store.WriteEngram(ctx, wsPrefix, c)

	// A→B with weight 0.8, B→C with weight 0.9 (both above 0.7 threshold)
	store.WriteAssociation(ctx, wsPrefix, idA, idB, &storage.Association{
		TargetID: idB, Weight: 0.8, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})
	store.WriteAssociation(ctx, wsPrefix, idB, idC, &storage.Association{
		TargetID: idC, Weight: 0.9, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})

	w := &Worker{MaxTransitive: 100}
	report := &ConsolidationReport{}
	if err := w.runPhase5TransitiveInference(ctx, store, wsPrefix, report); err != nil {
		t.Fatal(err)
	}

	if report.InferredEdges != 1 {
		t.Errorf("expected 1 inferred edge (A→C), got %d", report.InferredEdges)
	}

	// Verify A→C was created with weight = 0.8 * 0.9 * 0.8 = 0.576
	weight, err := store.GetAssocWeight(ctx, wsPrefix, idA, idC)
	if err != nil {
		t.Fatal(err)
	}
	expected := float32(0.8 * 0.9 * 0.8)
	if weight < expected-0.01 || weight > expected+0.01 {
		t.Errorf("inferred weight = %v, want ~%v", weight, expected)
	}
}

func TestTransitiveInference_SkipsBelowThreshold(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	wsPrefix := store.ResolveVaultPrefix("transitive_low")

	a := &storage.Engram{Concept: "a", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	b := &storage.Engram{Concept: "b", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	c := &storage.Engram{Concept: "c", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}

	idA, _ := store.WriteEngram(ctx, wsPrefix, a)
	idB, _ := store.WriteEngram(ctx, wsPrefix, b)
	idC, _ := store.WriteEngram(ctx, wsPrefix, c)

	// A→B with weight 0.5 (below 0.7 threshold)
	store.WriteAssociation(ctx, wsPrefix, idA, idB, &storage.Association{
		TargetID: idB, Weight: 0.5, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})
	store.WriteAssociation(ctx, wsPrefix, idB, idC, &storage.Association{
		TargetID: idC, Weight: 0.9, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})

	w := &Worker{MaxTransitive: 100}
	report := &ConsolidationReport{}
	if err := w.runPhase5TransitiveInference(ctx, store, wsPrefix, report); err != nil {
		t.Fatal(err)
	}

	if report.InferredEdges != 0 {
		t.Errorf("expected 0 inferred edges (A→B below threshold), got %d", report.InferredEdges)
	}
}

func TestTransitiveInference_SkipsExisting(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	wsPrefix := store.ResolveVaultPrefix("transitive_exists")

	a := &storage.Engram{Concept: "a", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	b := &storage.Engram{Concept: "b", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	c := &storage.Engram{Concept: "c", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}

	idA, _ := store.WriteEngram(ctx, wsPrefix, a)
	idB, _ := store.WriteEngram(ctx, wsPrefix, b)
	idC, _ := store.WriteEngram(ctx, wsPrefix, c)

	// A→B, B→C both above threshold
	store.WriteAssociation(ctx, wsPrefix, idA, idB, &storage.Association{
		TargetID: idB, Weight: 0.8, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})
	store.WriteAssociation(ctx, wsPrefix, idB, idC, &storage.Association{
		TargetID: idC, Weight: 0.9, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})
	// A→C already exists
	store.WriteAssociation(ctx, wsPrefix, idA, idC, &storage.Association{
		TargetID: idC, Weight: 0.3, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})

	w := &Worker{MaxTransitive: 100}
	report := &ConsolidationReport{}
	if err := w.runPhase5TransitiveInference(ctx, store, wsPrefix, report); err != nil {
		t.Fatal(err)
	}

	if report.InferredEdges != 0 {
		t.Errorf("expected 0 inferred edges (A→C already exists), got %d", report.InferredEdges)
	}
}

func TestTransitiveInference_RespectsMaxLimit(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	wsPrefix := store.ResolveVaultPrefix("transitive_limit")

	// Create a hub B connected to many C nodes via A→B→C1, A→B→C2, etc.
	a := &storage.Engram{Concept: "a", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	b := &storage.Engram{Concept: "b", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	idA, _ := store.WriteEngram(ctx, wsPrefix, a)
	idB, _ := store.WriteEngram(ctx, wsPrefix, b)

	store.WriteAssociation(ctx, wsPrefix, idA, idB, &storage.Association{
		TargetID: idB, Weight: 0.8, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})

	for i := 0; i < 10; i++ {
		c := &storage.Engram{Concept: "c", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
		idC, _ := store.WriteEngram(ctx, wsPrefix, c)
		store.WriteAssociation(ctx, wsPrefix, idB, idC, &storage.Association{
			TargetID: idC, Weight: 0.8, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
		})
	}

	w := &Worker{MaxTransitive: 3}
	report := &ConsolidationReport{}
	if err := w.runPhase5TransitiveInference(ctx, store, wsPrefix, report); err != nil {
		t.Fatal(err)
	}

	if report.InferredEdges > 3 {
		t.Errorf("expected max 3 inferred edges, got %d", report.InferredEdges)
	}
}

func TestTransitiveInference_EmptyVault(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	wsPrefix := store.ResolveVaultPrefix("transitive_empty")

	w := &Worker{MaxTransitive: 100}
	report := &ConsolidationReport{}
	if err := w.runPhase5TransitiveInference(ctx, store, wsPrefix, report); err != nil {
		t.Fatal(err)
	}

	if report.InferredEdges != 0 {
		t.Errorf("empty vault should produce 0 inferred edges, got %d", report.InferredEdges)
	}
}

func TestTransitiveInference_NoOutgoingFromB(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	wsPrefix := store.ResolveVaultPrefix("transitive_no_b")

	a := &storage.Engram{Concept: "a", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	b := &storage.Engram{Concept: "b", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}

	idA, _ := store.WriteEngram(ctx, wsPrefix, a)
	idB, _ := store.WriteEngram(ctx, wsPrefix, b)

	// A→B high weight, but B has no outgoing edges
	store.WriteAssociation(ctx, wsPrefix, idA, idB, &storage.Association{
		TargetID: idB, Weight: 0.9, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})

	w := &Worker{MaxTransitive: 100}
	report := &ConsolidationReport{}
	if err := w.runPhase5TransitiveInference(ctx, store, wsPrefix, report); err != nil {
		t.Fatal(err)
	}

	if report.InferredEdges != 0 {
		t.Errorf("no B→C edges: expected 0 inferred, got %d", report.InferredEdges)
	}
}

func TestTransitiveInference_DryRun(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	ctx := context.Background()
	wsPrefix := store.ResolveVaultPrefix("transitive_dry")

	a := &storage.Engram{Concept: "a", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	b := &storage.Engram{Concept: "b", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}
	c := &storage.Engram{Concept: "c", Content: "x", Confidence: 1.0, Relevance: 0.8, Stability: 30}

	idA, _ := store.WriteEngram(ctx, wsPrefix, a)
	idB, _ := store.WriteEngram(ctx, wsPrefix, b)
	idC, _ := store.WriteEngram(ctx, wsPrefix, c)

	store.WriteAssociation(ctx, wsPrefix, idA, idB, &storage.Association{
		TargetID: idB, Weight: 0.8, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})
	store.WriteAssociation(ctx, wsPrefix, idB, idC, &storage.Association{
		TargetID: idC, Weight: 0.9, Confidence: 1.0, RelType: storage.RelSupports, CreatedAt: time.Now(),
	})

	w := &Worker{DryRun: true, MaxTransitive: 100}
	report := &ConsolidationReport{}
	if err := w.runPhase5TransitiveInference(ctx, store, wsPrefix, report); err != nil {
		t.Fatal(err)
	}

	if report.InferredEdges != 1 {
		t.Errorf("expected 1 inferred edge in dry run, got %d", report.InferredEdges)
	}

	weight, _ := store.GetAssocWeight(ctx, wsPrefix, idA, idC)
	if weight > 0 {
		t.Errorf("dry run should not write association, but found weight %v", weight)
	}
}
