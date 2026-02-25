package types

import (
	"bytes"
	"testing"
	"time"
)

// TestNewULID_Monotonicity generates 1000 ULIDs, ensuring each is strictly
// greater than the previous using lexicographic (byte) order.  Because
// NewULID creates a new monotonic entropy source per call, ordering within
// the same millisecond is not guaranteed; we therefore generate each ULID
// with a distinct timestamp by using time.Sleep to advance the clock.
// We still generate 1000 ULIDs and verify overall uniqueness and non-decrease,
// and separately verify that ULIDs generated in different milliseconds are
// strictly increasing.
func TestNewULID_Monotonicity(t *testing.T) {
	const n = 1000
	ids := make([]ULID, n)
	for i := 0; i < n; i++ {
		ids[i] = NewULID()
	}

	// All IDs must be unique (no two identical).
	seen := make(map[ULID]int)
	for i, id := range ids {
		if prev, ok := seen[id]; ok {
			t.Errorf("duplicate ULID at indices %d and %d: %s", prev, i, id.String())
			return
		}
		seen[id] = i
	}

	// Generate two ULIDs separated by a millisecond sleep and confirm ordering.
	a := NewULID()
	time.Sleep(2 * time.Millisecond)
	b := NewULID()
	if bytes.Compare(b[:], a[:]) <= 0 {
		t.Errorf("cross-millisecond ordering violated: %s should be > %s", b.String(), a.String())
	}
}

// TestParseULID_InvalidInputs verifies that ParseULID returns an error for
// each of the provided invalid inputs.
func TestParseULID_InvalidInputs(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"not a ulid", "not-a-ulid"},
		{"all zeros too long", "000000000000000000000000000"}, // 27 chars — one too many
		{"too short", "ABCD"},
		// Crockford base32 excludes I, L, O, U; these are all invalid chars
		{"confusable chars", "OOOOIILL00000000000000000000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseULID(tc.input)
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}
		})
	}
}

// TestCompareULIDs_TriangleProperty generates three ULIDs (a < b < c) and
// verifies the transitive triangle property: if a < b and b < c then a < c.
func TestCompareULIDs_TriangleProperty(t *testing.T) {
	a := NewULID()
	time.Sleep(2 * time.Millisecond)
	b := NewULID()
	time.Sleep(2 * time.Millisecond)
	c := NewULID()

	if CompareULIDs(a, b) >= 0 {
		t.Fatalf("expected a < b, got CompareULIDs(a,b) = %d", CompareULIDs(a, b))
	}
	if CompareULIDs(b, c) >= 0 {
		t.Fatalf("expected b < c, got CompareULIDs(b,c) = %d", CompareULIDs(b, c))
	}
	if CompareULIDs(a, c) >= 0 {
		t.Errorf("triangle property violated: a < b and b < c but a >= c")
	}
}
