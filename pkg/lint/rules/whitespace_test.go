package rules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
			// The paragraph holding the first inline HTML that opens an
			// element, and every one after it, must stay byte for byte the
			// same; the one before it keeps its fix.
			name:      "fix only in the paragraph before inline HTML",
			input:     "plain   \nmore\n\nText <b>x</b>   \nmore\n",
			wantDiags: 2,
			wantFix:   "plain  \nmore\n\nText <b>x</b>   \nmore\n",
		},
		{
			name:      "no fix in a paragraph after inline HTML",
			input:     "Text <b>x</b>   \nmore\n\nplain   \nmore\n",
			wantDiags: 2,
			wantFix:   "Text <b>x</b>   \nmore\n\nplain   \nmore\n",
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
			// From raw HTML that opens an element to the end of the file
			// nothing may change: the line ending would join the value of
			// the textarea.
			name:      "no fix after an open textarea",
			input:     "<textarea>alpha",
			wantDiags: 1,
			wantFix:   "<textarea>alpha",
		},
		{
			name:      "no fix after an open pre element",
			input:     "<pre>alpha",
			wantDiags: 1,
			wantFix:   "<pre>alpha",
		},
		{
			name:      "no fix after an open styled div",
			input:     "<div style=\"white-space: pre\">\nalpha",
			wantDiags: 1,
			wantFix:   "<div style=\"white-space: pre\">\nalpha",
		},
		{
			name:      "fix after void elements only",
			input:     "Line<br>\n<img src=\"a.png\">",
			wantDiags: 1,
			wantFix:   "Line<br>\n<img src=\"a.png\">\n",
		},
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
			// HTML block above it, which the fix check does not decide, so
			// nothing after the div changes. The end tag must not crash it.
			name:      "no final newline after a paragraph that closes an HTML element",
			input:     "<div>\n\nhello </div>",
			wantDiags: 1,
			wantFix:   "<div>\n\nhello </div>",
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
				assert.Empty(t, filterFixableDiags(diags2), "fix should be idempotent")
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
			// Like markdownlint, a maximum of 0 counts as 1.
			name:      "max consecutive 0 allows one blank line",
			input:     "A\n\nB\n",
			wantDiags: 0,
			config:    map[string]any{"max_consecutive": 0},
		},
		{
			name:      "max consecutive 0 reports two blank lines",
			input:     "A\n\n\nB\n",
			wantDiags: 1,
			wantFix:   "A\n\nB\n",
			config:    map[string]any{"max_consecutive": 0},
		},
		{
			name:      "max consecutive 0 keeps the final line ending",
			input:     "A\n",
			wantDiags: 0,
			config:    map[string]any{"max_consecutive": 0},
		},
		{
			name:      "negative max consecutive counts as 1",
			input:     "A\n\nB\n\n\n",
			wantDiags: 1,
			wantFix:   "A\n\nB\n",
			config:    map[string]any{"max_consecutive": -2},
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
		{
			name:      "fix after void elements only",
			input:     "Line<br>\n\n\n<img src=\"a.png\">\n",
			wantDiags: 1,
			wantFix:   "Line<br>\n\n<img src=\"a.png\">\n",
		},
		{
			// The div may show the blank lines between the paragraphs.
			name:      "no fix after raw HTML that opens an element",
			input:     "<div style=\"white-space: pre\">\n\nalpha\n\n\nbeta\n",
			wantDiags: 1,
			wantFix:   "<div style=\"white-space: pre\">\n\nalpha\n\n\nbeta\n",
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
				assert.Empty(t, filterFixableDiags(diags2), "fix should be idempotent")
			}
		})
	}
}

