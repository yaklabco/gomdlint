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

func TestHeadingIncrementRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantMsgs  []string
	}{
		{
			name:      "valid increments",
			input:     "# H1\n\n## H2\n\n### H3\n",
			wantDiags: 0,
		},
		{
			name:      "skip level H1 to H3",
			input:     "# H1\n\n### H3\n",
			wantDiags: 1,
			wantMsgs:  []string{"jumped from H1 to H3"},
		},
		{
			name:      "skip level H2 to H4",
			input:     "## H2\n\n#### H4\n",
			wantDiags: 1,
			wantMsgs:  []string{"jumped from H2 to H4"},
		},
		{
			name:      "multiple skips",
			input:     "# H1\n\n### H3\n\n##### H5\n",
			wantDiags: 2,
			wantMsgs:  []string{"jumped from H1 to H3", "jumped from H3 to H5"},
		},
		{
			name:      "decreasing levels allowed",
			input:     "# H1\n\n## H2\n\n# H1 again\n",
			wantDiags: 0,
		},
		{
			name:      "first heading can be any level",
			input:     "### H3\n\n#### H4\n",
			wantDiags: 0,
		},
		{
			name:      "single heading",
			input:     "## H2\n",
			wantDiags: 0,
		},
		{
			name:      "no headings",
			input:     "Just some text\n",
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

			rule := NewHeadingIncrementRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			for i, msg := range tt.wantMsgs {
				if i < len(diags) {
					assert.Contains(t, diags[i].Message, msg)
				}
			}
		})
	}
}

func TestSingleH1Rule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "single H1",
			input:     "# Title\n\n## Section\n",
			wantDiags: 0,
		},
		{
			name:      "multiple H1s",
			input:     "# Title\n\n# Another Title\n",
			wantDiags: 1,
		},
		{
			name:      "three H1s",
			input:     "# One\n\n# Two\n\n# Three\n",
			wantDiags: 2,
		},
		{
			name:      "no H1 allowed by default",
			input:     "## Section\n\n### Subsection\n",
			wantDiags: 0,
		},
		{
			name:      "no H1 not allowed",
			input:     "## Section\n\n### Subsection\n",
			wantDiags: 1,
			config:    map[string]any{"allow_no_h1": false},
		},
		{
			name:      "no headings",
			input:     "Just text\n",
			wantDiags: 0,
		},
		{
			name:      "no headings not allowed",
			input:     "Just text\n",
			wantDiags: 1,
			config:    map[string]any{"allow_no_h1": false},
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

			rule := NewSingleH1Rule()
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

func TestHeadingStyleRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "all ATX style",
			input:     "# H1\n\n## H2\n\n### H3\n",
			wantDiags: 0,
		},
		{
			name:      "ATX with closing hashes consistent",
			input:     "# H1 #\n\n## H2 ##\n",
			wantDiags: 0,
		},
		{
			name:      "mixed ATX styles default",
			input:     "# H1\n\n## H2 ##\n",
			wantDiags: 0, // By default, ATX and ATX_closed are compatible.
		},
		{
			name:      "require closing ATX",
			input:     "# H1\n\n## H2\n",
			wantDiags: 2,
			wantFix:   "# H1 #\n\n## H2 ##\n",
			config:    map[string]any{"require_closing_atx": true},
		},
		{
			name:      "consistent style from first",
			input:     "## H2\n\n### H3\n",
			wantDiags: 0,
			config:    map[string]any{"style": "consistent"},
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no headings",
			input:     "Just text\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewHeadingStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Verify fix application if expected.
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
				}
			}
		})
	}
}

func TestHeadingIncrementRule_Metadata(t *testing.T) {
	rule := NewHeadingIncrementRule()

	assert.Equal(t, "MD001", rule.ID())
	assert.Equal(t, "heading-increment", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.False(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestSingleH1Rule_Metadata(t *testing.T) {
	rule := NewSingleH1Rule()

	assert.Equal(t, "MD025", rule.ID())
	assert.Equal(t, "single-h1", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.False(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestHeadingStyleRule_Metadata(t *testing.T) {
	rule := NewHeadingStyleRule()

	assert.Equal(t, "MD003", rule.ID())
	assert.Equal(t, "heading-style", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.Contains(t, rule.Tags(), "style")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestDetectHeadingStyle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  HeadingStyle
	}{
		{
			name:  "ATX style",
			input: "# Heading\n",
			want:  StyleATX,
		},
		{
			name:  "ATX closed style",
			input: "# Heading #\n",
			want:  StyleATXClosed,
		},
		{
			name:  "ATX level 2",
			input: "## Heading\n",
			want:  StyleATX,
		},
		{
			name:  "ATX level 2 closed",
			input: "## Heading ##\n",
			want:  StyleATXClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			headings := lint.Headings(snapshot.Root)
			require.Len(t, headings, 1)

			got := detectHeadingStyle(snapshot, headings[0])
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAllSameChar(t *testing.T) {
	tests := []struct {
		name string
		b    []byte
		c    byte
		want bool
	}{
		{"all equals", []byte("==="), '=', true},
		{"all dashes", []byte("---"), '-', true},
		{"mixed", []byte("=-="), '=', false},
		{"empty", []byte(""), '=', false},
		{"single", []byte("="), '=', true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allSameChar(tt.b, tt.c)
			assert.Equal(t, tt.want, got)
		})
	}
}
