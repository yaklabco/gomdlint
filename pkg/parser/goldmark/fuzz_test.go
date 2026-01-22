package goldmark

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// FuzzTokenize fuzzes the tokenizer with random input.
func FuzzTokenize(f *testing.F) {
	// Add seed corpus.
	seeds := []string{
		"",
		"Hello, world!",
		"# Heading",
		"## Heading 2",
		"- list item",
		"1. ordered item",
		"> blockquote",
		"```\ncode\n```",
		"```go\nfunc main() {}\n```",
		"*emphasis*",
		"**strong**",
		"`code`",
		"[link](url)",
		"![image](src)",
		"---",
		"***",
		"___",
		"\\*escaped\\*",
		"<div>html</div>",
		"Title\n=====",
		"line1\nline2",
		"line1\r\nline2",
		"# Heading\n\nParagraph with *emphasis* and **strong**.\n\n- item 1\n- item 2\n",
	}

	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Tokenize should never panic.
		tokens := Tokenize(data)

		// If we have content, we should have tokens.
		if len(data) > 0 && len(tokens) == 0 {
			t.Error("expected tokens for non-empty input")
		}

		// Tokens should be valid (contiguous and covering).
		if len(data) > 0 && !mdast.ValidateTokens(tokens, len(data)) {
			t.Errorf("tokens are not valid for input of length %d", len(data))
		}
	})
}

// FuzzParse fuzzes the full parser with random input.
func FuzzParse(f *testing.F) {
	// Add seed corpus.
	seeds := []string{
		"",
		"Hello, world!",
		"# Heading",
		"- list\n- items",
		"```\ncode\n```",
		"*emphasis* and **strong**",
		"[link](url) and ![image](src)",
		"# Title\n\nParagraph.\n\n- item\n\n> quote\n",
	}

	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := context.Background()
		p := New(FlavorCommonMark)

		// Parse should never panic.
		snapshot, err := p.Parse(ctx, "fuzz.md", data)

		// Error is acceptable for malformed input, but panic is not.
		if err != nil {
			return
		}

		// If parsing succeeded, validate the snapshot.
		if snapshot == nil {
			t.Error("expected non-nil snapshot when err is nil")
			return
		}

		// Content should match.
		if !bytes.Equal(snapshot.Content, data) {
			t.Error("content mismatch")
		}

		// Tokens should be valid.
		if len(data) > 0 && !mdast.ValidateTokens(snapshot.Tokens, len(data)) {
			t.Error("tokens are not valid")
		}

		// Root should exist and be a document.
		if snapshot.Root == nil {
			t.Error("expected non-nil root")
			return
		}

		if snapshot.Root.Kind != mdast.NodeDocument {
			t.Errorf("root kind = %v, want NodeDocument", snapshot.Root.Kind)
		}

		// All nodes should have File reference set.
		err = mdast.Walk(snapshot.Root, func(n *mdast.Node) error {
			if n.File != snapshot {
				t.Error("node has incorrect File reference")
			}
			return nil
		})
		if err != nil {
			t.Errorf("walk error: %v", err)
		}
	})
}

// FuzzParseGFM fuzzes the GFM parser with random input.
func FuzzParseGFM(f *testing.F) {
	// Add seed corpus with GFM-specific constructs.
	seeds := []string{
		"",
		"- [x] task 1\n- [ ] task 2",
		"| a | b |\n|---|---|\n| 1 | 2 |",
		"~~strikethrough~~",
		"https://example.com",
		"# GFM\n\n- [x] done\n\n| h |\n|---|\n| c |",
	}

	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := context.Background()
		p := New(FlavorGFM)

		// Parse should never panic.
		snapshot, err := p.Parse(ctx, "fuzz.md", data)

		if err != nil {
			return
		}

		if snapshot == nil {
			t.Error("expected non-nil snapshot when err is nil")
			return
		}

		// Basic validation.
		if snapshot.Root == nil {
			t.Error("expected non-nil root")
		}
	})
}

// FuzzParseDeterministic verifies that parsing is deterministic.
func FuzzParseDeterministic(f *testing.F) {
	seeds := []string{
		"# Hello",
		"*emphasis*",
		"- list",
	}

	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := context.Background()
		p := New(FlavorCommonMark)

		// Parse twice.
		s1, err1 := p.Parse(ctx, "test.md", data)
		s2, err2 := p.Parse(ctx, "test.md", data)

		// Both should succeed or both should fail.
		if (err1 == nil) != (err2 == nil) {
			t.Error("parsing should be deterministic")
			return
		}

		if err1 != nil {
			return
		}

		// Token counts should match.
		if len(s1.Tokens) != len(s2.Tokens) {
			t.Errorf("token count mismatch: %d vs %d", len(s1.Tokens), len(s2.Tokens))
		}

		// Node counts should match.
		count1 := countNodes(s1.Root)
		count2 := countNodes(s2.Root)
		if count1 != count2 {
			t.Errorf("node count mismatch: %d vs %d", count1, count2)
		}
	})
}
