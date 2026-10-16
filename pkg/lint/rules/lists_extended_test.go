package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestListIndentRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "consistent indentation",
			input:     "* Item 1\n* Item 2\n* Item 3\n",
			wantDiags: 0,
		},
		{
			name:      "inconsistent indentation",
			input:     "* Item 1\n * Item 2\n* Item 3\n",
			wantDiags: 1,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "single item",
			input:     "* Item 1\n",
			wantDiags: 0,
		},
		{
			name:      "ordered list consistent",
			input:     "1. Item 1\n2. Item 2\n3. Item 3\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewListIndentRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestULIndentRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "correct 2-space indent",
			input:     "* Item 1\n  * Nested\n",
			wantDiags: 0,
		},
		{
			name:      "incorrect 3-space indent",
			input:     "* Item 1\n   * Nested\n",
			wantDiags: 1,
		},
		{
			name:      "correct 4-space indent configured",
			input:     "* Item 1\n    * Nested\n",
			wantDiags: 0,
			config:    map[string]any{"indent": 4},
		},
		{
			name:      "first level not indented by default",
			input:     "* Item 1\n",
			wantDiags: 0,
		},
		{
			name:      "first level indented when configured",
			input:     "  * Item 1\n",
			wantDiags: 0,
			config:    map[string]any{"start_indented": true},
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "nested list on the parent marker line",
			input:     "- - x\n",
			wantDiags: 0,
		},
		{
			name:      "list in a blockquote measured from the quote",
			input:     "> - a\n>   - b\n",
			wantDiags: 0,
		},
		{
			name:      "wrong nesting in a blockquote",
			input:     "> - a\n>    - b\n",
			wantDiags: 1,
		},
		{
			name:      "list in a blockquote inside a list item starts a new nesting",
			input:     "- a\n\n  > - b\n",
			wantDiags: 0,
		},
		{
			name:      "ordered parent is not checked",
			input:     "1. a\n   - b\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewULIndentRule()
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

func TestListMarkerSpaceRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "correct single space",
			input:     "* Item 1\n* Item 2\n",
			wantDiags: 0,
		},
		{
			name:      "two spaces",
			input:     "*  Item 1\n",
			wantDiags: 1,
		},
		{
			name:      "ordered list single space",
			input:     "1. Item 1\n2. Item 2\n",
			wantDiags: 0,
		},
		{
			name:      "ordered list two spaces",
			input:     "1.  Item 1\n",
			wantDiags: 1,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no list",
			input:     "Just some text\n",
			wantDiags: 0,
		},
		{
			name:      "list inside a blockquote",
			input:     "> -  item\n",
			wantDiags: 1,
		},
		{
			name:      "list inside a nested blockquote",
			input:     "> > -  item\n",
			wantDiags: 1,
		},
		{
			name:      "ordered list nested on a bullet line",
			input:     "- 1.  alpha\n",
			wantDiags: 1,
		},
		{
			name:      "correct spacing inside blockquotes",
			input:     "> - item\n> > 1. item\n",
			wantDiags: 0,
		},
		{
			// markdownlint uses the multi-line setting when any item spans
			// more than one line, even in a tight list.
			name:      "tight list with a wrapped item uses ul_multi",
			input:     "- a\n  b\n- c\n",
			wantDiags: 2,
			config:    map[string]any{"ul_multi": 2},
		},
		{
			name:      "single-line items use ul_single",
			input:     "- a\n- c\n",
			wantDiags: 0,
			config:    map[string]any{"ul_multi": 2},
		},
		{
			name:      "indented code after the marker counts as one space",
			input:     "-     code\n",
			wantDiags: 0,
		},
		{
			name:      "empty item is skipped",
			input:     "-\n- a\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewListMarkerSpaceRule()
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

func TestBlanksAroundListsRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "list with blanks around",
			input:     "Some text\n\n* Item 1\n* Item 2\n\nMore text\n",
			wantDiags: 0,
		},
		{
			name:      "missing blank before",
			input:     "Some text\n* Item 1\n* Item 2\n\nMore text\n",
			wantDiags: 1,
		},
		{
			name:      "missing blank after with thematic break",
			input:     "Some text\n\n* Item 1\n* Item 2\n***\n",
			wantDiags: 1,
		},
		{
			name:      "list at start of file",
			input:     "* Item 1\n* Item 2\n\nMore text\n",
			wantDiags: 0,
		},
		{
			name:      "list at end of file",
			input:     "Some text\n\n* Item 1\n* Item 2\n",
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

			rule := NewBlanksAroundListsRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestListIndentRule_Metadata(t *testing.T) {
	rule := NewListIndentRule()

	assert.Equal(t, "MD005", rule.ID())
	assert.Equal(t, "list-indent", rule.Name())
	assert.Contains(t, rule.Tags(), "indentation")
	assert.True(t, rule.CanFix())
}

func TestULIndentRule_Metadata(t *testing.T) {
	rule := NewULIndentRule()

	assert.Equal(t, "MD007", rule.ID())
	assert.Equal(t, "ul-indent", rule.Name())
	assert.Contains(t, rule.Tags(), "indentation")
	assert.True(t, rule.CanFix())
}

func TestListMarkerSpaceRule_Metadata(t *testing.T) {
	rule := NewListMarkerSpaceRule()

	assert.Equal(t, "MD030", rule.ID())
	assert.Equal(t, "list-marker-space", rule.Name())
	assert.Contains(t, rule.Tags(), "whitespace")
	assert.True(t, rule.CanFix())
}

func TestBlanksAroundListsRule_Metadata(t *testing.T) {
	rule := NewBlanksAroundListsRule()

	assert.Equal(t, "MD032", rule.ID())
	assert.Equal(t, "blanks-around-lists", rule.Name())
	assert.Contains(t, rule.Tags(), "blank_lines")
	assert.True(t, rule.CanFix())
}

func TestListIndentRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input - consistent indentation",
			input:     "* Item 1\n* Item 2\n* Item 3\n",
			wantDiags: 0,
			wantFix:   "* Item 1\n* Item 2\n* Item 3\n",
		},
		{
			// Like markdownlint, MD005 does not fix unordered lists; MD007
			// moves their items to the configured indentation.
			name:      "single violation - second item extra indent",
			input:     "* Item 1\n * Item 2\n* Item 3\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n * Item 2\n* Item 3\n",
		},
		{
			name:      "multiple violations - varying indentation",
			input:     "* Item 1\n * Item 2\n  * Item 3\n",
			wantDiags: 2,
			wantFix:   "* Item 1\n * Item 2\n  * Item 3\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "single item - no violations possible",
			input:     "* Item 1\n",
			wantDiags: 0,
			wantFix:   "* Item 1\n",
		},
		{
			name:      "ordered list consistent indentation",
			input:     "1. Item 1\n2. Item 2\n3. Item 3\n",
			wantDiags: 0,
			wantFix:   "1. Item 1\n2. Item 2\n3. Item 3\n",
		},
		{
			name:      "ordered list inconsistent indentation",
			input:     "1. Item 1\n 2. Item 2\n1. Item 3\n",
			wantDiags: 1,
			wantFix:   "1. Item 1\n2. Item 2\n1. Item 3\n",
		},
		{
			name:      "ordered list nested on a bullet line is aligned",
			input:     "- 1. alpha\n  2. beta\n",
			wantDiags: 0,
			wantFix:   "- 1. alpha\n  2. beta\n",
		},
		{
			name:      "right-aligned numbers are allowed",
			input:     " 9. nine\n10. ten\n",
			wantDiags: 0,
			wantFix:   " 9. nine\n10. ten\n",
		},
		{
			name:      "right-aligned list with a misaligned item",
			input:     " 9. nine\n10. ten\n 11. eleven\n",
			wantDiags: 1,
			wantFix:   " 9. nine\n10. ten\n11. eleven\n",
		},
		{
			name:      "ordered item moved left keeps its continuation",
			input:     "1. Item 1\n  2. Item 2\n     continued\n",
			wantDiags: 1,
			wantFix:   "1. Item 1\n2. Item 2\n     continued\n",
		},
		{
			name:      "ordered item is moved right to the first item column",
			input:     "  1. Item 1\n2. Item 2\n",
			wantDiags: 1,
			wantFix:   "  1. Item 1\n  2. Item 2\n",
		},
		{
			// Moving the item left would leave the nested item four columns
			// past the content, turning it into paragraph text.
			name:      "ordered item is not moved when its nested list would break",
			input:     "1. Item 1\n 2. Item 2\n       - nested\n",
			wantDiags: 1,
			wantFix:   "1. Item 1\n 2. Item 2\n       - nested\n",
		},
		{
			name:      "ordered list in a blockquote is fixed",
			input:     "> 1. Item 1\n>  2. Item 2\n",
			wantDiags: 1,
			wantFix:   "> 1. Item 1\n> 2. Item 2\n",
		},
		{
			name:      "ordered list in a nested blockquote is fixed",
			input:     "> > 1. Item 1\n> >  2. Item 2\n",
			wantDiags: 1,
			wantFix:   "> > 1. Item 1\n> > 2. Item 2\n",
		},
		{
			// Without the space after ">", columns still count from the
			// blockquote content.
			name:      "blockquote markers without spaces",
			input:     ">1. Item 1\n> 2. Item 2\n",
			wantDiags: 0,
			wantFix:   ">1. Item 1\n> 2. Item 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewListIndentRule()
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
				assert.Empty(t, filterFixableDiags(diags2), "fix should be idempotent")
			}
		})
	}
}

func TestULIndentRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "clean input - correct 2-space indent",
			input:     "* Item 1\n  * Nested\n",
			wantDiags: 0,
			wantFix:   "* Item 1\n  * Nested\n",
		},
		{
			name:      "single violation - wrong nested indent",
			input:     "* Item 1\n   * Nested\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n  * Nested\n",
		},
		{
			name:      "single violation - deeply nested wrong indent",
			input:     "* Item 1\n  * Nested 1\n      * Nested 2\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n  * Nested 1\n    * Nested 2\n",
		},
		{
			name:      "multiple violations - two nested items wrong indent",
			input:     "* Item 1\n    * Nested 1\n    * Nested 2\n",
			wantDiags: 2,
			wantFix:   "* Item 1\n  * Nested 1\n  * Nested 2\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "single top-level item - no indent issues",
			input:     "* Item 1\n",
			wantDiags: 0,
			wantFix:   "* Item 1\n",
		},
		{
			name:      "start_indented violation - first level not indented",
			input:     "* Item 1\n",
			wantDiags: 1,
			wantFix:   "  * Item 1\n",
			config:    map[string]any{"start_indented": true},
		},
		{
			// Four spaces at the top level make an indented code block, so
			// the fix is withheld.
			name:      "start_indented with custom start_indent - violation",
			input:     "  * Item 1\n",
			wantDiags: 1,
			wantFix:   "  * Item 1\n",
			config:    map[string]any{"start_indented": true, "start_indent": 4},
		},
		{
			name:      "start_indented with start_indent 3 is fixed",
			input:     "  * Item 1\n",
			wantDiags: 1,
			wantFix:   "   * Item 1\n",
			config:    map[string]any{"start_indented": true, "start_indent": 3},
		},
		{
			name:      "custom indent=4 - correct",
			input:     "* Item 1\n    * Nested\n",
			wantDiags: 0,
			wantFix:   "* Item 1\n    * Nested\n",
			config:    map[string]any{"indent": 4},
		},
		{
			name:      "custom indent=4 - violation with 2-space indent",
			input:     "* Item 1\n  * Nested\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n    * Nested\n",
			config:    map[string]any{"indent": 4},
		},
		{
			name:      "indented top-level item keeps its continuation",
			input:     "   - Item\n     continued\n",
			wantDiags: 1,
			wantFix:   "- Item\n     continued\n",
		},
		{
			name:      "whole indented list moves together",
			input:     "  - Alpha\n    - Beta\n  - Gamma\n   - Delta\n",
			wantDiags: 4,
			wantFix:   "- Alpha\n  - Beta\n- Gamma\n- Delta\n",
		},
		{
			// The parent content starts at column 4, so moving the child
			// to column 2 would make it a sibling of its parent.
			name:      "no fix when the nested item would leave its parent",
			input:     "-   Parent\n    - Child\n",
			wantDiags: 1,
			wantFix:   "-   Parent\n    - Child\n",
		},
		{
			name:      "nested item in a blockquote is fixed",
			input:     "> - a\n>    - b\n",
			wantDiags: 1,
			wantFix:   "> - a\n>   - b\n",
		},
		{
			name:      "indented list in a blockquote is fixed",
			input:     ">  - a\n>  - b\n",
			wantDiags: 2,
			wantFix:   "> - a\n> - b\n",
		},
		{
			name:      "no fix when the item holds a code block",
			input:     "   - Item\n\n         code\n",
			wantDiags: 1,
			wantFix:   "   - Item\n\n         code\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewULIndentRule()
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

			// Verify idempotency - re-running on fixed content should produce no diagnostics
			if tt.wantDiags > 0 {
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

func TestListMarkerSpaceRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "clean input - single space after marker",
			input:     "* Item 1\n* Item 2\n",
			wantDiags: 0,
			wantFix:   "* Item 1\n* Item 2\n",
		},
		{
			name:      "single violation - two spaces after marker",
			input:     "*  Item 1\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n",
		},
		{
			name:      "multiple violations - all items have extra spaces",
			input:     "*  Item 1\n*   Item 2\n",
			wantDiags: 2,
			wantFix:   "* Item 1\n* Item 2\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "ordered list - extra spaces",
			input:     "1.  Item 1\n2.   Item 2\n",
			wantDiags: 2,
			wantFix:   "1. Item 1\n2. Item 2\n",
		},
		{
			name:      "loose list (ul_multi) - extra spaces",
			input:     "*  Item 1\n\n*  Item 2\n",
			wantDiags: 2,
			wantFix:   "* Item 1\n\n* Item 2\n",
		},
		{
			name:      "config ul_single=2 - violation needs more spaces",
			input:     "* Item 1\n",
			wantDiags: 1,
			wantFix:   "*  Item 1\n",
			config:    map[string]any{"ul_single": 2},
		},
		{
			name:      "config ul_multi=2 - loose list needs more spaces",
			input:     "* Item 1\n\n* Item 2\n",
			wantDiags: 2,
			wantFix:   "*  Item 1\n\n*  Item 2\n",
			config:    map[string]any{"ul_multi": 2},
		},
		{
			name:      "config ol_single=2 - ordered list needs more spaces",
			input:     "1. Item 1\n2. Item 2\n",
			wantDiags: 2,
			wantFix:   "1.  Item 1\n2.  Item 2\n",
			config:    map[string]any{"ol_single": 2},
		},
		{
			name:      "mixed markers with violations",
			input:     "-  Item 1\n+  Item 2\n*  Item 3\n",
			wantDiags: 3,
			wantFix:   "- Item 1\n+ Item 2\n* Item 3\n",
		},
		{
			name:      "wrapped item is fixed with its continuation",
			input:     "-  Item one\n   continues\n-  Item two\n",
			wantDiags: 2,
			wantFix:   "- Item one\n   continues\n- Item two\n",
		},
		{
			name:      "nested list stays nested when the parent spacing shrinks",
			input:     "-   Parent\n    - Child\n",
			wantDiags: 1,
			wantFix:   "- Parent\n    - Child\n",
		},
		{
			// Narrowing the first item would pull the indented sibling into it.
			name:      "no fix when a sibling would become nested",
			input:     "-  Top\n  -  Next\n",
			wantDiags: 2,
			wantFix:   "-  Top\n  - Next\n",
		},
		{
			// The paragraph after the list would join the item.
			name:      "no fix when a following paragraph would join the item",
			input:     "-  item\n\n  after\n",
			wantDiags: 1,
			wantFix:   "-  item\n\n  after\n",
		},
		{
			name:      "no fix when the item holds more than a paragraph and lists",
			input:     "-  item\n\n   ```\n   code\n   ```\n",
			wantDiags: 1,
			wantFix:   "-  item\n\n   ```\n   code\n   ```\n",
		},
		{
			name:      "spacing in a blockquote is fixed",
			input:     "> -  item\n> -  other\n",
			wantDiags: 2,
			wantFix:   "> - item\n> - other\n",
		},
		{
			name:      "spacing in a nested blockquote is fixed",
			input:     "> > 1.  item\n",
			wantDiags: 1,
			wantFix:   "> > 1. item\n",
		},
		{
			// The quoted paragraph after the list would join the narrowed item.
			name:      "no fix in a blockquote when a following line would join the item",
			input:     "> -  item\n>\n>   after\n",
			wantDiags: 1,
			wantFix:   "> -  item\n>\n>   after\n",
		},
		{
			name:      "no fix when the expected spacing would make code",
			input:     "* Item 1\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n",
			config:    map[string]any{"ul_single": 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewListMarkerSpaceRule()
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

			// Verify idempotency - re-running on fixed content should produce no diagnostics
			if tt.wantDiags > 0 {
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

func TestBlanksAroundListsRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input - blanks around list",
			input:     "Some text\n\n* Item 1\n* Item 2\n\nMore text\n",
			wantDiags: 0,
			wantFix:   "Some text\n\n* Item 1\n* Item 2\n\nMore text\n",
		},
		{
			name:      "single violation - missing blank before list",
			input:     "Some text\n* Item 1\n* Item 2\n\nMore text\n",
			wantDiags: 1,
			wantFix:   "Some text\n\n* Item 1\n* Item 2\n\nMore text\n",
		},
		{
			name:      "single violation - missing blank after list",
			input:     "* Item 1\n* Item 2\n# Heading\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n* Item 2\n\n# Heading\n",
		},
		{
			name:      "heading before list - missing blank",
			input:     "# Heading\n* Item 1\n",
			wantDiags: 1,
			wantFix:   "# Heading\n\n* Item 1\n",
		},
		{
			name:      "thematic break after list - missing blank",
			input:     "* Item 1\n* Item 2\n---\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n* Item 2\n\n---\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "list at start of file - no blank needed before",
			input:     "* Item 1\n* Item 2\n\nMore text\n",
			wantDiags: 0,
			wantFix:   "* Item 1\n* Item 2\n\nMore text\n",
		},
		{
			name:      "list at end of file - no blank needed after",
			input:     "Some text\n\n* Item 1\n* Item 2\n",
			wantDiags: 0,
			wantFix:   "Some text\n\n* Item 1\n* Item 2\n",
		},
		{
			name:      "ordered list missing blank before",
			input:     "Text before\n1. First\n2. Second\n\nText after\n",
			wantDiags: 1,
			wantFix:   "Text before\n\n1. First\n2. Second\n\nText after\n",
		},
		{
			// Rule detects missing blank before list; the missing blank after is a separate
			// violation that would be caught on a subsequent run (or in a different list context)
			name:      "multiple violations - two separate lists",
			input:     "Text\n* List 1\n\nMore text\n* List 2\n\nEnd\n",
			wantDiags: 2,
			wantFix:   "Text\n\n* List 1\n\nMore text\n\n* List 2\n\nEnd\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewBlanksAroundListsRule()
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
				assert.Empty(t, filterFixableDiags(diags2), "fix should be idempotent")
			}
		})
	}
}

