package storage

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertEntityRecord_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	record := EntityRecord{
		Name:       "PostgreSQL",
		Type:       "database",
		Confidence: 0.95,
	}
	err := store.UpsertEntityRecord(ctx, record, "inline:test")
	require.NoError(t, err)

	// Normalized lookup (lowercase)
	got, err := store.GetEntityRecord(ctx, "postgresql")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "PostgreSQL", got.Name)
	require.Equal(t, "database", got.Type)
	require.Equal(t, "inline:test", got.Source)

	// Different case resolves to same record
	got2, err := store.GetEntityRecord(ctx, "POSTGRESQL")
	require.NoError(t, err)
	require.NotNil(t, got2)
	require.Equal(t, got.Name, got2.Name)
}

func TestGetEntityRecord_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	got, err := store.GetEntityRecord(ctx, "nonexistent-entity")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestWriteEntityEngramLink(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	ws := store.VaultPrefix("entity-link-vault")

	// Write an entity first
	err := store.UpsertEntityRecord(ctx, EntityRecord{Name: "PostgreSQL", Type: "database", Confidence: 0.9}, "test")
	require.NoError(t, err)

	engramID := NewULID()

	// Write the link — should succeed without error
	err = store.WriteEntityEngramLink(ctx, ws, engramID, "PostgreSQL")
	require.NoError(t, err)
}

func TestUpsertRelationshipRecord(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	ws := store.VaultPrefix("relationship-vault")
	engramID := NewULID()

	record := RelationshipRecord{
		FromEntity: "payment-service",
		ToEntity:   "PostgreSQL",
		RelType:    "uses",
		Weight:     0.9,
		Source:     "plugin:enrich",
	}

	err := store.UpsertRelationshipRecord(ctx, ws, engramID, record)
	require.NoError(t, err)
}

func TestUpsertEntityRecord_UpdatePreservesName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Write initial record
	err := store.UpsertEntityRecord(ctx, EntityRecord{
		Name:       "Redis",
		Type:       "cache",
		Confidence: 0.7,
	}, "inline:test")
	require.NoError(t, err)

	// Overwrite with higher confidence
	err = store.UpsertEntityRecord(ctx, EntityRecord{
		Name:       "Redis",
		Type:       "database",
		Confidence: 0.95,
	}, "plugin:enrich")
	require.NoError(t, err)

	got, err := store.GetEntityRecord(ctx, "redis")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Redis", got.Name)
	require.Equal(t, "database", got.Type)
	require.Equal(t, "plugin:enrich", got.Source)
	require.InDelta(t, 0.95, got.Confidence, 0.001)
}

func TestUpsertEntityRecord_KeepsHigherConfidence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Write initial record with high confidence.
	err := store.UpsertEntityRecord(ctx, EntityRecord{
		Name: "PostgreSQL", Type: "technology", Confidence: 0.9,
	}, "plugin:enrich")
	require.NoError(t, err)

	// Upsert with lower confidence — existing confidence must be preserved.
	err = store.UpsertEntityRecord(ctx, EntityRecord{
		Name: "PostgreSQL", Type: "technology", Confidence: 0.4,
	}, "inline")
	require.NoError(t, err)

	got, err := store.GetEntityRecord(ctx, "PostgreSQL")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.InDelta(t, float32(0.9), got.Confidence, 0.001, "higher confidence must be preserved")
}

func TestUpsertEntityRecord_UpdatesWhenHigherConfidence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.UpsertEntityRecord(ctx, EntityRecord{
		Name: "PostgreSQL", Type: "technology", Confidence: 0.4,
	}, "inline")
	require.NoError(t, err)

	// Upsert with higher confidence — should update.
	err = store.UpsertEntityRecord(ctx, EntityRecord{
		Name: "PostgreSQL", Type: "technology", Confidence: 0.9,
	}, "plugin:enrich")
	require.NoError(t, err)

	got, err := store.GetEntityRecord(ctx, "PostgreSQL")
	require.NoError(t, err)
	assert.InDelta(t, float32(0.9), got.Confidence, 0.001, "higher confidence must be accepted")
	assert.Equal(t, "plugin:enrich", got.Source)
}

func TestUpsertEntityRecord_ConcurrentPreservesHighestConfidence(t *testing.T) {
	ps := newTestStore(t)
	ctx := context.Background()

	// Seed with a low baseline.
	require.NoError(t, ps.UpsertEntityRecord(ctx, EntityRecord{
		Name: "ConcurrentEntity", Type: "test", Confidence: 0.1,
	}, "baseline"))

	var wg sync.WaitGroup
	// 20 goroutines: 10 write 0.8, 10 write 0.7.
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = ps.UpsertEntityRecord(ctx, EntityRecord{
				Name: "ConcurrentEntity", Type: "test", Confidence: 0.8,
			}, "high")
		}()
		go func() {
			defer wg.Done()
			_ = ps.UpsertEntityRecord(ctx, EntityRecord{
				Name: "ConcurrentEntity", Type: "test", Confidence: 0.7,
			}, "mid")
		}()
	}
	wg.Wait()

	got, err := ps.GetEntityRecord(ctx, "ConcurrentEntity")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.InDelta(t, float32(0.8), got.Confidence, 0.001, "highest confidence must survive concurrent writes")
}
