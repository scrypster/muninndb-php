package engine

import (
	"context"
	"testing"
)

// TestWorkerStats_ReturnsWithoutPanic verifies that WorkerStats() returns without
// panicking. In the test environment, all cognitive workers are nil, so all
// WorkerStats fields should be zero-valued.
func TestWorkerStats_ReturnsWithoutPanic(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	// testEnv wires nil hebbianWorker, contradictWorker, and confidenceWorker,
	// so WorkerStats should return zero-value EngineWorkerStats.
	stats := eng.WorkerStats()

	// In test env workers are nil — all fields must be the zero value.
	if stats.Hebbian.Processed != 0 {
		t.Errorf("Hebbian.Processed = %d, want 0 (nil worker in test env)", stats.Hebbian.Processed)
	}
	if stats.Contradict.Processed != 0 {
		t.Errorf("Contradict.Processed = %d, want 0 (nil worker in test env)", stats.Contradict.Processed)
	}
	if stats.Confidence.Processed != 0 {
		t.Errorf("Confidence.Processed = %d, want 0 (nil worker in test env)", stats.Confidence.Processed)
	}
	if stats.Hebbian.Errors != 0 {
		t.Errorf("Hebbian.Errors = %d, want 0", stats.Hebbian.Errors)
	}
}

// TestUnsubscribe_InvalidID verifies that calling Unsubscribe with a non-existent
// subscription ID does not panic and returns nil.
// Engine.Unsubscribe delegates to trigger.TriggerSystem.Unsubscribe which calls
// sync.Map.Delete — a no-op for missing keys — so the result is always nil.
func TestUnsubscribe_InvalidID(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	err := eng.Unsubscribe(ctx, "nonexistent-subscription-id")
	if err != nil {
		t.Errorf("Unsubscribe(nonexistent): expected nil error, got %v", err)
	}
}
