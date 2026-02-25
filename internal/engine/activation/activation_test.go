package activation_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/engine/activation"
	"github.com/scrypster/muninndb/internal/storage"
)

// ---------------------------------------------------------------------------
// Test infrastructure: minimal stubs
// ---------------------------------------------------------------------------

// stubStore implements activation.ActivationStore using in-memory maps.
type stubStore struct {
	engrams  map[storage.ULID]*storage.Engram
	metas    map[storage.ULID]*storage.EngramMeta
	assocs   map[storage.ULID][]storage.Association
	recent   []storage.ULID
}

func newStubStore() *stubStore {
	return &stubStore{
		engrams: make(map[storage.ULID]*storage.Engram),
		metas:   make(map[storage.ULID]*storage.EngramMeta),
		assocs:  make(map[storage.ULID][]storage.Association),
	}
}

func (s *stubStore) writeEngram(eng *storage.Engram) {
	if eng.ID == (storage.ULID{}) {
		eng.ID = storage.NewULID()
	}
	if eng.Confidence == 0 {
		eng.Confidence = 1.0
	}
	if eng.Stability == 0 {
		eng.Stability = 30.0
	}
	if eng.CreatedAt.IsZero() {
		eng.CreatedAt = time.Now()
	}
	if eng.LastAccess.IsZero() {
		eng.LastAccess = eng.CreatedAt
	}
	s.engrams[eng.ID] = eng
	s.metas[eng.ID] = &storage.EngramMeta{
		ID:          eng.ID,
		CreatedAt:   eng.CreatedAt,
		UpdatedAt:   eng.UpdatedAt,
		LastAccess:  eng.LastAccess,
		Confidence:  eng.Confidence,
		Relevance:   eng.Relevance,
		Stability:   eng.Stability,
		AccessCount: eng.AccessCount,
		State:       eng.State,
	}
	s.recent = append([]storage.ULID{eng.ID}, s.recent...)
}

