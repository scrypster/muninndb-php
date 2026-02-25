package storage

import (
	"context"
	"testing"
	"time"
)

func TestTransitionCache_IncrAndRead(t *testing.T) {
	store := newTestStore(t)
	cache := store.TransitionCache()
	ctx := context.Background()
	ws := store.VaultPrefix("cache-test")

	src := NewULID()
	dstA := NewULID()
	dstB := NewULID()
	dstC := NewULID()

	// dstA: 1 hit, dstB: 3 hits, dstC: 2 hits
	cache.Incr(ws, [16]byte(src), [16]byte(dstA))
	cache.Incr(ws, [16]byte(src), [16]byte(dstB))
	cache.Incr(ws, [16]byte(src), [16]byte(dstB))
	cache.Incr(ws, [16]byte(src), [16]byte(dstB))
	cache.Incr(ws, [16]byte(src), [16]byte(dstC))
	cache.Incr(ws, [16]byte(src), [16]byte(dstC))

	targets, err := cache.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatalf("GetTopTransitions: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	if targets[0].Count != 3 || targets[0].ID != [16]byte(dstB) {
		t.Errorf("expected dstB(3) first, got count=%d", targets[0].Count)
	}
	if targets[1].Count != 2 || targets[1].ID != [16]byte(dstC) {
		t.Errorf("expected dstC(2) second, got count=%d", targets[1].Count)
	}
	if targets[2].Count != 1 || targets[2].ID != [16]byte(dstA) {
		t.Errorf("expected dstA(1) third, got count=%d", targets[2].Count)
	}
}

func TestTransitionCache_ReadThrough(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("readthrough")

	src := NewULID()
	dstA := NewULID()
	dstB := NewULID()

	// Pre-populate Pebble directly (bypass cache).
	if err := store.IncrTransitionBatch(ctx, []TransitionUpdate{
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstA)},
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstA)},
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstA)},
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstB)},
	}); err != nil {
		t.Fatalf("IncrTransitionBatch: %v", err)
	}

	// Create a fresh cache over the same Pebble — simulates process restart.
	cache := NewTransitionCache(store)
	defer cache.Close()

	targets, err := cache.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatalf("GetTopTransitions: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Count != 3 || targets[0].ID != [16]byte(dstA) {
		t.Errorf("expected dstA(3) first, got ID=%x count=%d", targets[0].ID, targets[0].Count)
	}
	if targets[1].Count != 1 || targets[1].ID != [16]byte(dstB) {
		t.Errorf("expected dstB(1) second, got ID=%x count=%d", targets[1].ID, targets[1].Count)
	}
}

func TestTransitionCache_MergeOnLoad(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("merge")

	src := NewULID()
	dst := NewULID()

	// Pre-populate Pebble: dst has count 5.
	if err := store.SetTransitionBatch(ctx, []TransitionSet{
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dst), Count: 5},
	}); err != nil {
		t.Fatalf("SetTransitionBatch: %v", err)
	}

	// Create a fresh cache and Incr before any read (source not loaded yet).
	cache := NewTransitionCache(store)
	defer cache.Close()
	cache.Incr(ws, [16]byte(src), [16]byte(dst))
	cache.Incr(ws, [16]byte(src), [16]byte(dst))

	// Now read — should load from Pebble (5) and merge with in-memory delta (2) = 7.
	targets, err := cache.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatalf("GetTopTransitions: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Count != 7 {
		t.Errorf("expected merged count 7, got %d", targets[0].Count)
	}
}

func TestTransitionCache_FlushPersists(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("flush")

	src := NewULID()
	dst := NewULID()

	cache := NewTransitionCache(store)
	cache.Incr(ws, [16]byte(src), [16]byte(dst))
	cache.Incr(ws, [16]byte(src), [16]byte(dst))
	cache.Incr(ws, [16]byte(src), [16]byte(dst))

	// Flush to Pebble.
	if err := cache.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	cache.Close()

	// Create a brand new cache — should read flushed data from Pebble.
	cache2 := NewTransitionCache(store)
	defer cache2.Close()

	targets, err := cache2.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatalf("GetTopTransitions: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Count != 3 {
		t.Errorf("expected count 3 after flush, got %d", targets[0].Count)
	}
}

