package erf

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func makeTestEngram() *Engram {
	return &Engram{
		ID:          newTestULID(),
		CreatedAt:   time.Now().Truncate(time.Nanosecond),
		UpdatedAt:   time.Now().Truncate(time.Nanosecond),
		LastAccess:  time.Now().Truncate(time.Nanosecond),
		Confidence:  0.85,
		Relevance:   0.72,
		Stability:   1.5,
		AccessCount: 17,
		State:       0x01,
		EmbedDim:    0x02,
		Concept:     "neural pathway consolidation",
		CreatedBy:   "cortex",
		Content:     "Long-term memory formation involves synaptic strengthening through repeated activation patterns.",
		Tags:        []string{"memory", "neuroscience", "synaptic"},
		Embedding:   generateTestEmbedding(384),
		Associations: []Association{
			{
				TargetID:      newTestULID(),
				RelType:       0x0001,
				Weight:        0.8,
				Confidence:    0.9,
				CreatedAt:     time.Now().Truncate(time.Nanosecond),
				LastActivated: 1700000000,
			},
		},
	}
}

func mustEncode(t *testing.T, eng *Engram) []byte {
	t.Helper()
	data, err := Encode(eng)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	return data
}

func cloneBytes(data []byte) []byte {
	c := make([]byte, len(data))
	copy(c, data)
	return c
}

func TestDecode_TruncatedRecord(t *testing.T) {
	data := mustEncode(t, makeTestEngram())

	cases := []struct {
		name string
		size int
	}{
		{"empty header", 4},
		{"partial metadata", 50},
		{"partial offset table", 120},
		{"one byte short", FixedOverhead - 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(data[:tc.size])
			if err == nil {
				t.Errorf("expected error for buffer truncated to %d bytes", tc.size)
			}
		})
	}
}

func TestDecode_BadMagic(t *testing.T) {
	data := cloneBytes(mustEncode(t, makeTestEngram()))

	data[0] = 0xFF
	data[1] = 0x00
	data[2] = 0xDE
	data[3] = 0xAD

	_, err := Decode(data)
	if err == nil {
		t.Error("expected error for corrupted magic bytes")
	}
}

func TestDecode_BadCRC16(t *testing.T) {
	data := cloneBytes(mustEncode(t, makeTestEngram()))

	// Flip bits in the flags byte (covered by CRC16) without updating CRC16
	data[5] ^= 0xFF

	_, err := Decode(data)
	if err == nil {
		t.Error("expected error for CRC16 mismatch")
	}
}

func TestDecode_BadCRC32(t *testing.T) {
	data := cloneBytes(mustEncode(t, makeTestEngram()))

	// Corrupt a metadata byte outside CRC16 coverage (bytes 0-5) but inside CRC32 coverage
	data[OffsetID+2] ^= 0xFF

	_, err := Decode(data)
	if err == nil {
		t.Error("expected error for CRC32 mismatch")
	}
}

func TestDecode_OffsetPastEnd(t *testing.T) {
	data := cloneBytes(mustEncode(t, makeTestEngram()))

	// Offset table is at byte 108+, outside CRC16 (bytes 0-5).
	// Bounds check fires before CRC32 check.
	binary.BigEndian.PutUint32(data[OffsetConceptOff:OffsetConceptOff+4], uint32(len(data)+1000))

	_, err := Decode(data)
	if err == nil {
		t.Error("expected error for concept offset past end of buffer")
	}
}

func TestDecode_ContentLengthOverflow(t *testing.T) {
	data := cloneBytes(mustEncode(t, makeTestEngram()))

	binary.BigEndian.PutUint32(data[OffsetContentLen:OffsetContentLen+4], uint32(len(data)*10))

	_, err := Decode(data)
	if err == nil {
		t.Error("expected error for content length exceeding buffer size")
	}
}

