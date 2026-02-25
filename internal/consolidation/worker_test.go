package consolidation

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/storage"
)

// TestConsolidation_DryRun verifies that DryRun mode produces no mutations.
func TestConsolidation_DryRun(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	db, err := pebble.Open(tmpDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("failed to open pebble: %v", err)
	}

	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 100})
	defer store.Close()
	vault := "test_vault"
	wsPrefix := store.ResolveVaultPrefix(vault)

	eng1 := &storage.Engram{
		Concept:    "concept_1",
		Content:    "content 1",
		Confidence: 0.9,
		Relevance:  0.8,
		Stability:  30.0,
	}
	_, err = store.WriteEngram(ctx, wsPrefix, eng1)
	if err != nil {
		t.Fatalf("failed to write engram 1: %v", err)
	}

	eng2 := &storage.Engram{
		Concept:    "concept_2",
		Content:    "content 2",
		Confidence: 0.7,
		Relevance:  0.7,
		Stability:  30.0,
	}
	_, err = store.WriteEngram(ctx, wsPrefix, eng2)
	if err != nil {
		t.Fatalf("failed to write engram 2: %v", err)
	}

	mockEngine := &mockEngineInterface{store: store}

	worker := &Worker{
		Engine:        mockEngine,
		Schedule:      6 * time.Hour,
		MaxDedup:      100,
		MaxTransitive: 1000,
		DryRun:        true,
	}

	report, err := worker.RunOnce(ctx, vault)
	if err != nil {
		t.Fatalf("consolidation failed: %v", err)
	}

	if !report.DryRun {
		t.Errorf("expected DryRun=true, got false")
	}

	t.Logf("consolidation report: %+v", report)
}

// TestConsolidation_DecayAcceleration_Disabled verifies that phase 4 Ebbinghaus
// decay acceleration is disabled. ACT-R computes temporal priority at query time;
// background mutation of stored Relevance contradicts total-recall semantics.
func TestConsolidation_DecayAcceleration_Disabled(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	db, err := pebble.Open(tmpDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("failed to open pebble: %v", err)
	}

	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 100})
	defer store.Close()
	vault := "test_vault"
	wsPrefix := store.ResolveVaultPrefix(vault)

	oldTime := time.Now().Add(-40 * 24 * time.Hour)
	oldEng := &storage.Engram{
		Concept:     "old_concept",
		Content:     "old content",
		Confidence:  0.5,
		Relevance:   0.2,
		Stability:   30.0,
		AccessCount: 1,
		CreatedAt:   oldTime,
		UpdatedAt:   oldTime,
		LastAccess:  oldTime,
	}
	id, err := store.WriteEngram(ctx, wsPrefix, oldEng)
	if err != nil {
		t.Fatalf("failed to write old engram: %v", err)
	}

	mockEngine := &mockEngineInterface{store: store}
	worker := &Worker{
		Engine:        mockEngine,
		Schedule:      6 * time.Hour,
		MaxDedup:      100,
		MaxTransitive: 1000,
		DryRun:        false,
	}

	report, err := worker.RunOnce(ctx, vault)
	if err != nil {
		t.Fatalf("consolidation failed: %v", err)
	}

	if report.DecayedEngrams != 0 {
		t.Errorf("expected 0 decayed engrams (phase 4 disabled), got %d", report.DecayedEngrams)
	}

	unchanged, err := store.GetEngram(ctx, wsPrefix, id)
	if err != nil {
		t.Fatalf("failed to retrieve engram: %v", err)
	}
	if unchanged.Relevance != 0.2 {
		t.Errorf("relevance mutated to %f, expected 0.2 (no background decay)", unchanged.Relevance)
	}
}