func TestTransitionCache_VaultIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cache := store.TransitionCache()

	wsA := store.VaultPrefix("vault-a")
	wsB := store.VaultPrefix("vault-b")

	src := NewULID()
	dstA := NewULID()
	dstB := NewULID()

	cache.Incr(wsA, [16]byte(src), [16]byte(dstA))
	cache.Incr(wsB, [16]byte(src), [16]byte(dstB))

	targetsA, err := cache.GetTopTransitions(ctx, wsA, [16]byte(src), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(targetsA) != 1 || targetsA[0].ID != [16]byte(dstA) {
		t.Error("vault A should only see dstA")
	}

	targetsB, err := cache.GetTopTransitions(ctx, wsB, [16]byte(src), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(targetsB) != 1 || targetsB[0].ID != [16]byte(dstB) {
		t.Error("vault B should only see dstB")
	}
}

func TestTransitionCache_CloseFlushes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("close-flush")

	src := NewULID()
	dst := NewULID()

	cache := NewTransitionCache(store)
	cache.Incr(ws, [16]byte(src), [16]byte(dst))
	cache.Incr(ws, [16]byte(src), [16]byte(dst))

	// Close triggers final flush.
	cache.Close()

	// Verify Pebble has the data via direct read (bypass cache).
	targets, err := store.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatalf("direct Pebble read: %v", err)
	}
	if len(targets) != 1 || targets[0].Count != 2 {
		t.Errorf("expected count 2 after close-flush, got %v", targets)
	}
}

func TestTransitionCache_TopKLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cache := store.TransitionCache()
	ws := store.VaultPrefix("topk")

	src := NewULID()
	for i := 0; i < 10; i++ {
		dst := NewULID()
		cache.Incr(ws, [16]byte(src), [16]byte(dst))
	}

	targets, err := cache.GetTopTransitions(ctx, ws, [16]byte(src), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 {
		t.Errorf("expected 3, got %d", len(targets))
	}
}

func TestTransitionCache_IncrAfterReadUpdates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cache := store.TransitionCache()
	ws := store.VaultPrefix("incr-after-read")

	src := NewULID()
	dst := NewULID()

	cache.Incr(ws, [16]byte(src), [16]byte(dst))

	// Read to populate loaded state.
	targets, err := cache.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Count != 1 {
		t.Fatalf("first read: expected count 1, got %v", targets)
	}

	// Incr again after source is loaded.
	cache.Incr(ws, [16]byte(src), [16]byte(dst))

	targets, err = cache.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Count != 2 {
		t.Errorf("second read: expected count 2, got %v", targets)
	}
}

func TestTransitionCache_EvictCold(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("evict")

	src := NewULID()
	dst := NewULID()

	cache := NewTransitionCache(store)
	defer cache.Close()

	cache.Incr(ws, [16]byte(src), [16]byte(dst))

	// Force the source to be loaded so eviction can target it.
	_, _ = cache.GetTopTransitions(ctx, ws, [16]byte(src), 10)

	// Flush so it's no longer dirty.
	if err := cache.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	// Backdate the lastRead timestamp to trigger eviction.
	sk := makeSourceKey(ws, [16]byte(src))
	if v, ok := cache.loaded.Load(sk); ok {
		v.(*sourceEntry).lastRead.Store(time.Now().Add(-10 * time.Minute).UnixNano())
	}

	cache.evictCold()

	// Source should be evicted from loaded map.
	if _, ok := cache.loaded.Load(sk); ok {
		t.Error("expected source to be evicted from loaded map")
	}

	// Reading again should reload from Pebble (data was flushed).
	targets, err := cache.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Count != 1 {
		t.Errorf("after evict+reload: expected count 1, got %v", targets)
	}
}
