package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// TestRegistryMarkdownlintRuleConfigReadsNumericStrings covers numeric
// strings for numeric options in a markdownlint configuration. Each rule of
// markdownlint 0.41.1 reads its numeric options with JavaScript's Number,
// as Number(value || default), such as MD013's line_length, or as
// value === undefined ? default : Number(value), such as MD009's br_spaces,
// so a string such as "100", "1e2" or " 0x64 " reads as its number there,
// and markdownlint-cli2 reports with it. It used to count as of the wrong
// type. A numeric string that is no whole number counts as of the wrong
// type, as that number does, and so does a string that Number reads as NaN,
// or as 0 from nothing but whitespace. MD022 uses the items of a list as
// they are, so a string item stays of the wrong type, and booleans read no
// numbers. gomdlint's own config files, and the options of gomdlint's own
// rules, which markdownlint lacks, keep reading numbers only.
func TestRegistryMarkdownlintRuleConfigReadsNumericStrings(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)

	tests := []struct {
		id      string
		value   map[string]any
		options map[string]any
		wrong   []string // the keys with a value of the wrong type
	}{
		{id: "MD013", value: map[string]any{"line_length": "100"}, options: map[string]any{"max": float64(100)}},
		{id: "MD013", value: map[string]any{"line_length": "1e2"}, options: map[string]any{"max": float64(100)}},
		{id: "MD013", value: map[string]any{"line_length": " 0x64 "}, options: map[string]any{"max": float64(100)}},
		{id: "MD013", value: map[string]any{"heading_line_length": "30"}, options: map[string]any{"heading_max": float64(30)}},
		{id: "MD007", value: map[string]any{"indent": "4"}, options: map[string]any{"indent": float64(4)}},
		{id: "MD009", value: map[string]any{"br_spaces": "3"}, options: map[string]any{"br_spaces": float64(3)}},
		{id: "MD010", value: map[string]any{"spaces_per_tab": "4"}, options: map[string]any{"spaces_per_tab": float64(4)}},
		{id: "MD012", value: map[string]any{"maximum": "2"}, options: map[string]any{"max_consecutive": float64(2)}},
		{id: "MD022", value: map[string]any{"lines_above": "2"}, options: map[string]any{"lines_above": float64(2)}},
		{id: "MD025", value: map[string]any{"level": "2"}, options: map[string]any{"level": float64(2)}},
		{id: "MD030", value: map[string]any{"ul_single": "2"}, options: map[string]any{"ul_single": float64(2)}},
		{id: "MD041", value: map[string]any{"level": "2"}, options: map[string]any{"level": float64(2)}},
		{id: "MD013", value: map[string]any{"line_length": "10.5"}, wrong: []string{"line_length"}},
		{id: "MD013", value: map[string]any{"line_length": "abc"}, wrong: []string{"line_length"}},
		{id: "MD013", value: map[string]any{"line_length": " "}, wrong: []string{"line_length"}},
		{id: "MD022", value: map[string]any{"lines_above": []any{"2", 1}}, wrong: []string{"lines_above"}},
		{id: "MD009", value: map[string]any{"strict": "1"}, wrong: []string{"strict"}},
		{id: "MDL003", value: map[string]any{"min_dashes": "4"}, wrong: []string{"min_dashes"}},
	}
	for _, tt := range tests {
		cfg, issues := registry.MarkdownlintRuleConfig(tt.id, tt.value)

		assert.Equal(t, tt.options, cfg.Options, "%s %v", tt.id, tt.value)
		var wrong []string
		for _, issue := range issues {
			require.Equal(t, lint.OptionIssueWrongType, issue.Kind, "%s %v", tt.id, tt.value)
			wrong = append(wrong, issue.Key)
		}
		assert.Equal(t, tt.wrong, wrong, "%s %v", tt.id, tt.value)
	}

	_, issues, _ := registry.NormalizeOptions("MD013", map[string]any{"max": "100"})
	require.Len(t, issues, 1, "gomdlint's own config files read numbers only")
	assert.Equal(t, lint.OptionIssueWrongType, issues[0].Kind)
}
