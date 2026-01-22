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

func TestNoMissingSpaceATXRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "correct ATX heading",
			input:     "# Heading\n",
			wantDiags: 0,
		},
		{
			name:      "missing space level 1",
			input:     "#Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "missing space level 2",
			input:     "##Heading\n",
			wantDiags: 1,
			wantFix:   "## Heading\n",
		},
		{
			name:      "missing space level 3",
			input:     "###Heading\n",
			wantDiags: 1,
			wantFix:   "### Heading\n",
		},
		{
			name:      "multiple headings missing space",
			input:     "#Heading 1\n\n##Heading 2\n",
			wantDiags: 2,
			wantFix:   "# Heading 1\n\n## Heading 2\n",
		},
		{
			name:      "correct heading with text",
			input:     "# Heading with text\n",
			wantDiags: 0,
		},
		{
			name:      "not a heading - hash in text",
			input:     "This is #not a heading\n",
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

			rule := NewNoMissingSpaceATXRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

func TestNoMultipleSpaceATXRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "correct ATX heading",
			input:     "# Heading\n",
			wantDiags: 0,
		},
		{
			name:      "two spaces",
			input:     "#  Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "three spaces",
			input:     "#   Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "level 2 multiple spaces",
			input:     "##  Heading\n",
			wantDiags: 1,
			wantFix:   "## Heading\n",
		},
		{
			name:      "multiple headings with extra spaces",
			input:     "#  Heading 1\n\n##   Heading 2\n",
			wantDiags: 2,
			wantFix:   "# Heading 1\n\n## Heading 2\n",
		},
		{
			name:      "no space - different rule",
			input:     "#Heading\n",
			wantDiags: 0, // MD018 handles this.
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

			rule := NewNoMultipleSpaceATXRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

func TestNoMissingSpaceClosedATXRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "correct closed ATX heading",
			input:     "# Heading #\n",
			wantDiags: 0,
		},
		{
			name:      "missing space both sides",
			input:     "#Heading#\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "missing space after open",
			input:     "#Heading #\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "missing space before close",
			input:     "# Heading#\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "level 2 missing space",
			input:     "##Heading##\n",
			wantDiags: 1,
			wantFix:   "## Heading ##\n",
		},
		{
			name:      "not closed ATX - regular ATX",
			input:     "# Heading\n",
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

			rule := NewNoMissingSpaceClosedATXRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

func TestNoMultipleSpaceClosedATXRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "correct closed ATX heading",
			input:     "# Heading #\n",
			wantDiags: 0,
		},
		{
			name:      "multiple spaces both sides",
			input:     "#  Heading  #\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "multiple spaces after open",
			input:     "#  Heading #\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "multiple spaces before close",
			input:     "# Heading  #\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "level 2 multiple spaces",
			input:     "##  Heading  ##\n",
			wantDiags: 1,
			wantFix:   "## Heading ##\n",
		},
		{
			name:      "single space - ok",
			input:     "# Heading #\n",
			wantDiags: 0,
		},
		{
			name:      "not closed ATX",
			input:     "#  Heading\n",
			wantDiags: 0, // MD019 handles non-closed.
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

			rule := NewNoMultipleSpaceClosedATXRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

func TestNoMissingSpaceATXRule_Metadata(t *testing.T) {
	rule := NewNoMissingSpaceATXRule()

	assert.Equal(t, "MD018", rule.ID())
	assert.Equal(t, "no-missing-space-atx", rule.Name())
	assert.Contains(t, rule.Tags(), "atx")
	assert.Contains(t, rule.Tags(), "headings")
	assert.Contains(t, rule.Tags(), "spaces")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoMultipleSpaceATXRule_Metadata(t *testing.T) {
	rule := NewNoMultipleSpaceATXRule()

	assert.Equal(t, "MD019", rule.ID())
	assert.Equal(t, "no-multiple-space-atx", rule.Name())
	assert.Contains(t, rule.Tags(), "atx")
	assert.Contains(t, rule.Tags(), "headings")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoMissingSpaceClosedATXRule_Metadata(t *testing.T) {
	rule := NewNoMissingSpaceClosedATXRule()

	assert.Equal(t, "MD020", rule.ID())
	assert.Equal(t, "no-missing-space-closed-atx", rule.Name())
	assert.Contains(t, rule.Tags(), "atx_closed")
	assert.Contains(t, rule.Tags(), "headings")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoMultipleSpaceClosedATXRule_Metadata(t *testing.T) {
	rule := NewNoMultipleSpaceClosedATXRule()

	assert.Equal(t, "MD021", rule.ID())
	assert.Equal(t, "no-multiple-space-closed-atx", rule.Name())
	assert.Contains(t, rule.Tags(), "atx_closed")
	assert.Contains(t, rule.Tags(), "headings")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestHeadingStartLeftRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "heading at start",
			input:     "# Heading\n",
			wantDiags: 0,
		},
		{
			name:      "indented heading one space",
			input:     " # Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "indented heading two spaces",
			input:     "  # Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "indented heading with tab",
			input:     "\t# Heading\n",
			wantDiags: 0, // Tab creates a code block in CommonMark, so no heading to detect.
		},
		{
			name:      "multiple indented headings",
			input:     " # H1\n\n  ## H2\n",
			wantDiags: 2,
			wantFix:   "# H1\n\n## H2\n",
		},
		{
			name:      "code block with hash",
			input:     "    # Not a heading\n",
			wantDiags: 0, // Code block.
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

			rule := NewHeadingStartLeftRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

func TestNoDuplicateHeadingRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "unique headings",
			input:     "# One\n\n## Two\n\n### Three\n",
			wantDiags: 0,
		},
		{
			name:      "duplicate headings",
			input:     "# Same\n\n## Same\n",
			wantDiags: 1,
		},
		{
			name:      "multiple duplicates",
			input:     "# Same\n\n# Same\n\n# Same\n",
			wantDiags: 2,
		},
		{
			name:      "siblings only - different parents allowed",
			input:     "# Version 1\n\n## Features\n\n# Version 2\n\n## Features\n",
			wantDiags: 0,
			config:    map[string]any{"siblings_only": true},
		},
		{
			name:      "siblings only - same parent not allowed",
			input:     "# Title\n\n## Same\n\n## Same\n",
			wantDiags: 1,
			config:    map[string]any{"siblings_only": true},
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

			rule := NewNoDuplicateHeadingRule()
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

func TestNoTrailingPunctuationRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "no trailing punctuation",
			input:     "# Heading\n",
			wantDiags: 0,
		},
		{
			name:      "trailing period",
			input:     "# Heading.\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "trailing comma",
			input:     "# Heading,\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "trailing exclamation",
			input:     "# Heading!\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "trailing colon",
			input:     "# Heading:\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "trailing semicolon",
			input:     "# Heading;\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "question mark allowed by default",
			input:     "# FAQ?\n",
			wantDiags: 0,
		},
		{
			name:      "custom punctuation",
			input:     "# Heading!\n",
			wantDiags: 0,
			config:    map[string]any{"punctuation": "."},
		},
		{
			name:      "empty punctuation disables rule",
			input:     "# Heading.\n",
			wantDiags: 0,
			config:    map[string]any{"punctuation": ""},
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

			rule := NewNoTrailingPunctuationRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

func TestHeadingStartLeftRule_Metadata(t *testing.T) {
	rule := NewHeadingStartLeftRule()

	assert.Equal(t, "MD023", rule.ID())
	assert.Equal(t, "heading-start-left", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.Contains(t, rule.Tags(), "spaces")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoDuplicateHeadingRule_Metadata(t *testing.T) {
	rule := NewNoDuplicateHeadingRule()

	assert.Equal(t, "MD024", rule.ID())
	assert.Equal(t, "no-duplicate-heading", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.False(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoTrailingPunctuationRule_Metadata(t *testing.T) {
	rule := NewNoTrailingPunctuationRule()

	assert.Equal(t, "MD026", rule.ID())
	assert.Equal(t, "no-trailing-punctuation", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}