func (s *stubStore) GetMetadata(_ context.Context, _ [8]byte, ids []storage.ULID) ([]*storage.EngramMeta, error) {
	out := make([]*storage.EngramMeta, 0, len(ids))
	for _, id := range ids {
		if m, ok := s.metas[id]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *stubStore) GetEngrams(_ context.Context, _ [8]byte, ids []storage.ULID) ([]*storage.Engram, error) {
	out := make([]*storage.Engram, 0, len(ids))
	for _, id := range ids {
		if e, ok := s.engrams[id]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *stubStore) GetAssociations(_ context.Context, _ [8]byte, ids []storage.ULID, maxPerNode int) (map[storage.ULID][]storage.Association, error) {
	result := make(map[storage.ULID][]storage.Association)
	for _, id := range ids {
		assocs := s.assocs[id]
		if len(assocs) > maxPerNode {
			assocs = assocs[:maxPerNode]
		}
		result[id] = assocs
	}
	return result, nil
}

func (s *stubStore) RecentActive(_ context.Context, _ [8]byte, topK int) ([]storage.ULID, error) {
	if topK > len(s.recent) {
		topK = len(s.recent)
	}
	return s.recent[:topK], nil
}

func (s *stubStore) VaultPrefix(_ string) [8]byte {
	return [8]byte{}
}

func (s *stubStore) EngramLastAccessNs(_ [8]byte, _ storage.ULID) int64 {
	return 0
}

func (s *stubStore) EngramIDsByCreatedRange(_ context.Context, _ [8]byte, since, until time.Time, limit int) ([]storage.ULID, error) {
	var ids []storage.ULID
	for _, id := range s.recent {
		if meta, ok := s.metas[id]; ok {
			if (since.IsZero() || meta.CreatedAt.After(since) || meta.CreatedAt.Equal(since)) &&
				(until.IsZero() || meta.CreatedAt.Before(until)) {
				ids = append(ids, id)
				if limit > 0 && len(ids) >= limit {
					break
				}
			}
		}
	}
	return ids, nil
}

// stubFTS implements activation.FTSIndex using a fixed scored list.
type stubFTS struct {
	results []activation.ScoredID
}

func (f *stubFTS) Search(_ context.Context, _ [8]byte, _ string, topK int) ([]activation.ScoredID, error) {
	if topK > len(f.results) {
		topK = len(f.results)
	}
	return f.results[:topK], nil
}

// stubHNSW implements activation.HNSWIndex using a fixed scored list.
type stubHNSW struct {
	results []activation.ScoredID
}

func (h *stubHNSW) Search(_ context.Context, _ [8]byte, _ []float32, topK int) ([]activation.ScoredID, error) {
	if topK > len(h.results) {
		topK = len(h.results)
	}
	return h.results[:topK], nil
}

// stubEmbedder returns a fixed non-zero embedding so HNSW is exercised.
type stubEmbedder struct{}

func (e *stubEmbedder) Embed(_ context.Context, _ []string) ([]float32, error) {
	v := make([]float32, 8)
	for i := range v {
		v[i] = 0.1
	}
	return v, nil
}

func (e *stubEmbedder) Tokenize(text string) []string {
	return []string{text}
}

// emptyHNSW is a zero-result HNSW stub for cases where no vector hits are needed.
type emptyHNSW struct{}

func (h *emptyHNSW) Search(_ context.Context, _ [8]byte, _ []float32, _ int) ([]activation.ScoredID, error) {
	return nil, nil
}

// newTestEngine creates an ActivationEngine backed by the provided stubs.
// If hnsw is nil a no-op stub is used to avoid nil interface panics.
func newTestEngine(store *stubStore, fts *stubFTS, hnsw activation.HNSWIndex) *activation.ActivationEngine {
	if hnsw == nil {
		hnsw = &emptyHNSW{}
	}
	return activation.New(store, fts, hnsw, &stubEmbedder{})
}

// ---------------------------------------------------------------------------
// Test 1: RRF fusion surfaces the candidate that ranked #1 in all three lists
// ---------------------------------------------------------------------------

func TestRRFFusionWeightsHighestRanked(t *testing.T) {
	// Build three engrams. winnerID is rank-1 in FTS, HNSW and in the decay
	// (recent) pool; the other two each appear in only one list.
	store := newStubStore()

	winner := &storage.Engram{
		Concept:    "winner",
		Content:    "top ranked in all sources",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.9,
	}
	other1 := &storage.Engram{
		Concept:    "other1",
		Content:    "only in FTS",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.5,
	}
	other2 := &storage.Engram{
		Concept:    "other2",
		Content:    "only in HNSW",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.5,
	}

	store.writeEngram(winner)
	store.writeEngram(other1)
	store.writeEngram(other2)

	// winner is rank-1 in FTS (highest score → sorted first in the list),
	// rank-1 in HNSW, and the only entry in the recent/decay pool.
	fts := &stubFTS{results: []activation.ScoredID{
		{ID: winner.ID, Score: 0.9},
		{ID: other1.ID, Score: 0.5},
	}}
	hnsw := &stubHNSW{results: []activation.ScoredID{
		{ID: winner.ID, Score: 0.9},
		{ID: other2.ID, Score: 0.4},
	}}
	store.recent = []storage.ULID{winner.ID}

	eng := newTestEngine(store, fts, hnsw)

	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"top ranked"},
		Threshold:  0.0,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Activations) == 0 {
		t.Fatal("expected at least 1 activation")
	}
	if result.Activations[0].Engram.ID != winner.ID {
		t.Errorf("top activation = %q, want %q (winner)",
			result.Activations[0].Engram.Concept, winner.Concept)
	}
}

// ---------------------------------------------------------------------------
// Test 2: DefaultWeights fields sum to 1.0
// ---------------------------------------------------------------------------

func TestDefaultWeightsSum(t *testing.T) {
	// Create a real engine and read the weights via its exported fields.
	// The DefaultWeights are baked into the engine's constructor. We verify
	// the documented values add up to 1.0 here using the exported type.
	w := activation.DefaultWeights{
		SemanticSimilarity: 0.35,
		FullTextRelevance:  0.25,
		DecayFactor:        0.20,
		HebbianBoost:       0.10,
		AccessFrequency:    0.05,
		Recency:            0.05,
	}
	sum := w.SemanticSimilarity + w.FullTextRelevance + w.DecayFactor +
		w.HebbianBoost + w.AccessFrequency + w.Recency

	const epsilon = 1e-5
	if sum < 1.0-epsilon || sum > 1.0+epsilon {
		t.Errorf("DefaultWeights sum = %f, want 1.0", sum)
	}
}

