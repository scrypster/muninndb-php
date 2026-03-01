package engine

import (
	"context"
	"testing"

	"github.com/scrypster/muninndb/internal/storage"
	"github.com/scrypster/muninndb/internal/transport/mbp"
	"github.com/stretchr/testify/require"
)

// writeEntityEngram is a test helper that writes a memory with inline entities
// and returns the engram ID.
func writeEntityEngram(t *testing.T, eng *Engine, vault, content string, entities ...mbp.InlineEntity) string {
	t.Helper()
	resp, err := eng.Write(context.Background(), &mbp.WriteRequest{
		Vault:    vault,
		Content:  content,
		Entities: entities,
	})
	require.NoError(t, err)
	return resp.ID
}

// ── trigram helpers ──────────────────────────────────────────────────────────

func TestTrigramSim_Identical(t *testing.T) {
	sim := trigramSim("PostgreSQL", "PostgreSQL")
	if sim != 1.0 {
		t.Errorf("identical strings: expected 1.0, got %f", sim)
	}
}

func TestTrigramSim_Different(t *testing.T) {
	sim := trigramSim("PostgreSQL", "MongoDB")
	if sim >= 0.5 {
		t.Errorf("very different strings: expected low similarity, got %f", sim)
	}
}

func TestTrigramSim_Similar(t *testing.T) {
	// "PostgreSQL" vs "PostgreSQL DB" share many trigrams.
	sim := trigramSim("PostgreSQL", "PostgreSQL DB")
	if sim < 0.7 {
		t.Errorf("similar strings: expected high similarity, got %f", sim)
	}
}

func TestTrigramSim_ShortString(t *testing.T) {
	// Should not panic for very short strings.
	sim := trigramSim("AB", "ABC")
	if sim < 0 || sim > 1 {
		t.Errorf("short string similarity out of range: %f", sim)
	}
}

// ── FindSimilarEntities ──────────────────────────────────────────────────────

func TestFindSimilarEntities_FindsTypo(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	// Write two memories that reference very similar entity names.
	writeEntityEngram(t, eng, "default", "PostgreSQL is the primary DB",
		mbp.InlineEntity{Name: "PostgreSQL", Type: "database"})
	writeEntityEngram(t, eng, "default", "PostgreSQL DB is used for analytics",
		mbp.InlineEntity{Name: "PostgreSQL DB", Type: "database"})

	pairs, err := eng.FindSimilarEntities(ctx, "default", 0.7, 20)
	require.NoError(t, err)

	// At threshold 0.7 the two similar names should appear.
	found := false
	for _, p := range pairs {
		if (p.EntityA == "PostgreSQL" && p.EntityB == "PostgreSQL DB") ||
			(p.EntityA == "PostgreSQL DB" && p.EntityB == "PostgreSQL") {
			found = true
			if p.Similarity < 0.7 {
				t.Errorf("similarity %f below threshold 0.7", p.Similarity)
			}
		}
	}
	if !found {
		t.Errorf("expected pair (PostgreSQL, PostgreSQL DB), got: %v", pairs)
	}
}

func TestFindSimilarEntities_NoPairsForDifferentNames(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	writeEntityEngram(t, eng, "default", "PostgreSQL is the primary DB",
		mbp.InlineEntity{Name: "PostgreSQL", Type: "database"})
	writeEntityEngram(t, eng, "default", "Kubernetes orchestrates containers",
		mbp.InlineEntity{Name: "Kubernetes", Type: "technology"})

	pairs, err := eng.FindSimilarEntities(ctx, "default", 0.85, 20)
	require.NoError(t, err)

	// "PostgreSQL" and "Kubernetes" are very different — no pairs at 0.85.
	for _, p := range pairs {
		if (p.EntityA == "PostgreSQL" && p.EntityB == "Kubernetes") ||
			(p.EntityA == "Kubernetes" && p.EntityB == "PostgreSQL") {
			t.Errorf("unexpected similar pair: %v (similarity %f)", p, p.Similarity)
		}
	}
}

func TestFindSimilarEntities_EmptyVault(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	pairs, err := eng.FindSimilarEntities(ctx, "default", 0.85, 20)
	require.NoError(t, err)
	require.Empty(t, pairs)
}

