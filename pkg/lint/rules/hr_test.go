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

func TestHRStyleRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "single hr",
			input:     "---\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no hr",
			input:     "Just text\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewHRStyleRule()
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

// TestHRStyleRule_SafeFix verifies MD035 ignores lines that are not thematic
// breaks and that its fix rewrites only the marker characters, never turning
// a paragraph line into a setext heading.
func TestHRStyleRule_SafeFix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		options   map[string]any
		wantLines []int
		wantFix   bool
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{
			name:      "setext underline is not a horizontal rule",
			input:     "Heading\n---\n\n***\n",
			options:   map[string]any{"style": "***"},
			wantLines: []int{},
		},
		{
			name:      "front matter delimiters are not horizontal rules",
			input:     "---\ntitle: x\n---\n\n***\n",
			wantLines: []int{},
		},
		{
			name:      "HTML block content is not a horizontal rule",
			input:     "<div>\n***\n</div>\n\n---\n",
			options:   map[string]any{"style": "---"},
			wantLines: []int{},
		},
		{
			// "---" under the paragraph would make it a setext heading.
			name:      "no dashes directly under a paragraph line",
			input:     "Some paragraph text\n***\n",
			options:   map[string]any{"style": "---"},
			wantLines: []int{2},
		},
		{
			name:      "spaced dashes cannot be a setext underline",
			input:     "Para\n***\n",
			options:   map[string]any{"style": "- - -"},
			wantLines: []int{2},
			wantFix:   true,
			wantFixed: "Para\n- - -\n",
		},
		{
			name:      "indentation inside a list item is kept",
			input:     "- item\n\n  ___\n\n---\n",
			options:   map[string]any{"style": "---"},
			wantLines: []int{3},
			wantFix:   true,
			wantFixed: "- item\n\n  ---\n\n---\n",
		},
		{
			name:      "trailing whitespace is kept",
			input:     "Text\n\n---\n\n***   \n",
			wantLines: []int{5},
			wantFix:   true,
			wantFixed: "Text\n\n---\n\n---   \n",
		},
		{
			// "---" on the first line and again below would read as front
			// matter.
			name:      "no fix that makes front matter",
			input:     "---\n\n***\n",
			wantLines: []int{3},
		},
		{
			name:      "no fix that closes front matter opened far above",
			input:     "---\n\nText.\n\n***\n",
			wantLines: []int{5},
		},
		{
			name:      "no fix to a style that is not a horizontal rule",
			input:     "***\n",
			options:   map[string]any{"style": "==="},
			wantLines: []int{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewHRStyleRule(), tt.input, tt.options)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			for _, diag := range diags {
				assert.Equal(t, tt.wantFix, diag.HasFix())
			}
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, applyAllEdits(t, tt.input, diags))
		})
	}
}

// TestHRStyleRule_FixesSafeTogether verifies that MD035 offers only fixes that
// are safe together: either fix alone keeps two thematic breaks, but both
// would make front matter of the first five lines.
func TestHRStyleRule_FixesSafeTogether(t *testing.T) {
	input := "***\n\ntitle: abc\n\n***\n"
	diags := applyRuleWithOptions(t, NewHRStyleRule(), input, map[string]any{"style": "---"})
	assert.Equal(t, []int{1, 5}, diagLines(diags))
	assert.Equal(t, "---\n\ntitle: abc\n\n***\n", applyAllEdits(t, input, diags))
}

// TestHRStyleRule_Containers verifies MD035 finds horizontal rules inside
// blockquotes and list items and only fixes them when the result keeps the
// document structure.
func TestHRStyleRule_Containers(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
		wantFix   bool
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{
			name:      "blockquote",
			input:     "---\n\n> * * *\n",
			wantLines: []int{3},
			wantFix:   true,
			wantFixed: "---\n\n> ---\n",
		},
		{
			// "- ---" is itself a horizontal rule and would end the list.
			name:      "list item line with the same marker",
			input:     "---\n\n- * * *\n",
			wantLines: []int{3},
		},
		{
			name:      "list item line with a different marker",
			input:     "---\n\n* - - -\n",
			wantLines: []int{3},
			wantFix:   true,
			wantFixed: "---\n\n* ---\n",
		},
		{
			name:      "under a paragraph in a blockquote",
			input:     "---\n\n> para\n> ***\n",
			wantLines: []int{4},
		},
		{
			// "  ---" under "- item" would make the item a setext heading.
			name:      "under a paragraph in a list item",
			input:     "---\n\n- item\n  ***\n",
			wantLines: []int{4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewHRStyleRule(), tt.input, nil)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			for _, diag := range diags {
				assert.Equal(t, tt.wantFix, diag.HasFix())
			}
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, applyAllEdits(t, tt.input, diags))
		})
	}
}

func TestHRStyleRule_Metadata(t *testing.T) {
	rule := NewHRStyleRule()

	assert.Equal(t, "MD035", rule.ID())
	assert.Equal(t, "hr-style", rule.Name())
	assert.Contains(t, rule.Tags(), "hr")
	assert.True(t, rule.CanFix())
}

func TestHRStyleRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "clean input - all HRs same style",
			input:     "# Title\n\n---\n\nSome text\n\n---\n",
			wantDiags: 0,
			wantFix:   "# Title\n\n---\n\nSome text\n\n---\n",
		},
		{
			// A leading heading keeps the dashes from forming front matter.
			name:      "single violation - consistent mode",
			input:     "# Title\n\n---\n\nSome text\n\n***\n",
			wantDiags: 1,
			wantFix:   "# Title\n\n---\n\nSome text\n\n---\n",
		},
		{
			name:      "multiple violations - consistent mode",
			input:     "# Title\n\n---\n\nText\n\n***\n\nMore text\n\n___\n",
			wantDiags: 2,
			wantFix:   "# Title\n\n---\n\nText\n\n---\n\nMore text\n\n---\n",
		},
		{
			// With "---" on the first line, a second "---" line would make
			// front matter of the lines between, so no fix is offered.
			name:      "single violation under a first-line rule",
			input:     "---\n\nSome text\n\n***\n",
			wantDiags: 1,
			wantFix:   "---\n\nSome text\n\n***\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			// A leading heading keeps the dashes from forming front matter.
			name:      "explicit style config - fix dashes to asterisks",
			input:     "# Title\n\n---\n\nSome text\n\n---\n",
			wantDiags: 2,
			wantFix:   "# Title\n\n***\n\nSome text\n\n***\n",
			config:    map[string]any{"style": "***"},
		},
		{
			// A document that starts with "---" and has a second "---" line
			// opens with front matter, whose delimiters are not rules.
			name:      "front matter delimiters are not horizontal rules",
			input:     "---\ntitle: Example\n---\n\nSome text\n\n***\n",
			wantDiags: 0,
			wantFix:   "---\ntitle: Example\n---\n\nSome text\n\n***\n",
			config:    map[string]any{"style": "***"},
		},
		{
			name:      "explicit style config - mixed styles",
			input:     "# Title\n\n---\n\nText\n\n***\n\nMore\n\n___\n",
			wantDiags: 2,
			wantFix:   "# Title\n\n---\n\nText\n\n---\n\nMore\n\n---\n",
			config:    map[string]any{"style": "---"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewHRStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
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

			// Verify idempotency when a fix was applied.
			if string(fixed) == tt.input {
				return
			}
			snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
			require.NoError(t, err)
			ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, ruleCfg)
			diags2, err := rule.Apply(ruleCtx2)
			require.NoError(t, err)
			assert.Empty(t, diags2, "fix should be idempotent")
		})
	}
}