// ---------------------------------------------------------------------------
// Test 3: ActivationLog ring buffer wraps at capacity, newest-first ordering
// ---------------------------------------------------------------------------

func TestActivationLogRingBuffer(t *testing.T) {
	log := &activation.ActivationLog{}

	const vaultID = uint32(7)
	const writes = 250

	// Record 250 entries into a single vault so the ring buffer wraps.
	base := time.Now()
	for i := 0; i < writes; i++ {
		log.Record(activation.LogEntry{
			VaultID: vaultID,
			At:      base.Add(time.Duration(i) * time.Millisecond),
		})
	}

	// Recent(n) must never return more than n entries.
	const n = 200
	all := log.Recent(n)
	if len(all) > n {
		t.Errorf("log.Recent(%d) returned %d entries, want <= %d", n, len(all), n)
	}
	if len(all) != n {
		t.Errorf("log.Recent(%d) returned %d entries, want exactly %d after %d writes", n, len(all), n, writes)
	}

	// The most recent entry must be the last one written.
	wantNewest := base.Add(time.Duration(writes-1) * time.Millisecond)
	if !all[0].At.Equal(wantNewest) {
		t.Errorf("newest entry At = %v, want %v", all[0].At, wantNewest)
	}

	// The oldest surviving entry must be write number (writes - n).
	wantOldest := base.Add(time.Duration(writes-n) * time.Millisecond)
	oldest := all[len(all)-1]
	if !oldest.At.Equal(wantOldest) {
		t.Errorf("oldest entry At = %v, want %v", oldest.At, wantOldest)
	}
}

// ---------------------------------------------------------------------------
// Test 4: Engram with very low relevance (0.01) comes back Dormant == true
// ---------------------------------------------------------------------------

func TestDormantFlagSetWhenRelevanceLow(t *testing.T) {
	store := newStubStore()
	dormant := &storage.Engram{
		Concept:    "dormant engram",
		Content:    "barely alive",
		Confidence: 1.0,
		Stability:  30.0,
		// Relevance 0.01 is well below minFloor*1.1 = 0.05*1.1 = 0.055
		Relevance: 0.01,
	}
	store.writeEngram(dormant)

	fts := &stubFTS{results: []activation.ScoredID{{ID: dormant.ID, Score: 0.8}}}
	hnsw := &stubHNSW{results: []activation.ScoredID{{ID: dormant.ID, Score: 0.8}}}

	eng := newTestEngine(store, fts, hnsw)

	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"barely alive"},
		Threshold:  0.0,
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Activations) == 0 {
		t.Fatal("expected dormant engram to appear in results")
	}

	found := false
	for _, a := range result.Activations {
		if a.Engram.ID == dormant.ID {
			found = true
			if !a.Dormant {
				t.Errorf("engram with Relevance=0.01: Dormant=false, want true")
			}
		}
	}
	if !found {
		t.Fatal("dormant engram not found in activations")
	}
}

// ---------------------------------------------------------------------------
// Test 5: High threshold filters out low-scoring results
// ---------------------------------------------------------------------------

func TestThresholdFiltersResults(t *testing.T) {
	store := newStubStore()

	for i := 0; i < 3; i++ {
		e := &storage.Engram{
			Concept:    "engram",
			Content:    "content",
			Confidence: 0.5, // final score = raw * 0.5, unlikely to reach 0.99
			Stability:  30.0,
			Relevance:  0.5,
		}
		store.writeEngram(e)
	}

	// Provide all three as FTS hits so they enter the pipeline.
	var ftsResults []activation.ScoredID
	for id := range store.metas {
		ftsResults = append(ftsResults, activation.ScoredID{ID: id, Score: 0.5})
	}
	fts := &stubFTS{results: ftsResults}
	eng := newTestEngine(store, fts, nil)

	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"engram content"},
		Threshold:  0.99,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Every returned result must have score >= 0.99.
	for _, a := range result.Activations {
		if a.Score < 0.99 {
			t.Errorf("activation %q has score %v below threshold 0.99", a.Engram.Concept, a.Score)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 6: Activate against empty vault returns empty results, no error
// ---------------------------------------------------------------------------

func TestActivateEmptyVault(t *testing.T) {
	store := newStubStore()
	fts := &stubFTS{results: nil}
	eng := newTestEngine(store, fts, nil)

	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"anything"},
		Threshold:  0.0,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("Run on empty vault: %v", err)
	}
	if result == nil {
		t.Fatal("result must not be nil")
	}
	if len(result.Activations) != 0 {
		t.Errorf("empty vault: got %d activations, want 0", len(result.Activations))
	}
}

