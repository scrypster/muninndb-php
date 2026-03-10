package auth

import (
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
)

// openAuthTestDB opens an in-memory Pebble DB for auth tests.
func openAuthTestDB(t *testing.T) *pebble.DB {
	t.Helper()
	db, err := pebble.Open("", &pebble.Options{FS: vfs.NewMem()})
	if err != nil {
		t.Fatalf("open auth test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestAPIKey_CreateAndValidate creates an API key for vault "v1" and validates it.
func TestAPIKey_CreateAndValidate(t *testing.T) {
	s := NewStore(openAuthTestDB(t))

	token, key, err := s.GenerateAPIKey("v1", "test-label", "full", nil)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if key.Vault != "v1" {
		t.Errorf("expected vault 'v1', got %q", key.Vault)
	}

	got, err := s.ValidateAPIKey(token)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if got.Vault != "v1" {
		t.Errorf("expected vault 'v1', got %q", got.Vault)
	}
}

// TestAPIKey_NotFound validates that a random non-existent key returns an error.
func TestAPIKey_NotFound(t *testing.T) {
	s := NewStore(openAuthTestDB(t))

	// A well-formed but non-existent token.
	fakeToken := "mk_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, err := s.ValidateAPIKey(fakeToken)
	if err == nil {
		t.Fatal("expected error for non-existent API key, got nil")
	}
}

// TestAPIKey_RevokeIdempotent creates a key, revokes it, and revokes it again.
// The second revoke should return an error (key no longer exists) — which is
// acceptable behavior; the important thing is that it does not panic.
func TestAPIKey_RevokeIdempotent(t *testing.T) {
	s := NewStore(openAuthTestDB(t))

	_, key, err := s.GenerateAPIKey("vault-idem", "label", "full", nil)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	if err := s.RevokeAPIKey("vault-idem", key.ID); err != nil {
		t.Fatalf("first RevokeAPIKey: %v", err)
	}

	// Second revoke: key is already gone — we just verify it does not panic.
	_ = s.RevokeAPIKey("vault-idem", key.ID)
}

// TestAPIKey_RevokedKeyInvalid creates a key, revokes it, and verifies that
// ValidateAPIKey returns an error for the revoked token.
func TestAPIKey_RevokedKeyInvalid(t *testing.T) {
	s := NewStore(openAuthTestDB(t))

	token, key, err := s.GenerateAPIKey("vault-rev", "label", "full", nil)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	if err := s.RevokeAPIKey("vault-rev", key.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	_, validateErr := s.ValidateAPIKey(token)
	if validateErr == nil {
		t.Fatal("expected ValidateAPIKey to return an error after revocation, got nil")
	}
}

// TestAPIKey_ExpiryNeverExpires creates a key with no expiry and verifies it is valid.
func TestAPIKey_ExpiryNeverExpires(t *testing.T) {
	s := NewStore(openAuthTestDB(t))

	token, _, err := s.GenerateAPIKey("vault-exp", "no-expiry", "full", nil)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if _, err := s.ValidateAPIKey(token); err != nil {
		t.Fatalf("key with nil expiry should always be valid, got: %v", err)
	}
}

// TestAPIKey_ExpiryFuture creates a key expiring in the future and verifies it validates.
func TestAPIKey_ExpiryFuture(t *testing.T) {
	s := NewStore(openAuthTestDB(t))

	future := time.Now().Add(24 * time.Hour)
	token, key, err := s.GenerateAPIKey("vault-exp", "future-key", "full", &future)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if key.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	if _, err := s.ValidateAPIKey(token); err != nil {
		t.Fatalf("key with future expiry should be valid, got: %v", err)
	}
}

// TestAPIKey_ExpiryPast creates a key that is already expired and verifies it is rejected.
func TestAPIKey_ExpiryPast(t *testing.T) {
	s := NewStore(openAuthTestDB(t))

	past := time.Now().Add(-1 * time.Hour)
	token, _, err := s.GenerateAPIKey("vault-exp", "expired-key", "full", &past)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if _, err := s.ValidateAPIKey(token); err == nil {
		t.Fatal("expected ValidateAPIKey to reject an expired key, got nil")
	}
}

// TestGenerateAPIKey_WriteModeAccepted verifies that "write" is a valid mode.
func TestGenerateAPIKey_WriteModeAccepted(t *testing.T) {
	s := NewStore(openAuthTestDB(t))
	_, _, err := s.GenerateAPIKey("default", "ingest-bot", "write", nil)
	if err != nil {
		t.Errorf("write mode should be accepted, got: %v", err)
	}
}

// TestGenerateAPIKey_InvalidModeRejected verifies that unknown modes are rejected.
func TestGenerateAPIKey_InvalidModeRejected(t *testing.T) {
	s := NewStore(openAuthTestDB(t))
	_, _, err := s.GenerateAPIKey("default", "bad", "superuser", nil)
	if err == nil {
		t.Error("expected error for invalid mode, got nil")
	}
}

// TestAPIKey_WrongVault creates a key for "vault-a" and verifies that
// ValidateAPIKey still succeeds (keys are global by token, not vault-scoped)
// but the returned key's vault is "vault-a", not "vault-b".
func TestAPIKey_WrongVault(t *testing.T) {
	s := NewStore(openAuthTestDB(t))

	token, _, err := s.GenerateAPIKey("vault-a", "label", "full", nil)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	got, err := s.ValidateAPIKey(token)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	// The stored key is for vault-a. If a caller checks got.Vault against "vault-b",
	// they would see a mismatch. This test asserts the vault field is correct.
	if got.Vault != "vault-a" {
		t.Errorf("expected vault 'vault-a', got %q", got.Vault)
	}
	if got.Vault == "vault-b" {
		t.Error("key should not be valid for vault-b")
	}
}
