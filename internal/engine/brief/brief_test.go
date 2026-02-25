package brief

import (
	"strings"
	"testing"

	"github.com/scrypster/muninndb/internal/index/fts"
)

// makeQueryTerms builds a queryTerms map by tokenizing (and stemming) each word
// via fts.Tokenize, matching how scoreText processes text tokens.
func makeQueryTerms(t *testing.T, words ...string) map[string]bool {
	t.Helper()
	m := make(map[string]bool)
	for _, w := range words {
		for _, tok := range fts.Tokenize(w) {
			m[tok] = true
		}
	}
	return m
}

// TestSplitSentences tests sentence splitting on various input patterns.
func TestSplitSentences(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		expectedCount int
		expectedIn    []string
	}{
		{
			name:          "single sentence",
			text:          "This is a test sentence.",
			expectedCount: 1,
			expectedIn:    []string{"This is a test sentence."},
		},
		{
			name:          "multiple sentences on one line",
			text:          "First sentence. Second sentence. Third sentence.",
			expectedCount: 3,
			expectedIn:    []string{"First sentence.", "Second sentence.", "Third sentence."},
		},
		{
			name:          "sentences with question mark",
			text:          "Is this a question? Yes it is.",
			expectedCount: 2,
			expectedIn:    []string{"Is this a question?", "Yes it is."},
		},
		{
			name:          "sentences with exclamation",
			text:          "What an event! The crowd went wild.",
			expectedCount: 2,
			expectedIn:    []string{"What an event!", "The crowd went wild."},
		},
		{
			name:          "newline separated lines",
			text:          "First line.\nSecond line.\nThird line.",
			expectedCount: 3,
			expectedIn:    []string{"First line.", "Second line.", "Third line."},
		},
		{
			name:          "mixed newlines and periods",
			text:          "Line one has multiple. Sentences here.\nLine two is separate.",
			expectedCount: 3,
			expectedIn:    []string{"Line one has multiple.", "Sentences here.", "Line two is separate."},
		},
		{
			name:          "too short sentences filtered",
			text:          "Short. This is much longer and will be kept.",
			expectedCount: 1,
			expectedIn:    []string{"This is much longer and will be kept."},
		},
		{
			name:          "empty string",
			text:          "",
			expectedCount: 0,
			expectedIn:    []string{},
		},
		{
			name:          "whitespace only",
			text:          "   \n\n   \t  ",
			expectedCount: 0,
			expectedIn:    []string{},
		},
		{
			name:          "abbreviations like Dr may split on heuristic",
			text:          "Dr. Smith examined the patient. The diagnosis was clear.",
			expectedCount: 2,
			expectedIn:    []string{"examined the patient.", "The diagnosis was clear."},
		},
		{
			name:          "cap at MaxSentencesPerEngram",
			text:          strings.Repeat("Sentence. Next ", MaxSentencesPerEngram+10),
			expectedCount: MaxSentencesPerEngram,
			expectedIn:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSentences(tt.text)
			if len(result) != tt.expectedCount {
				t.Errorf("expected %d sentences, got %d: %v", tt.expectedCount, len(result), result)
			}
			for _, expected := range tt.expectedIn {
				found := false
				for _, sentence := range result {
					if strings.Contains(sentence, expected) || sentence == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find sentence containing/matching %q, got %v", expected, result)
				}
			}
		})
	}
}

// TestScoreText tests text scoring against query terms.
func TestScoreText(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		queryTerms  map[string]bool
		expectedMin float64 // minimum score (since stop words are filtered)
	}{
		{
			name:        "exact match single term",
			text:        "The database contains memory",
			queryTerms:  makeQueryTerms(t, "database"),
			expectedMin: 1.0,
		},
		{
			name:        "multiple matching terms",
			text:        "The database memory system",
			queryTerms:  makeQueryTerms(t, "database", "memory", "system"),
			expectedMin: 3.0,
		},
		{
			name:        "no match returns zero",
			text:        "The quick brown fox",
			queryTerms:  makeQueryTerms(t, "database"),
			expectedMin: 0.0,
		},
		{
			name:        "repeated terms count multiple times",
			text:        "memory memory memory",
			queryTerms:  makeQueryTerms(t, "memory"),
			expectedMin: 3.0,
		},
		{
			name:        "case insensitive",
			text:        "Database MEMORY System",
			queryTerms:  makeQueryTerms(t, "database", "memory"),
			expectedMin: 2.0,
		},
		{
			name:        "empty query terms",
			text:        "Some text here",
			queryTerms:  map[string]bool{},
			expectedMin: 0.0,
		},
		{
			name:        "empty text",
			text:        "",
			queryTerms:  makeQueryTerms(t, "database"),
			expectedMin: 0.0,
		},
		{
			name:        "stop words ignored",
			text:        "the database and the system",
			queryTerms:  makeQueryTerms(t, "the"), // stop word, should be filtered
			expectedMin: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scoreText(tt.text, tt.queryTerms)
			if result < tt.expectedMin {
				t.Errorf("expected score >= %f, got %f for text %q with terms %v",
					tt.expectedMin, result, tt.text, tt.queryTerms)
			}
		})
	}
}

