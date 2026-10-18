package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestRuleConfig_WrittenSettingsDecodeOnce checks that the rule settings
// that gomdlint writes hold no key that could be a flat option, so reading
// them back, as Clone does for each file with a markdownlint-configure-file
// comment, decodes each rule's mapping once.
func TestRuleConfig_WrittenSettingsDecodeOnce(t *testing.T) {
	t.Parallel()

	severity := "error"
	cfg := NewConfig()
	cfg.Rules["MD013"] = RuleConfig{
		Enabled: Bool(true), Severity: &severity, AutoFix: Bool(false),
		Options: map[string]any{"max": 100, "enabled": true},
	}
	cfg.Rules["MD009"] = RuleConfig{}
	cfg.RuleDefaults = map[string]RuleConfig{"MD010": {Enabled: Bool(false)}}

	written, err := cfg.ToYAML()
	require.NoError(t, err)
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(written, &doc))

	root := doc.Content[0]
	mappings := 0
	for idx := 0; idx+1 < len(root.Content); idx += 2 {
		if key := root.Content[idx].Value; key != "rules" && key != "rule_defaults" {
			continue
		}
		section := root.Content[idx+1]
		for rule := 0; rule+1 < len(section.Content); rule += 2 {
			mappings++
			assert.False(t, mayHoldFlatOptions(section.Content[rule+1]),
				"%s holds a key that is no rule setting:\n%s", section.Content[rule].Value, written)
		}
	}
	assert.Equal(t, 3, mappings)
}
