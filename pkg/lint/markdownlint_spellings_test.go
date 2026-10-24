package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// unknownOption is an option that a markdownlint setting writes and the
// rule does not read there, with the spellings that its issue suggests.
type unknownOption struct {
	key         string
	suggestions []string
}

// TestRegistryMarkdownlintRuleConfigReadsMarkdownlintSpellings covers the
// options of a rule that markdownlint has, in a markdownlint configuration.
// markdownlint 0.41.1 reads each option of its rules under its own spelling
// only, so gomdlint's name for an option that markdownlint spells
// otherwise, such as max for line_length, and an option that only gomdlint
// has, such as ignore_urls, are unknown options there, which change
// nothing. The issue for gomdlint's name, or a misspelling of it, offers
// markdownlint's spelling. Two spellings of one option no longer conflict:
// gomdlint's is unknown. In a setting that turns the rule off, which resets
// nothing, gomdlint's name changes nothing either, so the option keeps the
// default that the setting writes out. The options of gomdlint's own rules,
// which markdownlint lacks, keep every spelling.
func TestRegistryMarkdownlintRuleConfigReadsMarkdownlintSpellings(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)

	tests := []struct {
		name    string
		id      string
		value   map[string]any
		options map[string]any
		unknown []unknownOption
	}{
		{
			name: "gomdlint's name", id: "MD013", value: map[string]any{"max": 20},
			unknown: []unknownOption{{key: "max", suggestions: []string{"line_length"}}},
		},
		{
			name: "both spellings", id: "MD013", value: map[string]any{"line_length": 20, "max": 90},
			options: map[string]any{"max": 20},
			unknown: []unknownOption{{key: "max", suggestions: []string{"line_length"}}},
		},
		{
			name: "an inverted name", id: "MD009", value: map[string]any{"ignore_code_blocks": true, "code_blocks": true},
			options: map[string]any{"ignore_code_blocks": false},
			unknown: []unknownOption{{key: "ignore_code_blocks", suggestions: []string{"code_blocks"}}},
		},
		{
			name: "a misspelled name of gomdlint's", id: "MD013", value: map[string]any{"ignore_tabels": true},
			unknown: []unknownOption{{key: "ignore_tabels", suggestions: []string{"tables"}}},
		},
		{
			name: "a gomdlint-only option", id: "MD013", value: map[string]any{"ignore_urls": true, "strict": true},
			options: map[string]any{"strict": true}, unknown: []unknownOption{{key: "ignore_urls"}},
		},
		{
			name: "a value of the wrong type beside gomdlint's name", id: "MD013",
			value:   map[string]any{"line_length": "abc", "max": 20},
			unknown: []unknownOption{{key: "max", suggestions: []string{"line_length"}}},
		},
		{
			name: "gomdlint's name in a setting that resets nothing", id: "MD013",
			value: map[string]any{"enabled": false, "max": 50},
			options: map[string]any{
				"max": 80, "ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true,
			},
			unknown: []unknownOption{{key: "max", suggestions: []string{"line_length"}}},
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
			var unknown []unknownOption
			for _, issue := range issues {
				if issue.Kind == lint.OptionIssueUnknown {
					unknown = append(unknown, unknownOption{key: issue.Key, suggestions: issue.Suggestions})
				}
			}
			assert.Equal(t, tt.unknown, unknown)
		})
	}
}

// TestRegistryMarkdownlintRuleConfigTagOptions covers options under a tag
// in a markdownlint configuration, which markdownlint gives every rule the
// tag covers: markdownlint's spelling of an option reaches each rule that
// reads it, as MD009's inverted code_blocks and MD010's code_blocks do
// under whitespace, while gomdlint's name of an option reaches none.
func TestRegistryMarkdownlintRuleConfigTagOptions(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)

	md009, issues := registry.MarkdownlintRuleConfig("MD009", map[string]any{"code_blocks": false})
	assert.Equal(t, map[string]any{"ignore_code_blocks": true}, md009.Options)
	assert.Empty(t, issues)
	md010, issues := registry.MarkdownlintRuleConfig("MD010", map[string]any{"code_blocks": false})
	assert.Equal(t, map[string]any{"code_blocks": false}, md010.Options)
	assert.Empty(t, issues)

	for _, id := range []string{"MD009", "MD010"} {
		cfg, issues := registry.MarkdownlintRuleConfig(id, map[string]any{"ignore_code_blocks": true})
		assert.Nil(t, cfg.Options, id)
		require.Len(t, issues, 1, id)
		assert.Equal(t, lint.OptionIssueUnknown, issues[0].Kind, id)
	}
}

// TestRegistryMarkdownlintRuleConfigCustomRules covers custom rules, which
// markdownlint lacks, whatever their IDs: X001, and MD900, whose ID has the
// shape of markdownlint's own but names none of its rules. A markdownlint
// configuration reads their options under every spelling that they
// declare, and options that they mark gomdlint-only apply too, as
// markdownlint passes every option of a custom rule to it.
func TestRegistryMarkdownlintRuleConfigCustomRules(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"X001", "MD900"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			registry := lint.NewRegistry()
			registry.Register(&optionRule{id: id, name: "custom"})
			registry.Describe(lint.RuleMeta{ID: id, Name: "custom", OptionsDeclared: true, Options: []lint.OptionSpec{
				{Name: "depth", Aliases: []lint.OptionAlias{{Name: "old_depth"}}, Type: lint.OptionInteger, Default: 1},
				{Name: "level", Type: lint.OptionInteger, Default: 1, GomdlintOnly: true},
				{Name: "style", Aliases: []lint.OptionAlias{{Name: "old_style"}}, Type: lint.OptionString, Default: "a"},
			}})

			for _, value := range []map[string]any{
				{"depth": 4, "level": 2, "style": "b"}, {"old_depth": 4, "level": 2, "old_style": "b"},
			} {
				cfg, issues := registry.MarkdownlintRuleConfig(id, value)
				assert.Equal(t, map[string]any{"depth": 4, "level": 2, "style": "b"}, cfg.Options, "%v", value)
				assert.Empty(t, issues, "%v", value)
			}

			// A numeric string is no number to a custom rule, which reads its
			// options as they are.
			_, issues := registry.MarkdownlintRuleConfig(id, map[string]any{"depth": "4"})
			require.Len(t, issues, 1)
			assert.Equal(t, lint.OptionIssueWrongType, issues[0].Kind)

			_, issues = registry.MarkdownlintRuleConfig(id, map[string]any{"style": "b", "old_style": "c"})
			require.Len(t, issues, 1)
			assert.Equal(t, lint.OptionIssueConflict, issues[0].Kind)
		})
	}
}

// TestNoOptionsInAMarkdownlintConfiguration covers the note that a rule
// has no options, for an unknown option in a markdownlint configuration:
// single-trailing-newline (MD047) has an option of gomdlint's only, so a
// markdownlint configuration sees none, as markdownlint does, while a
// gomdlint config file sees its option.
func TestNoOptionsInAMarkdownlintConfiguration(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)

	_, issues := registry.MarkdownlintRuleConfig("MD047", map[string]any{"max_trailing_blank_lines": 1})
	require.Len(t, issues, 1)
	assert.True(t, issues[0].NoOptions)

	_, issues, _ = registry.NormalizeOptions("MD047", map[string]any{"bogus": 1})
	require.Len(t, issues, 1)
	assert.False(t, issues[0].NoOptions)
}
