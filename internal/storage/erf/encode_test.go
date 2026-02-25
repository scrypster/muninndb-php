package erf

import (
	"crypto/rand"
	"testing"
	"time"
)

func newTestULID() [16]byte {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return id
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	eng := &Engram{
		ID:          newTestULID(),
		CreatedAt:   time.Now().Truncate(time.Nanosecond),
		UpdatedAt:   time.Now().Truncate(time.Nanosecond),
		LastAccess:  time.Now().Truncate(time.Nanosecond),
		Confidence:  0.8,
		Relevance:   0.9,
		Stability:   2.5,
		AccessCount: 42,
		State:       0x01, // StateActive
		EmbedDim:    0x02, // Embed768
		Concept:     "Test concept",
		CreatedBy:   "tester",
		Content:     "This is test content with some length to make it meaningful.",
		Tags:        []string{"tag1", "tag2", "tag3"},
		Embedding:   generateTestEmbedding(768),
	}

	// Add associations
	eng.Associations = []Association{
		{
			TargetID:      newTestULID(),
			RelType:       0x0001, // RelSupports
			Weight:        0.75,
			Confidence:    0.85,
			CreatedAt:     time.Now().Truncate(time.Nanosecond),
			LastActivated: 1234567890,
		},
		{
			TargetID:      newTestULID(),
			RelType:       0x0002, // RelContradicts
			Weight:        0.5,
			Confidence:    0.6,
			CreatedAt:     time.Now().Truncate(time.Nanosecond),
			LastActivated: 1234567891,
		},
	}

	// Encode
	erfBytes, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Verify magic and version
	if len(erfBytes) < 8 {
		t.Fatal("encoded data too short")
	}
	if erfBytes[0] != 0x4D || erfBytes[1] != 0x55 || erfBytes[2] != 0x4E || erfBytes[3] != 0x4E {
		t.Error("magic bytes incorrect")
	}
	if erfBytes[4] != 0x01 {
		t.Error("version incorrect")
	}

	// Verify CRC16 at offset 6
	if !VerifyCRC16(erfBytes[0:8]) {
		t.Error("CRC16 verification failed")
	}

	// Decode
	decoded, err := Decode(erfBytes)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify all fields match
	if decoded.ID != eng.ID {
		t.Error("ID mismatch")
	}
	if decoded.Concept != eng.Concept {
		t.Error("Concept mismatch")
	}
	if decoded.Content != eng.Content {
		t.Error("Content mismatch")
	}
	if decoded.CreatedBy != eng.CreatedBy {
		t.Error("CreatedBy mismatch")
	}
	if len(decoded.Tags) != len(eng.Tags) {
		t.Error("Tags count mismatch")
	}
	for i, tag := range eng.Tags {
		if decoded.Tags[i] != tag {
			t.Errorf("Tag %d mismatch: %s != %s", i, decoded.Tags[i], tag)
		}
	}
	if len(decoded.Associations) != len(eng.Associations) {
		t.Error("Associations count mismatch")
	}
	if decoded.Confidence != eng.Confidence {
		t.Errorf("Confidence mismatch: %f != %f", decoded.Confidence, eng.Confidence)
	}
	if decoded.State != eng.State {
		t.Errorf("State mismatch: %d != %d", decoded.State, eng.State)
	}
}

