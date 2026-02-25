package activation_test

import (
	"context"
	"testing"
	"time"

	"github.com/scrypster/muninndb/internal/engine/activation"
	"github.com/scrypster/muninndb/internal/storage"
)

// makeTestEngine creates a minimal ActivationEngine with no indices or embedder.
func makeTestEngine() *activation.ActivationEngine {
	return activation.New(newStubStore(), nil, nil, nil)
}

// makeActivations builds a slice of N ScoredEngrams for use in stream tests.
func makeActivations(n int) []activation.ScoredEngram {
	out := make([]activation.ScoredEngram, n)
	for i := 0; i < n; i++ {
		out[i] = activation.ScoredEngram{
			Engram: &storage.Engram{
				ID:         storage.NewULID(),
				Concept:    "concept",
				Confidence: 1.0,
				Stability:  30.0,
				CreatedAt:  time.Now(),
				LastAccess: time.Now(),
			},
			Score: float64(n-i) * 0.01,
		}
	}
	return out
}

// TestActivationStream_EmptyResult verifies that streaming a result with zero
// activations calls send exactly once with FrameNum=1, TotalFrames=1, and an
// empty Activations slice.
func TestActivationStream_EmptyResult(t *testing.T) {
	eng := makeTestEngine()
	defer eng.Close()

	result := &activation.ActivateResult{
		QueryID:     "q-test-empty",
		Activations: nil,
		TotalFound:  0,
	}

	var frames []*activation.ActivateResponseFrame
	err := eng.Stream(context.Background(), result, func(f *activation.ActivateResponseFrame) error {
		frames = append(frames, f)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	f := frames[0]
	if f.Frame != 1 {
		t.Errorf("expected Frame=1, got %d", f.Frame)
	}
	if f.TotalFrames != 1 {
		t.Errorf("expected TotalFrames=1, got %d", f.TotalFrames)
	}
	if len(f.Activations) != 0 {
		t.Errorf("expected 0 activations, got %d", len(f.Activations))
	}
}

// TestActivationStream_MultiFrame verifies that streaming N activations where
// N > frameSize (100) produces ceil(N/frameSize) frames, with consecutive frame
// numbers and no missing or duplicated activations.
func TestActivationStream_MultiFrame(t *testing.T) {
	eng := makeTestEngine()
	defer eng.Close()

	// frameSize is 100 (from engine.go const frameSize = 100).
	// Use 250 activations to get ceil(250/100) = 3 frames.
	const N = 250
	activations := makeActivations(N)

	result := &activation.ActivateResult{
		QueryID:     "q-test-multi",
		Activations: activations,
		TotalFound:  N,
	}

	var frames []*activation.ActivateResponseFrame
	err := eng.Stream(context.Background(), result, func(f *activation.ActivateResponseFrame) error {
		frames = append(frames, f)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	const frameSize = 100
	expectedFrameCount := (N + frameSize - 1) / frameSize // ceil(250/100) = 3
	if len(frames) != expectedFrameCount {
		t.Fatalf("expected %d frames, got %d", expectedFrameCount, len(frames))
	}

	// Verify consecutive frame numbering.
	for i, f := range frames {
		wantFrameNum := i + 1
		if f.Frame != wantFrameNum {
			t.Errorf("frame[%d]: expected Frame=%d, got %d", i, wantFrameNum, f.Frame)
		}
		if f.TotalFrames != expectedFrameCount {
			t.Errorf("frame[%d]: expected TotalFrames=%d, got %d", i, expectedFrameCount, f.TotalFrames)
		}
	}

	// Collect all activations from all frames and verify completeness.
	seen := make(map[storage.ULID]bool, N)
	total := 0
	for _, f := range frames {
		for _, a := range f.Activations {
			id := a.Engram.ID
			if seen[id] {
				t.Errorf("duplicate engram ID %v across frames", id)
			}
			seen[id] = true
			total++
		}
	}

	if total != N {
		t.Errorf("expected %d total activations across all frames, got %d", N, total)
	}

	// Verify every original activation appears in some frame.
	for i, a := range activations {
		if !seen[a.Engram.ID] {
			t.Errorf("activation[%d] (ID=%v) missing from streamed frames", i, a.Engram.ID)
		}
	}
}
