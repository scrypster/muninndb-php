package brief

import (
	"strings"
	"testing"
)

// TestSplit_Abbreviations documents a known limitation of the simple sentence
// splitter: "Dr." followed by a space is treated as a sentence boundary, so
// "Dr. Smith said hello." may be split into 2 sentences. The test verifies only
// that Split does not panic and returns at least 1 sentence.
func TestSplit_Abbreviations(t *testing.T) {
	input := "Dr. Smith said hello."
	result := Split(input, 0)
	if len(result) < 1 {
		t.Fatalf("expected >= 1 sentence, got %d", len(result))
	}
}

// TestSplit_EmptyString verifies that an empty input produces no panic and
// returns nil or an empty slice.
func TestSplit_EmptyString(t *testing.T) {
	result := Split("", 0)
	if len(result) != 0 {
		t.Fatalf("expected empty result for empty input, got %v", result)
	}
}

// TestSplit_SingleChar verifies that a single non-punctuation character does
// not panic and is returned as-is (the remaining-text path).
func TestSplit_SingleChar(t *testing.T) {
	result := Split("a", 0)
	if len(result) != 1 {
		t.Fatalf("expected 1 sentence for single char input, got %d", len(result))
	}
	if result[0] != "a" {
		t.Fatalf("expected \"a\", got %q", result[0])
	}
}

// TestSplit_OnlyWhitespace verifies that a whitespace-only input does not
// panic and produces 0 or 1 items (no non-empty sentence should be emitted).
func TestSplit_OnlyWhitespace(t *testing.T) {
	result := Split("   ", 0)
	// The splitter may return nil or an empty slice, but must not return a
	// non-empty string as a sentence.
	for _, s := range result {
		if strings.TrimSpace(s) == "" {
			t.Fatalf("expected no whitespace-only sentences, got %q", s)
		}
	}
}

// TestTruncateAtWordBoundary_NoSpace verifies that when a string has no spaces
// and exceeds maxLen, the result is truncated to at most maxLen characters plus
// an ellipsis ("...") suffix, without panicking.
func TestTruncateAtWordBoundary_NoSpace(t *testing.T) {
	// 100 characters, no spaces
	input := strings.Repeat("a", 100)
	maxLen := 20

	// Exercise via the exported Split function with a maxLen that forces truncation.
	// Append "." so Split treats the whole string as a sentence.
	sentences := Split(input+".", maxLen)
	if len(sentences) == 0 {
		t.Fatal("expected at least 1 sentence after truncation")
	}

	// Also call truncateAtWordBoundary directly (it is unexported but accessible
	// from within the same package).
	result := truncateAtWordBoundary(input, maxLen)
	// No space found path: result should be text[:maxLen] + "..."
	wantSuffix := "..."
	if !strings.HasSuffix(result, wantSuffix) {
		t.Fatalf("expected result to end with %q for no-space input, got %q", wantSuffix, result)
	}
	// Length check: text[:maxLen] is 20 chars, plus "..." is 23 total
	if len(result) > maxLen+len(wantSuffix) {
		t.Fatalf("result length %d exceeds maxLen+suffix (%d)", len(result), maxLen+len(wantSuffix))
	}
}

// TestTruncateAtWordBoundary_ExactLen verifies that a string whose length
// exactly equals maxLen is returned unchanged (no truncation or ellipsis added).
func TestTruncateAtWordBoundary_ExactLen(t *testing.T) {
	input := "hello world" // 11 chars
	maxLen := len(input)   // exactly 11

	result := truncateAtWordBoundary(input, maxLen)
	if result != input {
		t.Fatalf("expected unchanged string %q, got %q", input, result)
	}
}
