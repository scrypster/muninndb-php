// Package autoassoc provides write-time automatic association creation.
package autoassoc

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/scrypster/muninndb/internal/index/hnsw"
	"github.com/scrypster/muninndb/internal/storage"
)

const (
	goalBufSize     = 512
	goalTopK        = 5
	goalMinSim      = float32(0.6)
	goalAssocWeight = float32(0.4)
	goalJobTimeout  = 5 * time.Second
	maxGoalLinks    = 20
)

// GoalJob is a pending goal→neighbor linking task.
type GoalJob struct {
	WS        [8]byte
	ID        [16]byte
	Embedding []float32
}

// GoalStore is the storage interface needed by GoalLinkWorker.
type GoalStore interface {
	WriteAssociation(ctx context.Context, wsPrefix [8]byte, sourceID, targetID storage.ULID, assoc *storage.Association) error
}

// GoalHNSW is the HNSW search interface needed by GoalLinkWorker.
type GoalHNSW interface {
	Search(ctx context.Context, ws [8]byte, vec []float32, topK int) ([]hnsw.ScoredID, error)
}

// GoalLinkWorker auto-links goal engrams to semantically related engrams at write time.
// For each new TypeGoal engram, it queries HNSW for topK=5 neighbors with
// cosine similarity >= 0.6 and creates RelSupports associations.
type GoalLinkWorker struct {
	jobs  chan GoalJob
	store GoalStore
	hnsw  GoalHNSW
	wg    sync.WaitGroup
}

// NewGoalLinkWorker creates a new GoalLinkWorker and starts a single worker goroutine.
// Call Stop() to drain the queue and shut down cleanly.
func NewGoalLinkWorker(store GoalStore, hnswIdx GoalHNSW) *GoalLinkWorker {
	w := &GoalLinkWorker{
		jobs:  make(chan GoalJob, goalBufSize),
		store: store,
		hnsw:  hnswIdx,
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// EnqueueGoalJob submits a job. If the queue is full, the job is dropped silently.
func (w *GoalLinkWorker) EnqueueGoalJob(job GoalJob) {
	select {
	case w.jobs <- job:
	default:
		slog.Warn("goal_link: job queue full, dropping", "id", storage.ULID(job.ID).String())
	}
}

// Stop drains the queue and waits for the worker to finish.
func (w *GoalLinkWorker) Stop() {
	close(w.jobs)
	w.wg.Wait()
}

func (w *GoalLinkWorker) run() {
	defer w.wg.Done()
	for job := range w.jobs {
		w.process(job)
	}
}

func (w *GoalLinkWorker) process(job GoalJob) {
	if w.hnsw == nil {
		slog.Warn("goal_link: hnsw index not initialized, skipping", "id", storage.ULID(job.ID).String())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), goalJobTimeout)
	defer cancel()

	neighbors, err := w.hnsw.Search(ctx, job.WS, job.Embedding, goalTopK)
	if err != nil {
		slog.Warn("goal_link: hnsw search failed", "id", storage.ULID(job.ID).String(), "err", err)
		return
	}

	srcID := storage.ULID(job.ID)
	linked := 0
	for _, n := range neighbors {
		if linked >= maxGoalLinks {
			break
		}
		if float32(n.Score) < goalMinSim {
			continue
		}
		if n.ID == job.ID {
			continue // skip self
		}
		dstID := storage.ULID(n.ID)
		assoc := &storage.Association{
			TargetID:   dstID,
			RelType:    storage.RelSupports,
			Weight:     goalAssocWeight,
			Confidence: 1.0,
			CreatedAt:  time.Now(),
		}
		if err := w.store.WriteAssociation(ctx, job.WS, srcID, dstID, assoc); err != nil {
			slog.Warn("goal_link: write assoc failed", "src", srcID.String(), "dst", dstID.String(), "err", err)
		} else {
			linked++
		}
	}
}
