package mcp

import (
	"testing"
)

func TestLookupMode_KnownModes(t *testing.T) {
	for _, name := range []string{"semantic", "recent", "balanced", "deep"} {
		m, err := lookupMode(name)
		if err != nil {
			t.Errorf("lookupMode(%q): unexpected error: %v", name, err)
		}
		_ = m
	}
}

func TestLookupMode_UnknownMode(t *testing.T) {
	_, err := lookupMode("turbo")
	if err == nil {
		t.Error("lookupMode(unknown): expected error, got nil")
	}
}

func TestLookupMode_DeepPreset(t *testing.T) {
	m, err := lookupMode("deep")
	if err != nil {
		t.Fatalf("lookupMode(deep): %v", err)
	}
	if m.MaxHops != 4 {
		t.Errorf("deep MaxHops = %d, want 4", m.MaxHops)
	}
	if m.Threshold != 0.1 {
		t.Errorf("deep Threshold = %v, want 0.1", m.Threshold)
	}
}

func TestLookupMode_SemanticPreset(t *testing.T) {
	m, err := lookupMode("semantic")
	if err != nil {
		t.Fatalf("lookupMode(semantic): %v", err)
	}
	if m.Threshold != 0.3 {
		t.Errorf("semantic Threshold = %v, want 0.3", m.Threshold)
	}
}
