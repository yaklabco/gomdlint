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
			input:     "Line one \nLine two  \nLine three\n",
			wantDiags: 2,
			wantFix:   "Line one\nLine two\nLine three\n",
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
