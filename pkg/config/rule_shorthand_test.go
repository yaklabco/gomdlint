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
// in YAML and in JSON, which config files may also use. A string is the
// other shorthand, a severity (see TestRuleConfig_SeverityShorthand).
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
// boolean, a string nor a mapping, such as a number or a list. A string is
// a severity (see TestRuleConfig_SeverityShorthand). A scalar that YAML
// reads as another type, such as one with a binary or a custom tag, is no
// string here, though the severity setting would read its text.
func TestRuleConfig_RejectsOtherValues(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"80", "1.5", "2001-12-14", "[a, b]", "[warning]", "!!binary d2FybmluZw==", "!custom warning",
	} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			_, err := config.FromYAML([]byte("rules:\n  MD013: " + value + "\n"))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "line 2")
			assert.Contains(t, err.Error(), "true, false, a severity (error, warning or info) or a mapping")
		})
	}
}

// TestRuleConfig_SeverityShorthand covers markdownlint's shorthand of a
// severity as a rule's value, error or warning, and gomdlint's own info,
// under any rule key, in YAML and in JSON, under rules and rule_defaults.
// As in markdownlint, it turns the rule on at that severity, so it
// decodes exactly as the mapping that sets enabled to true and the
// severity does, and leaves the other settings alone.
func TestRuleConfig_SeverityShorthand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		shorthand string
		mapping   string
		section   func(*config.Config) map[string]config.RuleConfig
		key       string
		severity  string
	}{
		{
			name:      "ID error",
			shorthand: "rules:\n  MD013: error\n",
			mapping:   "rules:\n  MD013:\n    enabled: true\n    severity: error\n",
			section:   rulesSection, key: "MD013", severity: "error",
		},
		{
			name:      "name warning",
			shorthand: "rules:\n  line-length: warning\n",
			mapping:   "rules:\n  line-length:\n    enabled: true\n    severity: warning\n",
			section:   rulesSection, key: "line-length", severity: "warning",
		},
		{
			name:      "markdownlint name info",
			shorthand: "rules:\n  ul-style: info\n",
			mapping:   "rules:\n  ul-style:\n    enabled: true\n    severity: info\n",
			section:   rulesSection, key: "ul-style", severity: "info",
		},
		{
			name:      "quoted",
			shorthand: "rules:\n  MD013: 'warning'\n",
			mapping:   "rules:\n  MD013:\n    enabled: true\n    severity: 'warning'\n",
			section:   rulesSection, key: "MD013", severity: "warning",
		},
		{
			name:      "JSON",
			shorthand: `{"rules": {"line-length": "error"}}`,
			mapping:   `{"rules": {"line-length": {"enabled": true, "severity": "error"}}}`,
			section:   rulesSection, key: "line-length", severity: "error",
		},
		{
			name:      "rule_defaults",
			shorthand: "rule_defaults:\n  MD009: info\n",
			mapping:   "rule_defaults:\n  MD009:\n    enabled: true\n    severity: info\n",
			section:   ruleDefaultsSection, key: "MD009", severity: "info",
		},
		{
			name:      "through an alias",
			shorthand: "ignore: [&level warning]\nrules:\n  MD013: *level\n",
			mapping:   "ignore: [&level warning]\nrules:\n  MD013:\n    enabled: true\n    severity: *level\n",
			section:   rulesSection, key: "MD013", severity: "warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.FromYAML([]byte(tt.shorthand))
			require.NoError(t, err)

			require.Contains(t, tt.section(cfg), tt.key)
			rule := tt.section(cfg)[tt.key]
			require.NotNil(t, rule.Severity)
			assert.Equal(t, tt.severity, *rule.Severity)
			require.NotNil(t, rule.Enabled)
			assert.True(t, *rule.Enabled, "the shorthand turns the rule on")
			assert.Nil(t, rule.AutoFix, "the shorthand leaves auto-fix alone")
			assert.Nil(t, rule.Options, "the shorthand leaves the options alone")

			mapping, err := config.FromYAML([]byte(tt.mapping))
			require.NoError(t, err)
			assert.Equal(t, mapping, cfg, "the shorthand reads as enabled: true with the severity")

			var decoded config.Config
			require.NoError(t, yaml.Unmarshal([]byte(tt.shorthand), &decoded))
			assert.Equal(t, rule, tt.section(&decoded)[tt.key], "yaml.Unmarshal reads the shorthand too")
		})
	}
}

// TestRuleConfig_SeverityShorthandReadsLikeTheSetting covers a string that
// is no severity gomdlint knows. It decodes as the severity setting that
// holds it does, and loading rejects both alike when it validates the
// configuration (see the configuration loader's tests), so decoding
// accepts any string here, as it does under severity.
func TestRuleConfig_SeverityShorthandReadsLikeTheSetting(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"bogus", "Warning", "off", "yes", `"false"`, "''"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			shorthand, err := config.FromYAML([]byte("rules:\n  MD013: " + value + "\n"))
			require.NoError(t, err)
			mapping, err := config.FromYAML([]byte("rules:\n  MD013:\n    enabled: true\n    severity: " + value + "\n"))
			require.NoError(t, err)

			assert.Equal(t, mapping, shorthand)
			require.NotNil(t, shorthand.Rules["MD013"].Severity)
		})
	}
}

// TestRuleConfig_SeverityShorthandSurvivesClone covers the YAML round trip
// that Config.Clone uses, which must keep the severity that the shorthand
// sets.
func TestRuleConfig_SeverityShorthandSurvivesClone(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte("rules:\n  MD013: info\n"))
	require.NoError(t, err)

	clone := cfg.Clone()

	assert.Equal(t, settled(cfg.Rules), settled(clone.Rules))
	require.NotNil(t, clone.Rules["MD013"].Severity)
	assert.Equal(t, "info", *clone.Rules["MD013"].Severity)
	require.NotNil(t, clone.Rules["MD013"].Enabled)
	assert.True(t, *clone.Rules["MD013"].Enabled)
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
