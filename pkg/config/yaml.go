package config

import (
	"bytes"
	"fmt"
	"maps"
	"slices"

	"gopkg.in/yaml.v3"
)

// ToYAML serializes the configuration to YAML format.
// It produces human-readable output with appropriate formatting.
func (c *Config) ToYAML() ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	return encodeYAML(c)
}

// encodeYAML serializes value to YAML, indented by two spaces.
func encodeYAML(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(YAMLIndent())

	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}

	return buf.Bytes(), nil
}

// ruleSettings holds the rule settings of a configuration, under the keys
// that Config gives them.
type ruleSettings struct {
	Rules        map[string]RuleConfig `yaml:"rules,omitempty"`
	RuleDefaults map[string]RuleConfig `yaml:"rule_defaults,omitempty"`
}

// RuleSettingsToYAML serializes the rule settings of the configuration,
// rules and rule_defaults, and nothing else, after header unless it is
// empty. It suits a configuration that sets nothing but rules, such as a
// migrated markdownlint configuration: a file that holds it leaves every
// other setting to the layers below it, where ToYAML would write settings
// it does not set, such as an empty ignore list.
func (c *Config) RuleSettingsToYAML(header string) ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	yamlBytes, err := encodeYAML(ruleSettings{Rules: c.Rules, RuleDefaults: c.RuleDefaults})
	if err != nil {
		return nil, err
	}
	return withHeader(header, yamlBytes), nil
}

// RuleSettingsToJSON is RuleSettingsToYAML in JSON, which has no comments,
// and so no header.
func (c *Config) RuleSettingsToJSON() ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	yamlBytes, err := c.RuleSettingsToYAML("")
	if err != nil {
		return nil, err
	}
	return templateToJSON(yamlBytes)
}

// ToJSON serializes the configuration to indented JSON: the settings ToYAML
// writes, under the same keys, so a JSON config file reads back the same.
// JSON has no comments.
func (c *Config) ToJSON() ([]byte, error) {
	if c == nil {
		return nil, nil
	}

	yamlBytes, err := c.ToYAML()
	if err != nil {
		return nil, err
	}
	return templateToJSON(yamlBytes)
}

// ToYAMLWithHeader serializes the configuration with a header comment.
func (c *Config) ToYAMLWithHeader(header string) ([]byte, error) {
	yamlBytes, err := c.ToYAML()
	if err != nil {
		return nil, err
	}
	return withHeader(header, yamlBytes), nil
}

