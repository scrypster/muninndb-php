package storage

import (
	"context"
	"testing"
)

func TestIncrTransitionBatch_IncrementsCount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("trans-test")

	src := NewULID()
	dst := NewULID()

	updates := []TransitionUpdate{{WS: ws, Src: [16]byte(src), Dst: [16]byte(dst)}}

	if err := store.IncrTransitionBatch(ctx, updates); err != nil {
		t.Fatalf("first incr: %v", err)
	}

	targets, err := store.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatalf("get after first incr: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Count != 1 {
		t.Errorf("expected count 1, got %d", targets[0].Count)
	}
	if targets[0].ID != [16]byte(dst) {
		t.Error("wrong target ID")
	}

	// Increment again
	if err := store.IncrTransitionBatch(ctx, updates); err != nil {
		t.Fatalf("second incr: %v", err)
	}

	targets, err = store.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatalf("get after second incr: %v", err)
	}
	if len(targets) != 1 || targets[0].Count != 2 {
		t.Errorf("expected count 2, got %d", targets[0].Count)
	}
}

func TestGetTopTransitions_SortedDescending(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("trans-sort")

	src := NewULID()
	dstA := NewULID()
	dstB := NewULID()
	dstC := NewULID()

	// dstA: 1 hit, dstB: 3 hits, dstC: 2 hits
	updates := []TransitionUpdate{
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstA)},
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstB)},
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstB)},
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstB)},
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstC)},
		{WS: ws, Src: [16]byte(src), Dst: [16]byte(dstC)},
	}
	if err := store.IncrTransitionBatch(ctx, updates); err != nil {
		t.Fatalf("incr: %v", err)
	}

	targets, err := store.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected 3, got %d", len(targets))
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

func TestGetTopTransitions_RespectsTopK(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("trans-topk")

	src := NewULID()
	for i := 0; i < 10; i++ {
		dst := NewULID()
		if err := store.IncrTransitionBatch(ctx, []TransitionUpdate{
			{WS: ws, Src: [16]byte(src), Dst: [16]byte(dst)},
		}); err != nil {
			t.Fatalf("incr %d: %v", i, err)
		}
	}

	targets, err := store.GetTopTransitions(ctx, ws, [16]byte(src), 3)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(targets) != 3 {
		t.Errorf("expected 3, got %d", len(targets))
	}
}

func TestGetTopTransitions_VaultIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	wsA := store.VaultPrefix("vault-a")
	wsB := store.VaultPrefix("vault-b")

	src := NewULID()
	dstA := NewULID()
	dstB := NewULID()

	if err := store.IncrTransitionBatch(ctx, []TransitionUpdate{
		{WS: wsA, Src: [16]byte(src), Dst: [16]byte(dstA)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.IncrTransitionBatch(ctx, []TransitionUpdate{
		{WS: wsB, Src: [16]byte(src), Dst: [16]byte(dstB)},
	}); err != nil {
		t.Fatal(err)
	}

	targetsA, err := store.GetTopTransitions(ctx, wsA, [16]byte(src), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(targetsA) != 1 || targetsA[0].ID != [16]byte(dstA) {
		t.Error("vault A should only see dstA")
	}

	targetsB, err := store.GetTopTransitions(ctx, wsB, [16]byte(src), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(targetsB) != 1 || targetsB[0].ID != [16]byte(dstB) {
		t.Error("vault B should only see dstB")
	}
}

func TestGetTopTransitions_EmptyReturnsNil(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("trans-empty")
	src := NewULID()

	targets, err := store.GetTopTransitions(ctx, ws, [16]byte(src), 10)
	if err != nil {
		t.Fatal(err)
	}
	if targets != nil {
		t.Errorf("expected nil, got %d targets", len(targets))
	}
}

func TestIncrTransitionBatch_EmptyIsNoop(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.IncrTransitionBatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
}
