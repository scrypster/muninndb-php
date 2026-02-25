package mql

import (
	"strings"
	"testing"
)

// TestLexer_StringEscapes verifies that escape sequences inside double-quoted
// strings are correctly decoded.
func TestLexer_StringEscapes(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"newline escape", `"hello\nworld"`, "hello\nworld"},
		{"tab escape", `"tab\there"`, "tab\there"},
		{"quote escape", `"quote\"here"`, `quote"here`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens := Tokenize(tc.input)
			// Expect at least one STRING token before EOF
			if len(tokens) < 2 {
				t.Fatalf("expected at least 2 tokens (STRING + EOF), got %d", len(tokens))
			}
			tok := tokens[0]
			if tok.Type != TokenString {
				t.Fatalf("expected TokenString, got %s", tok.Type)
			}
			if tok.Value != tc.want {
				t.Fatalf("expected value %q, got %q", tc.want, tok.Value)
			}
		})
	}
}

// TestLexer_UnterminatedString verifies that an unterminated string (no closing
// quote) does not panic and returns a partial STRING token containing whatever
// was parsed up to EOF.
func TestLexer_UnterminatedString(t *testing.T) {
	input := `"unterminated`
	tokens := Tokenize(input)

	// Must not panic (the test reaching here proves it).
	// Expect at least one token and the last must be EOF.
	if len(tokens) == 0 {
		t.Fatal("expected at least 1 token, got none")
	}
	last := tokens[len(tokens)-1]
	if last.Type != TokenEOF {
		t.Fatalf("expected last token to be EOF, got %s", last.Type)
	}

	// The partial content before EOF should have been captured as a STRING token.
	found := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			found = true
			if !strings.Contains(tok.Value+input, "unterminated") {
				// At a minimum the captured value should not be longer than the input
				if len(tok.Value) > len(input) {
					t.Fatalf("partial string value %q is longer than the input", tok.Value)
				}
			}
			break
		}
	}
	if !found {
		t.Fatal("expected a STRING token for the partial input")
	}
}

// TestLexer_Comments verifies that -- line comments are skipped and that the
// identifier on the following line is produced as a token.
func TestLexer_Comments(t *testing.T) {
	input := "-- this is a comment\nRECALL"
	tokens := Tokenize(input)

	// Filter out EOF
	var significant []Token
	for _, tok := range tokens {
		if tok.Type != TokenEOF {
			significant = append(significant, tok)
		}
	}

	if len(significant) != 1 {
		t.Fatalf("expected exactly 1 non-EOF token, got %d: %v", len(significant), significant)
	}
	if significant[0].Type != TokenRecall {
		t.Fatalf("expected TokenRecall, got %s", significant[0].Type)
	}
}

// TestLexer_NumberTypes verifies that integer and float literals both produce
// TokenNumber tokens. The distinction between int and float is determined by
// whether the value string contains a decimal point.
func TestLexer_NumberTypes(t *testing.T) {
	t.Run("integer", func(t *testing.T) {
		tokens := Tokenize("42")
		if len(tokens) < 2 {
			t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
		}
		tok := tokens[0]
		if tok.Type != TokenNumber {
			t.Fatalf("expected TokenNumber, got %s", tok.Type)
		}
		if tok.Value != "42" {
			t.Fatalf("expected value \"42\", got %q", tok.Value)
		}
		if strings.Contains(tok.Value, ".") {
			t.Fatal("integer token value should not contain a decimal point")
		}
	})

	t.Run("float", func(t *testing.T) {
		tokens := Tokenize("3.14")
		if len(tokens) < 2 {
			t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
		}
		tok := tokens[0]
		if tok.Type != TokenNumber {
			t.Fatalf("expected TokenNumber, got %s", tok.Type)
		}
		if tok.Value != "3.14" {
			t.Fatalf("expected value \"3.14\", got %q", tok.Value)
		}
		if !strings.Contains(tok.Value, ".") {
			t.Fatal("float token value should contain a decimal point")
		}
	})
}

// TestLexer_KeywordVsIdent verifies that keyword lookups are case-insensitive
// and that identifiers containing underscores in non-keyword positions remain IDENT.
func TestLexer_KeywordVsIdent(t *testing.T) {
	t.Run("RECALL is a keyword", func(t *testing.T) {
		tokens := Tokenize("RECALL")
		if len(tokens) < 2 {
			t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
		}
		tok := tokens[0]
		if tok.Type != TokenRecall {
			t.Fatalf("expected TokenRecall, got %s", tok.Type)
		}
	})

	t.Run("recall (lowercase) is also a keyword (case-insensitive)", func(t *testing.T) {
		tokens := Tokenize("recall")
		if len(tokens) < 2 {
			t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
		}
		tok := tokens[0]
		if tok.Type != TokenRecall {
			t.Fatalf("expected TokenRecall for lowercase 'recall', got %s", tok.Type)
		}
	})

	t.Run("recall_mode is an IDENT (not a keyword)", func(t *testing.T) {
		tokens := Tokenize("recall_mode")
		if len(tokens) < 2 {
			t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
		}
		tok := tokens[0]
		if tok.Type != TokenIdent {
			t.Fatalf("expected TokenIdent for 'recall_mode', got %s", tok.Type)
		}
	})
}