// withHeader returns yamlBytes after header and a blank line, or yamlBytes
// itself when header is empty.
func withHeader(header string, yamlBytes []byte) []byte {
	if header == "" {
		return yamlBytes
	}

	var buf bytes.Buffer
	buf.WriteString(header)
	if header[len(header)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')
	buf.Write(yamlBytes)

	return buf.Bytes()
}

// UnmarshalYAML decodes the settings of one rule: a mapping, or
// markdownlint's shorthand of a bare boolean, which enables (true) or
// disables (false) the rule and leaves its other settings alone. Strings
// such as "yes" are not booleans here, as in markdownlint.
//
// In a mapping, the keys of RuleSettingKeys are the rule's settings, and
// every other key is an option written directly under the rule key, the
// flat shape that markdownlint uses. The flat options join the options
// mapping, so a mapping may write options either way, or both. A key that
// is a setting is always the setting, so an option spelled like one can be
// set only under options. An option written both flat and under options is
// an error, since either value could win. Two spellings of one option,
// such as markdownlint's and gomdlint's, decode side by side; the rule's
// declared options tell that they are one.
//
// YAML is a superset of JSON, so JSON config files get the shorthand and
// the flat shape too.
func (rc *RuleConfig) UnmarshalYAML(value *yaml.Node) error {
	switch {
	case value.Kind == yaml.ScalarNode && value.ShortTag() == "!!bool":
		var enabled bool
		if err := value.Decode(&enabled); err != nil {
			return fmt.Errorf("decode rule shorthand: %w", err)
		}
		*rc = RuleConfig{Enabled: &enabled}
		return nil
	case value.Kind == yaml.MappingNode:
		// The settings type without this method decodes the mapping as
		// usual, field by field, and skips the other keys.
		type settings RuleConfig
		var decoded settings
		if err := value.Decode(&decoded); err != nil {
			return err //nolint:wrapcheck // yaml.v3 reports a *yaml.TypeError with the document's other errors only when it is returned unwrapped.
		}
		cfg := RuleConfig(decoded)
		if err := cfg.addFlatOptions(value); err != nil {
			return err
		}
		*rc = cfg
		return nil
	default:
		return &yaml.TypeError{Errors: []string{fmt.Sprintf(
			"line %d: rule settings must be true, false or a mapping, not %s", value.Line, describeNode(value))}}
	}
}

// addFlatOptions adds the options that mapping, the settings of the rule,
// writes directly under the rule key to rc's options: every key that
// decoding reads from mapping, after YAML's merge keys and aliases, that is
// not a rule setting. An option that rc's options set already is an error
// at the line of the flat key.
func (rc *RuleConfig) addFlatOptions(mapping *yaml.Node) error {
	if !mayHoldFlatOptions(mapping) {
		return nil
	}
	var entries map[string]any
	if err := mapping.Decode(&entries); err != nil {
		return err //nolint:wrapcheck // yaml.v3 reports a *yaml.TypeError with the document's other errors only when it is returned unwrapped.
	}

	// Sorted keys keep the error message stable.
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		if isRuleSettingKey(key) {
			continue
		}
		if _, nested := rc.Options[key]; nested {
			return &yaml.TypeError{Errors: []string{fmt.Sprintf(
				"line %d: %q and %q both set option %q; keep one of them",
				keyLine(mapping, key), key, "options."+key, key)}}
		}
		if rc.Options == nil {
			rc.Options = make(map[string]any, len(entries))
		}
		rc.Options[key] = entries[key]
	}
	return nil
}

// mayHoldFlatOptions reports whether mapping, the settings of a rule, may
// hold a key that is not a rule setting: whether it writes one, or a key
// whose name only decoding tells, such as an alias, or a merge key, which
// may bring one in. The settings that gomdlint writes, which Clone reads
// back, hold none, so they decode once.
func mayHoldFlatOptions(mapping *yaml.Node) bool {
	for idx := 0; idx+1 < len(mapping.Content); idx += 2 {
		key := mapping.Content[idx]
		if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" || !isRuleSettingKey(key.Value) {
			return true
		}
	}
	return false
}

// keyLine returns the line of the entry of mapping whose key reads as name,
// or the line of mapping itself when the entry is not written there, as
// when a merge key brings it in.
func keyLine(mapping *yaml.Node, name string) int {
	for idx := 0; idx+1 < len(mapping.Content); idx += 2 {
		var key string
		if mapping.Content[idx].Decode(&key) == nil && key == name {
			return mapping.Content[idx].Line
		}
	}
	return mapping.Line
}

// describeNode names the value of a YAML node for an error message.
func describeNode(node *yaml.Node) string {
	if node.Kind == yaml.ScalarNode {
		return fmt.Sprintf("%s %q", node.ShortTag(), node.Value)
	}
	return node.ShortTag()
}

// FromYAML parses a configuration from YAML bytes.
func FromYAML(data []byte) (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// Ensure Rules map is initialized
	if cfg.Rules == nil {
		cfg.Rules = make(map[string]RuleConfig)
	}

	return cfg, nil
}

// Clone creates a deep copy of the configuration.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}

	// Use YAML round-trip for deep copy of serializable fields
	yamlBytes, err := c.ToYAML()
	if err != nil {
		// Fallback to manual deep copy on error
		return c.deepCopy()
	}

	clone, err := FromYAML(yamlBytes)
	if err != nil {
		// Fallback to manual deep copy on error
		return c.deepCopy()
	}

	// Copy CLI-only fields that aren't serialized to YAML
	c.copyCLIFields(clone)

	return clone
}

