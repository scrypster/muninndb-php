package plugin

import (
	"context"
	"testing"
)

func TestStoreAdapter_NoOpMethods(t *testing.T) {
	// pluginStoreAdapter has several no-op methods that always return nil.
	// We can test them via the PluginStore interface by passing nil store/hnsw
	// since these methods never touch the backing store.
	adapter := &pluginStoreAdapter{}
	ctx := context.Background()
	var id ULID

	if err := adapter.UpdateDigest(ctx, id, &EnrichmentResult{}); err != nil {
		t.Errorf("UpdateDigest should return nil, got %v", err)
	}

	if err := adapter.UpsertEntity(ctx, ExtractedEntity{Name: "test"}); err != nil {
		t.Errorf("UpsertEntity should return nil, got %v", err)
	}

	if err := adapter.LinkEngramToEntity(ctx, id, "entity"); err != nil {
		t.Errorf("LinkEngramToEntity should return nil, got %v", err)
	}

	if err := adapter.UpsertRelationship(ctx, id, ExtractedRelation{FromEntity: "a", ToEntity: "b"}); err != nil {
		t.Errorf("UpsertRelationship should return nil, got %v", err)
	}
}

func TestNewStoreAdapter(t *testing.T) {
	adapter := NewStoreAdapter(nil, nil)
	if adapter == nil {
		t.Fatal("NewStoreAdapter returned nil")
	}
}

func TestStoreAdapter_ScanWithoutFlagNilStore(t *testing.T) {
	// ScanWithoutFlag on a nil-store adapter will panic if the store is nil,
	// but we can test the nil-iterator guard by creating an adapter with
	// a store that returns nil from ScanWithoutFlag. Since we can't easily
	// create a real PebbleStore here, we verify the interface compliance.
	var _ PluginStore = &pluginStoreAdapter{}
}
