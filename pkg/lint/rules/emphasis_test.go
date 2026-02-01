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

func TestNoEmphasisAsHeadingRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "normal emphasis in text",
			input:     "Some *emphasized* text here.\n",
			wantDiags: 0,
		},
		{
			name:      "normal heading",
			input:     "# Heading\n\nSome text.\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "emphasis ending with punctuation",
			input:     "**Question?**\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoEmphasisAsHeadingRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestNoSpaceInEmphasisRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "correct emphasis",
			input:     "Some *emphasized* text.\n",
			wantDiags: 0,
		},
		{
			name:      "spaces in emphasis",
			input:     "Some * spaced * text.\n",
			wantDiags: 1,
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

			rule := NewNoSpaceInEmphasisRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestEmphasisStyleRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "single emphasis",
			input:     "Some *text* here.\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no emphasis",
			input:     "Plain text.\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewEmphasisStyleRule()
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

func TestStrongStyleRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "single strong",
			input:     "Some **text** here.\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no strong",
			input:     "Plain text.\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewStrongStyleRule()
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

func TestNoEmphasisAsHeadingRule_Metadata(t *testing.T) {
	rule := NewNoEmphasisAsHeadingRule()

	assert.Equal(t, "MD036", rule.ID())
	assert.Equal(t, "no-emphasis-as-heading", rule.Name())
	assert.Contains(t, rule.Tags(), "emphasis")
	assert.True(t, rule.CanFix()) // Now auto-fixable for bold-only paragraphs
}

func TestNoSpaceInEmphasisRule_Metadata(t *testing.T) {
	rule := NewNoSpaceInEmphasisRule()

	assert.Equal(t, "MD037", rule.ID())
	assert.Equal(t, "no-space-in-emphasis", rule.Name())
	assert.Contains(t, rule.Tags(), "emphasis")
	assert.True(t, rule.CanFix())
}

func TestEmphasisStyleRule_Metadata(t *testing.T) {
	rule := NewEmphasisStyleRule()

	assert.Equal(t, "MD049", rule.ID())
	assert.Equal(t, "emphasis-style", rule.Name())
	assert.Contains(t, rule.Tags(), "emphasis")
	assert.True(t, rule.CanFix())
}

func TestStrongStyleRule_Metadata(t *testing.T) {
	rule := NewStrongStyleRule()

	assert.Equal(t, "MD050", rule.ID())
	assert.Equal(t, "strong-style", rule.Name())
	assert.Contains(t, rule.Tags(), "emphasis")
	assert.True(t, rule.CanFix())
}

func TestNoSpaceInEmphasisRule_Fix(t *testing.T) {
	// NOTE: The emphasisSpacePattern regex `(\*{1,2}|_{1,2})\s+([^*_]+)\s+(\*{1,2}|_{1,2})`
	// is greedy and can match incorrectly when there are multiple emphasis markers on
	// the same line. For example, in "*valid* and * invalid *", it matches "* and *"
	// instead of "* invalid *" because [^*_]+ captures everything between any two markers.
	// This is a known limitation of the rule implementation.
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input - no spaces in emphasis",
			input:     "Some *text* here.\n",
			wantDiags: 0,
			wantFix:   "Some *text* here.\n",
		},
		{
			name:      "single violation - spaces in asterisk emphasis",
			input:     "Some * text * here.\n",
			wantDiags: 1,
			wantFix:   "Some *text* here.\n",
		},
		{
			name:      "strong emphasis with spaces",
			input:     "Some ** text ** here.\n",
			wantDiags: 1,
			wantFix:   "Some **text** here.\n",
		},
		{
			name:      "underscore emphasis with spaces",
			input:     "Some _ text _ here.\n",
			wantDiags: 1,
			wantFix:   "Some _text_ here.\n",
		},
		{
			name:      "strong underscore emphasis with spaces",
			input:     "Some __ text __ here.\n",
			wantDiags: 1,
			wantFix:   "Some __text__ here.\n",
		},
		{
			name:      "multiple lines with violations",
			input:     "First * line * text.\n\nSecond * para * here.\n",
			wantDiags: 2,
			wantFix:   "First *line* text.\n\nSecond *para* here.\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "violation at start of line",
			input:     "* spaced * text.\n",
			wantDiags: 1,
			wantFix:   "*spaced* text.\n",
		},
		{
			name:      "violation at end of line",
			input:     "text * spaced *\n",
			wantDiags: 1,
			wantFix:   "text *spaced*\n",
		},
		{
			name:      "no text content",
			input:     "Just plain text.\n",
			wantDiags: 0,
			wantFix:   "Just plain text.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoSpaceInEmphasisRule()
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
			snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
			require.NoError(t, err)
			ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
			diags2, err := rule.Apply(ruleCtx2)
			require.NoError(t, err)
			assert.Empty(t, diags2, "fix should be idempotent")
		})
	}
}

func TestEmphasisStyleRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "clean input - consistent asterisk style",
			input:     "Some *text* and *more* here.\n",
			wantDiags: 0,
			wantFix:   "Some *text* and *more* here.\n",
		},
		{
			name:      "clean input - consistent underscore style",
			input:     "Some _text_ and _more_ here.\n",
			wantDiags: 0,
			wantFix:   "Some _text_ and _more_ here.\n",
		},
		{
			name:      "style mismatch - consistent mode",
			input:     "Some *text* and _more_ here.\n",
			wantDiags: 1,
			wantFix:   "Some *text* and _more_ here.\n",
		},
		{
			name:      "explicit style config - asterisk required",
			input:     "Some _text_ here.\n",
			wantDiags: 1,
			wantFix:   "Some _text_ here.\n",
			config:    map[string]any{"style": "asterisk"},
		},
		{
			name:      "explicit style config - underscore required",
			input:     "Some *text* here.\n",
			wantDiags: 1,
			wantFix:   "Some *text* here.\n",
			config:    map[string]any{"style": "underscore"},
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "no emphasis",
			input:     "Plain text.\n",
			wantDiags: 0,
			wantFix:   "Plain text.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewEmphasisStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes (will be empty since buildStyleFix returns nil)
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))
		})
	}
}

func TestStrongStyleRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "clean input - consistent asterisk style",
			input:     "Some **text** and **more** here.\n",
			wantDiags: 0,
			wantFix:   "Some **text** and **more** here.\n",
		},
		{
			name:      "clean input - consistent underscore style",
			input:     "Some __text__ and __more__ here.\n",
			wantDiags: 0,
			wantFix:   "Some __text__ and __more__ here.\n",
		},
		{
			name:      "style mismatch - consistent mode",
			input:     "Some **text** and __more__ here.\n",
			wantDiags: 1,
			wantFix:   "Some **text** and __more__ here.\n",
		},
		{
			name:      "explicit style config - asterisk required",
			input:     "Some __text__ here.\n",
			wantDiags: 1,
			wantFix:   "Some __text__ here.\n",
			config:    map[string]any{"style": "asterisk"},
		},
		{
			name:      "explicit style config - underscore required",
			input:     "Some **text** here.\n",
			wantDiags: 1,
			wantFix:   "Some **text** here.\n",
			config:    map[string]any{"style": "underscore"},
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "no strong",
			input:     "Plain text.\n",
			wantDiags: 0,
			wantFix:   "Plain text.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewStrongStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes (will be empty since buildStyleFix returns nil)
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))
		})
	}
}
