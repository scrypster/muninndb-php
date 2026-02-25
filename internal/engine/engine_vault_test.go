package engine

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// writeReq is a helper that builds a WriteRequest for the given vault, concept, and content.
func writeReq(vault, concept, content string) *mbp.WriteRequest {
	return &mbp.WriteRequest{
		Vault:   vault,
		Concept: concept,
		Content: content,
	}
}

// activateReq is a helper that builds an ActivateRequest for the given vault and query.
func activateReq(vault, query string) *mbp.ActivateRequest {
	return &mbp.ActivateRequest{
		Vault:      vault,
		Context:    []string{query},
		MaxResults: 20,
		Threshold:  0.0,
	}
}

func TestEngineClearVault_MemoriesGone(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := eng.Write(ctx, writeReq("clear-me", "quantum entanglement", "some content about quantum")); err != nil {
		t.Fatal(err)
	}
	// Let FTS worker flush.
	time.Sleep(300 * time.Millisecond)

	if err := eng.ClearVault(ctx, "clear-me"); err != nil {
		t.Fatalf("ClearVault: %v", err)
	}

	resp, err := eng.Activate(ctx, activateReq("clear-me", "quantum"))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Activations) > 0 {
		t.Errorf("expected 0 activations after ClearVault, got %d", len(resp.Activations))
	}

	// Also verify FTS posting lists are cleared
	ws := eng.store.VaultPrefix("clear-me")
	ftsResults, err := eng.fts.Search(context.Background(), ws, "quantum", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ftsResults) > 0 {
		t.Errorf("expected FTS to return nothing after ClearVault, got %d results", len(ftsResults))
	}

	// Vault name should still be registered after ClearVault.
	vaults, err := eng.ListVaults(ctx)
	if err != nil {
		t.Fatalf("ListVaults: %v", err)
	}
	found := false
	for _, v := range vaults {
		if v == "clear-me" {
			found = true
		}
	}
	if !found {
		t.Error("ClearVault should preserve vault registration")
	}
}

func TestEngineDeleteVault_VaultNotListed(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := eng.Write(ctx, writeReq("to-delete", "some concept", "content")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	if err := eng.DeleteVault(ctx, "to-delete"); err != nil {
		t.Fatalf("DeleteVault: %v", err)
	}

	vaults, err := eng.ListVaults(ctx)
	if err != nil {
		t.Fatalf("ListVaults: %v", err)
	}
	for _, v := range vaults {
		if v == "to-delete" {
			t.Error("deleted vault still appears in ListVaults")
		}
	}
}

func TestEngineDeleteVault_GlobalEngramCountDecreases(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	eng.Write(ctx, writeReq("vault-keep", "keep1", "c"))
	eng.Write(ctx, writeReq("vault-keep", "keep2", "c"))
	eng.Write(ctx, writeReq("vault-del", "del1", "c"))
	eng.Write(ctx, writeReq("vault-del", "del2", "c"))
	eng.Write(ctx, writeReq("vault-del", "del3", "c"))
	time.Sleep(300 * time.Millisecond)

	beforeCount := eng.engramCount.Load()
	if err := eng.DeleteVault(ctx, "vault-del"); err != nil {
		t.Fatalf("DeleteVault: %v", err)
	}
	afterCount := eng.engramCount.Load()

	// Global engramCount must strictly decrease after deleting a non-empty vault.
	if afterCount >= beforeCount {
		t.Errorf("engramCount should decrease after DeleteVault: before=%d after=%d", beforeCount, afterCount)
	}
	// Count must not go negative — the floor guard in ClearVault prevents this
	// even in crash-recovery scenarios where the persistent count may diverge.
	// We do not assert an exact -3 delta because the in-memory counter is seeded
	// from a persistent scan at startup and can skew if prior tests left state.
	if afterCount < 0 {
		t.Errorf("engramCount went negative after DeleteVault: %d (floor guard failed)", afterCount)
	}
}

func TestEngineClearVault_NotFound(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	err := eng.ClearVault(ctx, "does-not-exist")
	if err == nil {
		t.Error("expected error for unknown vault, got nil")
	}
}

func TestEngineDeleteVault_NotFound(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	err := eng.DeleteVault(ctx, "does-not-exist")
	if err == nil {
		t.Error("expected error for unknown vault, got nil")
	}
}

// TestClearVault_Idempotent verifies that calling ClearVault twice on the same
// vault does not return an error on the second call and leaves the vault empty.
func TestClearVault_Idempotent(t *testing.T) {
	eng, _, store, cleanup := testEnvWithAuth(t)
	defer cleanup()
	ctx := context.Background()

	const vaultName = "idempotent-clear-vault"

	// Write 3 engrams using eng.Write — this also registers the vault name.
	for i := 0; i < 3; i++ {
		if _, err := eng.Write(ctx, writeReq(vaultName, "concept", "content")); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}

	ws := store.VaultPrefix(vaultName)

	// Verify we have at least 3 engrams (may be more if engine background init wrote extras).
	if count := store.GetVaultCount(ctx, ws); count < 3 {
		t.Fatalf("expected at least 3 engrams before first ClearVault, got %d", count)
	}

	// First ClearVault — must succeed.
	if err := eng.ClearVault(ctx, vaultName); err != nil {
		t.Fatalf("first ClearVault: %v", err)
	}

	if count := store.GetVaultCount(ctx, ws); count != 0 {
		t.Errorf("expected 0 engrams after first ClearVault, got %d", count)
	}

	// Second ClearVault on an already-empty vault — must also succeed (idempotent).
	if err := eng.ClearVault(ctx, vaultName); err != nil {
		t.Fatalf("second ClearVault (idempotent): %v", err)
	}

	if count := store.GetVaultCount(ctx, ws); count != 0 {
		t.Errorf("expected 0 engrams after second ClearVault, got %d", count)
	}
}

func TestEngineClearVault_CoherenceGone(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	eng.Write(ctx, writeReq("coh-vault", "test concept", "content"))
	time.Sleep(200 * time.Millisecond)

	// Confirm coherence entry exists before clearing (Snapshots returns entries for known vaults).
	var hadEntry bool
	if eng.coherence != nil {
		for _, snap := range eng.coherence.Snapshots() {
			if snap.VaultName == "coh-vault" {
				hadEntry = true
				break
			}
		}
		if !hadEntry {
			t.Log("coherence entry not yet populated — may be due to timing; test continues")
		}
	}

	eng.ClearVault(ctx, "coh-vault")

	// After ClearVault the coherence entry should be absent from Snapshots.
	if eng.coherence != nil {
		for _, snap := range eng.coherence.Snapshots() {
			if snap.VaultName == "coh-vault" {
				t.Error("coherence entry should be removed after ClearVault")
			}
		}
	}
}
