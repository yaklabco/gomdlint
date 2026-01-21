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
			name:      "single violation - second item extra indent",
			input:     "* Item 1\n * Item 2\n* Item 3\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n* Item 2\n* Item 3\n",
		},
		{
			name:      "multiple violations - varying indentation",
			input:     "* Item 1\n * Item 2\n  * Item 3\n",
			wantDiags: 2,
			wantFix:   "* Item 1\n* Item 2\n* Item 3\n",
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
				assert.Empty(t, diags2, "fix should be idempotent")
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
			name:      "start_indented with custom start_indent - violation",
			input:     "  * Item 1\n",
			wantDiags: 1,
			wantFix:   "    * Item 1\n",
			config:    map[string]any{"start_indented": true, "start_indent": 4},
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
				assert.Empty(t, diags2, "fix should be idempotent")
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
				assert.Empty(t, diags2, "fix should be idempotent")
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
				assert.Empty(t, diags2, "fix should be idempotent")
			}
		})
	}
}
