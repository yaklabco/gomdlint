package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// TestRegistryMarkdownlintRuleConfigReadsMarkdownlintSpellings covers the
// options of a rule that markdownlint has, in a markdownlint configuration.
// markdownlint 0.41.1 reads each option under its own spelling only, so
// gomdlint's name for an option that markdownlint spells otherwise, such as
// max for line_length, and an option that only gomdlint has, such as
// ignore_urls, are unknown options there, which change nothing. The notice
// for gomdlint's name offers markdownlint's spelling. Two spellings of one
// option no longer conflict: gomdlint's is unknown. The options of
// gomdlint's own rules, which markdownlint lacks, keep every spelling.
func TestRegistryMarkdownlintRuleConfigReadsMarkdownlintSpellings(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)

	tests := []struct {
		name        string
		id          string
		value       map[string]any
		options     map[string]any
		unknown     []string
		suggestions [][]string
	}{
		{
			name: "gomdlint's name", id: "MD013", value: map[string]any{"max": 20},
			unknown: []string{"max"}, suggestions: [][]string{{"line_length"}},
		},
		{
			name: "both spellings", id: "MD013", value: map[string]any{"line_length": 20, "max": 90},
			options: map[string]any{"max": 20}, unknown: []string{"max"}, suggestions: [][]string{{"line_length"}},
		},
		{
			name: "an inverted name", id: "MD009", value: map[string]any{"ignore_code_blocks": false, "code_blocks": true},
			options: map[string]any{"ignore_code_blocks": false}, unknown: []string{"ignore_code_blocks"},
			suggestions: [][]string{{"code_blocks"}},
		},
		{
			name: "a gomdlint-only option", id: "MD013", value: map[string]any{"ignore_urls": true, "strict": true},
			options: map[string]any{"strict": true}, unknown: []string{"ignore_urls"}, suggestions: [][]string{nil},
		},
		{
			name: "markdownlint's spellings", id: "MD012", value: map[string]any{"maximum": 2},
			options: map[string]any{"max_consecutive": 2},
		},
		{
			name: "gomdlint's own rule", id: "MDL003", value: map[string]any{"min_dashes": 4},
			options: map[string]any{"min_dashes": 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, issues := registry.MarkdownlintRuleConfig(tt.id, tt.value)

			assert.Equal(t, tt.options, cfg.Options)
			require.Len(t, issues, len(tt.unknown))
			for idx, issue := range issues {
				assert.Equal(t, lint.OptionIssueUnknown, issue.Kind)
				assert.Equal(t, tt.unknown[idx], issue.Key)
				assert.Equal(t, tt.suggestions[idx], issue.Suggestions)
			}
		})
	}
}
