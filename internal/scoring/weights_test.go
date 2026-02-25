package scoring

import (
	"math"
	"testing"
	"time"
)

func TestSoftmax_SumsToOne(t *testing.T) {
	w := [NumDims]float64{1.0, 2.0, 3.0, 0.5, 1.5, 2.5}
	normalized := Softmax(w)

	sum := 0.0
	for _, v := range normalized {
		sum += v
		if v < 0 || v > 1 {
			t.Errorf("softmax value out of range [0,1]: %v", v)
		}
	}

	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("softmax sum = %v, want ~1.0", sum)
	}
}

func TestSoftmax_PreservesOrder(t *testing.T) {
	w := [NumDims]float64{1.0, 5.0, 3.0, 0.5, 2.0, 4.0}
	normalized := Softmax(w)

	// Order should be preserved: 5.0 > 4.0 > 3.0 > 2.0 > 1.0 > 0.5
	expectedOrder := []int{1, 5, 2, 4, 0, 3}
	for i := 0; i < len(expectedOrder)-1; i++ {
		if normalized[expectedOrder[i]] < normalized[expectedOrder[i+1]] {
			t.Errorf("softmax order not preserved: %v[%d]=%v should be > %v[%d]=%v",
				expectedOrder[i], expectedOrder[i], normalized[expectedOrder[i]],
				expectedOrder[i+1], expectedOrder[i+1], normalized[expectedOrder[i+1]])
		}
	}
}

func TestUpdate_PositiveSignal(t *testing.T) {
	vw := &VaultWeights{
		VaultPrefix:  [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		Weights:      DefaultWeights(),
		LearningRate: 0.1,
		UpdatedAt:    time.Now(),
	}

	signal := FeedbackSignal{
		EngramID:  [16]byte{0x01},
		Accessed:  true, // positive signal
		Timestamp: time.Now(),
	}

	// Score vector heavily emphasizes FTS and HNSW
	signal.ScoreVector[DimFTS] = 0.8
	signal.ScoreVector[DimHNSW] = 0.7
	signal.ScoreVector[DimHebbian] = 0.2
	signal.ScoreVector[DimDecay] = 0.1
	signal.ScoreVector[DimRecency] = 0.05
	signal.ScoreVector[DimAssociation] = 0.05

	oldWeights := vw.Weights
	vw.Update(signal)

	// FTS and HNSW dimensions should have increased
	if vw.Weights[DimFTS] <= oldWeights[DimFTS] {
		t.Errorf("FTS weight should increase after positive signal: %v -> %v",
			oldWeights[DimFTS], vw.Weights[DimFTS])
	}
	if vw.Weights[DimHNSW] <= oldWeights[DimHNSW] {
		t.Errorf("HNSW weight should increase after positive signal: %v -> %v",
			oldWeights[DimHNSW], vw.Weights[DimHNSW])
	}

	// Decay and recency dimensions should have decreased (low scores)
	if vw.Weights[DimDecay] >= oldWeights[DimDecay] {
		t.Errorf("Decay weight should decrease after positive signal: %v -> %v",
			oldWeights[DimDecay], vw.Weights[DimDecay])
	}
}

func TestUpdate_NegativeSignal(t *testing.T) {
	vw := &VaultWeights{
		VaultPrefix:  [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		Weights:      DefaultWeights(),
		LearningRate: 0.1,
		UpdatedAt:    time.Now(),
	}

	signal := FeedbackSignal{
		EngramID:  [16]byte{0x02},
		Accessed:  false, // negative signal
		Timestamp: time.Now(),
	}

	// Score vector emphasizes Hebbian
	signal.ScoreVector[DimHebbian] = 0.9
	signal.ScoreVector[DimHNSW] = 0.1

	oldWeights := vw.Weights
	vw.Update(signal)

	// Hebbian dimension should have decreased (negative gradient)
	if vw.Weights[DimHebbian] >= oldWeights[DimHebbian] {
		t.Errorf("Hebbian weight should decrease after negative signal: %v -> %v",
			oldWeights[DimHebbian], vw.Weights[DimHebbian])
	}

	// HNSW dimension should have increased (it was low in score)
	if vw.Weights[DimHNSW] <= oldWeights[DimHNSW] {
		t.Errorf("HNSW weight should increase after negative signal: %v -> %v",
			oldWeights[DimHNSW], vw.Weights[DimHNSW])
	}
}

func TestUpdate_MinFloor(t *testing.T) {
	vw := &VaultWeights{
		VaultPrefix:  [8]byte{0x01},
		Weights:      [NumDims]float64{0.16, 0.16, 0.16, 0.16, 0.18, 0.18},
		LearningRate: 1.0, // aggressive learning for testing
		UpdatedAt:    time.Now(),
	}

	// Apply many negative signals to try to drive weights below floor
	for i := 0; i < 10; i++ {
		signal := FeedbackSignal{
			EngramID:  [16]byte{byte(i)},
			Accessed:  false,
			Timestamp: time.Now(),
			ScoreVector: [NumDims]float64{
				1.0, 0.1, 0.1, 0.1, 0.1, 0.1,
			},
		}
		vw.Update(signal)
	}

	// All dimensions should be >= 0.05 (floor)
	for i := 0; i < NumDims; i++ {
		if vw.Weights[i] < 0.05 {
			t.Errorf("weight[%d] = %v, should be >= 0.05 (floor)", i, vw.Weights[i])
		}
	}
}

func TestUpdate_NormalizesAfterUpdate(t *testing.T) {
	vw := &VaultWeights{
		VaultPrefix:  [8]byte{0x01},
		Weights:      DefaultWeights(),
		LearningRate: 0.1,
		UpdatedAt:    time.Now(),
	}

	signal := FeedbackSignal{
		EngramID:    [16]byte{0x01},
		Accessed:    true,
		Timestamp:   time.Now(),
		ScoreVector: [NumDims]float64{0.8, 0.7, 0.2, 0.1, 0.05, 0.05},
	}

	vw.Update(signal)

	sum := 0.0
	for i := 0; i < NumDims; i++ {
		if vw.Weights[i] < 0 {
			t.Errorf("weight[%d] is negative: %v", i, vw.Weights[i])
		}
		sum += vw.Weights[i]
	}

	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("weights sum = %v after update, want ~1.0", sum)
	}
}

func TestBlend_WeightedAverage(t *testing.T) {
	vw := &VaultWeights{
		VaultPrefix: [8]byte{0x01},
		Weights:     [NumDims]float64{0.5, 0.1, 0.1, 0.1, 0.1, 0.1},
	}

	parent := [NumDims]float64{0.1, 0.5, 0.1, 0.1, 0.1, 0.1}

	result := vw.Blend(parent)

	// result = 0.7 * vw + 0.3 * parent
	expected := [NumDims]float64{
		0.7*0.5 + 0.3*0.1, // 0.38
		0.7*0.1 + 0.3*0.5, // 0.22
		0.7*0.1 + 0.3*0.1, // 0.10
		0.7*0.1 + 0.3*0.1, // 0.10
		0.7*0.1 + 0.3*0.1, // 0.10
		0.7*0.1 + 0.3*0.1, // 0.10
	}

	for i := 0; i < NumDims; i++ {
		if math.Abs(result[i]-expected[i]) > 1e-9 {
			t.Errorf("blend[%d] = %v, want %v", i, result[i], expected[i])
		}
	}
}

func TestDefaultWeights_Valid(t *testing.T) {
	w := DefaultWeights()

	sum := 0.0
	for i := 0; i < NumDims; i++ {
		if w[i] < 0 || w[i] > 1 {
			t.Errorf("default weight[%d] = %v, out of range [0,1]", i, w[i])
		}
		sum += w[i]
	}

	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("default weights sum = %v, want ~1.0", sum)
	}
}

