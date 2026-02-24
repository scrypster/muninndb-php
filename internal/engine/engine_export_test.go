package engine

import (
	"bytes"
	"context"
	"testing"

	"github.com/scrypster/muninndb/internal/transport/mbp"
)

func TestEngineExportVault(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	ctx := context.Background()

	// Write some engrams.
	for i := 0; i < 3; i++ {
		_, err := eng.Write(ctx, &mbp.WriteRequest{
			Vault:   "export-src",
			Concept: "concept",
			Content: "test content",
		})
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	var buf bytes.Buffer
	result, err := eng.ExportVault(ctx, "export-src", "", 0, false, &buf)
	if err != nil {
		t.Fatalf("ExportVault: %v", err)
	}
	if result.EngramCount != 3 {
		t.Errorf("EngramCount: got %d, want 3", result.EngramCount)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty archive")
	}
}

func TestEngineExportVaultNotFound(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	ctx := context.Background()
	var buf bytes.Buffer
	_, err := eng.ExportVault(ctx, "no-such-vault", "", 0, false, &buf)
	if err == nil {
		t.Fatal("expected error for missing vault")
	}
}

func TestEngineStartImport(t *testing.T) {
	eng, cleanup := testEnv(t)
	defer cleanup()

	ctx := context.Background()

	// Write some engrams to source vault.
	for i := 0; i < 2; i++ {
		_, err := eng.Write(ctx, &mbp.WriteRequest{
			Vault:   "import-src",
			Concept: "concept",
			Content: "test content",
		})
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	// Export from source.
	var buf bytes.Buffer
	if _, err := eng.ExportVault(ctx, "import-src", "", 0, false, &buf); err != nil {
		t.Fatalf("ExportVault: %v", err)
	}

	// Import into new vault.
	job, err := eng.StartImport(ctx, "import-dst", "", 0, false, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	if job.ID == "" {
		t.Error("expected non-empty job ID")
	}
}
