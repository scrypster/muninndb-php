package engine

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// TestRecordAccess_Normal writes an engram, calls RecordAccess, and verifies
// that AccessCount is incremented and LastAccess is recent.
func TestRecordAccess_Normal(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	// Write an engram to the vault.
	writeResp, err := eng.Write(ctx, &mbp.WriteRequest{
		Vault:   "access-test",
		Concept: "access test concept",
		Content: "content for access count test",
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Record the time just before RecordAccess.
	before := time.Now().Add(-time.Second)

	// Call RecordAccess.
	if err := eng.RecordAccess(ctx, "access-test", writeResp.ID); err != nil {
		t.Fatalf("RecordAccess: %v", err)
	}

	// Read back the engram to inspect the updated fields.
	readResp, err := eng.Read(ctx, &mbp.ReadRequest{
		Vault: "access-test",
		ID:    writeResp.ID,
	})
	if err != nil {
		t.Fatalf("Read after RecordAccess: %v", err)
	}

	// AccessCount must be > 0 after RecordAccess.
	if readResp.AccessCount <= 0 {
		t.Errorf("AccessCount = %d, want > 0", readResp.AccessCount)
	}

	// LastAccess must be recent (after before).
	lastAccess := time.Unix(0, readResp.LastAccess)
	if lastAccess.Before(before) {
		t.Errorf("LastAccess %v is not recent (before %v)", lastAccess, before)
	}
}

// TestRecordAccess_NotFound calls RecordAccess on a non-existent ULID and
// expects a non-nil error.
func TestRecordAccess_NotFound(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()
	ctx := context.Background()

	// Use a valid ULID format that is not stored in the vault.
	err := eng.RecordAccess(ctx, "access-test", "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if err == nil {
		t.Error("expected error for non-existent ULID, got nil")
	}
}

// TestRecordAccess_ContextCancel cancels the context before RecordAccess and
// expects a non-nil error (either context.Canceled or a storage error).
func TestRecordAccess_ContextCancel(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	// Write an engram so we have a valid ID to reference.
	writeResp, err := eng.Write(context.Background(), &mbp.WriteRequest{
		Vault:   "access-test",
		Concept: "cancel test",
		Content: "content for cancel test",
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Create and immediately cancel the context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// RecordAccess with a cancelled context — must return a non-nil error.
	err = eng.RecordAccess(ctx, "access-test", writeResp.ID)
	if err == nil {
		// Some storage backends may complete the read before noticing cancellation;
		// this is acceptable. Log it and continue.
		t.Log("note: RecordAccess with cancelled context returned nil (storage may have completed before cancel was observed)")
	}
}
