package config_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestMarkdownlintRuleConfigEnabled covers whether the value of a rule key
// turns the rule on. getEffectiveConfig in markdownlint 0.41.1 reads it as
// JavaScript does: a falsy value, such as false, null, 0 or "", turns the
// rule off, and an object turns it on unless its enabled key is present and
// falsy, null included.
func TestMarkdownlintRuleConfigEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		enabled bool
	}{
		{name: "true", value: true, enabled: true},
		{name: "false", value: false},
		{name: "null", value: nil},
		{name: "zero", value: float64(0)},
		{name: "zero from YAML", value: 0},
		{name: "one", value: float64(1), enabled: true},
		{name: "empty string", value: ""},
		{name: "error", value: "error", enabled: true},
		{name: "array", value: []any{}, enabled: true},
		{name: "object", value: map[string]any{}, enabled: true},
		{name: "enabled false", value: map[string]any{"enabled": false}},
		{name: "enabled null", value: map[string]any{"enabled": nil}},
		{name: "enabled zero", value: map[string]any{"enabled": float64(0)}},
		{name: "enabled empty", value: map[string]any{"enabled": ""}},
		{name: "enabled string", value: map[string]any{"enabled": "no"}, enabled: true},
		{name: "enabled one", value: map[string]any{"enabled": float64(1)}, enabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := config.MarkdownlintRuleConfig("MD009", tt.value, nil)

			require.NotNil(t, got.Enabled)
			assert.Equal(t, tt.enabled, *got.Enabled)
			assert.NotContains(t, got.Options, "enabled", "enabled is no option")
		})
	}
}

// TestMarkdownlintDefaultRuleConfig covers the settings that markdownlint's
// default key gives a rule. getEffectiveConfig in markdownlint 0.41.1 turns
// the rules on when the value is truthy in JavaScript, and makes them
// warnings for "warning" and errors otherwise. It takes nothing else from
// the value, so an object turns the rules on whatever it holds, and the
// rule keeps markdownlint's option defaults: without a normalizer, which
// gives the options that every setting writes out, the settings hold no
// option.
func TestMarkdownlintDefaultRuleConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		enabled  bool
		severity string
	}{
		{name: "true", value: true, enabled: true},
		{name: "false", value: false},
		{name: "null", value: nil},
		{name: "zero", value: float64(0)},
		{name: "zero from YAML", value: 0},
		{name: "NaN", value: math.NaN()},
		{name: "one", value: float64(1), enabled: true},
		{name: "empty string", value: ""},
		{name: "string", value: "off", enabled: true},
		{name: "warning", value: "warning", enabled: true, severity: "warning"},
		{name: "object", value: map[string]any{"enabled": false, "severity": "warning", "br_spaces": 5}, enabled: true},
		{name: "array", value: []any{}, enabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := config.MarkdownlintDefaultRuleConfig("MD009", tt.value, nil)

			require.NotNil(t, got.Enabled)
			assert.Equal(t, tt.enabled, *got.Enabled)
			require.NotNil(t, got.Severity)
			if tt.severity == "" {
				tt.severity = "error"
			}
			assert.Equal(t, tt.severity, *got.Severity)
			assert.Nil(t, got.Options)
			assert.Nil(t, got.AutoFix)
		})
	}
}

// TestMarkdownlintRuleConfig covers the settings that a rule's value in a
// markdownlint configuration translates to: markdownlint reports every
// finding as an error, and the value sets the rule's state, its severity
// and its options.
func TestMarkdownlintRuleConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		enabled  bool
		severity string
		options  map[string]any
	}{
		{name: "true", value: true, enabled: true, severity: "error"},
		{name: "false", value: false, enabled: false, severity: "error"},
		{name: "null", value: nil, enabled: false, severity: "error"},
		{name: "warning", value: "warning", enabled: true, severity: "warning"},
		{name: "error", value: "error", enabled: true, severity: "error"},
		{name: "empty object", value: map[string]any{}, enabled: true, severity: "error"},
		{
			name: "object", value: map[string]any{"enabled": false, "severity": "warning", "line_length": 100},
			enabled: false, severity: "warning", options: map[string]any{"line_length": 100},
		},
		{
			name: "enabled and severity of other types", value: map[string]any{"enabled": "no", "severity": 2},
			enabled: true, severity: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := config.MarkdownlintRuleConfig("MD013", tt.value, nil)
			require.NotNil(t, got.Enabled)
			require.NotNil(t, got.Severity)
			assert.Equal(t, tt.enabled, *got.Enabled)
			assert.Equal(t, tt.severity, *got.Severity)
			assert.Equal(t, tt.options, got.Options)
			assert.Nil(t, got.AutoFix)
		})
	}
}

// TestMarkdownlintRuleConfigSetsOnlyWrittenOptions checks that, without a
// normalizer, the translation adds no option that the value leaves out: the
// options that every setting writes out come from the normalizer.
func TestMarkdownlintRuleConfigSetsOnlyWrittenOptions(t *testing.T) {
	t.Parallel()

	for _, ruleID := range []string{"MD001", "MD004", "MD009", "MD013", "MD022", "MD025", "MD029", "MD033", "MD046", "MD048", "MD059", "MD060"} {
		assert.Nil(t, config.MarkdownlintRuleConfig(ruleID, true, nil).Options, ruleID)
		assert.Equal(t, map[string]any{"style": "dash"},
			config.MarkdownlintRuleConfig(ruleID, map[string]any{"style": "dash"}, nil).Options, ruleID)
	}
}

// TestMarkdownlintRuleConfigNormalizesOptions checks that the options pass
// through the normalizer, which gets the rule's ID, that options it leaves
// empty are no options, and that a value without options asks it for the
// options that every setting of the rule writes out.
func TestMarkdownlintRuleConfigNormalizesOptions(t *testing.T) {
	t.Parallel()

	var gotID string
	var gotOptions map[string]any
	normalize := func(ruleID string, options map[string]any) map[string]any {
		gotID, gotOptions = ruleID, options
		return map[string]any{"max": options["line_length"]}
	}

	got := config.MarkdownlintRuleConfig("MD013", map[string]any{"line_length": 100, "enabled": true}, normalize)

	assert.Equal(t, "MD013", gotID)
	assert.Equal(t, map[string]any{"line_length": 100}, gotOptions, "enabled and severity are no options")
	assert.Equal(t, map[string]any{"max": 100}, got.Options)

	empty := config.MarkdownlintRuleConfig("MD013", map[string]any{"bogus": 1},
		func(string, map[string]any) map[string]any { return map[string]any{} })
	assert.Nil(t, empty.Options)

	pins := func(_ string, options map[string]any) map[string]any {
		assert.Empty(t, options, "the value writes no options")
		return map[string]any{"max": 80}
	}
	for _, value := range []any{true, false, nil, "warning", float64(0)} {
		assert.Equal(t, map[string]any{"max": 80}, config.MarkdownlintRuleConfig("MD013", value, pins).Options, "%#v", value)
		assert.Equal(t, map[string]any{"max": 80}, config.MarkdownlintDefaultRuleConfig("MD013", value, pins).Options,
			"default %#v", value)
	}
	assert.Equal(t, map[string]any{"max": 80},
		config.MarkdownlintDefaultRuleConfig("MD013", map[string]any{"line_length": 100}, pins).Options,
		"the default key writes no options of its own")
}
