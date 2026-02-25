package replication

import (
	"context"
	"testing"
	"time"
)

// TestLeaderElector_StartsAsNonLeader verifies that a freshly created
// LeaderElector reports IsLeader() == false before any tick or Run call.
func TestLeaderElector_StartsAsNonLeader(t *testing.T) {
	backend := NewMemoryLeaseBackend()
	elector := NewLeaderElector("node-1", backend)

	if elector.IsLeader() {
		t.Error("IsLeader() = true on a freshly created LeaderElector, want false")
	}
}

// TestLeaderElector_AcquiresLease verifies that a LeaderElector transitions to
// IsLeader() == true after Run() processes at least one tick with a free lease.
func TestLeaderElector_AcquiresLease(t *testing.T) {
	backend := NewMemoryLeaseBackend()
	elector := NewLeaderElector("node-1", backend)
	// Use a short renewal interval so the test doesn't have to wait long.
	elector.RenewEvery = 10 * time.Millisecond
	elector.LeaseTTL = 1 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run the elector in a goroutine; it blocks until ctx is done.
	errCh := make(chan error, 1)
	go func() {
		errCh <- elector.Run(ctx)
	}()

	// Poll until IsLeader() becomes true or we time out.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if elector.IsLeader() {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	if !elector.IsLeader() {
		t.Error("IsLeader() = false after Run(); expected transition to true with a free lease")
	}

	// Cancel the context to stop Run().
	cancel()
	select {
	case <-errCh:
		// Run() returned as expected.
	case <-time.After(2 * time.Second):
		t.Error("Run() did not return after context cancellation")
	}
}

// TestLeaderElector_LosesLeaseTransitionsBack verifies that when the backend
// starts denying lease acquisition, the elector's IsLeader() transitions back
// to false.
//
// Strategy: run two electors against the same backend. Once elector A holds the
// lease, we release it and have elector B acquire it. After elector A ticks and
// sees the lease is held by B, it should transition to non-leader.
func TestLeaderElector_LosesLeaseTransitionsBack(t *testing.T) {
	backend := NewMemoryLeaseBackend()

	electorA := NewLeaderElector("node-A", backend)
	electorA.RenewEvery = 10 * time.Millisecond
	electorA.LeaseTTL = 100 * time.Millisecond

	ctxA, cancelA := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelA()

	errChA := make(chan error, 1)
	go func() {
		errChA <- electorA.Run(ctxA)
	}()

	// Wait for elector A to acquire the lease.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if electorA.IsLeader() {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !electorA.IsLeader() {
		t.Fatal("elector A did not become leader; cannot test demotion")
	}

	// Release the lease from A and immediately acquire it with a different node.
	// The backend will now grant the lease only to "node-B".
	_ = backend.Release(context.Background(), "node-A")
	_, err := backend.TryAcquire(context.Background(), "node-B", 10*time.Second)
	if err != nil {
		t.Fatalf("TryAcquire for node-B: %v", err)
	}

	// Wait for elector A to detect that it lost the lease and demote itself.
	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if !electorA.IsLeader() {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	if electorA.IsLeader() {
		t.Error("IsLeader() = true after lease was taken by another node, want false")
	}

	cancelA()
	select {
	case <-errChA:
		// Run() returned.
	case <-time.After(2 * time.Second):
		t.Error("elector A Run() did not return after context cancellation")
	}
}