// copyCLIFields copies CLI-only fields (yaml:"-") to the target config.
func (c *Config) copyCLIFields(target *Config) {
	target.Fix = c.Fix
	target.DryRun = c.DryRun
	target.Format = c.Format
	target.RuleFormat = c.RuleFormat
	target.Jobs = c.Jobs
	target.NoBackups = c.NoBackups

	// Deep copy CLI-only slices
	if c.EnableRules != nil {
		target.EnableRules = make([]string, len(c.EnableRules))
		copy(target.EnableRules, c.EnableRules)
	}
	if c.DisableRules != nil {
		target.DisableRules = make([]string, len(c.DisableRules))
		copy(target.DisableRules, c.DisableRules)
	}
	if c.FixRules != nil {
		target.FixRules = make([]string, len(c.FixRules))
		copy(target.FixRules, c.FixRules)
	}
}

// deepCopy creates a manual deep copy of the configuration.
// This is used as a fallback when YAML round-trip fails.
func (c *Config) deepCopy() *Config {
	clone := &Config{
		Flavor:          c.Flavor,
		SeverityDefault: c.SeverityDefault,
		Backups:         BackupsConfig{Mode: c.Backups.Mode},
		Fix:             c.Fix,
		DryRun:          c.DryRun,
		Format:          c.Format,
		RuleFormat:      c.RuleFormat,
		Jobs:            c.Jobs,
		NoBackups:       c.NoBackups,
	}

	if c.Backups.Enabled != nil {
		clone.Backups.Enabled = Bool(*c.Backups.Enabled)
	}
	if c.Gitignore != nil {
		clone.Gitignore = Bool(*c.Gitignore)
	}
	if c.DefaultIgnores != nil {
		clone.DefaultIgnores = Bool(*c.DefaultIgnores)
	}

	// Deep copy Ignore slice
	if c.Ignore != nil {
		clone.Ignore = make([]string, len(c.Ignore))
		copy(clone.Ignore, c.Ignore)
	}

	// Deep copy Rules map
	if c.Rules != nil {
		clone.Rules = make(map[string]RuleConfig, len(c.Rules))
		for k, v := range c.Rules {
			clone.Rules[k] = v.clone()
		}
	}

	// Deep copy rule defaults.
	if c.RuleDefaults != nil {
		clone.RuleDefaults = make(map[string]RuleConfig, len(c.RuleDefaults))
		for key, value := range c.RuleDefaults {
			clone.RuleDefaults[key] = value.clone()
		}
	}

	// Deep copy EnableRules slice
	if c.EnableRules != nil {
		clone.EnableRules = make([]string, len(c.EnableRules))
		copy(clone.EnableRules, c.EnableRules)
	}

	// Deep copy DisableRules slice
	if c.DisableRules != nil {
		clone.DisableRules = make([]string, len(c.DisableRules))
		copy(clone.DisableRules, c.DisableRules)
	}

	// Deep copy FixRules slice
	if c.FixRules != nil {
		clone.FixRules = make([]string, len(c.FixRules))
		copy(clone.FixRules, c.FixRules)
	}

	return clone
}

// clone creates a deep copy of a RuleConfig.
func (rc RuleConfig) clone() RuleConfig {
	clone := RuleConfig{}

	if rc.Enabled != nil {
		enabled := *rc.Enabled
		clone.Enabled = &enabled
	}

	if rc.Severity != nil {
		severity := *rc.Severity
		clone.Severity = &severity
	}

	if rc.AutoFix != nil {
		autoFix := *rc.AutoFix
		clone.AutoFix = &autoFix
	}

	if rc.Options != nil {
		clone.Options = make(map[string]any, len(rc.Options))
		maps.Copy(clone.Options, rc.Options) // Note: nested maps/slices in Options are not deep copied
	}

	return clone
}

// YAMLIndent returns the default YAML indentation.
func YAMLIndent() int {
	return 2
}
