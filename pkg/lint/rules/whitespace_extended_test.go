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

func TestHardTabsRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "no tabs",
			input:     "Hello world\nSecond line\n",
			wantDiags: 0,
			wantFix:   "Hello world\nSecond line\n",
		},
		{
			// A leading tab indents to column 4, which makes an indented code
			// block; it is replaced by the spaces that keep that column.
			name:      "single tab",
			input:     "\tIndented\n",
			wantDiags: 1,
			wantFix:   "    Indented\n",
		},
		{
			// The first tab is the code block indentation; the second is code
			// and gets spaces_per_tab spaces.
			name:      "multiple tabs same line",
			input:     "\t\tDouble indent\n",
			wantDiags: 1,
			wantFix:   "    \tDouble indent\n",
		},
		{
			name:      "tabs on multiple lines",
			input:     "\tLine one\n\tLine two\n",
			wantDiags: 2,
			wantFix:   "    Line one\n    Line two\n",
		},
		{
			name:      "mixed spaces and tabs",
			input:     "  \tMixed\n",
			wantDiags: 1,
			wantFix:   "    Mixed\n",
		},
		{
			name:      "tab indenting a nested list keeps it nested",
			input:     "- first\n\t- nested\n",
			wantDiags: 1,
			wantFix:   "- first\n    - nested\n",
		},
		{
			name:      "tab after a list marker keeps the content column",
			input:     "- first\n-\tsecond\n\n  para\n",
			wantDiags: 1,
			wantFix:   "- first\n-   second\n\n  para\n",
		},
		{
			name:      "tab after an ordered list marker",
			input:     "1.\tItem\n",
			wantDiags: 1,
			wantFix:   "1.  Item\n",
		},
		{
			// The link far above resolves to the destination, which a space
			// instead of the tab would change.
			name:      "tab in a definition destination is reported without a fix",
			input:     "[x][r]\n\n# A\n\n# B\n\n[r]: </a\tb>\n",
			wantDiags: 1,
			wantFix:   "[x][r]\n\n# A\n\n# B\n\n[r]: </a\tb>\n",
		},
		{
			name:      "tab in an HTML block is reported without a fix",
			input:     "<div>\n\tx\n</div>\n",
			wantDiags: 1,
			wantFix:   "<div>\n\tx\n</div>\n",
		},
		{
			// The heading anchor drops a tab but turns a space into a hyphen.
			name:      "tab in a heading is reported without a fix",
			input:     "# Foo\tBar\n",
			wantDiags: 1,
			wantFix:   "# Foo\tBar\n",
		},
		{
			// The fence and its content stay in the list item: every tab
			// becomes the spaces that reach the column it reached.
			name:      "tabs that put a fenced code block in a list item",
			input:     "-\t~~~\n\t$ ls\n\t~~~\n",
			wantDiags: 3,
			wantFix:   "-   ~~~\n    $ ls\n    ~~~\n",
		},
		{
			name:      "tab after a blockquote marker",
			input:     ">\tquote\n",
			wantDiags: 1,
			wantFix:   ">   quote\n",
		},
		{
			// A style sheet can make the text before it keep its tabs.
			name:      "tab before a style element is reported but not fixed",
			input:     "alpha\tbeta\n\n<style>p { white-space: pre }</style>\n",
			wantDiags: 1,
			wantFix:   "alpha\tbeta\n\n<style>p { white-space: pre }</style>\n",
		},
		{
			name:      "tab before a style sheet link is reported but not fixed",
			input:     "alpha\tbeta\n\n<link rel=stylesheet href=\"a.css\">\n",
			wantDiags: 1,
			wantFix:   "alpha\tbeta\n\n<link rel=stylesheet href=\"a.css\">\n",
		},
		{
			name:      "tab after a tag named with a Kelvin sign is not fixed",
			input:     "<hr><lin\u212a>\n\nalpha\tbeta\n",
			wantDiags: 1,
			wantFix:   "<hr><lin\u212a>\n\nalpha\tbeta\n",
		},
		{
			// An event handler can set styles on the whole document.
			name:      "tab before an image with an error handler is not fixed",
			input:     "alpha\tbeta\n\n<img src=\"data:image/png,broken\" onerror=\"document.body.style.whiteSpace='pre'\">\n",
			wantDiags: 1,
			wantFix:   "alpha\tbeta\n\n<img src=\"data:image/png,broken\" onerror=\"document.body.style.whiteSpace='pre'\">\n",
		},
		{
			name:      "tab next to line breaks and comments is fixed",
			input:     "alpha\tbeta\n\n<br>\n<!-- markdownlint-disable MD013 -->\n",
			wantDiags: 1,
			wantFix:   "alpha beta\n\n<br>\n<!-- markdownlint-disable MD013 -->\n",
		},
		{
			name:      "tabs in a Makefile block are reported but not fixed",
			input:     "```makefile\nall:\n\tgo build ./...\n\tgo test ./...\n```\n",
			wantDiags: 2,
			wantFix:   "```makefile\nall:\n\tgo build ./...\n\tgo test ./...\n```\n",
		},
		{
			name:      "tab in prose is fixed but not the one in a code span",
			input:     "Text\twith `code\there` tab\n",
			wantDiags: 1,
			wantFix:   "Text with `code\there` tab\n",
		},
		{
			name:      "tab in middle of line",
			input:     "Hello\tworld\n",
			wantDiags: 1,
			wantFix:   "Hello world\n",
		},
		{
			name:      "spaces_per_tab option",
			input:     "\tIndented\n",
			wantDiags: 1,
			wantFix:   "    Indented\n",
			config:    map[string]any{"spaces_per_tab": 4},
		},
		{
			// Code is reported like markdownlint does, but its tabs can be
			// significant, so they are kept.
			name:      "tab in code block is reported but not fixed",
			input:     "```\n\tcode\n```\n",
			wantDiags: 1,
			wantFix:   "```\n\tcode\n```\n",
		},
		{
			name:      "tab in code block excluded",
			input:     "```\n\tcode\n```\n",
			wantDiags: 0,
			wantFix:   "```\n\tcode\n```\n",
			config:    map[string]any{"code_blocks": false},
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "only spaces",
			input:     "    indented\n",
			wantDiags: 0,
			wantFix:   "    indented\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewHardTabsRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
			if tt.wantFix != "" {
				assert.Equal(t, listShape(t, tt.input), listShape(t, tt.wantFix), "the expected fix keeps the structure")
			}

			if tt.wantDiags > 0 && tt.wantFix == tt.input {
				assert.Empty(t, filterFixableDiags(diags), "the findings are reported without a fix")
			}

			// Verify fix application.
			if tt.wantDiags > 0 && tt.wantFix != tt.input {
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

func TestHardTabsRule_IgnoreCodeLanguages(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "makefile tabs ignored",
			input:     "```makefile\n\ttarget:\n```\n",
			wantDiags: 0,
			config:    map[string]any{"ignore_code_languages": []any{"makefile"}},
		},
		{
			name:      "go tabs not ignored",
			input:     "```go\n\tfunc main() {}\n```\n",
			wantDiags: 1,
			config:    map[string]any{"ignore_code_languages": []any{"makefile"}},
		},
		{
			name:      "multiple languages ignored",
			input:     "```makefile\n\ttarget:\n```\n\n```go\n\tcode\n```\n",
			wantDiags: 1,
			config:    map[string]any{"ignore_code_languages": []any{"makefile"}},
		},
		{
			name:      "ignored languages given as strings",
			input:     "```makefile\n\ttarget:\n```\n",
			wantDiags: 0,
			config:    map[string]any{"ignore_code_languages": []string{"makefile"}},
		},
		{
			// The language is the first word of the info string.
			name:      "language followed by more info",
			input:     "```makefile title=\"Build\"\n\ttarget:\n```\n",
			wantDiags: 0,
			config:    map[string]any{"ignore_code_languages": []any{"makefile"}},
		},
		{
			name:      "case insensitive language match",
			input:     "```Makefile\n\ttarget:\n```\n",
			wantDiags: 0,
			config:    map[string]any{"ignore_code_languages": []any{"makefile"}},
		},
		{
			// Like markdownlint, only the code between the fences is ignored.
			name:      "fence lines of an ignored language are checked",
			input:     "```makefile\t\n\ttarget:\n```\t\n",
			wantDiags: 2,
			config:    map[string]any{"ignore_code_languages": []any{"makefile"}},
		},
		{
			name:      "fence lines are checked when code blocks are excluded",
			input:     "```go\t\n\tcode\n```\n",
			wantDiags: 1,
			config:    map[string]any{"code_blocks": false},
		},
		{
			// markdownlint ignores a fenced block when every language its
			// fences name is ignored, which a fence that names none passes.
			name:      "fence without a language when languages are ignored",
			input:     "```\n\tcode\n```\n\n~~~\n\tcode\n~~~\n",
			wantDiags: 0,
			config:    map[string]any{"ignore_code_languages": []any{"makefile"}},
		},
		{
			name:      "fence without a language when no languages are ignored",
			input:     "```\n\tcode\n```\n",
			wantDiags: 1,
		},
		{
			name:      "indented code when languages are ignored",
			input:     "Text.\n\n    \tcode\n",
			wantDiags: 1,
			config:    map[string]any{"ignore_code_languages": []any{"makefile"}},
		},
		{
			name:      "code spans are excluded with code blocks",
			input:     "Text `a\tb` more\n",
			wantDiags: 0,
			config:    map[string]any{"code_blocks": false},
		},
		{
			name:      "code spans are checked by default",
			input:     "Text `a\tb` more\n",
			wantDiags: 1,
		},
		{
			name:      "text next to an excluded code span is checked",
			input:     "Text\t`a\tb` more\n",
			wantDiags: 1,
			config:    map[string]any{"code_blocks": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewHardTabsRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestHardTabsRule_Metadata(t *testing.T) {
	rule := NewHardTabsRule()

	assert.Equal(t, "MD010", rule.ID())
	assert.Equal(t, "no-hard-tabs", rule.Name())
	assert.Contains(t, rule.Tags(), "hard_tab")
	assert.Contains(t, rule.Tags(), "whitespace")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}