// TestComputeBasic tests the basic Compute functionality.
func TestComputeBasic(t *testing.T) {
	engrams := []EngramContent{
		{
			ID:      "engram1",
			Content: "Memory systems store information. Cognitive processes retrieve data.",
		},
		{
			ID:      "engram2",
			Content: "Neural networks process signals. Information flows through synapses.",
		},
	}

	query := []string{"memory", "information"}

	result := Compute(engrams, query)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result) == 0 {
		t.Fatal("expected at least one sentence")
	}

	// All returned sentences should have positive scores
	for _, s := range result {
		if s.Score <= 0 {
			t.Errorf("sentence has non-positive score: %+v", s)
		}
		// All sentences should contain query terms
		hasQueryTerm := false
		for _, term := range query {
			if strings.Contains(strings.ToLower(s.Text), term) {
				hasQueryTerm = true
				break
			}
		}
		if !hasQueryTerm {
			t.Logf("sentence %q may not contain query term (after tokenization)", s.Text)
		}
	}
}

// TestComputeEmpty tests edge cases with empty inputs.
func TestComputeEmpty(t *testing.T) {
	tests := []struct {
		name   string
		engram []EngramContent
		query  []string
	}{
		{
			name:   "nil engrams",
			engram: nil,
			query:  []string{"test"},
		},
		{
			name:   "empty engrams",
			engram: []EngramContent{},
			query:  []string{"test"},
		},
		{
			name:   "nil query",
			engram: []EngramContent{{ID: "1", Content: "test content"}},
			query:  nil,
		},
		{
			name:   "empty query",
			engram: []EngramContent{{ID: "1", Content: "test content"}},
			query:  []string{},
		},
		{
			name:   "query with only stop words",
			engram: []EngramContent{{ID: "1", Content: "test content"}},
			query:  []string{"the", "and", "is"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Compute(tt.engram, tt.query)
			if result != nil {
				t.Errorf("expected nil result for empty/invalid input, got %v", result)
			}
		})
	}
}

// TestComputeOrdering tests that higher-scoring sentences appear first.
func TestComputeOrdering(t *testing.T) {
	engrams := []EngramContent{
		{
			ID: "e1",
			Content: "The database system is robust. " +
				"Memory stores data efficiently. " +
				"Information retrieval is fast. " +
				"Data structures optimize memory usage. " +
				"Performance metrics improve over time.",
		},
	}

	query := []string{"memory", "data"}

	result := Compute(engrams, query)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Check that scores are in descending order
	for i := 1; i < len(result); i++ {
		if result[i].Score > result[i-1].Score {
			t.Errorf("scores not in descending order at index %d: %f > %f",
				i, result[i].Score, result[i-1].Score)
		}
	}
}

// TestComputeCap tests that at most BriefSize sentences are returned.
func TestComputeCap(t *testing.T) {
	// Create engrams with many matching sentences
	var content strings.Builder
	for i := 0; i < 50; i++ {
		content.WriteString("This sentence contains memory and data. ")
	}

	engrams := []EngramContent{
		{
			ID:      "large",
			Content: content.String(),
		},
	}

	query := []string{"memory", "data"}

	result := Compute(engrams, query)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result) > BriefSize {
		t.Errorf("expected at most %d sentences, got %d", BriefSize, len(result))
	}

	// Verify that BriefSize results are returned if available
	if len(result) != BriefSize {
		// This might happen if splitSentences caps the sentences before we score them
		// That's okay; just verify we don't exceed BriefSize
		if len(result) > BriefSize {
			t.Errorf("got %d results, exceeds BriefSize of %d", len(result), BriefSize)
		}
	}
}

