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

func TestUnorderedListStyleRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "consistent dash style",
			input:     "- Item 1\n- Item 2\n- Item 3\n",
			wantDiags: 0,
		},
		{
			name:      "consistent plus style",
			input:     "+ Item 1\n+ Item 2\n",
			wantDiags: 0,
			config:    map[string]any{"style": "plus"},
		},
		{
			name:      "consistent asterisk style",
			input:     "* Item 1\n* Item 2\n",
			wantDiags: 0,
			config:    map[string]any{"style": "asterisk"},
		},
		{
			name:      "mixed styles default dash",
			input:     "- Item 1\n* Item 2\n+ Item 3\n",
			wantDiags: 2,
			wantFix:   "- Item 1\n- Item 2\n- Item 3\n",
		},
		{
			name:      "consistent style from first",
			input:     "* Item 1\n- Item 2\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n* Item 2\n",
			config:    map[string]any{"style": "consistent"},
		},
		{
			name:      "ordered list ignored",
			input:     "1. Item 1\n2. Item 2\n",
			wantDiags: 0,
		},
		{
			name:      "no lists",
			input:     "Just text\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "nested list same style",
			input:     "- Item 1\n  - Nested 1\n  - Nested 2\n- Item 2\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewUnorderedListStyleRule()
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
			if tt.wantFix != "" && tt.wantDiags > 0 {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				if len(allEdits) > 0 {
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
			}
		})
	}
}

func TestOrderedListIncrementRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "sequential numbering",
			input:     "1. Item 1\n2. Item 2\n3. Item 3\n",
			wantDiags: 0,
		},
		{
			name:      "all ones",
			input:     "1. Item 1\n1. Item 2\n1. Item 3\n",
			wantDiags: 2,
			wantFix:   "1. Item 1\n2. Item 2\n3. Item 3\n",
		},
		{
			name:      "wrong sequence",
			input:     "1. Item 1\n3. Item 2\n5. Item 3\n",
			wantDiags: 2,
			wantFix:   "1. Item 1\n2. Item 2\n3. Item 3\n",
		},
		{
			name:      "start from 5",
			input:     "5. Item 1\n6. Item 2\n7. Item 3\n",
			wantDiags: 0,
		},
		{
			name:      "unordered list ignored",
			input:     "- Item 1\n- Item 2\n",
			wantDiags: 0,
		},
		{
			name:      "no lists",
			input:     "Just text\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "allow renumbering false",
			input:     "1. Item 1\n1. Item 2\n",
			wantDiags: 1,
			wantFix:   "", // No fix when renumbering disabled.
			config:    map[string]any{"allow_renumbering": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewOrderedListIncrementRule()
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
			if tt.wantFix != "" && tt.wantDiags > 0 {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				if len(allEdits) > 0 {
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
			}
		})
	}
}

func TestUnorderedListStyleRule_Metadata(t *testing.T) {
	rule := NewUnorderedListStyleRule()

	assert.Equal(t, "MD004", rule.ID())
	assert.Equal(t, "unordered-list-style", rule.Name())
	assert.Contains(t, rule.Tags(), "lists")
	assert.Contains(t, rule.Tags(), "style")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestOrderedListIncrementRule_Metadata(t *testing.T) {
	rule := NewOrderedListIncrementRule()

	assert.Equal(t, "MD029", rule.ID())
	assert.Equal(t, "ol-prefix", rule.Name())
	assert.Contains(t, rule.Tags(), "ol")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestExtractListItemNumber(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "simple number",
			input: "1. Item\n",
			want:  1,
		},
		{
			name:  "double digit",
			input: "12. Item\n",
			want:  12,
		},
		{
			name:  "with leading space",
			input: "  5. Item\n",
			want:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			lists := lint.Lists(snapshot.Root)
			require.Len(t, lists, 1)

			items := lint.ListItems(lists[0])
			require.Len(t, items, 1)

			got := extractListItemNumber(snapshot, items[0])
			assert.Equal(t, tt.want, got)
		})
	}
}