// listShape renders the nesting of lists, items, paragraphs and other blocks,
// so that a fix can be checked for keeping the document structure.
func listShape(t *testing.T, input string) string {
	t.Helper()
	snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	var shape strings.Builder
	var walk func(node *mdast.Node)
	walk = func(node *mdast.Node) {
		for child := node.FirstChild; child != nil; child = child.Next {
			switch {
			case child.Kind == mdast.NodeList:
				shape.WriteString("[")
				walk(child)
				shape.WriteString("]")
			case child.Kind == mdast.NodeListItem:
				shape.WriteString("(")
				walk(child)
				shape.WriteString(")")
			case isParagraphLike(child):
				shape.WriteString("p")
			case child.IsBlock():
				shape.WriteString(child.Kind.String())
				walk(child)
			}
		}
	}
	walk(snapshot.Root)
	return shape.String()
}

// TestListIndentationFixesPreserveStructure applies the MD005, MD007 and
// MD030 fixes together, the way one fix pass does, and checks that the lists
// keep their structure.
func TestListIndentationFixesPreserveStructure(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "indented list with a misaligned sibling",
			input: "  - Alpha\n    - Beta\n  - Gamma\n   - Delta\n",
			want:  "- Alpha\n  - Beta\n- Gamma\n- Delta\n",
		},
		{
			name:  "spacing and indentation on the same item",
			input: "  -   Alpha\n      - Beta\n",
		},
		{
			name:  "sibling indented past a narrowed item",
			input: "-  Top\n  -  Next\n",
		},
		{
			name:  "ordered list with misaligned items and spacing",
			input: "1.  One\n 2. Two\n  3.  Three\n",
		},
		{
			name:  "following paragraph close to the content column",
			input: "   -  Item\n\n  After\n",
		},
		{
			name:  "blockquoted list with misaligned items and spacing",
			input: ">  -  Alpha\n>     - Beta\n>  -  Gamma\n>   - Delta\n",
		},
		{
			name:  "blockquoted list followed by a lazy line",
			input: "> -  item\nlazy text\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			var edits []fix.TextEdit
			for _, rule := range []lint.Rule{NewListIndentRule(), NewULIndentRule(), NewListMarkerSpaceRule()} {
				ruleCtx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
				diags, err := rule.Apply(ruleCtx)
				require.NoError(t, err)
				for _, diag := range diags {
					edits = append(edits, diag.FixEdits...)
				}
			}
			accepted, _, _, err := fix.PrepareEditsFiltered(edits, len(tt.input))
			require.NoError(t, err)
			fixed := string(fix.ApplyEdits([]byte(tt.input), accepted))

			if tt.want != "" {
				assert.Equal(t, tt.want, fixed)
			}
			assert.Equal(t, listShape(t, tt.input), listShape(t, fixed), "fixed:\n%s", fixed)
		})
	}
}
