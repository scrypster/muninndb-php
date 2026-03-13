package migrate

import (
	"fmt"
	"log/slog"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/storage/keys"
)

// BackfillRelEntityIndex scans all existing 0x21 relationship keys and writes the
// corresponding 0x26 relationship entity index entries (one for fromEntity, one for
// toEntity) for any that are missing.
//
// This migration is required for the 0x26 secondary index introduced alongside the
// ScanEntityRelationships optimisation. New writes populate the index automatically
// via UpsertRelationshipRecord; this function backfills pre-existing data.
//
// The fromHash and toHash are extracted directly from the 0x21 key bytes — no msgpack
// decode is needed. The migration is idempotent: Set on an already-present key is a no-op.
//
// 0x21 key layout (42 bytes):
//
//	0x21(1) | ws(8) | engramID(16) | fromHash(8) | relTypeByte(1) | toHash(8)
//
// Extracted offsets: ws=[1:9], engramID=[9:25], fromHash=[25:33], toHash=[34:42]
func BackfillRelEntityIndex(db *pebble.DB) error {
	iter, err := db.NewIter(&pebble.IterOptions{
		LowerBound: []byte{0x21},
		UpperBound: []byte{0x22},
	})
	if err != nil {
		return fmt.Errorf("backfill rel entity index: new iter: %w", err)
	}
	defer iter.Close()

	const batchSize = 500
	const relKeyLen = 42

	batch := db.NewBatch()
	batchCount := 0
	// relationships counts 0x21 keys processed; indexKeys counts 0x26 entries written
	// (2 per relationship: one for fromHash, one for toHash).
	relationships, indexKeys, skipped := 0, 0, 0

	for valid := iter.First(); valid; valid = iter.Next() {
		k := iter.Key()
		if len(k) != relKeyLen {
			skipped++
			continue
		}

		var ws [8]byte
		var engramID [16]byte
		var fromHash [8]byte
		var toHash [8]byte

		copy(ws[:], k[1:9])
		copy(engramID[:], k[9:25])
		copy(fromHash[:], k[25:33])
		copy(toHash[:], k[34:42])

		fromIdxKey := keys.RelEntityIndexKey(ws, fromHash, engramID)
		toIdxKey := keys.RelEntityIndexKey(ws, toHash, engramID)

		if err := batch.Set(fromIdxKey, nil, nil); err != nil {
			batch.Close()
			return fmt.Errorf("backfill rel entity index: set from key: %w", err)
		}
		if err := batch.Set(toIdxKey, nil, nil); err != nil {
			batch.Close()
			return fmt.Errorf("backfill rel entity index: set to key: %w", err)
		}
		batchCount++
		relationships++
		indexKeys += 2 // one for fromHash, one for toHash

		if batchCount >= batchSize {
			if err := batch.Commit(pebble.Sync); err != nil {
				batch.Close()
				return fmt.Errorf("backfill rel entity index: commit batch: %w", err)
			}
			batch.Close()
			batch = db.NewBatch()
			batchCount = 0
		}
	}

	if err := iter.Error(); err != nil {
		batch.Close()
		return fmt.Errorf("backfill rel entity index: iter: %w", err)
	}

	if batchCount > 0 {
		if err := batch.Commit(pebble.Sync); err != nil {
			batch.Close()
			return fmt.Errorf("backfill rel entity index: commit final batch: %w", err)
		}
	}
	batch.Close()

	slog.Info("backfill rel entity index complete",
		"relationships", relationships,
		"index_keys", indexKeys,
		"skipped", skipped,
	)
	return nil
}
