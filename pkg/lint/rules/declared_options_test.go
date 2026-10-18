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

// TestRulesRegisteredAloneReadMarkdownlintSpellings registers built-in
// rules one by one, without their catalog entries, as a program that picks
// its own rules does, and lints through the engine: markdownlint's
// spellings of their options still configure them, as they do through
// RegisterAll.
func TestRulesRegisteredAloneReadMarkdownlintSpellings(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(NewMaxLineLengthRule())
	registry.Register(NewDescriptiveLinkTextRule())

	cfg := config.NewConfig()
	cfg.Rules = map[string]config.RuleConfig{
		"MD013": {Options: map[string]any{"line_length": 20}},
		"MD059": {Options: map[string]any{"prohibited_texts": []any{"details"}}},
	}
	input := "# Title\n\nThis line is longer than twenty.\n\n[details](https://example.com/a)\n"

	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
		context.Background(), "test.md", []byte(input), cfg)
	require.NoError(t, err)
	require.Empty(t, result.RuleErrors)

	lines := make(map[string][]int)
	for _, diag := range result.Diagnostics {
		lines[diag.RuleID] = append(lines[diag.RuleID], diag.StartLine)
	}
	assert.Equal(t, []int{3}, lines["MD013"], "line_length sets max, and a line of only a link is exempt")
	assert.Equal(t, []int{5}, lines["MD059"], "prohibited_texts sets patterns")

	for _, resolved := range lint.ResolveRules(registry, cfg) {
		switch resolved.Rule.ID() {
		case "MD013":
			assert.Equal(t, map[string]any{"max": 20}, resolved.Config.Options)
		case "MD059":
			assert.Equal(t, map[string]any{"patterns": []any{"details"}}, resolved.Config.Options)
		}
	}
}

// TestBuiltInRulesDeclareTheirOptions checks that every built-in rule
// declares its options itself, as its catalog entry declares them, so that
// a registry that holds the rule without the entry still knows them.
func TestBuiltInRulesDeclareTheirOptions(t *testing.T) {
	registry := catalogRegistry(t)
	for _, meta := range Catalog() {
		rule, ok := registry.GetByID(meta.ID)
		require.True(t, ok, meta.ID)
		declarer, ok := rule.(lint.OptionDeclarer)
		if !assert.True(t, ok, "%s declares its options", meta.ID) {
			continue
		}
		assert.Equal(t, meta.Options, declarer.DeclaredOptions(), meta.ID)

		alone := lint.NewRegistry()
		alone.Register(rule)
		_, _, declared := alone.NormalizeOptions(meta.ID, map[string]any{"any": 1})
		assert.True(t, declared, "%s registered alone has declared options", meta.ID)
	}
}
