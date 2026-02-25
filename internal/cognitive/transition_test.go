package cognitive

import (
	"sync"
	"testing"
	"time"
)

// mockTransitionCache records Incr calls for verification.
type mockTransitionCache struct {
	mu     sync.Mutex
	counts map[[40]byte]uint32
}

func newMockTransitionCache() *mockTransitionCache {
	return &mockTransitionCache{counts: make(map[[40]byte]uint32)}
}

func (m *mockTransitionCache) IncrBy(ws [8]byte, src, dst [16]byte, n uint32) {
	var k [40]byte
	copy(k[0:8], ws[:])
	copy(k[8:24], src[:])
	copy(k[24:40], dst[:])
	m.mu.Lock()
	m.counts[k] += n
	m.mu.Unlock()
}

func (m *mockTransitionCache) getCount(ws [8]byte, src, dst [16]byte) uint32 {
	var k [40]byte
	copy(k[0:8], ws[:])
	copy(k[8:24], src[:])
	copy(k[24:40], dst[:])
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counts[k]
}

func (m *mockTransitionCache) totalPairs() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.counts)
}

func TestTransitionWorker_ProcessesEvents(t *testing.T) {
	mock := newMockTransitionCache()
	tw := NewTransitionWorker(mock)

	var ws [8]byte
	ws[0] = 0x01

	prevA := TransitionEngram{ID: [16]byte{1}}
	prevB := TransitionEngram{ID: [16]byte{2}}
	currC := TransitionEngram{ID: [16]byte{3}}
	currD := TransitionEngram{ID: [16]byte{4}}

	tw.Submit(TransitionEvent{
		WS:       ws,
		Previous: []TransitionEngram{prevA, prevB},
		Current:  []TransitionEngram{currC, currD},
	})

	// Allow worker to process.
	time.Sleep(200 * time.Millisecond)
	tw.Stop()

	// Should generate 4 pairs: A→C, A→D, B→C, B→D
	if mock.totalPairs() != 4 {
		t.Fatalf("expected 4 unique pairs, got %d", mock.totalPairs())
	}
	if c := mock.getCount(ws, [16]byte{1}, [16]byte{3}); c != 1 {
		t.Errorf("A→C: expected 1, got %d", c)
	}
	if c := mock.getCount(ws, [16]byte{1}, [16]byte{4}); c != 1 {
		t.Errorf("A→D: expected 1, got %d", c)
	}
	if c := mock.getCount(ws, [16]byte{2}, [16]byte{3}); c != 1 {
		t.Errorf("B→C: expected 1, got %d", c)
	}
	if c := mock.getCount(ws, [16]byte{2}, [16]byte{4}); c != 1 {
		t.Errorf("B→D: expected 1, got %d", c)
	}
}

func TestTransitionWorker_SkipsSelfTransitions(t *testing.T) {
	mock := newMockTransitionCache()
	tw := NewTransitionWorker(mock)

	var ws [8]byte
	sameID := [16]byte{5}

	tw.Submit(TransitionEvent{
		WS:       ws,
		Previous: []TransitionEngram{{ID: sameID}},
		Current:  []TransitionEngram{{ID: sameID}},
	})

	time.Sleep(200 * time.Millisecond)
	tw.Stop()

	if mock.totalPairs() != 0 {
		t.Errorf("expected 0 pairs (self-transition skipped), got %d", mock.totalPairs())
	}
}

func TestTransitionWorker_AggregatesDuplicates(t *testing.T) {
	mock := newMockTransitionCache()
	tw := NewTransitionWorker(mock)

	var ws [8]byte
	src := [16]byte{10}
	dst := [16]byte{20}

	// Submit the same transition 3 times in one batch.
	for i := 0; i < 3; i++ {
		tw.Submit(TransitionEvent{
			WS:       ws,
			Previous: []TransitionEngram{{ID: src}},
			Current:  []TransitionEngram{{ID: dst}},
		})
	}

	time.Sleep(200 * time.Millisecond)
	tw.Stop()

	if c := mock.getCount(ws, src, dst); c != 3 {
		t.Errorf("expected count 3 (aggregated), got %d", c)
	}
}

func TestTransitionWorker_StopDrainsPending(t *testing.T) {
	mock := newMockTransitionCache()
	tw := NewTransitionWorker(mock)

	var ws [8]byte
	for i := 0; i < 50; i++ {
		tw.Submit(TransitionEvent{
			WS:       ws,
			Previous: []TransitionEngram{{ID: [16]byte{byte(i)}}},
			Current:  []TransitionEngram{{ID: [16]byte{byte(i + 100)}}},
		})
	}

	// Stop should drain all pending items.
	tw.Stop()

	if mock.totalPairs() != 50 {
		t.Errorf("expected 50 pairs after stop drain, got %d", mock.totalPairs())
	}
}
