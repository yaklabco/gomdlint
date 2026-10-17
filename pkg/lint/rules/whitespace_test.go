package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestTrailingWhitespaceRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "no trailing whitespace",
			input:     "Hello world\nSecond line\n",
			wantDiags: 0,
			wantFix:   "Hello world\nSecond line\n",
		},
		{
			name:      "single trailing space",
			input:     "Hello world \n",
			wantDiags: 1,
			wantFix:   "Hello world\n",
		},
		{
			name:      "multiple trailing spaces",
			input:     "Hello world   \n",
			wantDiags: 1,
			wantFix:   "Hello world\n",
		},
		{
			name:      "trailing tab",
			input:     "Hello world\t\n",
			wantDiags: 1,
			wantFix:   "Hello world\n",
		},
		{
			name:      "mixed trailing whitespace",
			input:     "Hello world \t \n",
			wantDiags: 1,
			wantFix:   "Hello world\n",
		},
		{
			name:      "multiple lines with trailing whitespace",
			input:     "Line one \nLine two   \nLine three \n",
			wantDiags: 3,
			wantFix:   "Line one\nLine two  \nLine three\n",
		},
		{
			name:      "blank line is not flagged",
			input:     "Line one\n\nLine three\n",
			wantDiags: 0,
			wantFix:   "Line one\n\nLine three\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "two-space hard break is allowed by default",
			input:     "Line one  \nLine two\n",
			wantDiags: 0,
		},
		{
			name:      "two spaces at paragraph end are allowed when not strict",
			input:     "Line one  \n",
			wantDiags: 0,
		},
		{
			name:      "strict reports two spaces at paragraph end",
			input:     "Line one  \n",
			wantDiags: 1,
			wantFix:   "Line one\n",
			config:    map[string]any{"strict": true},
		},
		{
			name:      "strict allows a hard break",
			input:     "Line one  \nLine two\n",
			wantDiags: 0,
			config:    map[string]any{"strict": true},
		},
		{
			name:      "strict allows a hard break in a tight list item",
			input:     "- item one  \n  continued\n- item two\n",
			wantDiags: 0,
			config:    map[string]any{"strict": true},
		},
		{
			name:      "strict reports two spaces after a heading",
			input:     "# Title  \n",
			wantDiags: 1,
			wantFix:   "# Title\n",
			config:    map[string]any{"strict": true},
		},
		{
			name:      "fix keeps a hard break written with extra spaces",
			input:     "Line one    \nLine two\n",
			wantDiags: 1,
			wantFix:   "Line one  \nLine two\n",
		},
		{
			name:      "fix keeps a hard break in a tight list item",
			input:     "- item one   \n  continued\n",
			wantDiags: 1,
			wantFix:   "- item one  \n  continued\n",
		},
		{
			name:      "fix keeps a hard break that follows a tab",
			input:     "Line one\t  \nLine two\n",
			wantDiags: 1,
			wantFix:   "Line one  \nLine two\n",
		},
		{
			name:      "fix keeps a hard break in a setext heading",
			input:     "Title   \nsubtitle\n===\n",
			wantDiags: 1,
			wantFix:   "Title  \nsubtitle\n===\n",
		},
		{
			name:      "fix keeps a hard break before a lazy continuation line",
			input:     "> Line one   \nLine two\n",
			wantDiags: 1,
			wantFix:   "> Line one  \nLine two\n",
		},
		{
			name:      "strict reports a hard break in a setext heading without a fix",
			input:     "Title  \nsubtitle\n===\n",
			wantDiags: 1,
			wantFix:   "Title  \nsubtitle\n===\n",
			config:    map[string]any{"strict": true},
		},
		{
			// CommonMark renderers such as pandoc read whitespace that
			// spans two or more columns as a hard break even when it ends in
			// a tab, while goldmark reads a soft break, so neither deleting
			// nor normalizing the whitespace is safe.
			name:      "no fix for whitespace ending in a tab before a line break",
			input:     "text  \t\nnext\n",
			wantDiags: 1,
			wantFix:   "text  \t\nnext\n",
		},
		{
			name:      "no fix for a tab before a line break",
			input:     "Line one\t\nLine two\n",
			wantDiags: 1,
			wantFix:   "Line one\t\nLine two\n",
		},
		{
			name:      "no fix for spaces around a tab before a line break",
			input:     "Line one \t \nLine two\n",
			wantDiags: 1,
			wantFix:   "Line one \t \nLine two\n",
		},
		{
			// In a code span the line ending becomes a space, so the
			// trailing whitespace is part of the code text.
			name:      "no fix inside a code span that continues on the next line",
			input:     "Some `code   \nspan` here\n",
			wantDiags: 1,
			wantFix:   "Some `code   \nspan` here\n",
		},
		{
			name:      "fix removes a single space where no hard break exists",
			input:     "Line one \nLine two\n",
			wantDiags: 1,
			wantFix:   "Line one\nLine two\n",
		},
		{
			name:      "extra spaces at paragraph end are removed",
			input:     "Line one   \n",
			wantDiags: 1,
			wantFix:   "Line one\n",
		},
		{
			// With br_spaces below 2 trailing-space hard breaks are reported,
			// but deleting the spaces would change the rendered output.
			name:      "br_spaces 0 reports a hard break without fixing it",
			input:     "Line one  \nLine two\n",
			wantDiags: 1,
			wantFix:   "Line one  \nLine two\n",
			config:    map[string]any{"br_spaces": 0},
		},
		{
			name:      "br_spaces 1 behaves like 0",
			input:     "Line one  \nLine two\n",
			wantDiags: 1,
			wantFix:   "Line one  \nLine two\n",
			config:    map[string]any{"br_spaces": 1},
		},
		{
			name:      "fenced code is ignored by default",
			input:     "```\ncode   \n```\n",
			wantDiags: 0,
		},
		{
			name:      "indented code is ignored by default",
			input:     "Para\n\n    code   \n",
			wantDiags: 0,
		},
		{
			// Trailing whitespace in code is part of the code, so it is
			// reported but kept.
			name:      "code is checked but not fixed when ignore_code_blocks is false",
			input:     "```\ncode   \n```\n",
			wantDiags: 1,
			wantFix:   "```\ncode   \n```\n",
			config:    map[string]any{"ignore_code_blocks": false},
		},
		{
			// A paragraph with inline HTML must stay byte for byte the
			// same; the one without keeps its fix.
			name:      "fix only in the paragraph without inline HTML",
			input:     "Text <b>x</b>   \nmore\n\nplain   \nmore\n",
			wantDiags: 2,
			wantFix:   "Text <b>x</b>   \nmore\n\nplain  \nmore\n",
		},
		{
			// Raw HTML is passed through as written, and <pre> keeps the
			// spaces.
			name:      "no fix inside a pre HTML block",
			input:     "<pre>\nalpha   \nbeta\n</pre>\n",
			wantDiags: 1,
			wantFix:   "<pre>\nalpha   \nbeta\n</pre>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewTrailingWhitespaceRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Verify fix application.
			if tt.wantDiags > 0 {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))

				// Verify idempotency: no fixable diagnostics remain.
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, ruleCfg)
				diags2, err := rule.Apply(ruleCtx2)
				require.NoError(t, err)
				assert.Empty(t, filterFixableDiags(diags2), "fix should be idempotent")
			}
		})
	}
}

func TestFinalNewlineRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "file ends with newline",
			input:     "Hello world\n",
			wantDiags: 0,
			wantFix:   "Hello world\n",
		},
		{
			name:      "file missing final newline",
			input:     "Hello world",
			wantDiags: 1,
			wantFix:   "Hello world\n",
		},
		{
			name:      "single line missing newline",
			input:     "x",
			wantDiags: 1,
			wantFix:   "x\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			// markdownlint MD047 only requires a final newline; extra blank
			// lines at the end of the file are MD012's concern.
			name:      "trailing blank line is not reported",
			input:     "Hello world\n\n",
			wantDiags: 0,
		},
		{
			name:      "several trailing blank lines are not reported",
			input:     "Hello world\n\n\n\n",
			wantDiags: 0,
		},
		{
			name:      "whitespace-only last line counts as blank",
			input:     "Hello world\n   ",
			wantDiags: 0,
		},
		{
			name:      "blockquote marker on last line counts as blank",
			input:     "> Quote\n>",
			wantDiags: 0,
		},
		{
			name:      "comment-only last line counts as blank",
			input:     "Text\n<!-- note -->",
			wantDiags: 0,
		},
		{
			// The paragraph that ends the file closes an element of the
			// HTML block above it.
			name:      "final newline after a paragraph that closes an HTML element",
			input:     "<div>\n\nhello </div>",
			wantDiags: 1,
			wantFix:   "<div>\n\nhello </div>\n",
		},
		{
			name:      "text after comment on last line is not blank",
			input:     "Text\n<!-- note --> more",
			wantDiags: 1,
			wantFix:   "Text\n<!-- note --> more\n",
		},
		{
			name:      "explicit max_trailing_blank_lines still limits blank lines",
			input:     "Hello world\n\n\n",
			wantDiags: 1,
			wantFix:   "Hello world\n",
			config:    map[string]any{"max_trailing_blank_lines": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewFinalNewlineRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Verify fix application.
			if tt.wantDiags > 0 {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))

				// Verify idempotency.
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, ruleCfg)
				diags2, err := rule.Apply(ruleCtx2)
				require.NoError(t, err)
				assert.Empty(t, diags2, "fix should be idempotent")
			}
		})
	}
}

func TestMultipleBlankLinesRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "no consecutive blank lines",
			input:     "Line one\n\nLine two\n",
			wantDiags: 0,
			wantFix:   "Line one\n\nLine two\n",
		},
		{
			name:      "two consecutive blank lines",
			input:     "Line one\n\n\nLine two\n",
			wantDiags: 1,
			wantFix:   "Line one\n\nLine two\n",
		},
		{
			name:      "three consecutive blank lines",
			input:     "Line one\n\n\n\nLine two\n",
			wantDiags: 1,
			wantFix:   "Line one\n\nLine two\n",
		},
		{
			name:      "max consecutive 2",
			input:     "A\n\n\nB\n",
			wantDiags: 0,
			wantFix:   "A\n\n\nB\n",
			config:    map[string]any{"max_consecutive": 2},
		},
		{
			name:      "no blank lines",
			input:     "Line one\nLine two\n",
			wantDiags: 0,
			wantFix:   "Line one\nLine two\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "blank lines inside fenced code are ignored",
			input:     "# Title\n\n```\na\n\n\nb\n```\n",
			wantDiags: 0,
		},
		{
			name:      "blank lines inside indented code are ignored",
			input:     "Para\n\n    a\n\n\n    b\n",
			wantDiags: 0,
		},
		{
			name:      "blank lines inside fenced code in a list item are ignored",
			input:     "- item\n\n  ```\n  a\n\n\n  b\n  ```\n",
			wantDiags: 0,
		},
		{
			name:      "blank lines before a fence are still reported",
			input:     "a\n\n\n```\nx\n```\n",
			wantDiags: 1,
			wantFix:   "a\n\n```\nx\n```\n",
		},
		{
			name:      "blank line before end of file",
			input:     "Line one\n\n",
			wantDiags: 1,
			wantFix:   "Line one\n",
		},
		{
			name:      "several blank lines before end of file",
			input:     "Line one\n\n\n",
			wantDiags: 1,
			wantFix:   "Line one\n",
		},
		{
			name:      "whitespace-only final line",
			input:     "Line one\n\n   ",
			wantDiags: 1,
			wantFix:   "Line one\n",
		},
		{
			name:      "blank lines before end of CRLF file",
			input:     "Line one\r\n\r\n",
			wantDiags: 1,
			wantFix:   "Line one\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewMultipleBlankLinesRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Verify fix application.
			if tt.wantDiags > 0 {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))

				// Verify idempotency.
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, ruleCfg)
				diags2, err := rule.Apply(ruleCtx2)
				require.NoError(t, err)
				assert.Empty(t, diags2, "fix should be idempotent")
			}
		})
	}
}

func TestTrailingWhitespaceRule_Metadata(t *testing.T) {
	rule := NewTrailingWhitespaceRule()

	assert.Equal(t, "MD009", rule.ID())
	assert.Equal(t, "no-trailing-spaces", rule.Name())
	assert.Contains(t, rule.Tags(), "whitespace")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestFinalNewlineRule_Metadata(t *testing.T) {
	rule := NewFinalNewlineRule()

	assert.Equal(t, "MD047", rule.ID())
	assert.Equal(t, "single-trailing-newline", rule.Name())
	assert.Contains(t, rule.Tags(), "blank_lines")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestMultipleBlankLinesRule_Metadata(t *testing.T) {
	rule := NewMultipleBlankLinesRule()

	assert.Equal(t, "MD012", rule.ID())
	assert.Equal(t, "no-multiple-blank-lines", rule.Name())
	assert.Contains(t, rule.Tags(), "whitespace")
	assert.Contains(t, rule.Tags(), "layout")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}
