package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// UpsertRelationshipRecord — 0x26 index writes
// ---------------------------------------------------------------------------

func TestUpsertRelationshipRecord_WritesRelEntityIndex(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("rel-index-write")

	eng := makeTestEngram("relationship index write test")
	_, err := store.WriteEngram(ctx, ws, eng)
	require.NoError(t, err)

	require.NoError(t, store.UpsertRelationshipRecord(ctx, ws, eng.ID, RelationshipRecord{
		FromEntity: "payment-service",
		ToEntity:   "PostgreSQL",
		RelType:    "uses",
		Weight:     0.9,
		Source:     "test",
	}))

	// ScanEntityRelationships must find the record via the 0x26 index for fromEntity.
	var fromRels []RelationshipRecord
	require.NoError(t, store.ScanEntityRelationships(ctx, ws, "payment-service",
		func(r RelationshipRecord) error {
			fromRels = append(fromRels, r)
			return nil
		}))
	require.Len(t, fromRels, 1, "0x26 index must route fromEntity query to the record")
	assert.Equal(t, "payment-service", fromRels[0].FromEntity)
	assert.Equal(t, "PostgreSQL", fromRels[0].ToEntity)

	// ScanEntityRelationships must also find the record via the 0x26 index for toEntity.
	var toRels []RelationshipRecord
	require.NoError(t, store.ScanEntityRelationships(ctx, ws, "PostgreSQL",
		func(r RelationshipRecord) error {
			toRels = append(toRels, r)
			return nil
		}))
	require.Len(t, toRels, 1, "0x26 index must route toEntity query to the record")
	assert.Equal(t, "PostgreSQL", toRels[0].ToEntity)
}

// ---------------------------------------------------------------------------
// ScanEntityRelationships — filtering
// ---------------------------------------------------------------------------

func TestScanEntityRelationships_ReturnsOnlyEntityRelationships(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("rel-index-filter")

	eng1 := makeTestEngram("engram one")
	eng2 := makeTestEngram("engram two")
	_, err := store.WriteEngram(ctx, ws, eng1)
	require.NoError(t, err)
	_, err = store.WriteEngram(ctx, ws, eng2)
	require.NoError(t, err)

	// eng1 has payment-service → PostgreSQL
	require.NoError(t, store.UpsertRelationshipRecord(ctx, ws, eng1.ID, RelationshipRecord{
		FromEntity: "payment-service",
		ToEntity:   "PostgreSQL",
		RelType:    "uses",
		Weight:     0.9,
		Source:     "test",
	}))
	// eng2 has auth-service → Redis (unrelated to PostgreSQL)
	require.NoError(t, store.UpsertRelationshipRecord(ctx, ws, eng2.ID, RelationshipRecord{
		FromEntity: "auth-service",
		ToEntity:   "Redis",
		RelType:    "uses",
		Weight:     0.8,
		Source:     "test",
	}))

	var rels []RelationshipRecord
	require.NoError(t, store.ScanEntityRelationships(ctx, ws, "PostgreSQL",
		func(r RelationshipRecord) error {
			rels = append(rels, r)
			return nil
		}))

	require.Len(t, rels, 1, "must only return relationships involving PostgreSQL")
	assert.Equal(t, "PostgreSQL", rels[0].ToEntity)
	assert.NotEqual(t, "Redis", rels[0].ToEntity, "unrelated Redis relationship must not appear")
}

func TestScanEntityRelationships_BothDirections(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("rel-index-directions")

	eng1 := makeTestEngram("from direction")
	eng2 := makeTestEngram("to direction")
	_, err := store.WriteEngram(ctx, ws, eng1)
	require.NoError(t, err)
	_, err = store.WriteEngram(ctx, ws, eng2)
	require.NoError(t, err)

	// eng1: PostgreSQL is fromEntity
	require.NoError(t, store.UpsertRelationshipRecord(ctx, ws, eng1.ID, RelationshipRecord{
		FromEntity: "PostgreSQL",
		ToEntity:   "payment-service",
		RelType:    "used_by",
		Weight:     0.8,
		Source:     "test",
	}))
	// eng2: PostgreSQL is toEntity
	require.NoError(t, store.UpsertRelationshipRecord(ctx, ws, eng2.ID, RelationshipRecord{
		FromEntity: "auth-service",
		ToEntity:   "PostgreSQL",
		RelType:    "uses",
		Weight:     0.9,
		Source:     "test",
	}))

	var rels []RelationshipRecord
	require.NoError(t, store.ScanEntityRelationships(ctx, ws, "PostgreSQL",
		func(r RelationshipRecord) error {
			rels = append(rels, r)
			return nil
		}))

	require.Len(t, rels, 2, "must find records where entity is either from or to")
}

