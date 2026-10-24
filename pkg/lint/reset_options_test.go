package lint_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// lineLengthID is the ID of line-length, which the tests of resetting
// options configure.
const lineLengthID = "MD013"

// TestRegistryMarkdownlintRuleConfigResetsOptions covers a markdownlint
// setting that turns a rule on: true, a severity or an object. In
// markdownlint 0.41.1 it gives the rule the options it writes and leaves
// every other option at its default, so the translation resets the options
// that lower layers give the rule and writes only its own. A setting that
// turns the rule off, and the default key, reset nothing (see
// TestRegistryMarkdownlintRuleConfigWritesPinnedOptions).
func TestRegistryMarkdownlintRuleConfigResetsOptions(t *testing.T) {
	t.Parallel()

	registry := pinnedRegistry()

	for _, value := range []any{true, "warning", "error", float64(1), map[string]any{}, map[string]any{"enabled": true}} {
		cfg, issues := registry.MarkdownlintRuleConfig("MD033", value)
		assert.True(t, cfg.ResetOptions, "%#v", value)
		assert.Nil(t, cfg.Options, "%#v: the reset leaves every option at its default", value)
		assert.Empty(t, issues, "%#v", value)
	}

	cfg, issues := registry.MarkdownlintRuleConfig("MD033", map[string]any{"table_allowed_elements": []any{"td"}})
	assert.True(t, cfg.ResetOptions)
	assert.Equal(t, map[string]any{"table_allowed_elements": []any{"td"}}, cfg.Options)
	assert.Empty(t, issues)

	for _, value := range []any{false, nil, "", float64(0), map[string]any{"enabled": false}} {
		cfg, _ := registry.MarkdownlintRuleConfig("MD033", value)
		assert.False(t, cfg.ResetOptions, "%#v", value)
	}
	for _, value := range []any{true, false, "warning", map[string]any{}} {
		assert.False(t, registry.MarkdownlintDefaultRuleConfig("MD033", value).ResetOptions, "default key %#v", value)
	}
}

// TestResolveRulesResetOptions covers a rule's settings under rules that
// reset its options: the options under rule_defaults do not apply, and the
// rule reads the options of its own settings alone.
func TestResolveRulesResetOptions(t *testing.T) {
	t.Parallel()

	registry := optionsRegistry()
	cfg := config.NewConfig()
	cfg.RuleDefaults = map[string]config.RuleConfig{lineLengthID: {Options: map[string]any{"max": 20, "style": "plus"}}}
	optionsFor := func(rule config.RuleConfig) map[string]any {
		cfg.Rules = map[string]config.RuleConfig{lineLengthID: rule}
		for _, resolved := range lint.ResolveRules(registry, cfg) {
			if resolved.Rule.ID() == lineLengthID {
				return resolved.Config.Options
			}
		}
		t.Fatal("MD013 does not run")
		return nil
	}

	assert.Equal(t, map[string]any{"max": 20, "style": "plus", "ignore_tables": true},
		optionsFor(config.RuleConfig{Options: map[string]any{"tables": false}}), "rules override rule_defaults by option")
	assert.Equal(t, map[string]any{"ignore_tables": true},
		optionsFor(config.RuleConfig{ResetOptions: true, Options: map[string]any{"tables": false}}), "the reset drops rule_defaults")
	assert.Empty(t, optionsFor(config.RuleConfig{ResetOptions: true}), "with nothing set, every option keeps its default")
}

// TestEngineConfigureFileResetsOptions covers a markdownlint-configure-file
// comment that turns line-length (MD013) on, over a config whose
// rule_defaults or rules give it a heading limit of 20. As in markdownlint,
// where the comment's setting replaces the configuration's, the rule then
// takes markdownlint's defaults for every option the comment does not set,
// so the heading of 29 characters is within the limit. A configure-file key
// has always replaced the settings under rules; the reset keeps
// rule_defaults out too.
func TestEngineConfigureFileResetsOptions(t *testing.T) {
	t.Parallel()

	const heading = "# A heading of thirty letters\n"
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry)
	headingLimit := map[string]any{"heading_max": 20}
	configs := map[string]*config.Config{
		"rule_defaults": {RuleDefaults: map[string]config.RuleConfig{lineLengthID: {Options: headingLimit}}},
		"rules":         {Rules: map[string]config.RuleConfig{lineLengthID: {Options: headingLimit}}},
	}
	lineLengthFindings := func(t *testing.T, cfg *config.Config, input string) int {
		t.Helper()
		result, err := engine.LintFile(context.Background(), "test.md", []byte(input), cfg)
		require.NoError(t, err)
		count := 0
		for _, diag := range result.Diagnostics {
			if diag.RuleID == lineLengthID {
				count++
			}
		}
		return count
	}

	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, 1, lineLengthFindings(t, cfg, heading), "the heading is over the configured limit")
			for _, setting := range []string{`true`, `"warning"`, `{}`, `{"strict": false}`} {
				input := heading + "\n<!-- markdownlint-configure-file {\"MD013\": " + setting + "} -->\n"
				assert.Zero(t, lineLengthFindings(t, cfg, input), "%s resets the heading limit", setting)
			}
		})
	}
}
