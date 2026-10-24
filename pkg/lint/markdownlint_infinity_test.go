package lint_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// TestRegistryMarkdownlintRuleConfigReadsALimitBeyondInt covers a limit in
// a markdownlint configuration beyond the range of an int: an infinite
// value, which YAML's .inf, a JSON number beyond float64's range and the
// string "Infinity" write, or a number of 2^63 or more, or below -2^63.
// markdownlint 0.41.1 compares counts with such an option's value, so a
// value beyond every count is one that nothing reaches: line-length (MD013)
// reports no line with such a line_length, heading_line_length or
// code_block_line_length, and no-multiple-blanks (MD012) no run of blank
// lines with such a maximum. gomdlint reads it as the largest int, which no
// count reaches either. A value below every count, such as -Infinity, reads
// as the smallest int: markdownlint's line-length then checks no line
// against the length, as gomdlint's does for a length below 1, and
// gomdlint's no-multiple-blanks counts a maximum below 1 as 1.
//
// Other options cannot use an infinite value: markdownlint fails on it in
// ul-indent (MD007), no-hard-tabs (MD010) and list-marker-space (MD030), and
// never finishes in blanks-around-headings (MD022), so gomdlint ignores it,
// as it ignores NaN, with a warning that writes the value as JavaScript
// does.
func TestRegistryMarkdownlintRuleConfigReadsALimitBeyondInt(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)

	limits := []struct {
		id, key, option string
		value           any
		want            int
	}{
		{id: "MD013", key: "line_length", option: "max", value: math.Inf(1), want: math.MaxInt},
		{id: "MD013", key: "line_length", option: "max", value: "Infinity", want: math.MaxInt},
		{id: "MD013", key: "heading_line_length", option: "heading_max", value: math.Inf(1), want: math.MaxInt},
		{id: "MD013", key: "code_block_line_length", option: "code_block_max", value: " +Infinity ", want: math.MaxInt},
		{id: "MD012", key: "maximum", option: "max_consecutive", value: math.Inf(1), want: math.MaxInt},
		// 2^63, which JSON's 9223372036854775807 reads as, and beyond.
		{id: "MD013", key: "line_length", option: "max", value: math.Exp2(63), want: math.MaxInt},
		{id: "MD013", key: "line_length", option: "max", value: 1e19, want: math.MaxInt},
		{id: "MD013", key: "line_length", option: "max", value: "9223372036854775808", want: math.MaxInt},
		{id: "MD013", key: "code_block_line_length", option: "code_block_max", value: "0xFFFFFFFFFFFFFFFFFF", want: math.MaxInt},
		{id: "MD012", key: "maximum", option: "max_consecutive", value: 1e19, want: math.MaxInt},
		// Below -2^63.
		{id: "MD013", key: "line_length", option: "max", value: math.Inf(-1), want: math.MinInt},
		{id: "MD013", key: "heading_line_length", option: "heading_max", value: "-Infinity", want: math.MinInt},
		{id: "MD013", key: "line_length", option: "max", value: -1e19, want: math.MinInt},
		{id: "MD012", key: "maximum", option: "max_consecutive", value: math.Inf(-1), want: math.MinInt},
	}
	for _, tt := range limits {
		t.Run(fmt.Sprintf("%s %s %v", tt.id, tt.key, tt.value), func(t *testing.T) {
			t.Parallel()

			cfg, issues := registry.MarkdownlintRuleConfig(tt.id, map[string]any{tt.key: tt.value})
			assert.Empty(t, issues)
			assert.Equal(t, map[string]any{tt.option: tt.want}, cfg.Options)
		})
	}

	t.Run("an int at the edge of the range", func(t *testing.T) {
		t.Parallel()

		// -2^63 is an int itself, which reads as it is.
		cfg, issues := registry.MarkdownlintRuleConfig("MD013", map[string]any{"line_length": -math.Exp2(63)})
		assert.Empty(t, issues)
		assert.Equal(t, map[string]any{"max": -math.Exp2(63)}, cfg.Options)
	})

	ignored := []struct {
		id, key string
		value   any
		message string
	}{
		{id: "MD013", key: "line_length", value: math.NaN(),
			message: `line-length option "line_length" must be an integer, not NaN; it is ignored`},
		{id: "MD007", key: "indent", value: math.Inf(1),
			message: `ul-indent option "indent" must be an integer, not Infinity; it is ignored`},
		{id: "MD007", key: "indent", value: math.Inf(-1),
			message: `ul-indent option "indent" must be an integer, not -Infinity; it is ignored`},
		{id: "MD007", key: "indent", value: 1e19,
			message: `ul-indent option "indent" must be an integer, not 10000000000000000000; it is ignored`},
		{id: "MD022", key: "lines_above", value: math.Inf(1),
			message: `heading-blank-lines option "lines_above" must be an integer or a list of integers, ` +
				`not Infinity; it is ignored`},
		{id: "MD030", key: "ul_single", value: "Infinity",
			message: `list-marker-space option "ul_single" must be an integer, not "Infinity"; it is ignored`},
	}
	for _, tt := range ignored {
		t.Run(fmt.Sprintf("%s %s %v", tt.id, tt.key, tt.value), func(t *testing.T) {
			t.Parallel()

			cfg, issues := registry.MarkdownlintRuleConfig(tt.id, map[string]any{tt.key: tt.value})
			assert.Empty(t, cfg.Options)
			require.Len(t, issues, 1)
			rule, ok := registry.GetByID(tt.id)
			require.True(t, ok)
			assert.Equal(t, tt.message, issues[0].Text(rule.Name()))
		})
	}

	t.Run("gomdlint config", func(t *testing.T) {
		t.Parallel()

		_, issues, _ := registry.NormalizeOptions("MD013", map[string]any{"max": math.Inf(1)})
		require.Len(t, issues, 1, "gomdlint's own config files take an integer only")
		assert.Equal(t, `line-length option "max" must be an integer, not Infinity; it is ignored`, issues[0].Text("line-length"))
	})
}
