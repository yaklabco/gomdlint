package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/parser/goldmark"
)

func TestMaxLineLengthRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "short lines",
			input:     "Hello world\nSecond line\n",
			wantDiags: 0,
		},
		{
			name:      "line at max length",
			input:     strings.Repeat("a", 120) + "\n",
			wantDiags: 0,
		},
		{
			name:      "line exceeds max length",
			input:     strings.Repeat("a", 121) + "\n",
			wantDiags: 1,
		},
		{
			name:      "multiple long lines",
			input:     strings.Repeat("a", 130) + "\n" + strings.Repeat("b", 125) + "\n",
			wantDiags: 2,
		},
		{
			name:      "custom max length",
			input:     strings.Repeat("a", 81) + "\n",
			wantDiags: 1,
			config:    map[string]any{"max": 80},
		},
		{
			name:      "line with URL ignored by default",
			input:     "Check out this link: https://example.com/very/long/path/that/exceeds/the/maximum/line/length/limit/significantly/more/content\n",
			wantDiags: 0,
		},
		{
			name:      "line with URL not ignored",
			input:     "Check out this link: https://example.com/very/long/path/that/exceeds/the/maximum/line/length/limit/significantly/more/content\n",
			wantDiags: 1,
			config:    map[string]any{"ignore_urls": false},
		},
		{
			name:      "code block ignored by default",
			input:     "```\n" + strings.Repeat("a", 150) + "\n```\n",
			wantDiags: 0,
		},
		{
			name:      "code block not ignored",
			input:     "```\n" + strings.Repeat("a", 150) + "\n```\n",
			wantDiags: 1,
			config:    map[string]any{"ignore_code_blocks": false},
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "blank lines",
			input:     "\n\n\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewMaxLineLengthRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Verify diagnostics have correct messages.
			for _, d := range diags {
				assert.Contains(t, d.Message, "exceeds maximum")
			}
		})
	}
}

func TestMaxLineLengthRule_Metadata(t *testing.T) {
	rule := NewMaxLineLengthRule()

	assert.Equal(t, "MD013", rule.ID())
	assert.Equal(t, "line-length", rule.Name())
	assert.Contains(t, rule.Tags(), "line_length")
	assert.True(t, rule.CanFix()) // Now auto-fixable via line wrapping
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestMaxLineLengthRule_DiagnosticPosition(t *testing.T) {
	// Test that the diagnostic position is correct.
	input := strings.Repeat("a", 130) + "\n"

	parser := goldmark.New(string(config.FlavorCommonMark))
	snapshot, err := parser.Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)

	rule := NewMaxLineLengthRule()
	cfg := config.NewConfig()
	ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

	diags, err := rule.Apply(ruleCtx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	// Position should start at column 121 (the first character past the limit).
	assert.Equal(t, 1, diags[0].StartLine)
	assert.Equal(t, 121, diags[0].StartColumn)
	assert.Equal(t, 1, diags[0].EndLine)
	assert.Equal(t, 130, diags[0].EndColumn)
}

func TestMaxLineLengthRule_Autofix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		wantFix  bool
		wantText string // expected text after applying fix
	}{
		{
			name:     "paragraph wraps at word boundary",
			input:    "This is a very long line that exceeds the maximum allowed length for a line in this markdown file.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "This is a very long line that exceeds the maximum\nallowed length for a line in this markdown file.\n",
		},
		{
			name:     "no space before limit - no fix",
			input:    strings.Repeat("a", 60) + "\n",
			maxLen:   50,
			wantFix:  false,
			wantText: strings.Repeat("a", 60) + "\n",
		},
		{
			name:     "heading - skipped",
			input:    "# This is a very long heading that exceeds the maximum allowed length for a line\n",
			maxLen:   50,
			wantFix:  false,
			wantText: "# This is a very long heading that exceeds the maximum allowed length for a line\n",
		},
		{
			name:     "table line - skipped",
			input:    "| Column 1 | Column 2 | Column 3 | Column 4 | Column 5 | Column 6 |\n",
			maxLen:   50,
			wantFix:  false,
			wantText: "| Column 1 | Column 2 | Column 3 | Column 4 | Column 5 | Column 6 |\n",
		},
		{
			name:     "list item wraps with indent",
			input:    "- This is a list item with a very long description that exceeds the line limit.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "- This is a list item with a very long description\n  that exceeds the line limit.\n",
		},
		{
			name:     "numbered list wraps with indent",
			input:    "1. This is a numbered list item that has quite a long description here.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "1. This is a numbered list item that has quite a\n   long description here.\n",
		},
		{
			name:     "blockquote wraps preserving prefix",
			input:    "> This is a blockquote that is too long and needs to be wrapped at a word boundary.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "> This is a blockquote that is too long and needs\n> to be wrapped at a word boundary.\n",
		},
		{
			name:     "nested blockquote and list",
			input:    "> - This is a nested list item in a blockquote that exceeds the limit.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "> - This is a nested list item in a blockquote\n>   that exceeds the limit.\n",
		},
		{
			name:     "indented content wraps with same indent",
			input:    "  This is indented content that is very long and needs wrapping.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "  This is indented content that is very long and\n  needs wrapping.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewMaxLineLengthRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{Options: map[string]any{"max": tt.maxLen}}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)

			if len(diags) == 0 {
				// No diagnostics means no fix needed (line is within limit)
				assert.Equal(t, tt.wantText, tt.input, "input should match expected when no diag")
				return
			}

			hasFix := len(diags[0].FixEdits) > 0
			assert.Equal(t, tt.wantFix, hasFix, "fix presence mismatch")

			if hasFix {
				// Apply the fix and verify result
				result := applyFixEdits([]byte(tt.input), diags[0].FixEdits)
				assert.Equal(t, tt.wantText, string(result), "fixed text mismatch")
			}
		})
	}
}

