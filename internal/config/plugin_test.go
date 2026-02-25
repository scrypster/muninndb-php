package config

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestEnrichStageEnabled_DefaultsTrue(t *testing.T) {
	cfg := &PluginConfig{}
	for _, stage := range []string{"entities", "relationships", "classification", "summary"} {
		if !cfg.EnrichStageEnabled(stage) {
			t.Errorf("stage %q should default to enabled", stage)
		}
	}
}

func TestEnrichStageEnabled_DisableEntities(t *testing.T) {
	cfg := &PluginConfig{EnrichEntities: boolPtr(false)}
	if cfg.EnrichStageEnabled("entities") {
		t.Error("entities should be disabled")
	}
	if !cfg.EnrichStageEnabled("relationships") {
		t.Error("relationships should still be enabled")
	}
}

func TestEnrichStageEnabled_DisableRelationships(t *testing.T) {
	cfg := &PluginConfig{EnrichRelationships: boolPtr(false)}
	if cfg.EnrichStageEnabled("relationships") {
		t.Error("relationships should be disabled")
	}
	if !cfg.EnrichStageEnabled("entities") {
		t.Error("entities should still be enabled")
	}
}

func TestEnrichStageEnabled_DisableClassification(t *testing.T) {
	cfg := &PluginConfig{EnrichClassification: boolPtr(false)}
	if cfg.EnrichStageEnabled("classification") {
		t.Error("classification should be disabled")
	}
}

func TestEnrichStageEnabled_DisableSummary(t *testing.T) {
	cfg := &PluginConfig{EnrichSummary: boolPtr(false)}
	if cfg.EnrichStageEnabled("summary") {
		t.Error("summary should be disabled")
	}
}

func TestEnrichStageEnabled_UnknownStage(t *testing.T) {
	cfg := &PluginConfig{}
	if !cfg.EnrichStageEnabled("unknown") {
		t.Error("unknown stages should default to enabled")
	}
}

func TestIsLightMode(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"light", true},
		{"full", false},
		{"", false},
		{"LIGHT", false},
	}
	for _, tt := range tests {
		cfg := &PluginConfig{EnrichMode: tt.mode}
		if cfg.IsLightMode() != tt.want {
			t.Errorf("IsLightMode() for mode %q = %v, want %v", tt.mode, cfg.IsLightMode(), tt.want)
		}
	}
}
