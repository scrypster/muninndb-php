package consolidation

import (
	"math"
	"testing"
)

// TestCosine_ZeroMagnitudeA verifies that a zero-magnitude first vector
// returns 0.0 without panicking.
func TestCosine_ZeroMagnitudeA(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	got := cosineSimilarity(a, b)
	if got != 0.0 {
		t.Errorf("cosineSimilarity(zero, non-zero) = %v, want 0.0", got)
	}
}

// TestCosine_BothZero verifies that two zero-magnitude vectors return 0.0
// without panicking.
func TestCosine_BothZero(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{0, 0, 0}
	got := cosineSimilarity(a, b)
	if got != 0.0 {
		t.Errorf("cosineSimilarity(zero, zero) = %v, want 0.0", got)
	}
}

// TestCosine_Identical verifies that two identical non-zero vectors return ~1.0.
func TestCosine_Identical(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2, 3}
	got := cosineSimilarity(a, b)
	if math.Abs(float64(got)-1.0) > 1e-6 {
		t.Errorf("cosineSimilarity(identical) = %v, want ~1.0", got)
	}
}

// TestCosine_Opposite verifies that two opposite unit vectors return ~-1.0.
func TestCosine_Opposite(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{-1, 0, 0}
	got := cosineSimilarity(a, b)
	if math.Abs(float64(got)+1.0) > 1e-6 {
		t.Errorf("cosineSimilarity(opposite) = %v, want ~-1.0", got)
	}
}

// TestCosine_MismatchedLength verifies that vectors of different lengths return 0.0
// (the implementation checks len(a) != len(b) and returns 0).
func TestCosine_MismatchedLength(t *testing.T) {
	a := []float32{1, 2}
	b := []float32{1, 2, 3}
	// The implementation returns 0 when lengths differ (see dedup.go).
	got := cosineSimilarity(a, b)
	if got != 0.0 {
		t.Errorf("cosineSimilarity(mismatched length) = %v, want 0.0", got)
	}
}