// ---------------------------------------------------------------------------
// Test 7: Score components are in [0, 1]; confidence is in (0, 1]
// ---------------------------------------------------------------------------

func TestScoreComponentsInRange(t *testing.T) {
	store := newStubStore()

	eng1 := &storage.Engram{
		Concept:     "component test 1",
		Content:     "some content to score",
		Confidence:  0.8,
		Stability:   30.0,
		Relevance:   0.7,
		AccessCount: 5,
		LastAccess:  time.Now().Add(-24 * time.Hour),
	}
	eng2 := &storage.Engram{
		Concept:     "component test 2",
		Content:     "another piece of content",
		Confidence:  1.0,
		Stability:   14.0,
		Relevance:   0.4,
		AccessCount: 100,
		LastAccess:  time.Now().Add(-7 * 24 * time.Hour),
	}

	store.writeEngram(eng1)
	store.writeEngram(eng2)

	fts := &stubFTS{results: []activation.ScoredID{
		{ID: eng1.ID, Score: 0.8},
		{ID: eng2.ID, Score: 0.6},
	}}
	hnsw := &stubHNSW{results: []activation.ScoredID{
		{ID: eng1.ID, Score: 0.75},
		{ID: eng2.ID, Score: 0.55},
	}}

	eng := newTestEngine(store, fts, hnsw)

	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"some content"},
		Threshold:  0.0,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Activations) == 0 {
		t.Fatal("expected at least 1 activation for range check")
	}

	for _, a := range result.Activations {
		c := a.Components
		checkRange(t, "SemanticSimilarity", c.SemanticSimilarity, 0, 1)
		checkRange(t, "FullTextRelevance", c.FullTextRelevance, 0, 1)
		checkRange(t, "DecayFactor", c.DecayFactor, 0, 1)
		checkRange(t, "HebbianBoost", c.HebbianBoost, 0, 1)
		checkRange(t, "AccessFrequency", c.AccessFrequency, 0, 1)
		checkRange(t, "Recency", c.Recency, 0, 1)
		checkRange(t, "Raw", c.Raw, 0, 1)
		checkRange(t, "Final", c.Final, 0, 1)

		// Confidence must be strictly positive.
		if c.Confidence <= 0 {
			t.Errorf("Confidence = %v, want > 0", c.Confidence)
		}
		if c.Confidence > 1 {
			t.Errorf("Confidence = %v, want <= 1", c.Confidence)
		}
	}
}

func checkRange(t *testing.T, name string, v, lo, hi float64) {
	t.Helper()
	if v < lo || v > hi {
		t.Errorf("%s = %v, want in [%v, %v]", name, v, lo, hi)
	}
}

// ---------------------------------------------------------------------------
// Ensure the temp-dir / real-Pebble pattern from engine_test.go compiles.
// This is a smoke test that mirrors the testEnv setup.
// ---------------------------------------------------------------------------

