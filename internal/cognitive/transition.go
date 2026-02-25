package cognitive

import (
	"context"
	"log/slog"
	"time"
)

const (
	TransitionPassInterval = 30 * time.Second
)

// TransitionEvent records the sequential relationship between two activations.
// Previous contains engram IDs from the prior activation; Current contains
// engram IDs from the activation that just completed. The worker generates
// all (prev[i] → curr[j]) transition pairs.
type TransitionEvent struct {
	WS       [8]byte
	Previous []TransitionEngram
	Current  []TransitionEngram
}

// TransitionEngram is one engram in a transition event.
type TransitionEngram struct {
	ID [16]byte
}

// TransitionCacheStore is the storage interface the TransitionWorker writes to.
// In production this is *storage.TransitionCache (the tiered cache).
type TransitionCacheStore interface {
	IncrBy(ws [8]byte, src, dst [16]byte, n uint32)
}

// TransitionWorker records sequential activation transitions for PAS.
// It follows the same lifecycle pattern as HebbianWorker.
type TransitionWorker struct {
	*Worker[TransitionEvent]
	store  TransitionCacheStore
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewTransitionWorker creates and starts a new TransitionWorker.
func NewTransitionWorker(store TransitionCacheStore) *TransitionWorker {
	tw := &TransitionWorker{
		store:  store,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}

	tw.Worker = NewWorker[TransitionEvent](
		5000, 200, TransitionPassInterval,
		tw.processBatch,
	)
	go func() {
		defer close(tw.doneCh)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-tw.stopCh
			cancel()
		}()
		tw.Worker.Run(ctx) //nolint:errcheck
	}()
	return tw
}

// Run bridges an external context to the auto-started worker's lifecycle.
func (tw *TransitionWorker) Run(ctx context.Context) {
	select {
	case <-ctx.Done():
		tw.Stop()
	case <-tw.stopCh:
	}
	<-tw.doneCh
}

// Stop signals the TransitionWorker to flush pending work and shut down.
func (tw *TransitionWorker) Stop() {
	select {
	case <-tw.stopCh:
	default:
		close(tw.stopCh)
	}
	<-tw.doneCh
}

func (tw *TransitionWorker) processBatch(ctx context.Context, batch []TransitionEvent) error {
	// Aggregate all (src → dst) pairs across the batch to minimize
	// cache operations. Each event generates len(prev) × len(curr) pairs.
	type transKey struct {
		ws  [8]byte
		src [16]byte
		dst [16]byte
	}
	counts := make(map[transKey]uint32)

	for _, ev := range batch {
		for _, prev := range ev.Previous {
			for _, curr := range ev.Current {
				if prev.ID == curr.ID {
					continue
				}
				counts[transKey{ws: ev.WS, src: prev.ID, dst: curr.ID}]++
			}
		}
	}

	if len(counts) == 0 {
		return nil
	}

	for k, n := range counts {
		tw.store.IncrBy(k.ws, k.src, k.dst, n)
	}

	slog.Debug("transition: processed batch",
		"events", len(batch),
		"unique_pairs", len(counts))

	return nil
}
