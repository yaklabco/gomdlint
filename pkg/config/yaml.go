package config

import (
	"bytes"
	"fmt"
	"maps"

	"gopkg.in/yaml.v3"
)

// ToYAML serializes the configuration to YAML format.
// It produces human-readable output with appropriate formatting.
func (c *Config) ToYAML() ([]byte, error) {
	if c == nil {
		return nil, nil
	}

	// Marshal to YAML
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(c); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}

	return buf.Bytes(), nil
}

// ToYAMLWithHeader serializes the configuration with a header comment.
func (c *Config) ToYAMLWithHeader(header string) ([]byte, error) {
	yamlBytes, err := c.ToYAML()
	if err != nil {
		return nil, err
	}

	if header == "" {
		return yamlBytes, nil
	}

	// Prepend header
	var buf bytes.Buffer
	buf.WriteString(header)
	if header[len(header)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')
	buf.Write(yamlBytes)

	return buf.Bytes(), nil
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
		Backups:         c.Backups, // BackupsConfig only has value types
		Fix:             c.Fix,
		DryRun:          c.DryRun,
		Format:          c.Format,
		RuleFormat:      c.RuleFormat,
		Jobs:            c.Jobs,
		NoBackups:       c.NoBackups,
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
