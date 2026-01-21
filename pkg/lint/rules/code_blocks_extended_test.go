package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/parser/goldmark"
)

func TestBlanksAroundFencesRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "missing blank before",
			input:     "Some text\n```\ncode\n```\n",
			wantDiags: 1,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no code block",
			input:     "Just text\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewBlanksAroundFencesRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestNoSpaceInCodeRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "no spaces",
			input:     "`code`\n",
			wantDiags: 0,
		},
		{
			name:      "single space padding allowed",
			input:     "` code `\n",
			wantDiags: 0,
		},
		{
			name:      "excessive leading spaces",
			input:     "`  code`\n",
			wantDiags: 1,
		},
		{
			name:      "excessive trailing spaces",
			input:     "`code  `\n",
			wantDiags: 1,
		},
		{
			name:      "excessive both sides",
			input:     "`  code  `\n",
			wantDiags: 1,
		},
		{
			name:      "backtick content with padding",
			input:     "`` `code` ``\n",
			wantDiags: 0,
		},
		{
			name:      "spaces only allowed",
			input:     "`   `\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoSpaceInCodeRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestBlanksAroundFencesRule_Metadata(t *testing.T) {
	rule := NewBlanksAroundFencesRule()

	assert.Equal(t, "MD031", rule.ID())
	assert.Equal(t, "blanks-around-fences", rule.Name())
	assert.Contains(t, rule.Tags(), "code")
	assert.True(t, rule.CanFix())
}

func TestNoSpaceInCodeRule_Metadata(t *testing.T) {
	rule := NewNoSpaceInCodeRule()

	assert.Equal(t, "MD038", rule.ID())
	assert.Equal(t, "no-space-in-code", rule.Name())
	assert.Contains(t, rule.Tags(), "code")
	assert.True(t, rule.CanFix())
}

func TestBlanksAroundFencesRule_Fix(t *testing.T) {
	// The BlanksAroundFencesRule checks for blank lines around fenced code blocks.
	// The parser's SourcePosition for code blocks points to the CODE CONTENT,
	// so the rule adjusts to find the actual fence lines (one line before/after content).
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input with blank lines",
			input:     "Some text\n\n```\ncode\n```\n\nMore text\n",
			wantDiags: 0,
			wantFix:   "Some text\n\n```\ncode\n```\n\nMore text\n",
		},
		{
			name:      "missing blank before code block",
			input:     "Some text\n```\ncode\n```\n",
			wantDiags: 1,
			wantFix:   "Some text\n\n```\ncode\n```\n",
		},
		{
			name:      "missing blank after code block",
			input:     "```\ncode\n```\nMore text\n",
			wantDiags: 1,
			wantFix:   "```\ncode\n```\n\nMore text\n",
		},
		{
			name:      "missing both blank lines",
			input:     "Some text\n```\ncode\n```\nMore text\n",
			wantDiags: 2,
			wantFix:   "Some text\n\n```\ncode\n```\n\nMore text\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "code block at start of file",
			input:     "```\ncode\n```\n\nText\n",
			wantDiags: 0,
			wantFix:   "```\ncode\n```\n\nText\n",
		},
		{
			name:      "code block at end of file",
			input:     "Text\n\n```\ncode\n```\n",
			wantDiags: 0,
			wantFix:   "Text\n\n```\ncode\n```\n",
		},
		{
			name:      "multiple code blocks missing blanks",
			input:     "Text\n```\ncode1\n```\nText\n```\ncode2\n```\nEnd\n",
			wantDiags: 4,
			wantFix:   "Text\n\n```\ncode1\n```\n\nText\n\n```\ncode2\n```\n\nEnd\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewBlanksAroundFencesRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))

			// Verify idempotency - re-running on fixed content should produce no diagnostics
			if tt.wantDiags > 0 {
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
				diags2, err := rule.Apply(ruleCtx2)
				require.NoError(t, err)
				assert.Empty(t, diags2, "fix should be idempotent")
			}
		})
	}
}

func TestNoSpaceInCodeRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input - no spaces",
			input:     "`code`\n",
			wantDiags: 0,
			wantFix:   "`code`\n",
		},
		{
			name:      "single space padding allowed",
			input:     "` code `\n",
			wantDiags: 0,
			wantFix:   "` code `\n",
		},
		{
			name:      "excessive leading spaces",
			input:     "`  code`\n",
			wantDiags: 1,
			wantFix:   "`code`\n",
		},
		{
			name:      "excessive trailing spaces",
			input:     "`code  `\n",
			wantDiags: 1,
			wantFix:   "`code`\n",
		},
		{
			name:      "excessive both sides",
			input:     "`  code  `\n",
			wantDiags: 1,
			wantFix:   "`code`\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "backtick content with padding allowed",
			input:     "`` `code` ``\n",
			wantDiags: 0,
			wantFix:   "`` `code` ``\n",
		},
		{
			name:      "spaces only allowed",
			input:     "`   `\n",
			wantDiags: 0,
			wantFix:   "`   `\n",
		},
		{
			name:      "multiple code spans with violations",
			input:     "Use `  cmd1  ` and `  cmd2  ` here.\n",
			wantDiags: 2,
			wantFix:   "Use `cmd1` and `cmd2` here.\n",
		},
		{
			name:      "mixed valid and invalid spans",
			input:     "Valid `code` and invalid `  spaces  ` here.\n",
			wantDiags: 1,
			wantFix:   "Valid `code` and invalid `spaces` here.\n",
		},
		{
			name:      "double backticks with excessive spaces",
			input:     "``  code  ``\n",
			wantDiags: 1,
			wantFix:   "``code``\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoSpaceInCodeRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))

			// Verify idempotency - re-running on fixed content should produce no diagnostics
			if tt.wantDiags > 0 {
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
				diags2, err := rule.Apply(ruleCtx2)
				require.NoError(t, err)
				assert.Empty(t, diags2, "fix should be idempotent")
			}
		})
	}
}