// TestMultipleBlankLinesRule_ConvergesWithFinalNewline checks that the fixes
// of MD012 and MD047 settle in one or two passes on a file that ends in extra
// blank lines, whatever the maximum, given natively or as markdownlint's
// maximum. With a maximum of 0 counting as 0, MD012 removed the final line
// ending that MD047 then put back, pass after pass.
func TestMultipleBlankLinesRule_ConvergesWithFinalNewline(t *testing.T) {
	configs := map[string]config.RuleConfig{
		"max_consecutive 0":        {Options: map[string]any{"max_consecutive": 0}},
		"max_consecutive 1":        {Options: map[string]any{"max_consecutive": 1}},
		"max_consecutive -1":       {Options: map[string]any{"max_consecutive": -1}},
		"markdownlint maximum 0":   config.MarkdownlintRuleConfig("MD012", map[string]any{"maximum": 0}),
		"markdownlint maximum nil": config.MarkdownlintRuleConfig("MD012", map[string]any{}),
	}
	for name, ruleCfg := range configs {
		for _, input := range []string{"Text\n\n", "Text\n\n\n\n", "Text\r\n\r\n\r\n", "Text"} {
			t.Run(fmt.Sprintf("%s %q", name, input), func(t *testing.T) {
				registry := lint.NewRegistry()
				registry.Register(NewMultipleBlankLinesRule())
				registry.Register(NewFinalNewlineRule())
				engine := lint.NewEngine(goldmark.New(goldmark.FlavorCommonMark), registry)
				cfg := config.NewConfig()
				cfg.Fix = true
				cfg.Rules = map[string]config.RuleConfig{"MD012": ruleCfg}

				content := []byte(input)
				passes := 0
				for range lint.DefaultMaxFixPasses {
					result, err := engine.LintFile(context.Background(), "test.md", content, cfg)
					require.NoError(t, err)
					if len(result.Edits) == 0 {
						break
					}
					content = fix.ApplyEdits(content, result.Edits)
					passes++
				}
				assert.LessOrEqual(t, passes, 2, "the fixes settle in one or two passes")
				assert.Equal(t, strings.TrimRight(input, "\r\n")+lineEnding(input), string(content))
			})
		}
	}
}

// lineEnding returns the line ending a test input uses, LF when it has none.
func lineEnding(input string) string {
	if strings.Contains(input, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// TestMultipleBlankLinesRule_FixesChangeContent checks that every fix MD012
// offers removes something. The extra blank line at the end of a file that
// ends in two line endings is the empty line after the last one, so the fix
// removes the line ending before it: deleting the empty line itself would be
// an empty edit. Once the fixes are applied, nothing is left to remove, so
// MD012 reports nothing and offers no fix.
func TestMultipleBlankLinesRule_FixesChangeContent(t *testing.T) {
	inputs := []struct{ name, input string }{
		{"blank line before end of file", "Text\n\n"},
		{"blank lines before end of file", "Text\n\n\n\n"},
		{"blank lines before end of CRLF file", "Text\r\n\r\n\r\n"},
		{"byte order mark and blank lines", "\uFEFFText\n\n\n"},
		{"only line endings", "\n\n\n"},
		{"whitespace-only final line", "Text\n\n  "},
		{"front matter and blank lines", "---\ntitle: x\n---\n\n\n"},
		{"code block and blank lines", "```\ncode\n```\n\n\n"},
		{"blank lines in the middle", "Text\n\n\n\nMore\n"},
	}
	for _, name := range []string{"AGENTS", "overlapping_deletions"} {
		content, err := os.ReadFile(filepath.Join("testdata", "real-world", name+".input.md"))
		require.NoError(t, err)
		inputs = append(inputs, struct{ name, input string }{"real-world " + name, string(content)})
	}

	parser := goldmark.New(string(config.FlavorCommonMark))
	rule := NewMultipleBlankLinesRule()
	apply := func(t *testing.T, content []byte, ruleCfg *config.RuleConfig) []lint.Diagnostic {
		t.Helper()
		snapshot, err := parser.Parse(context.Background(), "test.md", content)
		require.NoError(t, err)
		diags, err := rule.Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), ruleCfg))
		require.NoError(t, err)
		return diags
	}
	for _, tt := range inputs {
		for _, maximum := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s max %d", tt.name, maximum), func(t *testing.T) {
				ruleCfg := &config.RuleConfig{Options: map[string]any{"max_consecutive": maximum}}
				var edits []fix.TextEdit
				for _, diag := range apply(t, []byte(tt.input), ruleCfg) {
					if !diag.HasFix() {
						continue
					}
					prepared, err := fix.PrepareEdits(diag.FixEdits, len(tt.input))
					require.NoError(t, err)
					assert.NotEqual(t, tt.input, string(fix.ApplyEdits([]byte(tt.input), prepared)),
						"the fix for line %d changes nothing", diag.StartLine)
					edits = append(edits, diag.FixEdits...)
				}

				accepted, _, _, err := fix.PrepareEditsFiltered(edits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), accepted)
				assert.Empty(t, apply(t, fixed, ruleCfg), "nothing is left to remove in %q", fixed)
			})
		}
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
