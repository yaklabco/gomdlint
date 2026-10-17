package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestRuleConfig_BooleanShorthand covers markdownlint's shorthand for rule
// settings: true enables a rule and false disables it, under any rule key,
// in YAML and in JSON, which config files may also use.
func TestRuleConfig_BooleanShorthand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		key     string
		enabled bool
	}{
		{name: "name false", content: "rules:\n  line-length: false\n", key: "line-length", enabled: false},
		{name: "name true", content: "rules:\n  line-length: true\n", key: "line-length", enabled: true},
		{name: "ID false", content: "rules:\n  MD013: false\n", key: "MD013", enabled: false},
		{name: "ID true", content: "rules:\n  MD013: true\n", key: "MD013", enabled: true},
		{name: "JSON false", content: `{"rules": {"MD013": false}}`, key: "MD013", enabled: false},
		{name: "JSON true", content: `{"rules": {"line-length": true}}`, key: "line-length", enabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.FromYAML([]byte(tt.content))
			require.NoError(t, err)

			require.Contains(t, cfg.Rules, tt.key)
			rule := cfg.Rules[tt.key]
			require.NotNil(t, rule.Enabled)
			assert.Equal(t, tt.enabled, *rule.Enabled)
			assert.Nil(t, rule.Severity, "the shorthand leaves the severity alone")
			assert.Nil(t, rule.AutoFix, "the shorthand leaves auto-fix alone")
			assert.Nil(t, rule.Options, "the shorthand leaves the options alone")
		})
	}
}

func TestRuleConfig_ShorthandInRuleDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte("rule_defaults:\n  MD033: true\n"))
	require.NoError(t, err)

	require.NotNil(t, cfg.RuleDefaults["MD033"].Enabled)
	assert.True(t, *cfg.RuleDefaults["MD033"].Enabled)
}

// TestRuleConfig_MappingStillDecodes covers the full form next to the
// shorthand.
func TestRuleConfig_MappingStillDecodes(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte(`rules:
  MD013:
    enabled: false
    severity: error
    auto_fix: false
    options:
      max: 100
      ignore_urls: true
  MD009: true
  MD010:
`))
	require.NoError(t, err)

	rule := cfg.Rules["MD013"]
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
	require.NotNil(t, rule.Severity)
	assert.Equal(t, "error", *rule.Severity)
	require.NotNil(t, rule.AutoFix)
	assert.False(t, *rule.AutoFix)
	assert.Equal(t, map[string]any{"max": 100, "ignore_urls": true}, rule.Options)

	require.NotNil(t, cfg.Rules["MD009"].Enabled)
	assert.True(t, *cfg.Rules["MD009"].Enabled)
	assert.Equal(t, config.RuleConfig{}, cfg.Rules["MD010"], "an empty entry changes nothing")
}

// TestRuleConfig_RejectsOtherValues covers rule values that are neither a
// boolean nor a mapping. A YAML 1.1 boolean such as "yes" is a string to
// markdownlint, so it is rejected too.
func TestRuleConfig_RejectsOtherValues(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"maybe", "80", "warning", "yes", "[a, b]", `"false"`} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			_, err := config.FromYAML([]byte("rules:\n  MD013: " + value + "\n"))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "line 2")
			assert.Contains(t, err.Error(), "true, false or a mapping")
		})
	}
}

// TestRuleConfig_ShorthandSurvivesClone covers the YAML round trip that
// Config.Clone uses, which must keep a rule disabled by the shorthand.
func TestRuleConfig_ShorthandSurvivesClone(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte("rules:\n  MD013: false\n"))
	require.NoError(t, err)

	clone := cfg.Clone()

	require.NotNil(t, clone.Rules["MD013"].Enabled)
	assert.False(t, *clone.Rules["MD013"].Enabled)
}

// TestRuleConfig_ShorthandThroughAnAlias covers a YAML alias of a boolean.
func TestRuleConfig_ShorthandThroughAnAlias(t *testing.T) {
	t.Parallel()

	var cfg config.Config
	require.NoError(t, yaml.Unmarshal([]byte("off: &off false\nrules:\n  MD013: *off\n"), &cfg))

	require.NotNil(t, cfg.Rules["MD013"].Enabled)
	assert.False(t, *cfg.Rules["MD013"].Enabled)
}
