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

// fixWithRegistry fixes content the way --fix does, through lint.Pipeline,
// with the rules of registry that the default configuration enables. It
// returns the fixed content and the diagnostics of the fixed content.
func fixWithRegistry(t *testing.T, content string, registry *lint.Registry) (string, []lint.Diagnostic) {
	t.Helper()

	cfg := config.NewConfig()
	cfg.Fix = true
	pipeline := lint.NewPipeline(lint.NewEngine(goldmark.New(string(cfg.Flavor)), registry))
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

// TestFix_BlankLineBeforeATableKeepsTheListTight runs --fix on lists whose
// items hold a table directly after another block. A blank line before such
// a table makes a tight list loose, so no rule may insert one there: not
// MD022 after a heading, not MD031 after a fence, and not MD058 before the
// table. Each case runs with only the rule that wants the blank line and with
// every default rule. In a list that is loose already, the blank line changes
// nothing, and the fixes still apply.
func TestFix_BlankLineBeforeATableKeepsTheListTight(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		ruleID    string // The rule that wants a blank line before the table.
		wantFixed string // Output of --fix; empty means unchanged.
	}{
		{
			name:   "table after a heading",
			input:  "# Title\n\n- # Item\n  | a | b |\n  | --- | --- |\n- next\n",
			ruleID: "MD022",
		},
		{
			name:   "table after a fence",
			input:  "# Title\n\n- item\n  ```sh\n  code\n  ```\n  | a | b |\n  | --- | --- |\n- next\n",
			ruleID: "MD031",
		},
		{
			name:   "table after text",
			input:  "# Title\n\n- item\n  | a | b |\n  | --- | --- |\n- next\n",
			ruleID: "MD058",
		},
		{
			name:   "table after text in a nested list",
			input:  "# Title\n\n- outer\n  - item\n    | a | b |\n    | --- | --- |\n  - next\n",
			ruleID: "MD058",
		},
		{
			name:      "table after a heading in a loose list",
			input:     "# Title\n\n- # Item\n  | a | b |\n  | --- | --- |\n\n- next\n",
			ruleID:    "MD022",
			wantFixed: "# Title\n\n- # Item\n\n  | a | b |\n  | --- | --- |\n\n- next\n",
		},
		{
			name:      "table after a fence in a loose list",
			input:     "# Title\n\n- item\n  ```sh\n  code\n  ```\n  | a | b |\n  | --- | --- |\n\n- next\n",
			ruleID:    "MD031",
			wantFixed: "# Title\n\n- item\n\n  ```sh\n  code\n  ```\n\n  | a | b |\n  | --- | --- |\n\n- next\n",
		},
		{
			// goldmark makes a table and a setext heading of the item's first
			// paragraph, and the heading keeps the paragraph's blank-line flag.
			name:   "heading after a table and a setext underline",
			input:  "# Title\n\n- item\n  | a | b |\n  | --- | --- |\n  ===\n  # Later\n- next\n",
			ruleID: "MD022",
		},
		{
			name:   "heading after a table and a dash setext underline",
			input:  "# Title\n\n- item\n  | a | b |\n  | --- | --- |\n  ---\n  # Later\n- next\n",
			ruleID: "MD022",
		},
		{
			name:   "fence after a table and a setext underline",
			input:  "# Title\n\n- item\n  | a | b |\n  | --- | --- |\n  ===\n  ```sh\n  code\n  ```\n- next\n",
			ruleID: "MD031",
		},
		{
			name:   "fence and heading after a table and a setext underline",
			input:  "# Title\n\n- item\n  | a | b |\n  | --- | --- |\n  ===\n  ```sh\n  code\n  ```\n  # Later\n- next\n",
			ruleID: "MD031",
		},
	}

	for _, tt := range tests {
		rule, ok := lint.DefaultRegistry.GetByID(tt.ruleID)
		require.True(t, ok, "rule %s is registered", tt.ruleID)
		only := lint.NewRegistry()
		only.Register(rule)
		registries := []struct {
			name     string
			registry *lint.Registry
		}{
			{"only " + tt.ruleID, only},
			{"every default rule", lint.DefaultRegistry},
		}
		for _, run := range registries {
			t.Run(tt.name+"/"+run.name, func(t *testing.T) {
				fixed, diags := fixWithRegistry(t, tt.input, run.registry)
				want := tt.wantFixed
				if want == "" {
					want = tt.input
				}
				assert.Equal(t, want, fixed)
				if tt.wantFixed != "" {
					return
				}
				found := false
				for _, diag := range diags {
					if diag.RuleID == tt.ruleID {
						found = true
						assert.False(t, diag.HasFix(), "line %d keeps a fix", diag.StartLine)
					}
				}
				assert.True(t, found, "%s still reports the missing blank line", tt.ruleID)
			})
		}
	}
}
