package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// TestWriteRaceCondition_ConcurrentVaultAccess verifies that concurrent writes
// to the same vault from multiple goroutines do not cause data races or panics.
func TestWriteRaceCondition_ConcurrentVaultAccess(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	ctx := context.Background()
	const numGoroutines = 10
	const writesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < writesPerGoroutine; i++ {
				_, err := eng.Write(ctx, &mbp.WriteRequest{
					Vault:   "concurrent-test",
					Concept: "concurrent concept",
					Content: "concurrent content from goroutine",
				})
				if err != nil {
					// Errors are acceptable under load; panics are not.
					_ = err
				}
			}
		}(g)
	}

	wg.Wait()

	// Verify the vault has engrams after all concurrent writes.
	ws := eng.store.VaultPrefix("concurrent-test")
	count := eng.store.GetVaultCount(ctx, ws)
	if count == 0 {
		t.Error("expected vault to contain engrams after concurrent writes, got 0")
	}
}

// TestActivateSnapshotIsolation verifies that concurrent writes during an
// in-flight Activate do not cause panics or data races. The snapshot ensures
// all read phases see a consistent point-in-time view.
func TestActivateSnapshotIsolation(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	ctx := context.Background()

	// Seed a few engrams so Activate has data to work with.
	for i := 0; i < 20; i++ {
		_, _ = eng.Write(ctx, &mbp.WriteRequest{
			Vault:   "snapshot-test",
			Concept: "seed concept",
			Content: "some content for snapshot isolation test",
		})
	}

	var wg sync.WaitGroup
	const writers = 5
	const activators = 5

	// Launch concurrent writers.
	wg.Add(writers)
	for w := 0; w < writers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_, _ = eng.Write(ctx, &mbp.WriteRequest{
					Vault:   "snapshot-test",
					Concept: "concurrent write",
					Content: "written during activate",
				})
			}
		}()
	}

	// Launch concurrent activators.
	wg.Add(activators)
	for a := 0; a < activators; a++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				resp, err := eng.Activate(ctx, &mbp.ActivateRequest{
					Vault:      "snapshot-test",
					Context:    []string{"seed concept"},
					MaxResults: 5,
				})
				if err != nil {
					continue
				}
				_ = resp
			}
		}()
	}

	wg.Wait()
}

// TestWriteContextCancellation_StopsJobSubmission verifies that concurrent
// calls to Stop() and Write() do not cause panics. Writes may succeed or
// return errors — the only invariant is no panic.
func TestWriteContextCancellation_StopsJobSubmission(t *testing.T) {
	eng, cleanup := testEnv(t)
	// cleanup calls eng.Stop() internally; Stop() uses sync.Once so it is
	// safe to call it multiple times.
	defer cleanup()

	ctx := context.Background()
	const numWriters = 20

	var wg sync.WaitGroup
	wg.Add(numWriters)

	// Stop the engine after a short delay to race with the writers.
	go func() {
		time.Sleep(10 * time.Millisecond)
		eng.Stop()
	}()

	for g := 0; g < numWriters; g++ {
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("goroutine %d: unexpected panic: %v", id, r)
				}
			}()
			_, err := eng.Write(ctx, &mbp.WriteRequest{
				Vault:   "stop-race-test",
				Concept: "concept",
				Content: "content",
			})
			// Either success or error is acceptable; panic is not.
			_ = err
		}(g)
	}

	wg.Wait()
}
