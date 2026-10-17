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
			wantFix:   "     Double indent\n",
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
			name:      "tab in code block included by default",
			input:     "```\n\tcode\n```\n",
			wantDiags: 1,
			wantFix:   "```\n code\n```\n",
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
				assert.Empty(t, diags2, "fix should be idempotent")
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