func TestFindSimilarEntities_TopNCap(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	// Write memories with entity names that share partial similarity.
	names := []string{"AlphaService", "AlphaSvc", "AlphaAPI", "AlphaApp", "AlphaDB"}
	for i, name := range names {
		writeEntityEngram(t, eng, "default", "service "+name,
			mbp.InlineEntity{Name: name, Type: "service"})
		_ = i
	}

	pairs, err := eng.FindSimilarEntities(ctx, "default", 0.0, 3)
	require.NoError(t, err)
	require.LessOrEqual(t, len(pairs), 3, "should cap at topN=3")
}

func TestFindSimilarEntities_InvalidThreshold(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	_, err := eng.FindSimilarEntities(ctx, "default", 1.5, 20)
	require.Error(t, err)
}

// ── MergeEntity ──────────────────────────────────────────────────────────────

func TestMergeEntity_DryRun(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	writeEntityEngram(t, eng, "default", "PostgreSQL is the primary DB",
		mbp.InlineEntity{Name: "PostgreSQL", Type: "database"})
	writeEntityEngram(t, eng, "default", "Postgre SQL variant",
		mbp.InlineEntity{Name: "Postgre SQL", Type: "database"})

	result, err := eng.MergeEntity(ctx, "default", "Postgre SQL", "PostgreSQL", true)
	require.NoError(t, err)
	require.True(t, result.DryRun)
	require.Equal(t, "Postgre SQL", result.EntityA)
	require.Equal(t, "PostgreSQL", result.EntityB)
	require.GreaterOrEqual(t, result.EngramsRelinked, 0)

	// Verify that entity A is NOT changed to merged (dry run).
	recA, err := eng.store.GetEntityRecord(ctx, "Postgre SQL")
	require.NoError(t, err)
	require.NotNil(t, recA)
	require.NotEqual(t, "merged", recA.State, "dry_run must not modify entity A's state")
}

func TestMergeEntity_MergesAndRelinks(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	// Write two engrams linking to entity A ("Postgre SQL").
	id1 := writeEntityEngram(t, eng, "default", "Postgre SQL is legacy name",
		mbp.InlineEntity{Name: "Postgre SQL", Type: "database"})
	id2 := writeEntityEngram(t, eng, "default", "Also Postgre SQL config",
		mbp.InlineEntity{Name: "Postgre SQL", Type: "database"})
	// Write one engram linking to entity B ("PostgreSQL").
	writeEntityEngram(t, eng, "default", "PostgreSQL is the canonical name",
		mbp.InlineEntity{Name: "PostgreSQL", Type: "database"})

	result, err := eng.MergeEntity(ctx, "default", "Postgre SQL", "PostgreSQL", false)
	require.NoError(t, err)
	require.False(t, result.DryRun)
	require.Equal(t, 2, result.EngramsRelinked, "two engrams should have been relinked")

	// Entity A should now be state=merged.
	recA, err := eng.store.GetEntityRecord(ctx, "Postgre SQL")
	require.NoError(t, err)
	require.NotNil(t, recA)
	require.Equal(t, "merged", recA.State)
	require.Equal(t, "PostgreSQL", recA.MergedInto)

	// The two engrams previously linked to A should now also link to B.
	ws := eng.store.ResolveVaultPrefix("default")
	for _, rawID := range []string{id1, id2} {
		ulid, err := storage.ParseULID(rawID)
		require.NoError(t, err)

		var foundB bool
		err = eng.store.ScanEngramEntities(ctx, ws, ulid, func(name string) error {
			if name == "PostgreSQL" {
				foundB = true
			}
			return nil
		})
		require.NoError(t, err)
		require.True(t, foundB, "engram %s should now link to PostgreSQL after merge", rawID)
	}
}

func TestMergeEntity_EntityANotFound(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	writeEntityEngram(t, eng, "default", "PostgreSQL is the canonical name",
		mbp.InlineEntity{Name: "PostgreSQL", Type: "database"})

	_, err := eng.MergeEntity(ctx, "default", "NonExistent", "PostgreSQL", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestMergeEntity_EntityBNotFound(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	writeEntityEngram(t, eng, "default", "Postgre SQL legacy",
		mbp.InlineEntity{Name: "Postgre SQL", Type: "database"})

	_, err := eng.MergeEntity(ctx, "default", "Postgre SQL", "NonExistent", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestMergeEntity_SameEntityRejected(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	writeEntityEngram(t, eng, "default", "PostgreSQL",
		mbp.InlineEntity{Name: "PostgreSQL", Type: "database"})

	_, err := eng.MergeEntity(ctx, "default", "PostgreSQL", "PostgreSQL", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be different")
}
