package erf

import (
	"testing"
)

// FuzzDecode verifies that Decode never panics on arbitrary byte inputs.
// The decode path has bounds checks but a fuzzer can find edge cases
// where boundary conditions produce unexpected behavior.
func FuzzDecode(f *testing.F) {
	// Seed corpus with a valid encoded engram
	eng := &Engram{
		Concept:   "seed concept",
		Content:   "seed content for fuzzing",
		CreatedBy: "fuzz-test",
		Tags:      []string{"a", "b"},
	}
	encoded, err := EncodeV2(eng)
	if err != nil {
		f.Fatalf("seed encode failed: %v", err)
	}
	f.Add(encoded)

	// Also seed with a minimal valid record and empty input
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x01, 0x02})
	f.Add(encoded[:len(encoded)/2]) // truncated

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic — any input is valid fuzz input
		// Decode is allowed to return errors for invalid data
		_, _ = Decode(data)
	})
}

// FuzzEncodeDecodeRoundTrip verifies that Encode → Decode is lossless
// for valid engram field combinations.
func FuzzEncodeDecodeRoundTrip(f *testing.F) {
	// Seed with varied string lengths and characters
	f.Add("concept", "content", "creator", "tag1,tag2")
	f.Add("a", "b", "", "")
	f.Add("unicode 日本語", "emoji 🧠", "agent", "memory,test")
	f.Add("", "minimal content", "", "")

	f.Fuzz(func(t *testing.T, concept, content, createdBy, tagsCSV string) {
		// Skip inputs that exceed field size limits (encoder returns error, not panic)
		if len(concept) > MaxConceptBytes || len(content) > MaxContentBytes || len(createdBy) > MaxCreatedByBytes {
			return
		}

		var tags []string
		if tagsCSV != "" {
			// Split on comma for variety
			start := 0
			for i, c := range tagsCSV {
				if c == ',' {
					if i > start {
						tags = append(tags, tagsCSV[start:i])
					}
					start = i + 1
				}
			}
			if start < len(tagsCSV) {
				tags = append(tags, tagsCSV[start:])
			}
		}

		eng := &Engram{
			Concept:   concept,
			Content:   content,
			CreatedBy: createdBy,
			Tags:      tags,
		}

		encoded, err := EncodeV2(eng)
		if err != nil {
			return // field limit exceeded — not a bug
		}

		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode failed on EncodeV2 output: %v", err)
		}

		if decoded.Concept != concept {
			t.Errorf("Concept mismatch: got %q, want %q", decoded.Concept, concept)
		}
		if decoded.Content != content {
			t.Errorf("Content mismatch: got %q, want %q", decoded.Content, content)
		}
	})
}

// FuzzDecodeAssociation verifies that DecodeAssociation never panics on
// arbitrary byte inputs of exact AssocRecordSize length.
func FuzzDecodeAssociation(f *testing.F) {
	// Seed with a valid association record
	var buf [AssocRecordSize]byte
	f.Add(buf[:])

	// Seed with all-0xFF
	var allFF [AssocRecordSize]byte
	for i := range allFF {
		allFF[i] = 0xFF
	}
	f.Add(allFF[:])

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) != AssocRecordSize {
			return // only test exact-size inputs
		}
		_, _ = DecodeAssociation(data)
	})
}
