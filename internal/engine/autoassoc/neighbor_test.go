package autoassoc_test

import (
	"context"
	"sync"
	"testing"

	"github.com/scrypster/muninndb/internal/engine/autoassoc"
	"github.com/scrypster/muninndb/internal/index/hnsw"
	"github.com/scrypster/muninndb/internal/storage"
)

// mockStore records WriteAssociation calls for inspection.
type mockStore struct {
	mu    sync.Mutex
	calls []writeAssocCall
	err   error
}

type writeAssocCall struct {
	ws       [8]byte
	sourceID storage.ULID
	targetID storage.ULID
	assoc    *storage.Association
}

func (m *mockStore) WriteAssociation(ctx context.Context, wsPrefix [8]byte, sourceID, targetID storage.ULID, assoc *storage.Association) error {
	if m.err != nil {
		return m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, writeAssocCall{ws: wsPrefix, sourceID: sourceID, targetID: targetID, assoc: assoc})
	return nil
}

func (m *mockStore) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockStore) getCalls() []writeAssocCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]writeAssocCall, len(m.calls))
	copy(out, m.calls)
	return out
}

// mockHNSW returns a pre-configured set of results.
type mockHNSW struct {
	results []hnsw.ScoredID
	err     error
}

func (m *mockHNSW) Search(ctx context.Context, ws [8]byte, vec []float32, topK int) ([]hnsw.ScoredID, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

// TestProcessNeighborJob_NilEmbedding verifies that a job with a nil/empty
// embedding is a no-op: HNSW is never queried and no associations are written.
func TestProcessNeighborJob_NilEmbedding(t *testing.T) {
	store := &mockStore{}
	idx := &mockHNSW{results: []hnsw.ScoredID{
		{ID: [16]byte(storage.NewULID()), Score: 0.9},
	}}

	w := autoassoc.NewNeighborWorker(store, idx)

	w.EnqueueNeighborJob(autoassoc.NeighborJob{
		WS:        [8]byte{},
		ID:        [16]byte(storage.NewULID()),
		Embedding: nil,
	})

	// Stop drains the queue and waits for all workers to finish.
	w.Stop()

	if store.callCount() != 0 {
		t.Fatalf("expected 0 WriteAssociation calls for nil embedding, got %d", store.callCount())
	}
}

// TestProcessNeighborJob_ThresholdBoundary verifies the neighborMinSim=0.7
// boundary: 0.69 is excluded, 0.70 and 1.0 are included.
func TestProcessNeighborJob_ThresholdBoundary(t *testing.T) {
	tests := []struct {
		name      string
		score     float64
		wantEdges int
	}{
		{"below threshold 0.69", 0.69, 0},
		{"at threshold 0.70", 0.70, 1},
		{"above threshold 1.0", 1.0, 1},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{}
			neighborID := [16]byte(storage.NewULID())
			idx := &mockHNSW{results: []hnsw.ScoredID{
				{ID: neighborID, Score: tc.score},
			}}

			w := autoassoc.NewNeighborWorker(store, idx)

			w.EnqueueNeighborJob(autoassoc.NeighborJob{
				WS:        [8]byte{},
				ID:        [16]byte(storage.NewULID()),
				Embedding: []float32{1.0, 0.0},
			})

			w.Stop()

			got := store.callCount()
			if got != tc.wantEdges {
				t.Fatalf("score=%.2f: expected %d edge(s), got %d", tc.score, tc.wantEdges, got)
			}
		})
	}
}

// TestProcessNeighborJob_SkipsSelfLink verifies that a neighbor result whose
// ID matches the source engram ID does not produce a self-association.
func TestProcessNeighborJob_SkipsSelfLink(t *testing.T) {
	store := &mockStore{}
	selfID := [16]byte(storage.NewULID())

	idx := &mockHNSW{results: []hnsw.ScoredID{
		{ID: selfID, Score: 0.9},
	}}

	w := autoassoc.NewNeighborWorker(store, idx)

	w.EnqueueNeighborJob(autoassoc.NeighborJob{
		WS:        [8]byte{},
		ID:        selfID, // same ID returned by HNSW
		Embedding: []float32{1.0, 0.0},
	})

	w.Stop()

	if store.callCount() != 0 {
		t.Fatalf("expected 0 WriteAssociation calls (self-link skipped), got %d", store.callCount())
	}
}