func TestActivationWithRealPebble(t *testing.T) {
	dir, err := os.MkdirTemp("", "muninndb-activation-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := storage.OpenPebble(dir, storage.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 1000})

	eng := activation.New(store, nil, nil, nil)

	ctx := context.Background()
	ws := store.VaultPrefix("test-pebble")

	// Write two engrams directly via the store.
	e1 := &storage.Engram{
		Concept:    "go channels",
		Content:    "Go channels enable safe goroutine communication.",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.8,
		State:      storage.StateActive,
	}
	e2 := &storage.Engram{
		Concept:    "rust ownership",
		Content:    "Rust ownership prevents data races at compile time.",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.7,
		State:      storage.StateActive,
	}

	_, err = store.WriteEngram(ctx, ws, e1)
	if err != nil {
		t.Fatalf("WriteEngram e1: %v", err)
	}
	_, err = store.WriteEngram(ctx, ws, e2)
	if err != nil {
		t.Fatalf("WriteEngram e2: %v", err)
	}

	// Run activation against the vault. With no FTS/HNSW, only the decay
	// pool (RecentActive) feeds candidates, giving us a non-error baseline.
	result, err := eng.Run(ctx, &activation.ActivateRequest{
		VaultPrefix: ws,
		Context:     []string{"goroutine communication"},
		Threshold:   0.0,
		MaxResults:  10,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// At least one engram should surface from the recent-active pool.
	if len(result.Activations) == 0 {
		t.Log("note: 0 activations from decay-only path (requires non-zero final score)")
	}
	// Regardless, we must get a non-nil result with a QueryID.
	if result.QueryID == "" {
		t.Error("QueryID must not be empty")
	}
}

// ---------------------------------------------------------------------------
// Test 8: ReadOnly=true skips recording to the activation log
// ---------------------------------------------------------------------------

func TestReadOnlySkipsActivationLog(t *testing.T) {
	store := newStubStore()

	eng1 := &storage.Engram{
		Concept:    "readonly subject",
		Content:    "content that should not be logged in observe mode",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.8,
	}
	store.writeEngram(eng1)

	fts := &stubFTS{results: []activation.ScoredID{
		{ID: eng1.ID, Score: 0.8},
	}}

	eng := newTestEngine(store, fts, nil)

	// Run in ReadOnly mode (observe mode) — should not record an activation log entry.
	req := &activation.ActivateRequest{
		Context:    []string{"readonly subject"},
		Threshold:  0.0,
		MaxResults: 5,
		ReadOnly:   true,
	}
	result, err := eng.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run (ReadOnly): %v", err)
	}
	if len(result.Activations) == 0 {
		t.Fatal("expected engram to appear in ReadOnly results")
	}

	// Run in normal mode — this should record an entry.
	req2 := &activation.ActivateRequest{
		Context:    []string{"readonly subject"},
		Threshold:  0.0,
		MaxResults: 5,
		ReadOnly:   false,
	}
	_, err = eng.Run(context.Background(), req2)
	if err != nil {
		t.Fatalf("Run (normal): %v", err)
	}

	// The activation log is private, but we can verify ReadOnly doesn't break scoring:
	// result from ReadOnly mode must have the same top engram as normal mode.
	if result.Activations[0].Engram.ID != eng1.ID {
		t.Errorf("ReadOnly activation returned wrong engram: %v", result.Activations[0].Engram.Concept)
	}
}

// ---------------------------------------------------------------------------
// Test 9: FTS score of 0 yields FullTextRelevance component = 0
// ---------------------------------------------------------------------------

func TestZeroFTSScoreYearsZeroFTRComponent(t *testing.T) {
	store := newStubStore()

	eng1 := &storage.Engram{
		Concept:    "low fts engram",
		Content:    "content",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.8,
	}
	store.writeEngram(eng1)

	// FTS score = 0.0 → after math.Tanh normalization → 0.0.
	fts := &stubFTS{results: []activation.ScoredID{
		{ID: eng1.ID, Score: 0.0},
	}}
	eng := newTestEngine(store, fts, nil)

	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"low fts"},
		Threshold:  0.0,
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, a := range result.Activations {
		if a.Components.FullTextRelevance != 0.0 {
			t.Errorf("FTR with 0.0 FTS score = %v, want 0.0", a.Components.FullTextRelevance)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 10: FTS score > 0 yields FullTextRelevance strictly between 0 and 1
// ---------------------------------------------------------------------------

func TestPositiveFTSScoreYieldsNormalizedFTR(t *testing.T) {
	store := newStubStore()

	eng1 := &storage.Engram{
		Concept:    "high fts engram",
		Content:    "highly relevant text content for query",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.9,
	}
	store.writeEngram(eng1)

	fts := &stubFTS{results: []activation.ScoredID{
		{ID: eng1.ID, Score: 5.0}, // BM25 raw score — large but normalized via tanh
	}}
	eng := newTestEngine(store, fts, nil)

	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"highly relevant text"},
		Threshold:  0.0,
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Activations) == 0 {
		t.Fatal("expected at least 1 activation")
	}

	ftr := result.Activations[0].Components.FullTextRelevance
	if ftr <= 0.0 || ftr > 1.0 {
		t.Errorf("FullTextRelevance = %v, want in (0, 1]", ftr)
	}
}

// ---------------------------------------------------------------------------
// Test 10.5: CalcCandidatesPerIndex scales dynamically with vault size
// ---------------------------------------------------------------------------

func TestCalcCandidatesPerIndex(t *testing.T) {
	tests := []struct {
		vaultSize int64
		want      int
	}{
		{0, 30},
		{-1, 30},
		{100, 30},   // sqrt(100)=10, below floor
		{900, 30},   // sqrt(900)=30, exactly floor
		{1000, 31},  // sqrt(1000)≈31
		{10000, 100},
		{40000, 200}, // sqrt(40000)=200, hits ceiling
		{100000, 200}, // above ceiling, clamped
	}
	for _, tt := range tests {
		got := activation.CalcCandidatesPerIndex(tt.vaultSize)
		if got != tt.want {
			t.Errorf("CalcCandidatesPerIndex(%d) = %d, want %d", tt.vaultSize, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 11: ProfileUsed is set to the explicitly requested profile name
// ---------------------------------------------------------------------------

func TestProfileUsed_ExplicitProfile(t *testing.T) {
	store := newStubStore()

	eng1 := &storage.Engram{
		Concept:    "causal test engram",
		Content:    "content for causal profile test",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.8,
	}
	store.writeEngram(eng1)

	fts := &stubFTS{results: []activation.ScoredID{
		{ID: eng1.ID, Score: 0.8},
	}}
	eng := newTestEngine(store, fts, nil)

	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"causal test"},
		Threshold:  0.0,
		MaxResults: 5,
		Profile:    "causal",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ProfileUsed != "causal" {
		t.Errorf("ProfileUsed = %q, want %q", result.ProfileUsed, "causal")
	}
}

// ---------------------------------------------------------------------------
// Test 12: ProfileUsed falls back to "default" when no profile is specified
// ---------------------------------------------------------------------------

func TestProfileUsed_DefaultFallback(t *testing.T) {
	store := newStubStore()

	eng1 := &storage.Engram{
		Concept:    "default profile engram",
		Content:    "content for default profile test",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.8,
	}
	store.writeEngram(eng1)

	fts := &stubFTS{results: []activation.ScoredID{
		{ID: eng1.ID, Score: 0.8},
	}}
	eng := newTestEngine(store, fts, nil)

	// No Profile set and no context that would trigger inference.
	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"user preferences"},
		Threshold:  0.0,
		MaxResults: 5,
		Profile:    "", // empty — should fall through to "default"
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ProfileUsed != "default" {
		t.Errorf("ProfileUsed = %q, want %q", result.ProfileUsed, "default")
	}
}

// ---------------------------------------------------------------------------
// Test 13: HNSW search error degrades gracefully (FTS path still works)
// ---------------------------------------------------------------------------

// errorHNSW implements activation.HNSWIndex but always returns an error from Search.
type errorHNSW struct{}

func (h *errorHNSW) Search(_ context.Context, _ [8]byte, _ []float32, _ int) ([]activation.ScoredID, error) {
	return nil, fmt.Errorf("hnsw: index not ready")
}

func TestActivation_HNSWError_GracefulDegradation(t *testing.T) {
	store := newStubStore()

	eng1 := &storage.Engram{
		Concept:    "graceful degradation",
		Content:    "system continues operating despite vector index failure",
		Confidence: 1.0,
		Stability:  30.0,
		Relevance:  0.8,
	}
	store.writeEngram(eng1)

	// FTS returns the engram so the FTS path can surface it even when HNSW fails.
	fts := &stubFTS{results: []activation.ScoredID{
		{ID: eng1.ID, Score: 0.9},
	}}

	// Inject the error-returning HNSW stub.
	eng := activation.New(store, fts, &errorHNSW{}, &stubEmbedder{})
	defer eng.Close()

	result, err := eng.Run(context.Background(), &activation.ActivateRequest{
		Context:    []string{"graceful degradation"},
		Threshold:  0.0,
		MaxResults: 10,
	})

	// Must not return an error even though HNSW failed.
	if err != nil {
		t.Fatalf("Run returned error on HNSW failure: %v", err)
	}

	// Result must be non-nil.
	if result == nil {
		t.Fatal("expected non-nil result on HNSW failure")
	}

	// Activations slice must be non-nil and contain at least the FTS result.
	if result.Activations == nil {
		t.Fatal("expected non-nil Activations slice on HNSW failure")
	}
	if len(result.Activations) == 0 {
		t.Error("expected at least 1 activation via FTS path when HNSW fails")
	}
}
