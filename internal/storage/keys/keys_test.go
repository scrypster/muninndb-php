package keys

import (
	"testing"
)

func TestKeyPrefixesAreUnique(t *testing.T) {
	var ws [8]byte
	var id [16]byte
	var trigram [3]byte

	// One representative per prefix byte. Keys that share a prefix intentionally
	// (e.g. ProvenanceKey and ProvenanceSuffixKey both use 0x16) are represented
	// by only one entry.
	prefixKeys := []struct {
		name string
		key  []byte
	}{
		{"EngramKey", EngramKey(ws, id)},
		{"MetaKey", MetaKey(ws, id)},
		{"AssocFwdKey", AssocFwdKey(ws, id, 0.5, id)},
		{"AssocRevKey", AssocRevKey(ws, id, 0.5, id)},
		{"FTSPostingKey", FTSPostingKey(ws, "test", id)},
		{"TrigramKey", TrigramKey(ws, trigram, id)},
		{"HNSWNodeKey", HNSWNodeKey(ws, id, 0)},
		{"FTSStatsKey", FTSStatsKey(ws)},
		{"TermStatsKey", TermStatsKey(ws, "test")},
		{"ContradictionKey", ContradictionKey(ws, 0, 0, id)},
		{"StateIndexKey", StateIndexKey(ws, 0, id)},
		{"TagIndexKey", TagIndexKey(ws, 0, id)},
		{"CreatorIndexKey", CreatorIndexKey(ws, 0, id)},
		{"VaultMetaKey", VaultMetaKey(ws)},
		{"VaultNameIndexKey", VaultNameIndexKey("test")},
		{"RelevanceBucketKey", RelevanceBucketKey(ws, 0.5, id)},
		{"DigestFlagsKey", DigestFlagsKey(id)},
		{"CoherenceKey", CoherenceKey(ws)},
		{"VaultWeightsKey", VaultWeightsKey(ws)},
		{"AssocWeightIndexKey", AssocWeightIndexKey(ws, id, id)},
		{"VaultCountKey", VaultCountKey(ws)},
		{"ProvenanceKey", ProvenanceKey(ws, id)},
		{"EpisodeKey", EpisodeKey(ws, id)},
		{"BucketMigrationKey", BucketMigrationKey(ws)},
		{"EmbeddingKey", EmbeddingKey(ws, id)},
		{"TransitionKey", TransitionKey(ws, id, id)},
	}

	seen := make(map[byte]string)
	for _, pk := range prefixKeys {
		if len(pk.key) == 0 {
			t.Errorf("%s: key is empty", pk.name)
			continue
		}
		prefix := pk.key[0]
		if prev, exists := seen[prefix]; exists {
			t.Errorf("prefix 0x%02X used by both %s and %s", prefix, prev, pk.name)
		}
		seen[prefix] = pk.name
	}
}

func TestTransitionKey(t *testing.T) {
	var ws [8]byte
	var src, dst [16]byte
	ws[0] = 0xAA
	src[0] = 0xBB
	dst[0] = 0xCC

	k := TransitionKey(ws, src, dst)

	if len(k) != 41 {
		t.Fatalf("TransitionKey len = %d, want 41", len(k))
	}
	if k[0] != 0x1C {
		t.Errorf("TransitionKey prefix = 0x%02x, want 0x1C", k[0])
	}
	if k[1] != 0xAA {
		t.Errorf("TransitionKey ws[0] = 0x%02x, want 0xAA", k[1])
	}
	if k[9] != 0xBB {
		t.Errorf("TransitionKey src[0] = 0x%02x, want 0xBB", k[9])
	}
	if k[25] != 0xCC {
		t.Errorf("TransitionKey dst[0] = 0x%02x, want 0xCC", k[25])
	}

	// TransitionPrefixForSrc must be a proper prefix of TransitionKey
	pfx := TransitionPrefixForSrc(ws, src)
	if len(pfx) != 25 {
		t.Fatalf("TransitionPrefixForSrc len = %d, want 25", len(pfx))
	}
	for i := 0; i < len(pfx); i++ {
		if pfx[i] != k[i] {
			t.Errorf("TransitionPrefixForSrc[%d] = 0x%02x, want 0x%02x (from TransitionKey)", i, pfx[i], k[i])
		}
	}
}

func TestEmbeddingKey(t *testing.T) {
	var ws [8]byte
	var id [16]byte
	ws[0] = 0x01
	id[0] = 0x02
	k := EmbeddingKey(ws, id)
	if len(k) != 25 {
		t.Fatalf("EmbeddingKey len = %d, want 25", len(k))
	}
	if k[0] != 0x18 {
		t.Errorf("EmbeddingKey prefix = 0x%02x, want 0x18", k[0])
	}
	if k[1] != 0x01 {
		t.Errorf("EmbeddingKey ws[0] = 0x%02x, want 0x01", k[1])
	}
	if k[9] != 0x02 {
		t.Errorf("EmbeddingKey id[0] = 0x%02x, want 0x02", k[9])
	}
}
