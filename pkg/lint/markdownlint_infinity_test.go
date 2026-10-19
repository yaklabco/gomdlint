package lint_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// TestRegistryMarkdownlintRuleConfigReadsAnInfiniteLimit covers an infinite
// value in a markdownlint configuration, which YAML's .inf, a JSON number
// beyond float64's range and the string "Infinity" write. markdownlint
// 0.41.1 compares counts with an option's value, so an infinite limit is
// one that nothing reaches: line-length (MD013) reports no line with an
// infinite line_length, heading_line_length or code_block_line_length, and
// no-multiple-blanks (MD012) no run of blank lines with an infinite
// maximum. gomdlint reads such a limit as the largest int, which no count
// reaches either. Other options cannot use an infinite value: markdownlint
// fails on it in ul-indent (MD007), no-hard-tabs (MD010) and
// list-marker-space (MD030), and never finishes in blanks-around-headings
// (MD022), so gomdlint ignores it, as it ignores -Infinity and NaN, with a
// warning that writes the value as JavaScript does.
func TestRegistryMarkdownlintRuleConfigReadsAnInfiniteLimit(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)

	limits := []struct {
		id, key, option string
		value           any
	}{
		{id: "MD013", key: "line_length", option: "max", value: math.Inf(1)},
		{id: "MD013", key: "line_length", option: "max", value: "Infinity"},
		{id: "MD013", key: "heading_line_length", option: "heading_max", value: math.Inf(1)},
		{id: "MD013", key: "code_block_line_length", option: "code_block_max", value: " +Infinity "},
		{id: "MD012", key: "maximum", option: "max_consecutive", value: math.Inf(1)},
	}
	for _, tt := range limits {
		cfg, issues := registry.MarkdownlintRuleConfig(tt.id, map[string]any{tt.key: tt.value})
		assert.Empty(t, issues, "%s %s %v", tt.id, tt.key, tt.value)
		assert.Equal(t, map[string]any{tt.option: math.MaxInt}, cfg.Options, "%s %s %v", tt.id, tt.key, tt.value)
	}

	ignored := []struct {
		id, key string
		value   any
		message string
	}{
		{id: "MD013", key: "line_length", value: math.Inf(-1),
			message: `line-length option "line_length" must be an integer, not -Infinity; it is ignored`},
		{id: "MD013", key: "line_length", value: math.NaN(),
			message: `line-length option "line_length" must be an integer, not NaN; it is ignored`},
		{id: "MD007", key: "indent", value: math.Inf(1),
			message: `ul-indent option "indent" must be an integer, not Infinity; it is ignored`},
		{id: "MD022", key: "lines_above", value: math.Inf(1),
			message: `heading-blank-lines option "lines_above" must be an integer or a list of integers, ` +
				`not Infinity; it is ignored`},
		{id: "MD030", key: "ul_single", value: "Infinity",
			message: `list-marker-space option "ul_single" must be an integer, not "Infinity"; it is ignored`},
	}
	for _, tt := range ignored {
		cfg, issues := registry.MarkdownlintRuleConfig(tt.id, map[string]any{tt.key: tt.value})
		assert.Empty(t, cfg.Options, "%s %s %v", tt.id, tt.key, tt.value)
		require.Len(t, issues, 1, "%s %s %v", tt.id, tt.key, tt.value)
		rule, ok := registry.GetByID(tt.id)
		require.True(t, ok)
		assert.Equal(t, tt.message, issues[0].Text(rule.Name()))
	}

	_, issues, _ := registry.NormalizeOptions("MD013", map[string]any{"max": math.Inf(1)})
	require.Len(t, issues, 1, "gomdlint's own config files take an integer only")
	assert.Equal(t, `line-length option "max" must be an integer, not Infinity; it is ignored`, issues[0].Text("line-length"))
}
