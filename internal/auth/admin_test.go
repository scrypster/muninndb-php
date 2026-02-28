package auth_test

import (
	"strings"
	"testing"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	"github.com/scrypster/muninndb/internal/auth"
)

func openTestDB(t *testing.T) *pebble.DB {
	t.Helper()
	db, err := pebble.Open("", &pebble.Options{FS: vfs.NewMem()})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestChangeAdminPassword(t *testing.T) {
	store := auth.NewStore(openTestDB(t))
	if err := store.CreateAdmin("root", "oldpass"); err != nil {
		t.Fatal(err)
	}
	if err := store.ChangeAdminPassword("root", "newpass"); err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateAdmin("root", "newpass"); err != nil {
		t.Fatal("new password should validate")
	}
	if err := store.ValidateAdmin("root", "oldpass"); err == nil {
		t.Fatal("old password should no longer work")
	}
}

func TestAdminCreateAndValidate(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	if err := s.CreateAdmin("root", "s3cr3t"); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if err := s.ValidateAdmin("root", "s3cr3t"); err != nil {
		t.Errorf("ValidateAdmin correct password: %v", err)
	}
	if err := s.ValidateAdmin("root", "wrong"); err == nil {
		t.Error("expected error for wrong password")
	}
	if err := s.ValidateAdmin("nobody", "s3cr3t"); err == nil {
		t.Error("expected error for unknown user")
	}
}

func TestAdminExists(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	if s.AdminExists() {
		t.Error("expected no admin on empty db")
	}
	_ = s.CreateAdmin("root", "pass")
	if !s.AdminExists() {
		t.Error("expected admin to exist after creation")
	}
}

func TestAPIKeyGenerateAndValidate(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	token, key, err := s.GenerateAPIKey("default", "test-agent", "full", nil)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if !strings.HasPrefix(token, "mk_") {
		t.Errorf("token should start with mk_, got %q", token)
	}
	if key.Vault != "default" || key.Mode != "full" || key.Label != "test-agent" {
		t.Errorf("unexpected key fields: %+v", key)
	}

	got, err := s.ValidateAPIKey(token)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if got.Vault != "default" {
		t.Errorf("expected vault default, got %q", got.Vault)
	}
}

func TestAPIKeyInvalidMode(t *testing.T) {
	s := auth.NewStore(openTestDB(t))
	_, _, err := s.GenerateAPIKey("default", "agent", "superuser", nil)
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestAPIKeyList(t *testing.T) {
	s := auth.NewStore(openTestDB(t))
	s.GenerateAPIKey("vault-a", "agent-1", "full", nil)
	s.GenerateAPIKey("vault-a", "agent-2", "observe", nil)
	s.GenerateAPIKey("vault-b", "agent-3", "full", nil)

	keys, err := s.ListAPIKeys("vault-a")
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys for vault-a, got %d", len(keys))
	}
}

func TestAPIKeyRevoke(t *testing.T) {
	s := auth.NewStore(openTestDB(t))
	token, key, _ := s.GenerateAPIKey("default", "temp", "observe", nil)

	if err := s.RevokeAPIKey("default", key.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if _, err := s.ValidateAPIKey(token); err == nil {
		t.Error("expected error after revocation")
	}
}

func TestVaultConfig(t *testing.T) {
	s := auth.NewStore(openTestDB(t))

	// Unknown vault defaults to fail-closed (not public) for security.
	cfg, err := s.GetVaultConfig("default")
	if err != nil {
		t.Fatalf("GetVaultConfig default: %v", err)
	}
	if cfg.Public {
		t.Error("expected unconfigured vault to be non-public by default (fail-closed)")
	}

	// Lock the vault
	if err := s.SetVaultConfig(auth.VaultConfig{Name: "default", Public: false}); err != nil {
		t.Fatalf("SetVaultConfig: %v", err)
	}
	cfg, err = s.GetVaultConfig("default")
	if err != nil {
		t.Fatalf("GetVaultConfig after set: %v", err)
	}
	if cfg.Public {
		t.Error("expected vault to be locked after SetVaultConfig(public=false)")
	}

	// List all configured vaults
	if err := s.SetVaultConfig(auth.VaultConfig{Name: "project-x", Public: false}); err != nil {
		t.Fatalf("SetVaultConfig project-x: %v", err)
	}
	vaults, err := s.ListVaultConfigs()
	if err != nil {
		t.Fatalf("ListVaultConfigs: %v", err)
	}
	if len(vaults) < 2 {
		t.Errorf("expected at least 2 vault configs, got %d", len(vaults))
	}
}
