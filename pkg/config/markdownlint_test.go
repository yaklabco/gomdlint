package config_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestMarkdownlintDefaultRuleConfig covers the settings that markdownlint's
// default key gives a rule. getEffectiveConfig in markdownlint 0.41.1 turns
// the rules on when the value is truthy in JavaScript, and makes them
// warnings for "warning" and errors otherwise. It takes nothing else from
// the value, so an object turns the rules on whatever it holds, and the
// rule keeps markdownlint's option defaults.
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

			got := config.MarkdownlintDefaultRuleConfig("MD009", tt.value)

			require.NotNil(t, got.Enabled)
			assert.Equal(t, tt.enabled, *got.Enabled)
			require.NotNil(t, got.Severity)
			if tt.severity == "" {
				tt.severity = "error"
			}
			assert.Equal(t, tt.severity, *got.Severity)
			assert.Equal(t, map[string]any{"ignore_code_blocks": true, "br_spaces": 2}, got.Options)
			assert.Nil(t, got.AutoFix)
		})
	}
}
