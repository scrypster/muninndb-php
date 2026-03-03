package auth_test

import (
	"strings"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	"github.com/scrypster/muninndb/internal/auth"
)

func TestRenameVaultConfig_Success(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	// Set a public config for "old-vault".
	if err := s.SetVaultConfig(auth.VaultConfig{Name: "old-vault", Public: true}); err != nil {
		t.Fatalf("SetVaultConfig: %v", err)
	}

	// Rename old-vault → new-vault.
	if err := s.RenameVaultConfig("old-vault", "new-vault"); err != nil {
		t.Fatalf("RenameVaultConfig: %v", err)
	}

	// new-vault should have the config with Public=true and Name="new-vault".
	cfg, err := s.GetVaultConfig("new-vault")
	if err != nil {
		t.Fatalf("GetVaultConfig(new-vault): %v", err)
	}
	if cfg.Name != "new-vault" {
		t.Errorf("expected Name %q, got %q", "new-vault", cfg.Name)
	}
	if !cfg.Public {
		t.Error("expected new-vault to be Public=true")
	}

	// old-vault should return the fail-closed default (Public=false).
	old, err := s.GetVaultConfig("old-vault")
	if err != nil {
		t.Fatalf("GetVaultConfig(old-vault): %v", err)
	}
	if old.Public {
		t.Error("expected old-vault to return fail-closed default (Public=false)")
	}
}

func TestRenameVaultConfig_NoConfig(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	// Rename a vault that has never been configured — should be a no-op.
	if err := s.RenameVaultConfig("nonexistent", "new-name"); err != nil {
		t.Fatalf("RenameVaultConfig on unconfigured vault: %v", err)
	}

	// Verify nothing was created for "new-name" — should get the fail-closed default.
	cfg, err := s.GetVaultConfig("new-name")
	if err != nil {
		t.Fatalf("GetVaultConfig(new-name): %v", err)
	}
	if cfg.Public {
		t.Error("expected new-name to return fail-closed default (Public=false)")
	}

	// ListVaultConfigs should be empty.
	vaults, err := s.ListVaultConfigs()
	if err != nil {
		t.Fatalf("ListVaultConfigs: %v", err)
	}
	if len(vaults) != 0 {
		t.Errorf("expected 0 vault configs, got %d", len(vaults))
	}
}

func TestRenameVaultConfig_PreservesFields(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	hopDepth := 4
	semanticWeight := float32(0.8)
	boolTrue := true

	original := auth.VaultConfig{
		Name:   "src-vault",
		Public: true,
		Plasticity: &auth.PlasticityConfig{
			Version:        1,
			Preset:         "knowledge-graph",
			HopDepth:       &hopDepth,
			SemanticWeight: &semanticWeight,
			HebbianEnabled: &boolTrue,
		},
	}

	if err := s.SetVaultConfig(original); err != nil {
		t.Fatalf("SetVaultConfig: %v", err)
	}

	if err := s.RenameVaultConfig("src-vault", "dst-vault"); err != nil {
		t.Fatalf("RenameVaultConfig: %v", err)
	}

	cfg, err := s.GetVaultConfig("dst-vault")
	if err != nil {
		t.Fatalf("GetVaultConfig(dst-vault): %v", err)
	}

	if cfg.Name != "dst-vault" {
		t.Errorf("expected Name %q, got %q", "dst-vault", cfg.Name)
	}
	if !cfg.Public {
		t.Error("expected Public=true to be preserved")
	}
	if cfg.Plasticity == nil {
		t.Fatal("expected Plasticity config to be preserved, got nil")
	}
	if cfg.Plasticity.Version != 1 {
		t.Errorf("expected Plasticity.Version=1, got %d", cfg.Plasticity.Version)
	}
	if cfg.Plasticity.Preset != "knowledge-graph" {
		t.Errorf("expected Plasticity.Preset=%q, got %q", "knowledge-graph", cfg.Plasticity.Preset)
	}
	if cfg.Plasticity.HopDepth == nil || *cfg.Plasticity.HopDepth != 4 {
		t.Errorf("expected Plasticity.HopDepth=4, got %v", cfg.Plasticity.HopDepth)
	}
	if cfg.Plasticity.SemanticWeight == nil || *cfg.Plasticity.SemanticWeight != 0.8 {
		t.Errorf("expected Plasticity.SemanticWeight=0.8, got %v", cfg.Plasticity.SemanticWeight)
	}
	if cfg.Plasticity.HebbianEnabled == nil || !*cfg.Plasticity.HebbianEnabled {
		t.Error("expected Plasticity.HebbianEnabled=true to be preserved")
	}

	// Verify old name is gone.
	old, err := s.GetVaultConfig("src-vault")
	if err != nil {
		t.Fatalf("GetVaultConfig(src-vault): %v", err)
	}
	if old.Public {
		t.Error("expected src-vault to return fail-closed default after rename")
	}
}

