package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// fixWithDefaultRules fixes content the way --fix does: through
// lint.Pipeline, pass by pass, with the rules of lint.DefaultRegistry and
// the rule options in options. It returns the fixed content and the
// diagnostics of the fixed content.
func fixWithDefaultRules(t *testing.T, content string, options map[string]map[string]any) (string, []lint.Diagnostic) {
	t.Helper()

	cfg := config.NewConfig()
	cfg.Fix = true
	for ruleID, ruleOptions := range options {
		cfg.Rules[ruleID] = config.RuleConfig{Options: ruleOptions}
	}
	pipeline := lint.NewPipeline(lint.NewEngine(goldmark.New(string(cfg.Flavor)), lint.DefaultRegistry))
	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte(content), cfg,
		lint.PipelineOptions{Fix: true})
	require.NoError(t, err)
	require.Empty(t, result.RuleErrors)
	require.False(t, result.Skipped, result.SkipReason)

	if !result.Modified {
		return content, result.Diagnostics
	}
	return string(result.ModifiedContent), result.Diagnostics
}

// TestFix_RuleFixesKeepTheDocument runs --fix with every default rule on
// documents whose findings MD058, MD040 and MD026 once fixed by changing the
// document: a blank line that made a tight list loose, a language that
// allowed_languages leaves out, and a heading left empty. Those findings stay
// without fixes and the document stays as it is, while a fix that keeps the
// document is still applied.
func TestFix_RuleFixesKeepTheDocument(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		options   map[string]map[string]any
		ruleID    string
		wantLines []int  // Lines of the rule's findings left after fixing.
		wantFixed string // Output of --fix; empty means unchanged.
	}{
		{
			name:      "table in a tight list item",
			input:     "- item\n  | a | b |\n  | --- | --- |\n  | 1 | 2 |\n- next\n",
			ruleID:    "MD058",
			wantLines: []int{2, 4},
		},
		{
			name:      "table in a nested tight list",
			input:     "# Title\n\n- outer\n  - item\n    | a | b |\n    | --- | --- |\n  - next\n",
			ruleID:    "MD058",
			wantLines: []int{5, 6},
		},
		{
			name:      "table in a blockquote in a list item",
			input:     "# Title\n\n- item\n  > | a | b |\n  > | --- | --- |\n- next\n",
			ruleID:    "MD058",
			wantFixed: "# Title\n\n- item\n  >\n  > | a | b |\n  > | --- | --- |\n  >\n- next\n",
		},
		{
			name:      "detected language that allowed_languages leaves out",
			input:     "# Title\n\n```\n#!/bin/bash\necho hi\n```\n",
			options:   map[string]map[string]any{"MD040": {"allowed_languages": []any{"go", "ruby"}}},
			ruleID:    "MD040",
			wantLines: []int{3},
		},
		{
			name:      "detected language that allowed_languages names in another case",
			input:     "# Title\n\n```\n#!/bin/bash\necho hi\n```\n",
			options:   map[string]map[string]any{"MD040": {"allowed_languages": []any{"Bash"}}},
			ruleID:    "MD040",
			wantLines: []int{3},
		},
		{
			// A carriage return ends the fence line, so the language is "bash".
			name:    "language followed by a bare carriage return",
			input:   "# Title\n\n```bash\recho hi\n```\n",
			options: map[string]map[string]any{"MD040": {"allowed_languages": []any{"bash"}}},
			ruleID:  "MD040",
		},
		{
			name:      "fence holding only a no-break space",
			input:     "# Title\n\n```\u00a0\n#!/bin/bash\necho hi\n```\n",
			options:   map[string]map[string]any{"MD040": {"allowed_languages": []any{"bash"}}},
			ruleID:    "MD040",
			wantLines: []int{3},
		},
		{
			name:      "detected language that allowed_languages names",
			input:     "# Title\n\n```\n#!/bin/bash\necho hi\n```\n",
			options:   map[string]map[string]any{"MD040": {"allowed_languages": []any{"go", "bash"}}},
			ruleID:    "MD040",
			wantFixed: "# Title\n\n```bash\n#!/bin/bash\necho hi\n```\n",
		},
		{
			name:      "heading of a period",
			input:     "# Title\n\n## .\n",
			ruleID:    "MD026",
			wantLines: []int{3},
		},
		{
			name:      "heading of an escaped period",
			input:     "# Title\n\n## \\.\n",
			ruleID:    "MD026",
			wantLines: []int{3},
		},
		{
			name:      "heading of a variation selector and a period",
			input:     "# Title\n\n## \ufe0f.\n",
			ruleID:    "MD026",
			wantLines: []int{3},
		},
		{
			// GFM reads "[ ](/url)" as a link with a space as its text, which
			// goldmark reads as a task checkbox and text.
			name:      "heading of a link with a space as its text in a list item",
			input:     "# Title\n\n- ## [ ](/url).\n",
			ruleID:    "MD026",
			wantLines: []int{3},
		},
		{
			name:      "heading that ends with a period",
			input:     "# Title\n\n## Usage.\n",
			ruleID:    "MD026",
			wantFixed: "# Title\n\n## Usage\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixed, diags := fixWithDefaultRules(t, tt.input, tt.options)
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, fixed)

			var lines []int
			for _, diag := range diags {
				if diag.RuleID != tt.ruleID {
					continue
				}
				lines = append(lines, diag.StartLine)
				assert.False(t, diag.HasFix(), "line %d keeps a fix", diag.StartLine)
			}
			assert.Equal(t, tt.wantLines, lines)
		})
	}
}