// TestSoftmax_NaNInput verifies that a NaN in the input produces no NaN in the output.
// The implementation falls back to a uniform distribution when NaN is detected.
func TestSoftmax_NaNInput(t *testing.T) {
	w := [NumDims]float64{math.NaN(), 1.0, 0.0, 0.0, 0.0, 0.0}
	result := Softmax(w)

	for i, v := range result {
		if math.IsNaN(v) {
			t.Errorf("result[%d] = NaN, expected finite value", i)
		}
	}

	// Result must still be a valid probability distribution.
	sum := 0.0
	for _, v := range result {
		sum += v
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("softmax(NaN input) sum = %v, want ~1.0", sum)
	}
}

// TestSoftmax_InfInput verifies that +Inf in the input is handled gracefully.
// The +Inf element should dominate and receive weight ~1.0; the rest ~0.0.
func TestSoftmax_InfInput(t *testing.T) {
	w := [NumDims]float64{math.Inf(1), 1.0, 0.0, 0.0, 0.0, 0.0}
	result := Softmax(w)

	for i, v := range result {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("result[%d] = %v, expected finite value", i, v)
		}
	}

	// Result must be a valid probability distribution.
	sum := 0.0
	for _, v := range result {
		sum += v
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("softmax(Inf input) sum = %v, want ~1.0", sum)
	}
}

// TestSoftmax_AllZero verifies that all-zero input yields a uniform distribution.
func TestSoftmax_AllZero(t *testing.T) {
	w := [NumDims]float64{0, 0, 0, 0, 0, 0}
	result := Softmax(w)

	// All elements should be equal.
	expected := 1.0 / float64(NumDims)
	for i, v := range result {
		if math.Abs(v-expected) > 1e-9 {
			t.Errorf("result[%d] = %v, want %v (uniform)", i, v, expected)
		}
	}

	sum := 0.0
	for _, v := range result {
		sum += v
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("softmax(all-zero) sum = %v, want ~1.0", sum)
	}
}

// TestUpdate_NaNGradient verifies that a NaN gradient value is skipped and
// does not propagate NaN into stored weights.
func TestUpdate_NaNGradient(t *testing.T) {
	vw := &VaultWeights{
		VaultPrefix:  [8]byte{0x01},
		Weights:      DefaultWeights(),
		LearningRate: 0.1,
		UpdatedAt:    time.Now(),
	}

	// ScoreVector containing NaN — gradient = lr * direction * NaN = NaN.
	signal := FeedbackSignal{
		EngramID:  [16]byte{0x01},
		Accessed:  true,
		Timestamp: time.Now(),
		ScoreVector: [NumDims]float64{
			math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN(),
		},
	}

	// Must not panic.
	vw.Update(signal)

	// Weights must remain finite and still sum to ~1.
	sum := 0.0
	for i, v := range vw.Weights {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("weight[%d] = %v after NaN gradient, expected finite value", i, v)
		}
		sum += v
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("weights sum = %v after NaN gradient update, want ~1.0", sum)
	}
}
