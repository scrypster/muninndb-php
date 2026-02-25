package storage

import (
	"testing"
)

// TestWriteVaultNameListVaultNamesRoundtrip verifies that WriteVaultName persists the
// vault name and ListVaultNames returns it.
func TestWriteVaultNameListVaultNamesRoundtrip(t *testing.T) {
	db := openTestPebble(t)
	store := NewPebbleStore(db, PebbleStoreConfig{CacheSize: 100})

	ws := store.VaultPrefix("my-vault")

	if err := store.WriteVaultName(ws, "my-vault"); err != nil {
		t.Fatalf("WriteVaultName: %v", err)
	}

	names, err := store.ListVaultNames()
	if err != nil {
		t.Fatalf("ListVaultNames: %v", err)
	}

	found := false
	for _, n := range names {
		if n == "my-vault" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("vault name %q not found in ListVaultNames; got %v", "my-vault", names)
	}
}

// TestWriteVaultNameIdempotent verifies that calling WriteVaultName multiple times
// for the same vault does not cause errors or duplicate entries.
func TestWriteVaultNameIdempotent(t *testing.T) {
	db := openTestPebble(t)
	store := NewPebbleStore(db, PebbleStoreConfig{CacheSize: 100})

	ws := store.VaultPrefix("idem-vault")

	for i := 0; i < 5; i++ {
		if err := store.WriteVaultName(ws, "idem-vault"); err != nil {
			t.Fatalf("WriteVaultName (call %d): %v", i, err)
		}
	}

	names, err := store.ListVaultNames()
	if err != nil {
		t.Fatalf("ListVaultNames: %v", err)
	}

	count := 0
	for _, n := range names {
		if n == "idem-vault" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 entry for idem-vault, got %d", count)
	}
}

// TestResolveVaultPrefixReturnsCorrectPrefix verifies that after WriteVaultName,
// ResolveVaultPrefix returns the same prefix used to write.
func TestResolveVaultPrefixReturnsCorrectPrefix(t *testing.T) {
	db := openTestPebble(t)
	store := NewPebbleStore(db, PebbleStoreConfig{CacheSize: 100})

	vaultName := "resolve-test-vault"
	ws := store.VaultPrefix(vaultName)

	if err := store.WriteVaultName(ws, vaultName); err != nil {
		t.Fatalf("WriteVaultName: %v", err)
	}

	resolved := store.ResolveVaultPrefix(vaultName)
	if resolved != ws {
		t.Errorf("ResolveVaultPrefix: got %x, want %x", resolved, ws)
	}
}

// TestResolveVaultPrefixColdPath verifies that ResolveVaultPrefix works on a
// fresh store instance (cold path, no in-memory cache) by reading from Pebble.
func TestResolveVaultPrefixColdPath(t *testing.T) {
	db := openTestPebble(t)

	// Write vault name using first store instance.
	store1 := NewPebbleStore(db, PebbleStoreConfig{CacheSize: 100})
	vaultName := "cold-path-vault"
	ws := store1.VaultPrefix(vaultName)
	if err := store1.WriteVaultName(ws, vaultName); err != nil {
		t.Fatalf("WriteVaultName: %v", err)
	}

	// Create second store instance (no in-memory cache warm-up).
	// Both share the same underlying DB; openTestPebble's cleanup handles Close.
	store2 := NewPebbleStore(db, PebbleStoreConfig{CacheSize: 100})
	resolved := store2.ResolveVaultPrefix(vaultName)
	if resolved != ws {
		t.Errorf("cold path ResolveVaultPrefix: got %x, want %x", resolved, ws)
	}
}

// TestListVaultNamesMultipleVaults verifies that multiple vault names are all
// returned by ListVaultNames.
func TestListVaultNamesMultipleVaults(t *testing.T) {
	db := openTestPebble(t)
	store := NewPebbleStore(db, PebbleStoreConfig{CacheSize: 100})

	vaults := []string{"vault-alpha", "vault-beta", "vault-gamma"}
	for _, name := range vaults {
		ws := store.VaultPrefix(name)
		if err := store.WriteVaultName(ws, name); err != nil {
			t.Fatalf("WriteVaultName(%s): %v", name, err)
		}
	}

	names, err := store.ListVaultNames()
	if err != nil {
		t.Fatalf("ListVaultNames: %v", err)
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, expected := range vaults {
		if !nameSet[expected] {
			t.Errorf("vault %q missing from ListVaultNames; got %v", expected, names)
		}
	}
}
