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

func TestNoBareURLsRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "autolink URL",
			input:     "<https://example.com>\n",
			wantDiags: 0,
		},
		{
			name:      "bare URL",
			input:     "Visit https://example.com for info\n",
			wantDiags: 1,
		},
		{
			name:      "URL in code span",
			input:     "`https://example.com`\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no URLs",
			input:     "Just some text\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoBareURLsRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestNoBareURLsRule_Metadata(t *testing.T) {
	rule := NewNoBareURLsRule()

	assert.Equal(t, "MD034", rule.ID())
	assert.Equal(t, "no-bare-urls", rule.Name())
	assert.Contains(t, rule.Tags(), "links")
	assert.True(t, rule.CanFix())
}

func TestNoBareURLsRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input - already wrapped URL",
			input:     "Visit <https://example.com> for info\n",
			wantDiags: 0,
			wantFix:   "Visit <https://example.com> for info\n",
		},
		{
			name:      "single bare URL",
			input:     "Visit https://example.com for info\n",
			wantDiags: 1,
			wantFix:   "Visit <https://example.com> for info\n",
		},
		{
			name:      "multiple bare URLs same line",
			input:     "Check https://example.com and https://test.org today\n",
			wantDiags: 2,
			wantFix:   "Check <https://example.com> and <https://test.org> today\n",
		},
		{
			name:      "multiple bare URLs different lines",
			input:     "First https://example.com here\nSecond https://test.org there\n",
			wantDiags: 2,
			wantFix:   "First <https://example.com> here\nSecond <https://test.org> there\n",
		},
		{
			name:      "bare email",
			input:     "Contact a@b.co for help\n",
			wantDiags: 1,
			wantFix:   "Contact <a@b.co> for help\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "URL in code span - should be skipped",
			input:     "Use `https://example.com` in code\n",
			wantDiags: 0,
			wantFix:   "Use `https://example.com` in code\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoBareURLsRule()
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

			// Verify idempotency (always)
			snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
			require.NoError(t, err)
			ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
			diags2, err := rule.Apply(ruleCtx2)
			require.NoError(t, err)
			assert.Empty(t, diags2, "fix should be idempotent")
		})
	}
}
