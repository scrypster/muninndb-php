package mcp

import "fmt"

// RecallMode is a preset bundle of recall parameters for common retrieval patterns.
// Fields with zero values are not applied (caller defaults remain).
type RecallMode struct {
	MaxHops   int
	Threshold float32
	// Scoring weight hints (applied to mbp.ActivateRequest if non-zero)
	SemanticSimilarity float32
	FullTextRelevance  float32
	Recency            float32
}

// recallModes are the built-in presets for muninn_recall.
var recallModes = map[string]RecallMode{
	"semantic": {
		SemanticSimilarity: 0.8,
		FullTextRelevance:  0.2,
		MaxHops:            0,
		Threshold:          0.3,
	},
	"recent": {
		Recency:            0.7,
		SemanticSimilarity: 0.3,
		MaxHops:            1,
		Threshold:          0.2,
	},
	"balanced": {}, // zero value = engine defaults
	"deep": {
		MaxHops:   4,
		Threshold: 0.1,
	},
}

// lookupMode returns the RecallMode for the given name.
// Returns an error for unknown mode names — no silent degradation.
func lookupMode(name string) (RecallMode, error) {
	m, ok := recallModes[name]
	if !ok {
		return RecallMode{}, fmt.Errorf("unknown recall mode %q: valid modes are semantic, recent, balanced, deep", name)
	}
	return m, nil
}