func TestRenameVaultConfig_CorruptJSON(t *testing.T) {
	db := openTestDB(t)

	// Write corrupt (non-JSON) bytes directly to the vault config key.
	// Key format: 0x14 prefix + vault name bytes (matches vaultConfigKey).
	vaultName := "corrupt-vault"
	key := make([]byte, 1+len(vaultName))
	key[0] = 0x14 // prefixVaultCfg
	copy(key[1:], vaultName)

	if err := db.Set(key, []byte("NOT-JSON!!!!}}}}"), pebble.Sync); err != nil {
		t.Fatalf("writing corrupt data: %v", err)
	}

	s := auth.NewStore(db)

	// RenameVaultConfig should hit the GetVaultConfig error branch
	// and return nil (no-op).
	if err := s.RenameVaultConfig("corrupt-vault", "new-name"); err != nil {
		t.Fatalf("expected nil error for corrupt config, got: %v", err)
	}

	// Verify nothing was created for "new-name" — should get the
	// fail-closed default (Public=false).
	cfg, err := s.GetVaultConfig("new-name")
	if err != nil {
		t.Fatalf("GetVaultConfig(new-name): %v", err)
	}
	if cfg.Public {
		t.Error("expected new-name to return fail-closed default (Public=false)")
	}
}

// TestRenameVaultConfig_SetVaultConfigError exercises the error path at
// vault_config.go:75 where SetVaultConfig fails during a rename.
//
// Strategy: seed a vault config on a read-write DB, close it, then reopen the
// same in-memory FS as read-only. db.Get still works (finds the config) but
// db.Set inside SetVaultConfig returns pebble.ErrReadOnly.
func TestRenameVaultConfig_SetVaultConfigError(t *testing.T) {
	memfs := vfs.NewMem()

	// Phase 1: open RW and seed a config.
	db, err := pebble.Open("", &pebble.Options{FS: memfs})
	if err != nil {
		t.Fatalf("open pebble (rw): %v", err)
	}
	s := auth.NewStore(db)
	if err := s.SetVaultConfig(auth.VaultConfig{Name: "src", Public: true}); err != nil {
		t.Fatalf("SetVaultConfig seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close rw db: %v", err)
	}

	// Phase 2: reopen read-only on the same FS.
	roDB, err := pebble.Open("", &pebble.Options{FS: memfs, ReadOnly: true})
	if err != nil {
		t.Fatalf("open pebble (ro): %v", err)
	}
	t.Cleanup(func() { _ = roDB.Close() })

	roStore := auth.NewStore(roDB)

	// RenameVaultConfig: db.Get succeeds (finds "src" config),
	// but SetVaultConfig's db.Set returns ErrReadOnly.
	err = roStore.RenameVaultConfig("src", "dst")
	if err == nil {
		t.Fatal("expected error from RenameVaultConfig on read-only DB, got nil")
	}
	if !strings.Contains(err.Error(), "rename vault config: commit") {
		t.Errorf("expected 'rename vault config: commit' in error, got: %v", err)
	}
}

