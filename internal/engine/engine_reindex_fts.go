package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/metrics"
	"github.com/scrypster/muninndb/internal/storage"
	"github.com/scrypster/muninndb/internal/storage/keys"
)

// ReindexFTSVault clears all FTS posting lists for the named vault and rebuilds
// them using the current (Porter2-stemmed) tokenizer. Sets the FTS schema version
// marker (0x1B key) to 1 upon successful completion so dual-path fallback can be
// skipped for this vault in future queries.
//
// Returns the number of engrams re-indexed, or an error.
//
// NOTE: This method does NOT rebuild HNSW embeddings — it is FTS-only.
// The vault must exist in the registered name list or ErrVaultNotFound is returned.
func (e *Engine) ReindexFTSVault(ctx context.Context, vaultName string) (int64, error) {
	mu := e.getVaultMutex(vaultName)
	mu.Lock()
	defer mu.Unlock()

	names, err := e.store.ListVaultNames()
	if err != nil {
		return 0, fmt.Errorf("reindex-fts: list vault names: %w", err)
	}
	found := false
	for _, n := range names {
		if n == vaultName {
			found = true
			break
		}
	}
	if !found {
		return 0, fmt.Errorf("vault %q: %w", vaultName, ErrVaultNotFound)
	}

	ws := e.store.VaultPrefix(vaultName)

	// Prevent the FTS worker from submitting new jobs during the re-index window.
	if e.ftsWorker != nil {
		e.ftsWorker.SetClearing(ws, true)
		defer e.ftsWorker.SetClearing(ws, false)
	}

	// Step 1: Delete existing FTS keys for this vault via range tombstones.
	// Prefixes cleared: 0x05 (posting lists), 0x06 (trigrams),
	//                   0x08 (FTS global stats), 0x09 (per-term stats).
	db := e.store.GetDB()

	wsPlus, err := incrementWSBytes(ws)
	if err != nil {
		return 0, fmt.Errorf("reindex-fts: increment ws: %w", err)
	}

	ftsPrefixes := []byte{0x05, 0x06, 0x08, 0x09}
	batch := db.NewBatch()
	for _, p := range ftsPrefixes {
		lo := make([]byte, 9)
		lo[0] = p
		copy(lo[1:], ws[:])
		hi := make([]byte, 9)
		hi[0] = p
		copy(hi[1:], wsPlus[:])
		if err := batch.DeleteRange(lo, hi, nil); err != nil {
			batch.Close()
			return 0, fmt.Errorf("reindex-fts: delete range 0x%02X: %w", p, err)
		}
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		batch.Close()
		return 0, fmt.Errorf("reindex-fts: commit clear batch: %w", err)
	}
	batch.Close()

	// Invalidate in-memory IDF cache so stale scores are not carried forward.
	if e.fts != nil {
		e.fts.InvalidateIDFCache()
	}

	// Step 2: Re-index all engrams using the current stemmed tokenizer.
	var indexed int64
	scanErr := e.store.ScanEngrams(ctx, ws, func(eng *storage.Engram) error {
		if e.fts != nil {
			if err := e.fts.IndexEngram(ws, [16]byte(eng.ID), eng.Concept, eng.CreatedBy, eng.Content, eng.Tags); err != nil {
				slog.Warn("reindex-fts: IndexEngram failed", "vault", vaultName, "id", eng.ID, "err", err)
				metrics.FTSIndexFailures.WithLabelValues(vaultName).Inc()
			}
		}
		indexed++
		return nil
	})
	if scanErr != nil {
		return indexed, fmt.Errorf("reindex-fts: scan engrams: %w", scanErr)
	}

	// Step 3: Set the FTS version marker to 1 (fully re-indexed with stemming).
	versionKey := keys.FTSVersionKey(ws)
	if err := db.Set(versionKey, []byte{0x01}, pebble.Sync); err != nil {
		return indexed, fmt.Errorf("reindex-fts: set version marker: %w", err)
	}

	slog.Info("reindex-fts: complete", "vault", vaultName, "engrams", indexed)
	return indexed, nil
}

// incrementWSBytes returns the next workspace prefix for use as an exclusive
// upper bound in Pebble range operations.
func incrementWSBytes(ws [8]byte) ([8]byte, error) {
	result := ws
	for i := 7; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			return result, nil
		}
	}
	return [8]byte{}, fmt.Errorf("workspace prefix overflow")
}