// TestProcessNeighborJob_WeightCapped verifies that association weight is
// computed as score*0.5 and is correctly capped at neighborMaxWeight (0.5).
func TestProcessNeighborJob_WeightCapped(t *testing.T) {
	t.Run("score=1.0 weight capped at 0.5", func(t *testing.T) {
		store := &mockStore{}
		idx := &mockHNSW{results: []hnsw.ScoredID{
			{ID: [16]byte(storage.NewULID()), Score: 1.0},
		}}

		w := autoassoc.NewNeighborWorker(store, idx)

		w.EnqueueNeighborJob(autoassoc.NeighborJob{
			WS:        [8]byte{},
			ID:        [16]byte(storage.NewULID()),
			Embedding: []float32{1.0, 0.0},
		})

		w.Stop()

		calls := store.getCalls()
		if len(calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(calls))
		}
		const wantWeight = float32(0.5)
		if calls[0].assoc.Weight != wantWeight {
			t.Errorf("weight = %v, want %v (cap)", calls[0].assoc.Weight, wantWeight)
		}
	})

	t.Run("score=0.8 weight below cap", func(t *testing.T) {
		store := &mockStore{}
		idx := &mockHNSW{results: []hnsw.ScoredID{
			{ID: [16]byte(storage.NewULID()), Score: 0.8},
		}}

		w := autoassoc.NewNeighborWorker(store, idx)

		w.EnqueueNeighborJob(autoassoc.NeighborJob{
			WS:        [8]byte{},
			ID:        [16]byte(storage.NewULID()),
			Embedding: []float32{1.0, 0.0},
		})

		w.Stop()

		calls := store.getCalls()
		if len(calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(calls))
		}
		const wantWeight = float32(0.8 * 0.5) // 0.4
		if calls[0].assoc.Weight != wantWeight {
			t.Errorf("weight = %v, want %v", calls[0].assoc.Weight, wantWeight)
		}
	})
}

// TestEnqueueNeighborJob_QueueFullNoPanic verifies that enqueueing beyond the
// buffer capacity does not panic and that the Dropped counter is incremented.
func TestEnqueueNeighborJob_QueueFullNoPanic(t *testing.T) {
	// Use a blocking HNSW so workers stay busy and cannot drain the queue.
	blockCh := make(chan struct{})
	idx := &blockingHNSW{block: blockCh}
	store := &mockStore{}

	w := autoassoc.NewNeighborWorker(store, idx)
	defer func() {
		close(blockCh)
		w.Stop()
	}()

	// neighborBufSize = 4096; enqueue well past that to guarantee drops.
	const overFill = 4096 + 128
	for i := 0; i < overFill; i++ {
		w.EnqueueNeighborJob(autoassoc.NeighborJob{
			WS:        [8]byte{},
			ID:        [16]byte(storage.NewULID()),
			Embedding: []float32{1.0},
		})
	}

	_, _, dropped, _ := w.GetNeighborMetrics()
	if dropped < 1 {
		t.Fatalf("expected at least 1 dropped job, got %d", dropped)
	}
}

// blockingHNSW is a test double whose Search blocks until the block channel
// is closed.  It is used to keep workers busy so the queue fills up.
type blockingHNSW struct {
	block <-chan struct{}
}

func (b *blockingHNSW) Search(ctx context.Context, ws [8]byte, vec []float32, topK int) ([]hnsw.ScoredID, error) {
	select {
	case <-b.block:
	case <-ctx.Done():
	}
	return nil, nil
}

// TestProcessNeighborJob_MultipleNeighbors verifies that only results meeting
// the similarity threshold produce WriteAssociation calls.
func TestProcessNeighborJob_MultipleNeighbors(t *testing.T) {
	idA := [16]byte(storage.NewULID())
	idB := [16]byte(storage.NewULID())
	idC := [16]byte(storage.NewULID())

	store := &mockStore{}
	idx := &mockHNSW{results: []hnsw.ScoredID{
		{ID: idA, Score: 0.9}, // qualifies
		{ID: idB, Score: 0.5}, // below threshold, excluded
		{ID: idC, Score: 0.8}, // qualifies
	}}

	w := autoassoc.NewNeighborWorker(store, idx)

	w.EnqueueNeighborJob(autoassoc.NeighborJob{
		WS:        [8]byte{},
		ID:        [16]byte(storage.NewULID()),
		Embedding: []float32{1.0, 0.0},
	})

	w.Stop()

	calls := store.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 WriteAssociation calls (A and C), got %d", len(calls))
	}

	// Verify the two written target IDs are A and C (order may vary).
	written := make(map[[16]byte]bool, 2)
	for _, c := range calls {
		written[[16]byte(c.targetID)] = true
	}
	if !written[idA] {
		t.Errorf("expected edge to neighbor A (%v) but it was not written", idA)
	}
	if !written[idC] {
		t.Errorf("expected edge to neighbor C (%v) but it was not written", idC)
	}
	if written[idB] {
		t.Errorf("neighbor B (score=0.5) should have been excluded but was written")
	}
}
