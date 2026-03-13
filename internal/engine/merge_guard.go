package engine

import (
	"hash/fnv"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

// mergeGuardStripes is the number of merge-guard stripes. 256 gives a ~0.4%
// false-sharing probability for any two random entity names, which is negligible
// for an infrequent administrative operation like MergeEntity.
const mergeGuardStripes = 256

// mergeGuard serialises concurrent MergeEntity calls that touch the same entities.
//
// Why a separate guard from the storage-layer entityLocks?
// The storage layer acquires an entity stripe lock inside UpsertEntityRecord to
// prevent TOCTOU on individual reads. If MergeEntity held the same stripe lock
// for its entire duration and then called UpsertEntityRecord internally, it would
// deadlock — sync.Mutex is not reentrant. mergeGuard uses its own independent
// stripe array that never interacts with storage-layer locks.
//
// Concurrency guarantees:
//   - MergeEntity(A→B) and MergeEntity(A→C) are serialised because both acquire
//     stripe(A). One blocks until the other completes, preventing A's engrams from
//     being split across two targets.
//   - MergeEntity(A→B) and MergeEntity(B→C) are serialised because they share stripe(B).
//   - MergeEntity(A→B) and MergeEntity(A→B) (duplicate call) are serialised identically.
//   - MergeEntity(A→B) and MergeEntity(B→A) acquire the same two stripes in the same
//     canonical (ascending) order, so there is no deadlock.
//   - Unrelated merges proceed concurrently unless they happen to share a stripe
//     (acceptable false sharing at 1/256 probability per entity).
type mergeGuard struct {
	mu [mergeGuardStripes]sync.Mutex
}

// stripeIndex returns the stripe index for an entity name using the same
// NFKC normalisation + lowercase + trim pipeline as getEntityLock in the storage
// layer. Consistent normalisation ensures that entity names which differ only in
// Unicode representation (e.g. "café" vs "cafe\u0301") map to the same stripe.
func (g *mergeGuard) stripeIndex(name string) uint32 {
	normalized := strings.ToLower(strings.TrimSpace(norm.NFKC.String(name)))
	h := fnv.New32a()
	h.Write([]byte(normalized))
	return h.Sum32() % mergeGuardStripes
}

// Lock acquires the stripe locks for both entityA and entityB in ascending stripe
// index order to prevent deadlock. If both names hash to the same stripe it is
// locked exactly once.
func (g *mergeGuard) Lock(entityA, entityB string) {
	idxA := g.stripeIndex(entityA)
	idxB := g.stripeIndex(entityB)

	switch {
	case idxA == idxB:
		g.mu[idxA].Lock()
	case idxA < idxB:
		g.mu[idxA].Lock()
		g.mu[idxB].Lock()
	default:
		g.mu[idxB].Lock()
		g.mu[idxA].Lock()
	}
}

// Unlock releases the stripe locks acquired by Lock for the same (entityA, entityB) pair.
// Must be called with the same arguments as the preceding Lock call.
func (g *mergeGuard) Unlock(entityA, entityB string) {
	idxA := g.stripeIndex(entityA)
	idxB := g.stripeIndex(entityB)
	g.mu[idxA].Unlock()
	if idxA != idxB {
		g.mu[idxB].Unlock()
	}
}