// NOTE: RenameVaultConfig uses an atomic Pebble batch (Set new + Delete old in
// one commit). On a read-only DB, batch.Commit returns ErrReadOnly, which is
// tested above. The individual Set/Delete error paths no longer exist.
// The branch is defensive code for hypothetical storage backends where db.Set
// can succeed but a subsequent db.Delete fails.

// TestDeleteVaultConfig_DeletesExisting verifies that DeleteVaultConfig removes
// a previously-stored vault config entry.
func TestDeleteVaultConfig_DeletesExisting(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	// Set a config so there is something to delete.
	if err := s.SetVaultConfig(auth.VaultConfig{Name: "to-delete", Public: true}); err != nil {
		t.Fatalf("SetVaultConfig: %v", err)
	}

	// Confirm it was persisted.
	cfgs, err := s.ListVaultConfigs()
	if err != nil {
		t.Fatalf("ListVaultConfigs before delete: %v", err)
	}
	found := false
	for _, c := range cfgs {
		if c.Name == "to-delete" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected vault config to be present before delete")
	}

	// Delete it.
	if err := s.DeleteVaultConfig("to-delete"); err != nil {
		t.Fatalf("DeleteVaultConfig: %v", err)
	}

	// GetVaultConfig should now return the fail-closed default (no persisted entry).
	cfg, err := s.GetVaultConfig("to-delete")
	if err != nil {
		t.Fatalf("GetVaultConfig after delete: %v", err)
	}
	if cfg.Public {
		t.Error("expected fail-closed default (Public=false) after delete, got Public=true")
	}

	// ListVaultConfigs should no longer include the deleted vault.
	cfgs, err = s.ListVaultConfigs()
	if err != nil {
		t.Fatalf("ListVaultConfigs after delete: %v", err)
	}
	for _, c := range cfgs {
		if c.Name == "to-delete" {
			t.Error("deleted vault config still appears in ListVaultConfigs")
		}
	}
}

// TestDeleteVaultConfig_Idempotent verifies that calling DeleteVaultConfig on a
// vault that has no config entry returns nil (no error).
func TestDeleteVaultConfig_Idempotent(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	// Delete a vault that was never configured — must not error.
	if err := s.DeleteVaultConfig("nonexistent"); err != nil {
		t.Fatalf("DeleteVaultConfig on non-existent vault: %v", err)
	}

	// Calling again must still return nil.
	if err := s.DeleteVaultConfig("nonexistent"); err != nil {
		t.Fatalf("second DeleteVaultConfig on non-existent vault: %v", err)
	}
}

// TestDeleteVaultConfig_Roundtrip verifies that after a Set → Delete cycle,
// a subsequent SetVaultConfig on the same name works correctly.
func TestDeleteVaultConfig_Roundtrip(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	// First write.
	if err := s.SetVaultConfig(auth.VaultConfig{Name: "roundtrip-vault", Public: true}); err != nil {
		t.Fatalf("first SetVaultConfig: %v", err)
	}

	// Delete.
	if err := s.DeleteVaultConfig("roundtrip-vault"); err != nil {
		t.Fatalf("DeleteVaultConfig: %v", err)
	}

	// Second write with different settings.
	if err := s.SetVaultConfig(auth.VaultConfig{Name: "roundtrip-vault", Public: false}); err != nil {
		t.Fatalf("second SetVaultConfig: %v", err)
	}

	// Verify the second write is what we read back.
	cfg, err := s.GetVaultConfig("roundtrip-vault")
	if err != nil {
		t.Fatalf("GetVaultConfig after roundtrip: %v", err)
	}
	if cfg.Public {
		t.Error("expected Public=false from second SetVaultConfig, got Public=true")
	}
	if cfg.Name != "roundtrip-vault" {
		t.Errorf("expected Name=%q, got %q", "roundtrip-vault", cfg.Name)
	}
}