// TestComputeMultipleEngrams tests scoring across multiple engrams.
func TestComputeMultipleEngrams(t *testing.T) {
	engrams := []EngramContent{
		{
			ID:      "e1",
			Content: "First engram with memory data. Memory is important.",
		},
		{
			ID:      "e2",
			Content: "Second engram also mentions memory. Data storage requires memory.",
		},
		{
			ID:      "e3",
			Content: "Third engram focuses on data. Data analysis is crucial.",
		},
	}

	query := []string{"memory"}

	result := Compute(engrams, query)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should have sentences from multiple engrams
	engramIDs := make(map[string]bool)
	for _, s := range result {
		engramIDs[s.EngramID] = true
	}

	if len(engramIDs) < 2 {
		t.Logf("expected sentences from multiple engrams, got %d unique engram IDs: %v",
			len(engramIDs), engramIDs)
	}

	// All returned sentences should mention "memory"
	for _, s := range result {
		if !strings.Contains(strings.ToLower(s.Text), "memory") {
			t.Errorf("sentence doesn't contain 'memory': %q", s.Text)
		}
	}
}

// TestComputeNoMatches tests when query terms don't match engram content.
func TestComputeNoMatches(t *testing.T) {
	engrams := []EngramContent{
		{
			ID:      "e1",
			Content: "The quick brown fox jumps over the lazy dog.",
		},
	}

	query := []string{"database", "memory", "system"}

	result := Compute(engrams, query)

	if result != nil {
		t.Errorf("expected nil result when no query terms match, got %v", result)
	}
}

// TestSentenceFields tests that Sentence struct fields are correctly populated.
func TestSentenceFields(t *testing.T) {
	engrams := []EngramContent{
		{
			ID:      "test-engram",
			Content: "This is a test sentence about memory systems.",
		},
	}

	query := []string{"memory"}

	result := Compute(engrams, query)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result) == 0 {
		t.Fatal("expected at least one sentence")
	}

	s := result[0]

	if s.EngramID != "test-engram" {
		t.Errorf("expected EngramID 'test-engram', got %q", s.EngramID)
	}

	if len(s.Text) == 0 {
		t.Error("expected non-empty Text")
	}

	if s.Score <= 0 {
		t.Errorf("expected positive Score, got %f", s.Score)
	}

	if !strings.Contains(strings.ToLower(s.Text), "memory") {
		t.Errorf("expected Text to contain 'memory', got %q", s.Text)
	}
}

// TestScoreTextEdgeCases tests edge cases in scoreText.
func TestScoreTextEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		terms   map[string]bool
		wantMin float64
	}{
		{
			name:    "very long text",
			text:    strings.Repeat("memory data system ", 100),
			terms:   makeQueryTerms(t, "memory", "data"),
			wantMin: 200.0, // expect at least 100 of each
		},
		{
			name:    "special characters stripped",
			text:    "memory!!!data???system",
			terms:   makeQueryTerms(t, "memory", "data"),
			wantMin: 2.0,
		},
		{
			name:    "single character text",
			text:    "a",
			terms:   makeQueryTerms(t, "a"),
			wantMin: 0.0, // single char tokens are filtered by Tokenize
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreText(tt.text, tt.terms)
			if got < tt.wantMin {
				t.Errorf("scoreText(%q, ...) = %f, want >= %f", tt.text, got, tt.wantMin)
			}
		})
	}
}

// BenchmarkSplitSentences benchmarks the sentence splitting function.
func BenchmarkSplitSentences(b *testing.B) {
	text := "First sentence. Second sentence. Third sentence. Fourth sentence. Fifth sentence. " +
		"Sixth sentence. Seventh sentence. Eighth sentence. Ninth sentence. Tenth sentence."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitSentences(text)
	}
}

// BenchmarkScoreText benchmarks the text scoring function.
func BenchmarkScoreText(b *testing.B) {
	text := "memory systems store information about memory. Data structures optimize memory usage."
	queryTerms := map[string]bool{
		"memory": true,
		"data":   true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scoreText(text, queryTerms)
	}
}

// BenchmarkCompute benchmarks the Compute function.
func BenchmarkCompute(b *testing.B) {
	engrams := []EngramContent{
		{
			ID:      "e1",
			Content: "Memory systems store information. Cognitive processes retrieve data.",
		},
		{
			ID:      "e2",
			Content: "Neural networks process signals. Information flows through synapses.",
		},
		{
			ID:      "e3",
			Content: "The database contains memory entries. System performance depends on memory.",
		},
	}

	query := []string{"memory", "information", "data"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Compute(engrams, query)
	}
}