func TestDecode_ConceptLengthExceedsMax(t *testing.T) {
	data := cloneBytes(mustEncode(t, makeTestEngram()))

	// Max uint16 will exceed the buffer bounds for any reasonably sized record
	binary.BigEndian.PutUint16(data[OffsetConceptLen:OffsetConceptLen+2], 0xFFFF)

	_, err := Decode(data)
	if err == nil {
		t.Error("expected error for concept length exceeding buffer bounds")
	}
}

func TestDecode_ZeroLengthBuffer(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		_, err := Decode([]byte{})
		if err == nil {
			t.Error("expected error for zero-length buffer")
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		_, err := Decode(nil)
		if err == nil {
			t.Error("expected error for nil buffer")
		}
	})
}

func TestDecode_CorruptedZstdContent(t *testing.T) {
	eng := makeTestEngram()
	eng.Content = strings.Repeat("Memory consolidation during sleep involves replay of neural patterns. ", 20)

	data := cloneBytes(mustEncode(t, eng))

	if data[5]&FlagContentCompressed == 0 {
		t.Fatal("expected FlagContentCompressed to be set for large content")
	}

	contentOff := binary.BigEndian.Uint32(data[OffsetContentOff : OffsetContentOff+4])
	contentLen := binary.BigEndian.Uint32(data[OffsetContentLen : OffsetContentLen+4])

	mid := contentOff + contentLen/2
	for i := mid; i < mid+8 && i < contentOff+contentLen; i++ {
		data[i] = 0x00
	}

	_, err := Decode(data)
	if err == nil {
		t.Error("expected error for corrupted zstd-compressed content")
	}
}

func TestDecode_CorruptedQuantizedEmbedding(t *testing.T) {
	eng := makeTestEngram()
	data := cloneBytes(mustEncode(t, eng))

	if data[5]&FlagHasEmbedding == 0 {
		t.Fatal("expected FlagHasEmbedding to be set")
	}
	if data[5]&FlagEmbedQuantized == 0 {
		t.Fatal("expected FlagEmbedQuantized to be set")
	}

	embedOff := binary.BigEndian.Uint32(data[OffsetEmbedOff : OffsetEmbedOff+4])

	// Write NaN into both quantization params (scale + zero_point)
	binary.BigEndian.PutUint32(data[embedOff:embedOff+4], 0x7FC00000)
	binary.BigEndian.PutUint32(data[embedOff+4:embedOff+8], 0x7FC00000)

	_, err := Decode(data)
	if err == nil {
		t.Error("expected error for corrupted quantized embedding params")
	}
}

func TestDecode_AssocCountMismatch(t *testing.T) {
	eng := makeTestEngram()
	data := cloneBytes(mustEncode(t, eng))

	// Set metadata AssocCount far above actual association data
	binary.BigEndian.PutUint16(data[OffsetAssocCount:OffsetAssocCount+2], 999)

	_, err := Decode(data)
	if err == nil {
		t.Error("expected error for association count mismatch")
	}
}

func TestDecode_ValidAfterCorruptionFix(t *testing.T) {
	eng := makeTestEngram()
	data := cloneBytes(mustEncode(t, eng))

	saved := data[OffsetID+5]
	data[OffsetID+5] ^= 0xFF

	t.Run("corruption detected", func(t *testing.T) {
		_, err := Decode(data)
		if err == nil {
			t.Fatal("expected error for corrupted data")
		}
	})

	data[OffsetID+5] = saved

	t.Run("round-trip after fix", func(t *testing.T) {
		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("expected successful decode after fixing corruption: %v", err)
		}
		if decoded.Concept != eng.Concept {
			t.Errorf("Concept mismatch: got %q, want %q", decoded.Concept, eng.Concept)
		}
		if decoded.Content != eng.Content {
			t.Errorf("Content mismatch: got %q, want %q", decoded.Content, eng.Content)
		}
		if len(decoded.Embedding) != len(eng.Embedding) {
			t.Errorf("Embedding length mismatch: got %d, want %d", len(decoded.Embedding), len(eng.Embedding))
		}
		if len(decoded.Associations) != len(eng.Associations) {
			t.Errorf("Associations count mismatch: got %d, want %d", len(decoded.Associations), len(eng.Associations))
		}
	})
}