func TestScanEntityRelationships_NoopOnMissing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("rel-index-noop")

	var rels []RelationshipRecord
	require.NoError(t, store.ScanEntityRelationships(ctx, ws, "NonExistent",
		func(r RelationshipRecord) error {
			rels = append(rels, r)
			return nil
		}))
	assert.Empty(t, rels, "must return empty for entity with no relationships")
}

// ---------------------------------------------------------------------------
// DeleteEngram — 0x26 index cleanup
// ---------------------------------------------------------------------------

func TestDeleteEngram_CleansRelEntityIndex(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("rel-index-cleanup")

	eng := makeTestEngram("relationship cleanup test")
	_, err := store.WriteEngram(ctx, ws, eng)
	require.NoError(t, err)

	require.NoError(t, store.UpsertRelationshipRecord(ctx, ws, eng.ID, RelationshipRecord{
		FromEntity: "payment-service",
		ToEntity:   "PostgreSQL",
		RelType:    "uses",
		Weight:     0.9,
		Source:     "test",
	}))

	// Verify 0x26 index is populated before delete.
	var before []RelationshipRecord
	require.NoError(t, store.ScanEntityRelationships(ctx, ws, "PostgreSQL",
		func(r RelationshipRecord) error {
			before = append(before, r)
			return nil
		}))
	require.Len(t, before, 1, "0x26 index must be populated before delete")

	require.NoError(t, store.DeleteEngram(ctx, ws, eng.ID))

	// After hard delete: 0x26 entries must be gone.
	var after []RelationshipRecord
	require.NoError(t, store.ScanEntityRelationships(ctx, ws, "PostgreSQL",
		func(r RelationshipRecord) error {
			after = append(after, r)
			return nil
		}))
	assert.Empty(t, after, "0x26 relationship entity index must be cleaned up after DeleteEngram")

	// Also verify from-entity side.
	var fromAfter []RelationshipRecord
	require.NoError(t, store.ScanEntityRelationships(ctx, ws, "payment-service",
		func(r RelationshipRecord) error {
			fromAfter = append(fromAfter, r)
			return nil
		}))
	assert.Empty(t, fromAfter, "0x26 from-entity index must also be cleaned up after DeleteEngram")
}

// ---------------------------------------------------------------------------
// DeleteEntityEngramLink
// ---------------------------------------------------------------------------

func TestDeleteEntityEngramLink_RemovesForwardAndReverseKeys(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("delete-entity-link")

	require.NoError(t, store.UpsertEntityRecord(ctx, EntityRecord{
		Name: "PostgreSQL", Type: "database", Confidence: 0.9,
	}, "test"))

	eng := makeTestEngram("test engram")
	_, err := store.WriteEngram(ctx, ws, eng)
	require.NoError(t, err)
	require.NoError(t, store.WriteEntityEngramLink(ctx, ws, eng.ID, "PostgreSQL"))

	// Verify reverse link exists before delete.
	var before []ULID
	require.NoError(t, store.ScanEntityEngrams(ctx, "PostgreSQL", func(_ [8]byte, id ULID) error {
		before = append(before, id)
		return nil
	}))
	require.Len(t, before, 1)

	require.NoError(t, store.DeleteEntityEngramLink(ctx, ws, eng.ID, "PostgreSQL"))

	// Reverse link must be gone.
	var after []ULID
	require.NoError(t, store.ScanEntityEngrams(ctx, "PostgreSQL", func(_ [8]byte, id ULID) error {
		after = append(after, id)
		return nil
	}))
	assert.Empty(t, after, "0x23 reverse link must be removed by DeleteEntityEngramLink")

	// Forward link must also be gone.
	var entities []string
	require.NoError(t, store.ScanEngramEntities(ctx, ws, eng.ID, func(name string) error {
		entities = append(entities, name)
		return nil
	}))
	assert.Empty(t, entities, "0x20 forward link must be removed by DeleteEntityEngramLink")
}

func TestDeleteEntityEngramLink_NoopOnMissing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("delete-entity-link-noop")

	id := NewULID()
	// Must not error on a link that doesn't exist.
	require.NoError(t, store.DeleteEntityEngramLink(ctx, ws, id, "NonExistent"))
}