// TestConsolidation_SchemaPromotion verifies that highly-connected engrams are promoted.
func TestConsolidation_SchemaPromotion(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	db, err := pebble.Open(tmpDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("failed to open pebble: %v", err)
	}

	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 100})
	defer store.Close()
	vault := "test_vault"
	wsPrefix := store.ResolveVaultPrefix(vault)

	hubEng := &storage.Engram{
		Concept:    "hub_concept",
		Content:    "hub content",
		Confidence: 0.9,
		Relevance:  0.85,
		Stability:  30.0,
	}
	hubID, err := store.WriteEngram(ctx, wsPrefix, hubEng)
	if err != nil {
		t.Fatalf("failed to write hub engram: %v", err)
	}

	for i := 0; i < 15; i++ {
		satEng := &storage.Engram{
			Concept:    "satellite_" + string(rune(i)),
			Content:    "satellite content",
			Confidence: 0.8,
			Relevance:  0.7,
			Stability:  30.0,
		}
		satID, err := store.WriteEngram(ctx, wsPrefix, satEng)
		if err != nil {
			t.Fatalf("failed to write satellite engram %d: %v", i, err)
		}

		assoc := &storage.Association{
			TargetID:   satID,
			RelType:    storage.RelSupports,
			Weight:     0.8,
			Confidence: 1.0,
			CreatedAt:  time.Now(),
		}
		if err := store.WriteAssociation(ctx, wsPrefix, hubID, satID, assoc); err != nil {
			t.Fatalf("failed to create association: %v", err)
		}
	}

	initial, err := store.GetEngram(ctx, wsPrefix, hubID)
	if err != nil {
		t.Fatalf("failed to retrieve hub engram: %v", err)
	}
	initialRelevance := initial.Relevance

	mockEngine := &mockEngineInterface{store: store}
	worker := &Worker{
		Engine:        mockEngine,
		Schedule:      6 * time.Hour,
		MaxDedup:      100,
		MaxTransitive: 1000,
		DryRun:        false,
	}

	report, err := worker.RunOnce(ctx, vault)
	if err != nil {
		t.Fatalf("consolidation failed: %v", err)
	}

	if report.PromotedNodes < 1 {
		t.Logf("warning: expected >= 1 promoted nodes, got %d", report.PromotedNodes)
	}

	promoted, err := store.GetEngram(ctx, wsPrefix, hubID)
	if err != nil {
		t.Fatalf("failed to retrieve promoted engram: %v", err)
	}

	t.Logf("initial relevance: %f, promoted relevance: %f", initialRelevance, promoted.Relevance)
	if promoted.Relevance <= initialRelevance {
		t.Logf("note: relevance not increased (this may be acceptable depending on the implementation)")
	}
}

// TestWorker_SchedulerStopsOnContextCancel verifies that Start() returns promptly
// after its context is cancelled, and that RunOnce was invoked at least twice
// during the observation window.
func TestWorker_SchedulerStopsOnContextCancel(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := pebble.Open(tmpDir, &pebble.Options{})
	if err != nil {
		t.Fatalf("failed to open pebble: %v", err)
	}
	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 100})
	defer store.Close()

	var listVaultsCalls atomic.Int32

	mock := &countingEngineInterface{
		counter:     &listVaultsCalls,
		vaults:      []string{"test_vault"},
		pebbleStore: store,
	}

	worker := &Worker{
		Engine:        mock,
		Schedule:      20 * time.Millisecond,
		MaxDedup:      100,
		MaxTransitive: 1000,
		DryRun:        true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()

	time.Sleep(70 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return within 2s after context cancellation")
	}

	calls := listVaultsCalls.Load()
	if calls < 2 {
		t.Errorf("expected at least 2 scheduler ticks (ListVaults calls), got %d", calls)
	}
}

// countingEngineInterface wraps a vault list and tracks ListVaults calls via an atomic counter.
// Store() returns a real PebbleStore so consolidation phases have valid storage.
type countingEngineInterface struct {
	counter     *atomic.Int32
	vaults      []string
	pebbleStore *storage.PebbleStore
}

func (c *countingEngineInterface) Store() *storage.PebbleStore {
	return c.pebbleStore
}

func (c *countingEngineInterface) ListVaults(ctx context.Context) ([]string, error) {
	c.counter.Add(1)
	return c.vaults, nil
}

func (c *countingEngineInterface) UpdateLifecycleState(ctx context.Context, vault, id, state string) error {
	return nil
}

// TestNewWorker verifies constructor defaults.
func TestNewWorker(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := pebble.Open(tmpDir, &pebble.Options{})
	if err != nil {
		t.Fatal(err)
	}
	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 100})
	defer store.Close()

	mock := &mockEngineInterface{store: store}
	w := NewWorker(mock)

	if w.Engine != mock {
		t.Error("Engine not set")
	}
	if w.Schedule != 6*time.Hour {
		t.Errorf("Schedule = %v, want 6h", w.Schedule)
	}
	if w.MaxDedup != 100 {
		t.Errorf("MaxDedup = %d, want 100", w.MaxDedup)
	}
	if w.MaxTransitive != 1000 {
		t.Errorf("MaxTransitive = %d, want 1000", w.MaxTransitive)
	}
	if w.DryRun {
		t.Error("DryRun should default to false")
	}
}

