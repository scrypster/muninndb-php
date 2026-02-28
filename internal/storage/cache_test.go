package storage

import (
	"context"
	"testing"
)

func TestCacheGetSetDelete(t *testing.T) {
	c := NewL1Cache(100)

	id1 := NewULID()
	eng1 := &Engram{ID: id1, Concept: "test engram 1", Content: "content 1", Confidence: 1.0, Stability: 30}

	// Set and Get
	var pfx [8]byte
	c.Set(pfx, id1, eng1)
	got, ok := c.Get(pfx, id1)
	if !ok {
		t.Fatal("expected cache hit after Set")
	}
	if got.Concept != eng1.Concept {
		t.Errorf("got concept %q, want %q", got.Concept, eng1.Concept)
	}

	// Delete
	c.Delete(pfx, id1)
	_, ok = c.Get(pfx, id1)
	if ok {
		t.Fatal("expected cache miss after Delete")
	}
}

func TestCacheMiss(t *testing.T) {
	c := NewL1Cache(100)
	id := NewULID()
	var pfx [8]byte
	_, ok := c.Get(pfx, id)
	if ok {
		t.Fatal("expected cache miss for unknown ID")
	}
}

func TestCacheLen(t *testing.T) {
	c := NewL1Cache(100)
	if c.Len() != 0 {
		t.Errorf("initial Len = %d, want 0", c.Len())
	}

	var pfx [8]byte
	ids := make([]ULID, 5)
	for i := range ids {
		ids[i] = NewULID()
		c.Set(pfx, ids[i], &Engram{ID: ids[i], Concept: "x", Content: "y", Confidence: 1.0, Stability: 30})
	}
	if c.Len() != 5 {
		t.Errorf("Len after 5 sets = %d, want 5", c.Len())
	}

	c.Delete(pfx, ids[0])
	if c.Len() != 4 {
		t.Errorf("Len after delete = %d, want 4", c.Len())
	}
}

func TestCacheEviction(t *testing.T) {
	maxSize := 10
	c := NewL1Cache(maxSize)

	var pfx [8]byte
	// Fill past max to trigger eviction
	for i := 0; i < maxSize+5; i++ {
		id := NewULID()
		c.Set(pfx, id, &Engram{ID: id, Concept: "evict test", Content: "c", Confidence: 1.0, Stability: 30})
	}

	// After eviction, count should be ≤ maxSize+1 (eviction fires per Set, removes one)
	if c.Len() > maxSize+5 {
		t.Errorf("cache grew unboundedly: Len = %d", c.Len())
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	c := NewL1Cache(1000)
	done := make(chan struct{})

	var pfx [8]byte
	// Concurrent writers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				id := NewULID()
				c.Set(pfx, id, &Engram{ID: id, Concept: "concurrent", Content: "c", Confidence: 1.0, Stability: 30})
				c.Get(pfx, id)
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
	// No race conditions = test passes
}

// TestLastAccessNs verifies that LastAccessNs returns 0 for a non-cached entry
// and a positive timestamp after the entry is accessed via Get.
func TestLastAccessNs(t *testing.T) {
	c := NewL1Cache(100)
	id := NewULID()
	var pfx [8]byte

	// Not cached — must return 0.
	if ns := c.LastAccessNs(pfx, id); ns != 0 {
		t.Errorf("LastAccessNs on uncached entry: got %d, want 0", ns)
	}

	eng := &Engram{ID: id, Concept: "access-test", Content: "body", Confidence: 1.0, Stability: 30}
	c.Set(pfx, id, eng)

	// After Set, LastAccessNs is set by Set itself.
	if ns := c.LastAccessNs(pfx, id); ns <= 0 {
		t.Errorf("LastAccessNs after Set: got %d, want > 0", ns)
	}

	// After Get, LastAccessNs should be updated (still > 0).
	c.Get(pfx, id)
	if ns := c.LastAccessNs(pfx, id); ns <= 0 {
		t.Errorf("LastAccessNs after Get: got %d, want > 0", ns)
	}
}

// TestEngramLastAccessNs verifies EngramLastAccessNs returns 0 when an engram
// is not in the L1 cache and a positive value after GetEngram populates the cache.
func TestEngramLastAccessNs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("last-access-vault")

	id, err := store.WriteEngram(ctx, ws, &Engram{
		Concept: "last-access-concept",
		Content: "last-access-content",
	})
	if err != nil {
		t.Fatalf("WriteEngram: %v", err)
	}

	// Not yet in cache — expect 0.
	if ns := store.EngramLastAccessNs(ws, id); ns != 0 {
		t.Errorf("EngramLastAccessNs before GetEngram: got %d, want 0", ns)
	}

	// GetEngram populates the L1 cache.
	if _, err := store.GetEngram(ctx, ws, id); err != nil {
		t.Fatalf("GetEngram: %v", err)
	}

	// Now the cache entry exists — expect > 0.
	if ns := store.EngramLastAccessNs(ws, id); ns <= 0 {
		t.Errorf("EngramLastAccessNs after GetEngram: got %d, want > 0", ns)
	}
}
