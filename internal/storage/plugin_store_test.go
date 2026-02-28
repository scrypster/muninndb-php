package storage

import (
	"context"
	"testing"
)

// digestFlagAll is a convenience flag covering all bits, used to identify any
// flagged engram in CountWithFlag tests.
const digestFlagAll = uint8(0xFF)

// TestCountWithoutFlag writes 3 engrams, sets the digest flag on 1, and
// verifies CountWithoutFlag returns 2 (the unflagged ones).
func TestCountWithoutFlag(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("count-without-flag-vault")

	var ids []ULID
	for i := 0; i < 3; i++ {
		id, err := store.WriteEngram(ctx, ws, &Engram{
			Concept: "concept",
			Content: "content",
		})
		if err != nil {
			t.Fatalf("WriteEngram[%d]: %v", i, err)
		}
		ids = append(ids, id)
	}

	// Flag only the first engram.
	const flag = uint8(0x01)
	if err := store.SetDigestFlag(ctx, ids[0], flag); err != nil {
		t.Fatalf("SetDigestFlag: %v", err)
	}

	count, err := store.CountWithoutFlag(ctx, flag)
	if err != nil {
		t.Fatalf("CountWithoutFlag: %v", err)
	}
	// 2 of 3 engrams lack the flag.
	if count != 2 {
		t.Errorf("CountWithoutFlag: got %d, want 2", count)
	}
}

// TestCountWithFlag writes 3 engrams, sets the digest flag on 1, and verifies
// CountWithFlag returns 1 (only the flagged one).
func TestCountWithFlag(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("count-with-flag-vault")

	var ids []ULID
	for i := 0; i < 3; i++ {
		id, err := store.WriteEngram(ctx, ws, &Engram{
			Concept: "concept",
			Content: "content",
		})
		if err != nil {
			t.Fatalf("WriteEngram[%d]: %v", i, err)
		}
		ids = append(ids, id)
	}

	// Flag only the second engram.
	const flag = uint8(0x01)
	if err := store.SetDigestFlag(ctx, ids[1], flag); err != nil {
		t.Fatalf("SetDigestFlag: %v", err)
	}

	count, err := store.CountWithFlag(ctx, flag)
	if err != nil {
		t.Fatalf("CountWithFlag: %v", err)
	}
	if count != 1 {
		t.Errorf("CountWithFlag: got %d, want 1", count)
	}
}

// TestFindVaultPrefix writes an engram in a known vault, then calls
// FindVaultPrefix with the engram's ULID and verifies the correct ws is returned.
func TestFindVaultPrefix(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ws := store.VaultPrefix("find-vault-prefix-vault")

	id, err := store.WriteEngram(ctx, ws, &Engram{
		Concept: "find-me",
		Content: "some content",
	})
	if err != nil {
		t.Fatalf("WriteEngram: %v", err)
	}

	gotWS, ok := store.FindVaultPrefix(id)
	if !ok {
		t.Fatal("FindVaultPrefix: expected ok=true, got false")
	}
	if gotWS != ws {
		t.Errorf("FindVaultPrefix: got ws %x, want %x", gotWS, ws)
	}
}

// TestFindVaultPrefix_NotFound verifies that FindVaultPrefix returns ok=false
// for a ULID that was never written to any vault.
func TestFindVaultPrefix_NotFound(t *testing.T) {
	store := newTestStore(t)

	ghost := NewULID()
	_, ok := store.FindVaultPrefix(ghost)
	if ok {
		t.Error("FindVaultPrefix: expected ok=false for unwritten ULID, got true")
	}
}
