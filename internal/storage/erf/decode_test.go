package erf

import (
	"testing"
	"time"
)

func TestDecode_RejectsUnknownVersion(t *testing.T) {
	// Build a minimal valid v1 record and corrupt the version byte to 0x99
	eng := &Engram{
		Concept:   "test",
		Content:   "content",
		CreatedBy: "tester",
	}
	copy(eng.ID[:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	eng.CreatedAt = time.Now()
	eng.UpdatedAt = time.Now()

	data, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	data[4] = 0x99 // corrupt version
	_, err = Decode(data)
	if err == nil {
		t.Error("expected error for unknown version, got nil")
	}
}

func TestDecode_NewMemoryTypes(t *testing.T) {
	types := []struct {
		val  uint8
		name string
	}{
		{0, "fact"}, {1, "decision"}, {2, "observation"}, {3, "preference"},
		{4, "issue"}, {5, "task"}, {6, "procedure"}, {7, "event"},
		{8, "goal"}, {9, "constraint"}, {10, "identity"}, {11, "reference"},
	}
	for _, tc := range types {
		eng := &Engram{
			ID:         newTestULID(),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Concept:    "type test " + tc.name,
			Content:    "content",
			MemoryType: tc.val,
		}
		data, err := EncodeV2(eng)
		if err != nil {
			t.Fatalf("EncodeV2 for type %d (%s): %v", tc.val, tc.name, err)
		}
		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode for type %d (%s): %v", tc.val, tc.name, err)
		}
		if decoded.MemoryType != tc.val {
			t.Errorf("type %s: got MemoryType=%d, want %d", tc.name, decoded.MemoryType, tc.val)
		}
	}
}

func TestDecode_V2AcceptsVersion(t *testing.T) {
	// Manually construct a minimal v2 record (no assoc/embed) using EncodeV2
	eng := &Engram{
		Concept:   "v2test",
		Content:   "v2content",
		CreatedBy: "tester",
		Tags:      []string{"x"},
	}
	copy(eng.ID[:], []byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160})
	eng.CreatedAt = time.Now()
	eng.UpdatedAt = time.Now()
	eng.Confidence = 0.75

	data, err := EncodeV2(eng)
	if err != nil {
		t.Fatalf("EncodeV2: %v", err)
	}

	// Decode must succeed for v2
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode v2: %v", err)
	}

	if got.Concept != "v2test" {
		t.Errorf("concept = %q, want 'v2test'", got.Concept)
	}
	if got.Content != "v2content" {
		t.Errorf("content = %q, want 'v2content'", got.Content)
	}
	if len(got.Associations) != 0 {
		t.Errorf("associations not empty: %v", got.Associations)
	}
	if len(got.Embedding) != 0 {
		t.Errorf("embedding not empty: %v", got.Embedding)
	}
	if got.Confidence != 0.75 {
		t.Errorf("confidence = %v, want 0.75", got.Confidence)
	}
}
