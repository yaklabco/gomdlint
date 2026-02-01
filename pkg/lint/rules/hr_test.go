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
			input:     "---\n\nSome text\n\n---\n",
			wantDiags: 0,
			wantFix:   "---\n\nSome text\n\n---\n",
		},
		{
			name:      "single violation - consistent mode",
			input:     "---\n\nSome text\n\n***\n",
			wantDiags: 1,
			wantFix:   "---\n\nSome text\n\n---\n",
		},
		{
			name:      "multiple violations - consistent mode",
			input:     "---\n\nText\n\n***\n\nMore text\n\n___\n",
			wantDiags: 2,
			wantFix:   "---\n\nText\n\n---\n\nMore text\n\n---\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "explicit style config - fix dashes to asterisks",
			input:     "---\n\nSome text\n\n---\n",
			wantDiags: 2,
			wantFix:   "***\n\nSome text\n\n***\n",
			config:    map[string]any{"style": "***"},
		},
		{
			name:      "explicit style config - mixed styles",
			input:     "---\n\nText\n\n***\n\nMore\n\n___\n",
			wantDiags: 2,
			wantFix:   "---\n\nText\n\n---\n\nMore\n\n---\n",
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

			// Verify idempotency (always)
			snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
			require.NoError(t, err)
			ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, ruleCfg)
			diags2, err := rule.Apply(ruleCtx2)
			require.NoError(t, err)
			assert.Empty(t, diags2, "fix should be idempotent")
		})
	}
}
