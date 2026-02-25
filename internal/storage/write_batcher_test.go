package storage

import (
	"sync"
	"testing"
	"time"
)

// TestWriteBatcher_BatchesConcurrentWrites verifies that 10 concurrent goroutines
// can each enqueue a write job and all receive a successful result.
func TestWriteBatcher_BatchesConcurrentWrites(t *testing.T) {
	db := openTestPebble(t)
	b := newWriteBatcher(db)
	defer b.Close()

	const numWriters = 10
	results := make([]chan error, numWriters)
	for i := range results {
		results[i] = make(chan error, 1)
	}

	var wg sync.WaitGroup
	wg.Add(numWriters)

	for i := 0; i < numWriters; i++ {
		i := i
		go func() {
			defer wg.Done()
			// Build a simple key/value entry
			key := make([]byte, 4)
			key[0] = byte(i >> 24)
			key[1] = byte(i >> 16)
			key[2] = byte(i >> 8)
			key[3] = byte(i)
			job := writeBatchJob{
				entries: []batchKV{{key: key, val: []byte("v")}},
				result:  results[i],
			}
			b.jobs <- job
		}()
	}

	// Wait for all goroutines to enqueue.
	wg.Wait()

	// Collect all results with a generous timeout.
	for i := 0; i < numWriters; i++ {
		select {
		case err := <-results[i]:
			if err != nil {
				t.Errorf("writer %d: unexpected error: %v", i, err)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("writer %d: timed out waiting for result", i)
		}
	}
}

// TestWriteBatcher_ShutdownDrains verifies that Close() drains all pending jobs
// and that all result channels receive a value (no goroutine leak).
func TestWriteBatcher_ShutdownDrains(t *testing.T) {
	db := openTestPebble(t)
	b := newWriteBatcher(db)

	const numJobs = 5
	results := make([]chan error, numJobs)
	for i := range results {
		results[i] = make(chan error, 1)
	}

	// Enqueue jobs before calling Close.
	for i := 0; i < numJobs; i++ {
		key := []byte{0x55, byte(i)}
		b.jobs <- writeBatchJob{
			entries: []batchKV{{key: key, val: []byte("drain")}},
			result:  results[i],
		}
	}

	// Close() triggers the drain loop and blocks until the batcher goroutine exits.
	b.Close()

	// All jobs must have received a value (success or error — both are acceptable
	// since the stop signal may have arrived before some jobs were committed,
	// but the drain loop in Close() flushes them all).
	for i := 0; i < numJobs; i++ {
		select {
		case <-results[i]:
			// Received — no leak.
		default:
			t.Errorf("job %d: result channel empty after Close()", i)
		}
	}
}
