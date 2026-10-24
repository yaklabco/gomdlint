package lint_test

import (
	"fmt"
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
// type, as that number does, and so does a string that Number reads as NaN.
// TestRegistryMarkdownlintRuleConfigReadsStringsAsZero covers strings that
// read as 0. MD022 uses the items of a list as they are, so a string item
// stays of the wrong type, and booleans read no numbers. gomdlint's own
// config files, and the options of gomdlint's own rules, which markdownlint
// lacks, keep reading numbers only.
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
		{id: "MD022", value: map[string]any{"lines_above": []any{"2", 1}}, wrong: []string{"lines_above"}},
		{id: "MD009", value: map[string]any{"strict": "1"}, wrong: []string{"strict"}},
		{id: "MDL003", value: map[string]any{"min_dashes": "4"}, wrong: []string{"min_dashes"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s %v", tt.id, tt.value), func(t *testing.T) {
			t.Parallel()

			cfg, issues := registry.MarkdownlintRuleConfig(tt.id, tt.value)

			assert.Equal(t, tt.options, cfg.Options)
			var wrong []string
			for _, issue := range issues {
				require.Equal(t, lint.OptionIssueWrongType, issue.Kind)
				wrong = append(wrong, issue.Key)
			}
			assert.Equal(t, tt.wrong, wrong)
		})
	}

	t.Run("gomdlint config", func(t *testing.T) {
		t.Parallel()

		_, issues, _ := registry.NormalizeOptions("MD013", map[string]any{"max": "100"})
		require.Len(t, issues, 1, "gomdlint's own config files read numbers only")
		assert.Equal(t, lint.OptionIssueWrongType, issues[0].Kind)
	})
}

// TestRegistryMarkdownlintRuleConfigReadsStringsAsZero covers strings that
// JavaScript's Number reads as 0 in a markdownlint configuration. Most of
// markdownlint 0.41.1's rules read a numeric option as
// Number(value || default), where 0 is the default, and so do gomdlint's
// (OptionSpec.ZeroIsDefault). A string that reads as 0, such as "0", "0x0"
// or " ", is no false value to ||, so markdownlint uses 0 itself: a length
// of 0 checks no line, a maximum of 0 reports every blank line, and a
// marker spacing of 0 reports every list item. gomdlint reads such a
// string for line-length as -1, with which it checks no line either, and
// ignores it with a warning for the others, whose rules have no way to do
// as markdownlint does. The empty string is false to ||, so it means the
// default, as 0 does in gomdlint. Rules that read the option otherwise
// read such a string as 0, as before, such as ul-indent's indent, which
// gomdlint uses as it is, and no-trailing-spaces's br_spaces, which
// markdownlint reads as value === undefined ? default : Number(value).
func TestRegistryMarkdownlintRuleConfigReadsStringsAsZero(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)

	tests := []struct {
		id      string
		value   map[string]any
		options map[string]any
		message string // the warning for a value that gomdlint ignores
	}{
		{id: "MD013", value: map[string]any{"line_length": "0"}, options: map[string]any{"max": -1}},
		{id: "MD013", value: map[string]any{"line_length": " "}, options: map[string]any{"max": -1}},
		{id: "MD013", value: map[string]any{"heading_line_length": "0x0"}, options: map[string]any{"heading_max": -1}},
		{id: "MD013", value: map[string]any{"code_block_line_length": "-0"}, options: map[string]any{"code_block_max": -1}},
		{id: "MD013", value: map[string]any{"line_length": ""}, options: map[string]any{"max": 0}},
		{id: "MD012", value: map[string]any{"maximum": ""}, options: map[string]any{"max_consecutive": 0}},
		{id: "MD030", value: map[string]any{"ul_single": ""}, options: map[string]any{"ul_single": 0}},
		{
			id: "MD012", value: map[string]any{"maximum": "0"},
			message: `no-multiple-blank-lines option "maximum" is "0", which markdownlint reads as 0 ` +
				`and gomdlint only as the default; it is ignored`,
		},
		{
			id: "MD030", value: map[string]any{"ol_multi": " "},
			message: `list-marker-space option "ol_multi" is " ", which markdownlint reads as 0 ` +
				`and gomdlint only as the default; it is ignored`,
		},
		{id: "MD007", value: map[string]any{"indent": "0"}, options: map[string]any{"indent": float64(0)}},
		{id: "MD009", value: map[string]any{"br_spaces": " "}, options: map[string]any{"br_spaces": float64(0)}},
		// ul-indent, single-title and first-line-heading read 0 as it is,
		// but markdownlint reads these options as Number(value || default)
		// too (OptionSpec.MarkdownlintZeroIsDefault), so its 0, and the
		// empty string, leave the option unset.
		{id: "MD007", value: map[string]any{"indent": float64(0), "start_indent": ""}},
		{id: "MD025", value: map[string]any{"level": 0}},
		{id: "MD041", value: map[string]any{"level": float64(0)}},
		{id: "MD041", value: map[string]any{"level": "0"}, options: map[string]any{"level": float64(0)}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s %v", tt.id, tt.value), func(t *testing.T) {
			t.Parallel()

			cfg, issues := registry.MarkdownlintRuleConfig(tt.id, tt.value)
			switch {
			case tt.message == "" && tt.options == nil:
				assert.Empty(t, issues)
				assert.Empty(t, cfg.Options, "the options are unset")
				return
			case tt.message == "":
				assert.Empty(t, issues)
				assert.Equal(t, tt.options, cfg.Options)
				return
			}
			assert.Empty(t, cfg.Options)
			require.Len(t, issues, 1)
			assert.Equal(t, lint.OptionIssueWrongType, issues[0].Kind)
			rule, ok := registry.GetByID(tt.id)
			require.True(t, ok)
			assert.Equal(t, tt.message, issues[0].Text(rule.Name()))
		})
	}
}
