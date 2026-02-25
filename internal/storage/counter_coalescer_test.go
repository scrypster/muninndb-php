package storage

import (
	"encoding/binary"
	"fmt"
	"sync"
	"testing"

	"github.com/scrypster/muninndb/internal/storage/keys"
)

// TestCounterCoalescer_FlushWritesToPebble verifies that Submit followed by
// flush() persists the value in Pebble.
func TestCounterCoalescer_FlushWritesToPebble(t *testing.T) {
	db := openTestPebble(t)
	c := newCounterCoalescer(db)
	defer c.Close()

	var ws [8]byte
	ws[0] = 0xAA
	ws[1] = 0xBB

	c.Submit(ws, 5)
	c.flush()

	// Read the value back from Pebble.
	val, closer, err := db.Get(keys.VaultCountKey(ws))
	if err != nil {
		t.Fatalf("Get after flush: %v", err)
	}
	defer closer.Close()

	got := int64(binary.BigEndian.Uint64(val))
	if got != 5 {
		t.Errorf("expected count=5, got %d", got)
	}
}

// TestCounterCoalescer_ConcurrentSubmitSameVault verifies that 100 concurrent
// Submit calls to the same vault prefix result in SOME value in [1,100] being
// flushed — last-writer-wins semantics, NOT accumulation.
func TestCounterCoalescer_ConcurrentSubmitSameVault(t *testing.T) {
	db := openTestPebble(t)
	c := newCounterCoalescer(db)
	defer c.Close()

	var ws [8]byte
	ws[0] = 0xCC
	ws[1] = 0xDD

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 1; i <= n; i++ {
		i := i
		go func() {
			defer wg.Done()
			c.Submit(ws, int64(i))
		}()
	}
	wg.Wait()

	c.flush()

	val, closer, err := db.Get(keys.VaultCountKey(ws))
	if err != nil {
		t.Fatalf("Get after concurrent submit flush: %v", err)
	}
	defer closer.Close()

	got := int64(binary.BigEndian.Uint64(val))
	if got < 1 || got > n {
		t.Errorf("expected value in [1,%d] (last-writer-wins), got %d", n, got)
	}
	// Explicitly verify it is NOT expected to be 100 as a sum.
	// The comment below is intentional: last-writer-wins means any single value is valid.
	t.Logf("last-writer-wins value after %d concurrent submits: %d", n, got)
}

// TestCounterCoalescer_MultiVaultFlush verifies that submitting value=1 to 100
// distinct vault prefixes (each in its own goroutine) results in all 100 keys
// being present in Pebble after flush().
func TestCounterCoalescer_MultiVaultFlush(t *testing.T) {
	db := openTestPebble(t)
	c := newCounterCoalescer(db)
	defer c.Close()

	const numVaults = 100
	vaults := make([][8]byte, numVaults)
	for i := 0; i < numVaults; i++ {
		var ws [8]byte
		binary.BigEndian.PutUint64(ws[:], uint64(0xDEAD000000000000)+uint64(i))
		vaults[i] = ws
	}

	var wg sync.WaitGroup
	wg.Add(numVaults)
	for i := 0; i < numVaults; i++ {
		i := i
		go func() {
			defer wg.Done()
			c.Submit(vaults[i], 1)
		}()
	}
	wg.Wait()

	c.flush()

	// Verify all 100 vault keys exist with value 1.
	for i, ws := range vaults {
		val, closer, err := db.Get(keys.VaultCountKey(ws))
		if err != nil {
			t.Errorf("vault %d (%v): key missing after flush: %v", i, fmt.Sprintf("%x", ws), err)
			continue
		}
		got := int64(binary.BigEndian.Uint64(val))
		closer.Close()
		if got != 1 {
			t.Errorf("vault %d: expected count=1, got %d", i, got)
		}
	}
}
