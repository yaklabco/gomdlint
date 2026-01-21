package config

import (
	"bytes"
	"fmt"

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

	// Use YAML round-trip for deep copy
	yamlBytes, err := c.ToYAML()
	if err != nil {
		// Fallback to shallow copy on error
		clone := *c
		return &clone
	}

	clone, err := FromYAML(yamlBytes)
	if err != nil {
		// Fallback to shallow copy on error
		shallow := *c
		return &shallow
	}

	return clone
}

// YAMLIndent returns the default YAML indentation.
func YAMLIndent() int {
	return 2
}
