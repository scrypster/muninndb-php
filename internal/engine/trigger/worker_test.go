package trigger

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/storage"
)

// ---------------------------------------------------------------------------
// Mocks for TriggerWorker tests
// ---------------------------------------------------------------------------

type mockTriggerStore struct {
	mu       sync.Mutex
	metas    map[storage.ULID]*storage.EngramMeta
	engrams  map[storage.ULID]*storage.Engram
}

func newMockTriggerStore() *mockTriggerStore {
	return &mockTriggerStore{
		metas:   make(map[storage.ULID]*storage.EngramMeta),
		engrams: make(map[storage.ULID]*storage.Engram),
	}
}

func (m *mockTriggerStore) GetMetadata(_ context.Context, _ [8]byte, ids []storage.ULID) ([]*storage.EngramMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*storage.EngramMeta
	for _, id := range ids {
		if meta, ok := m.metas[id]; ok {
			out = append(out, meta)
		}
	}
	return out, nil
}

func (m *mockTriggerStore) GetEngrams(_ context.Context, _ [8]byte, ids []storage.ULID) ([]*storage.Engram, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*storage.Engram
	for _, id := range ids {
		if eng, ok := m.engrams[id]; ok {
			out = append(out, eng)
		}
	}
	return out, nil
}

func (m *mockTriggerStore) GetEmbedding(_ context.Context, _ [8]byte, _ storage.ULID) ([]float32, error) {
	return nil, nil
}

func (m *mockTriggerStore) VaultPrefix(_ string) [8]byte {
	return [8]byte{}
}


// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestTriggerWorker_HandleWrite_NewEngram(t *testing.T) {
	registry := newRegistry()
	deliver := &DeliveryRouter{registry: registry}

	var pushCount atomic.Int32
	sub := &Subscription{
		ID:             "test-sub-1",
		VaultID:        1,
		Context:        []string{"test context"},
		Threshold:      0.0,
		DeltaThreshold: 0.0,
		PushOnWrite:    true,
		expiresAt:      time.Now().Add(1 * time.Hour),
		Deliver: func(ctx context.Context, push *ActivationPush) error {
			pushCount.Add(1)
			return nil
		},
		pushedScores: make(map[storage.ULID]float64),
		rateLimiter:  newTokenBucket(10),
	}
	registry.Add(sub)

	writeCh := make(chan *EngramEvent, 10)
	cogCh := make(chan CognitiveEvent, 10)
	contraCh := make(chan ContradictEvent, 10)

	worker := &TriggerWorker{
		registry:     registry,
		embedCache:   newEmbedCache(),
		deliver:      deliver,
		writeEvents:  writeCh,
		cogEvents:    cogCh,
		contraEvents: contraCh,
	}

	engID := storage.NewULID()
	writeCh <- &EngramEvent{
		VaultID: 1,
		IsNew:   true,
		Engram: &storage.Engram{
			ID:         engID,
			Concept:    "test concept",
			Content:    "test content",
			Confidence: 0.9,
			Relevance:  0.8,
			Stability:  30,
			CreatedAt:  time.Now(),
			LastAccess: time.Now(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if pushCount.Load() < 1 {
		t.Errorf("expected at least 1 push for new write, got %d", pushCount.Load())
	}
}

func TestTriggerWorker_HandleWrite_SkipsUpdates(t *testing.T) {
	registry := newRegistry()
	deliver := &DeliveryRouter{registry: registry}

	var pushCount atomic.Int32
	sub := &Subscription{
		ID:          "test-sub-2",
		VaultID:     1,
		Context:     []string{"test"},
		Threshold:   0.0,
		PushOnWrite: true,
		expiresAt:   time.Now().Add(1 * time.Hour),
		Deliver: func(ctx context.Context, push *ActivationPush) error {
			pushCount.Add(1)
			return nil
		},
		pushedScores: make(map[storage.ULID]float64),
		rateLimiter:  newTokenBucket(10),
	}
	registry.Add(sub)

	writeCh := make(chan *EngramEvent, 10)
	cogCh := make(chan CognitiveEvent, 10)
	contraCh := make(chan ContradictEvent, 10)

	worker := &TriggerWorker{
		registry:     registry,
		embedCache:   newEmbedCache(),
		deliver:      deliver,
		writeEvents:  writeCh,
		cogEvents:    cogCh,
		contraEvents: contraCh,
	}

	writeCh <- &EngramEvent{
		VaultID: 1,
		IsNew:   false,
		Engram: &storage.Engram{
			ID:         storage.NewULID(),
			Concept:    "updated",
			Content:    "updated content",
			Confidence: 0.9,
			Relevance:  0.8,
			Stability:  30,
			CreatedAt:  time.Now(),
			LastAccess: time.Now(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if pushCount.Load() != 0 {
		t.Errorf("expected 0 pushes for update (not new), got %d", pushCount.Load())
	}
}

func TestTriggerWorker_HandleContradiction(t *testing.T) {
	registry := newRegistry()
	deliver := &DeliveryRouter{registry: registry}
	tStore := newMockTriggerStore()

	engA := storage.NewULID()
	engB := storage.NewULID()
	tStore.engrams[engA] = &storage.Engram{ID: engA, Concept: "claim 1", Content: "x", State: storage.StateActive}
	tStore.engrams[engB] = &storage.Engram{ID: engB, Concept: "claim 2", Content: "y", State: storage.StateActive}

	var pushCount atomic.Int32
	sub := &Subscription{
		ID:        "contra-sub",
		VaultID:   1,
		Context:   []string{"test"},
		Threshold: 0.0,
		expiresAt: time.Now().Add(1 * time.Hour),
		Deliver: func(ctx context.Context, push *ActivationPush) error {
			pushCount.Add(1)
			return nil
		},
		pushedScores: map[storage.ULID]float64{engA: 0.8},
		rateLimiter:  newTokenBucket(10),
	}
	registry.Add(sub)

	writeCh := make(chan *EngramEvent, 10)
	cogCh := make(chan CognitiveEvent, 10)
	contraCh := make(chan ContradictEvent, 10)

	worker := &TriggerWorker{
		registry:     registry,
		embedCache:   newEmbedCache(),
		store:        tStore,
		deliver:      deliver,
		writeEvents:  writeCh,
		cogEvents:    cogCh,
		contraEvents: contraCh,
	}

	contraCh <- ContradictEvent{
		VaultID:  1,
		EngramA:  engA,
		EngramB:  engB,
		Severity: 0.8,
		Type:     "semantic",
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if pushCount.Load() < 1 {
		t.Errorf("expected at least 1 contradiction push, got %d", pushCount.Load())
	}
}

func TestTriggerWorker_ContextCancellation(t *testing.T) {
	registry := newRegistry()
	deliver := &DeliveryRouter{registry: registry}

	writeCh := make(chan *EngramEvent, 10)
	cogCh := make(chan CognitiveEvent, 10)
	contraCh := make(chan ContradictEvent, 10)

	worker := &TriggerWorker{
		registry:     registry,
		embedCache:   newEmbedCache(),
		deliver:      deliver,
		writeEvents:  writeCh,
		cogEvents:    cogCh,
		contraEvents: contraCh,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("TriggerWorker.Run did not exit after context cancellation")
	}
}

func TestTriggerWorker_ChannelClose(t *testing.T) {
	registry := newRegistry()
	deliver := &DeliveryRouter{registry: registry}

	writeCh := make(chan *EngramEvent, 10)
	cogCh := make(chan CognitiveEvent, 10)
	contraCh := make(chan ContradictEvent, 10)

	worker := &TriggerWorker{
		registry:     registry,
		embedCache:   newEmbedCache(),
		deliver:      deliver,
		writeEvents:  writeCh,
		cogEvents:    cogCh,
		contraEvents: contraCh,
	}

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	close(contraCh)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("TriggerWorker.Run did not exit after channel close")
	}
}
