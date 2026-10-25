package config_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestMarkdownlintRuleConfigResetsOptions covers which settings of a rule
// in a markdownlint configuration replace the options that lower layers
// give the rule. getEffectiveConfig in markdownlint 0.41.1 gives a rule
// that a key turns on the options of the key's value and nothing else, so
// true, a severity and an object each leave every other option at its
// default. A setting that turns the rule off keeps the options of lower
// layers, as before.
func TestMarkdownlintRuleConfigResetsOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		reset bool
	}{
		{name: "true", value: true, reset: true},
		{name: "warning", value: "warning", reset: true},
		{name: "error", value: "error", reset: true},
		{name: "another string", value: "off", reset: true},
		{name: "one", value: float64(1), reset: true},
		{name: "array", value: []any{}, reset: true},
		{name: "empty object", value: map[string]any{}, reset: true},
		{name: "object with options", value: map[string]any{"line_length": 100}, reset: true},
		{name: "object enabled", value: map[string]any{"enabled": true, "severity": "warning"}, reset: true},
		{name: "false", value: false},
		{name: "null", value: nil},
		{name: "zero", value: float64(0)},
		{name: "empty string", value: ""},
		{name: "object disabled", value: map[string]any{"enabled": false, "line_length": 100}},
		{name: "object enabled zero", value: map[string]any{"enabled": float64(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := config.MarkdownlintRuleConfig("MD013", tt.value, nil)

			assert.Equal(t, tt.reset, got.ResetOptions)
		})
	}
}

// TestMarkdownlintDefaultRuleConfigKeepsOptions covers the default key of a
// markdownlint configuration, which gives every rule its state and
// severity: it replaces no option of a lower layer, whatever it holds.
func TestMarkdownlintDefaultRuleConfigKeepsOptions(t *testing.T) {
	t.Parallel()

	for _, value := range []any{true, false, nil, "warning", "error", float64(1), map[string]any{"br_spaces": 5}} {
		assert.False(t, config.MarkdownlintDefaultRuleConfig("MD009", value, nil).ResetOptions, "%#v", value)
	}
}

// TestRuleConfig_ResetOptionsSetting covers the reset_options setting of a
// rule in a gomdlint config file, which a migrated markdownlint config
// writes for a rule that markdownlint's true, a severity or an object turns
// on: it decodes with the other settings, is written only when set, and
// survives Clone. The shorthands of gomdlint's own config files keep their
// meaning and reset nothing.
func TestRuleConfig_ResetOptionsSetting(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte(`rules:
  MD013:
    enabled: true
    reset_options: true
    options:
      max: 60
  MD009: true
  MD010: warning
rule_defaults:
  MD033:
    reset_options: true
`))
	require.NoError(t, err)

	assert.True(t, cfg.Rules["MD013"].ResetOptions)
	assert.Equal(t, map[string]any{"max": 60}, cfg.Rules["MD013"].Options, "reset_options is no option")
	assert.False(t, cfg.Rules["MD009"].ResetOptions, "true resets nothing in gomdlint's own config")
	assert.False(t, cfg.Rules["MD010"].ResetOptions, "a severity resets nothing in gomdlint's own config")
	assert.True(t, cfg.RuleDefaults["MD033"].ResetOptions)
	assert.Contains(t, config.RuleSettingKeys(), "reset_options")

	clone := cfg.Clone()
	assert.True(t, clone.Rules["MD013"].ResetOptions)
	assert.False(t, clone.Rules["MD009"].ResetOptions)
	assert.True(t, clone.RuleDefaults["MD033"].ResetOptions)

	written, err := cfg.ToYAML()
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(written), "reset_options: true"), string(written))
	assert.NotContains(t, string(written), "reset_options: false", "an unset reset_options is left out")
}