func TestContentCompression(t *testing.T) {
	// Create large content > 512 bytes
	largeContent := ""
	for i := 0; i < 100; i++ {
		largeContent += "This is a longer piece of content that should be compressed. "
	}

	eng := &Engram{
		ID:        newTestULID(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Concept:   "Test",
		Content:   largeContent,
	}

	erfBytes, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Check flags for compression
	flags := erfBytes[5]
	if flags&FlagContentCompressed == 0 {
		t.Error("FlagContentCompressed not set")
	}

	// Decode and verify content matches
	decoded, err := Decode(erfBytes)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Content != eng.Content {
		t.Error("Compressed content mismatch after round-trip")
	}
}

func TestEmbeddingQuantization(t *testing.T) {
	embedding := []float32{
		0.1, 0.2, 0.3, 0.4, 0.5,
		-0.1, -0.2, -0.3, -0.4, -0.5,
	}

	eng := &Engram{
		ID:        newTestULID(),
		CreatedAt: time.Now(),
		Concept:   "Test",
		Content:   "Test",
		Embedding: embedding,
	}

	erfBytes, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Check flags
	flags := erfBytes[5]
	if flags&FlagHasEmbedding == 0 {
		t.Error("FlagHasEmbedding not set")
	}
	if flags&FlagEmbedQuantized == 0 {
		t.Error("FlagEmbedQuantized not set")
	}

	// Decode
	decoded, err := Decode(erfBytes)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify embedding length matches
	if len(decoded.Embedding) != len(eng.Embedding) {
		t.Errorf("Embedding length mismatch: %d != %d", len(decoded.Embedding), len(eng.Embedding))
	}

	// Check that dequantization error is small
	for i, orig := range eng.Embedding {
		dequant := decoded.Embedding[i]
		diff := orig - dequant
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.02 { // max error per dimension
			t.Errorf("Quantization error too large at index %d: %f", i, diff)
		}
	}
}

func TestFixedOverhead(t *testing.T) {
	// Minimal engram with no optional fields
	eng := &Engram{
		ID:        newTestULID(),
		CreatedAt: time.Now(),
		Concept:   "A",
		Content:   "B",
	}

	erfBytes, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Fixed overhead: 8 (header) + 100 (metadata) + 40 (offset table) + 4 (trailer) + variable data
	// Minimal variable: 1 byte concept + 1 byte content + variable tags/assoc/embed
	expectedMinSize := FixedOverhead + 1 + 1 + 1 // msgpack empty array for tags
	if len(erfBytes) < expectedMinSize {
		t.Errorf("encoded size too small: %d < %d", len(erfBytes), expectedMinSize)
	}
}

func generateTestEmbedding(dim int) []float32 {
	emb := make([]float32, dim)
	for i := 0; i < dim; i++ {
		emb[i] = float32(i%256) / 256.0
	}
	return emb
}

func TestTypeLabelRoundTrip_V1(t *testing.T) {
	eng := &Engram{
		ID:        newTestULID(),
		CreatedAt: time.Now().Truncate(time.Nanosecond),
		UpdatedAt: time.Now().Truncate(time.Nanosecond),
		Concept:   "TypeLabel test",
		Content:   "Testing TypeLabel round-trip through ERF v1",
		TypeLabel: "architectural_decision",
	}

	data, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if decoded.TypeLabel != "architectural_decision" {
		t.Errorf("TypeLabel = %q, want %q", decoded.TypeLabel, "architectural_decision")
	}
}

func TestTypeLabelRoundTrip_V2(t *testing.T) {
	eng := &Engram{
		ID:        newTestULID(),
		CreatedAt: time.Now().Truncate(time.Nanosecond),
		UpdatedAt: time.Now().Truncate(time.Nanosecond),
		Concept:   "TypeLabel v2 test",
		Content:   "Testing TypeLabel round-trip through ERF v2",
		TypeLabel: "coding_pattern",
	}

	data, err := EncodeV2(eng)
	if err != nil {
		t.Fatalf("EncodeV2: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if decoded.TypeLabel != "coding_pattern" {
		t.Errorf("TypeLabel = %q, want %q", decoded.TypeLabel, "coding_pattern")
	}
}

func TestTypeLabelEmpty_BackwardCompat(t *testing.T) {
	eng := &Engram{
		ID:        newTestULID(),
		CreatedAt: time.Now().Truncate(time.Nanosecond),
		UpdatedAt: time.Now().Truncate(time.Nanosecond),
		Concept:   "No TypeLabel",
		Content:   "Old engram without TypeLabel",
	}

	data, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if decoded.TypeLabel != "" {
		t.Errorf("TypeLabel = %q, want empty string for old records", decoded.TypeLabel)
	}
}

func TestEncodeV2_NoInlineAssocEmbed(t *testing.T) {
	eng := &Engram{
		Concept:   "test concept",
		Content:   "test content",
		CreatedBy: "tester",
		Tags:      []string{"a", "b"},
		// Associations and embedding are set but must NOT appear in v2 output
		Associations: []Association{{TargetID: [16]byte{1}, RelType: 1, Weight: 0.5}},
		Embedding:    []float32{0.1, 0.2, 0.3},
	}
	copy(eng.ID[:], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	eng.CreatedAt = time.Now()
	eng.UpdatedAt = time.Now()
	eng.Confidence = 0.9
	eng.State = 1

	data, err := EncodeV2(eng)
	if err != nil {
		t.Fatalf("EncodeV2: %v", err)
	}

	// Version byte must be 0x02
	if data[4] != Version2 {
		t.Errorf("version = 0x%02x, want 0x%02x", data[4], Version2)
	}

	// FlagHasEmbedding must NOT be set
	flags := data[5]
	if flags&FlagHasEmbedding != 0 {
		t.Error("FlagHasEmbedding should not be set in v2 record")
	}

	// Verify magic bytes
	if data[0] != 0x4D || data[1] != 0x55 || data[2] != 0x4E || data[3] != 0x4E {
		t.Error("magic bytes incorrect")
	}

	// Verify CRC16 at offset 6
	if !VerifyCRC16(data[0:8]) {
		t.Error("CRC16 verification failed")
	}

	// (Decode round-trip will be tested in Task 7 when Decode supports v2)
	// For now, just verify the basic structure is correct
}