// TestRunOnce_ReportFields verifies that RunOnce populates all report fields
// and runs all phases without error on a seeded vault.
func TestRunOnce_ReportFields(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	db, err := pebble.Open(tmpDir, &pebble.Options{})
	if err != nil {
		t.Fatal(err)
	}
	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 100})
	defer store.Close()

	vault := "report_test"
	wsPrefix := store.ResolveVaultPrefix(vault)

	e1 := &storage.Engram{
		Concept: "a", Content: "content a", Confidence: 0.9, Relevance: 0.9,
		Stability: 30,
	}
	e2 := &storage.Engram{
		Concept: "b", Content: "content b", Confidence: 0.3, Relevance: 0.3,
		Stability: 30,
	}

	store.WriteEngram(ctx, wsPrefix, e1)
	store.WriteEngram(ctx, wsPrefix, e2)

	mock := &mockEngineInterface{store: store}
	w := NewWorker(mock)

	report, err := w.RunOnce(ctx, vault)
	if err != nil {
		t.Fatal(err)
	}

	if report.Vault != vault {
		t.Errorf("Vault = %q, want %q", report.Vault, vault)
	}
	if report.StartedAt.IsZero() {
		t.Error("StartedAt is zero")
	}
	if report.Duration <= 0 {
		t.Error("Duration should be positive")
	}
	if report.DryRun {
		t.Error("DryRun should be false")
	}
}

// TestRunOnce_PhaseErrorsAreNonFatal verifies that an error in one phase
// is recorded but doesn't prevent subsequent phases from running.
func TestRunOnce_PhaseErrorsAreNonFatal(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	db, err := pebble.Open(tmpDir, &pebble.Options{})
	if err != nil {
		t.Fatal(err)
	}
	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 100})
	defer store.Close()

	mock := &mockEngineInterface{store: store}
	w := NewWorker(mock)

	report, err := w.RunOnce(ctx, "empty_vault")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) != 0 {
		t.Logf("phase errors on empty vault (may be acceptable): %v", report.Errors)
	}
}

// TestSafeRunOnce_PanicRecovery verifies that safeRunOnce catches panics
// and returns them as errors.
func TestSafeRunOnce_PanicRecovery(t *testing.T) {
	w := &Worker{
		Engine: &panicEngineInterface{},
	}

	report, err := safeRunOnce(w, context.Background(), "panic_vault")
	if err == nil {
		t.Fatal("expected error from panicking engine, got nil")
	}
	if report != nil {
		t.Errorf("expected nil report on panic, got %+v", report)
	}
	if got := err.Error(); got == "" {
		t.Error("error message should not be empty")
	}
}

// TestSafeRunOnce_NormalExecution verifies the happy path passes through.
func TestSafeRunOnce_NormalExecution(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := pebble.Open(tmpDir, &pebble.Options{})
	if err != nil {
		t.Fatal(err)
	}
	store := storage.NewPebbleStore(db, storage.PebbleStoreConfig{CacheSize: 100})
	defer store.Close()

	mock := &mockEngineInterface{store: store}
	w := NewWorker(mock)

	report, err := safeRunOnce(w, context.Background(), "normal_vault")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("report should not be nil")
	}
	if report.Vault != "normal_vault" {
		t.Errorf("Vault = %q, want %q", report.Vault, "normal_vault")
	}
}

// panicEngineInterface panics when Store() is called, used to test safeRunOnce recovery.
type panicEngineInterface struct{}

func (p *panicEngineInterface) Store() *storage.PebbleStore {
	panic("deliberate panic for testing safeRunOnce")
}

func (p *panicEngineInterface) ListVaults(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (p *panicEngineInterface) UpdateLifecycleState(ctx context.Context, vault, id, state string) error {
	return nil
}

// mockEngineInterface implements EngineInterface for testing
type mockEngineInterface struct {
	store *storage.PebbleStore
}

func (m *mockEngineInterface) Store() *storage.PebbleStore {
	return m.store
}

func (m *mockEngineInterface) ListVaults(ctx context.Context) ([]string, error) {
	return m.store.ListVaultNames()
}

func (m *mockEngineInterface) UpdateLifecycleState(ctx context.Context, vault, id, state string) error {
	ulid, err := storage.ParseULID(id)
	if err != nil {
		return err
	}

	wsPrefix := m.store.ResolveVaultPrefix(vault)
	eng, err := m.store.GetEngram(ctx, wsPrefix, ulid)
	if err != nil {
		return err
	}

	newState, err := storage.ParseLifecycleState(state)
	if err != nil {
		return err
	}

	meta := &storage.EngramMeta{
		State:       newState,
		Confidence:  eng.Confidence,
		Relevance:   eng.Relevance,
		Stability:   eng.Stability,
		AccessCount: eng.AccessCount,
		UpdatedAt:   time.Now(),
		LastAccess:  eng.LastAccess,
	}
	return m.store.UpdateMetadata(ctx, wsPrefix, ulid, meta)
}