func TestMaxLineLengthRule_HelperFunctions(t *testing.T) {
	t.Run("linePrefix for plain text", func(t *testing.T) {
		prefix, start := linePrefix("Hello world")
		assert.Empty(t, prefix)
		assert.Equal(t, 0, start)
	})

	t.Run("linePrefix for indented text", func(t *testing.T) {
		prefix, start := linePrefix("  Hello world")
		assert.Equal(t, "  ", prefix)
		assert.Equal(t, 2, start)
	})

	t.Run("linePrefix for list item", func(t *testing.T) {
		prefix, start := linePrefix("- List item")
		assert.Equal(t, "  ", prefix)
		assert.Equal(t, 2, start)
	})

	t.Run("linePrefix for numbered list", func(t *testing.T) {
		prefix, start := linePrefix("1. Numbered item")
		assert.Equal(t, "   ", prefix)
		assert.Equal(t, 3, start)
	})

	t.Run("linePrefix for blockquote", func(t *testing.T) {
		prefix, start := linePrefix("> Quoted text")
		assert.Equal(t, "> ", prefix)
		assert.Equal(t, 2, start)
	})

	t.Run("linePrefix for nested blockquote and list", func(t *testing.T) {
		prefix, start := linePrefix("> - Nested item")
		assert.Equal(t, ">   ", prefix)
		assert.Equal(t, 4, start)
	})

	t.Run("findWrapPoint finds last space before limit", func(t *testing.T) {
		line := "hello world test"
		// With maxLen=12, we want to find the space at position 11 (before "test")
		wp := findWrapPoint(line, 12)
		assert.Equal(t, 11, wp) // space before "test"
	})

	t.Run("findWrapPoint returns -1 for short line", func(t *testing.T) {
		line := "hello"
		wp := findWrapPoint(line, 10)
		assert.Equal(t, -1, wp)
	})

	t.Run("findWrapPoint returns -1 for no spaces", func(t *testing.T) {
		line := strings.Repeat("a", 20)
		wp := findWrapPoint(line, 10)
		assert.Equal(t, -1, wp)
	})

	t.Run("isHeading identifies headings", func(t *testing.T) {
		assert.True(t, isHeading("# Heading"))
		assert.True(t, isHeading("## Heading"))
		assert.True(t, isHeading("  # Indented heading"))
		assert.False(t, isHeading("Not a heading"))
		assert.False(t, isHeading(""))
	})

	t.Run("isTableLine identifies tables", func(t *testing.T) {
		assert.True(t, isTableLine("| cell |"))
		assert.True(t, isTableLine("  | indented |"))
		assert.False(t, isTableLine("Not a table"))
		assert.False(t, isTableLine(""))
	})
}

// applyFixEdits applies fix edits to content (test helper).
func applyFixEdits(content []byte, edits []fix.TextEdit) []byte {
	return fix.ApplyEdits(content, edits)
}
