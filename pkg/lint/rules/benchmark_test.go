package rules

import (
	"context"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/parser/goldmark"
)

// Benchmark line-length detection and fixing.
func BenchmarkLineLengthRule(b *testing.B) {
	content := []byte(`# Test Document

This is a very long line that exceeds the maximum line length and should trigger the line-length rule for demonstration purposes.

Another long line that contains many words to ensure it properly exceeds the configured maximum character limit and tests the performance.

Short line.

Yet another very long line that contains lots of text to exceed the standard 80 character limit that is commonly used in markdown linting.`)

	parser := goldmark.New("gfm")
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		snapshot, err := parser.Parse(ctx, "test.md", content)
		if err != nil || snapshot == nil {
			b.Fail()
		}
	}
}

// Benchmark line-length rule with fixes.
func BenchmarkLineLengthRuleWithFix(b *testing.B) {
	content := []byte(`# Test Document

This is a very long line that exceeds the maximum line length and should trigger the line-length rule for demonstration purposes and also test the fix generation.

Another long line that contains many words to ensure it properly exceeds the configured maximum character limit and tests the fix performance comprehensively.`)

	parser := goldmark.New("gfm")
	registry := lint.NewRegistry()
	RegisterAll(registry)
	engine := &lint.Engine{
		Parser:   parser,
		Registry: registry,
	}

	cfg := config.NewConfig()
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		result, err := engine.LintFile(ctx, "test.md", content, cfg)
		if err != nil || result == nil {
			b.Fail()
		}
	}
}
