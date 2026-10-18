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
// every other key may be an option written directly under the rule key,
// the flat shape that markdownlint uses. Only the options that the rule
// declares tell which of those keys are options, so decoding keeps them
// unread, values and all, for ResolveFlatOptions: the value of a key that
// is no option is never decoded, as before the flat shape was read. A key
// that is a setting is always the setting, so an option spelled like one
// can be set only under options.
//
// Two keys in one mapping that read as one name, such as a key and an
// alias of it, are an error, directly under the rule key and under
// options: YAML rejects a key written twice, but compares an alias key by
// its anchor, so decoding would keep the later value silently.
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

		entries, err := takenEntries(value, "")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			switch {
			case entry.name == "options":
				// Decoding has read the options; this checks their keys.
				if !plainMapping(entry.value) {
					if _, err := takenEntries(entry.value, "options."); err != nil {
						return err
					}
				}
			case !isRuleSettingKey(entry.name):
				cfg.flat = append(cfg.flat, entry)
			}
		}
		*rc = cfg
		return nil
	default:
		return &yaml.TypeError{Errors: []string{fmt.Sprintf(
			"line %d: rule settings must be true, false or a mapping, not %s", value.Line, describeNode(value))}}
	}
}

// FlatOptionConflictError is the error of an option written both directly
// under the rule key and under options, under the same name: either value
// could win.
type FlatOptionConflictError struct {
	// Name is the name that both write.
	Name string

	// Line is the line of the key written directly under the rule key.
	Line int
}

// Error describes the conflict in the manner of the error for two
// spellings of one option.
func (e *FlatOptionConflictError) Error() string {
	return fmt.Sprintf("line %d: %q and %q both set option %q; keep one of them",
		e.Line, e.Name, "options."+e.Name, e.Name)
}

// ResolveFlatOptions reads the keys that a configuration file writes
// directly under the rule key, other than the settings, into Options:
// those that reads accepts by name as options of the rule. It drops the
// others without decoding their values, so that a value that cannot be
// decoded, such as one that contains itself, does no harm, and returns
// their names, in order. An option that reads accepts and that Options
// already sets under the same name is a *FlatOptionConflictError. Either
// way, no key is left to read.
func (rc *RuleConfig) ResolveFlatOptions(reads func(name string) bool) ([]string, error) {
	flat := rc.flat
	rc.flat = nil

	var dropped []string
	var options map[string]any
	for _, option := range flat {
		if !reads(option.name) {
			dropped = append(dropped, option.name)
			continue
		}
		if _, nested := rc.Options[option.name]; nested {
			return dropped, &FlatOptionConflictError{Name: option.name, Line: option.key.Line}
		}
		var value any
		if err := option.value.Decode(&value); err != nil {
			return dropped, fmt.Errorf("line %d: option %q: %w", option.key.Line, option.name, err)
		}
		if options == nil {
			// Copies of the settings share their options, so the flat
			// options join a copy.
			options = make(map[string]any, len(rc.Options)+len(flat))
			maps.Copy(options, rc.Options)
		}
		options[option.name] = value
	}
	if options != nil {
		rc.Options = options
	}
	return dropped, nil
}

// mappingEntry is an entry that decoding takes from a mapping: its key's
// name as decoding reads it, and its key and value nodes.
type mappingEntry struct {
	name       string
	key, value *yaml.Node
}

// takenEntries returns the entries that decoding takes from node, a mapping
// or an alias of one, without decoding their values, in the order of
// yaml.v3's precedence: those written in the mapping, then, in turn, those
// of each mapping that its merge key brings in, each before those that its
// own merge key brings in. An entry whose name a previous entry has taken
// is left out, as decoding leaves it out, so a key written in the mapping
// replaces a merged one, and an earlier merged mapping replaces a later
// one. Two keys written in one mapping that read as one name are an error
// that names both, after prefix, such as "options.", at the line of the
// second. A node that is no mapping has no entries.
func takenEntries(node *yaml.Node, prefix string) ([]mappingEntry, error) {
	if plainMapping(node) {
		// Every key is written in the mapping as a string that YAML has
		// checked for repeats, as every mapping that gomdlint writes is.
		node = aliasTarget(node)
		entries := make([]mappingEntry, 0, len(node.Content)/2)
		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			key := node.Content[idx]
			entries = append(entries, mappingEntry{name: key.Value, key: key, value: node.Content[idx+1]})
		}
		return entries, nil
	}

	var entries []mappingEntry
	taken := make(map[string]bool)
	walked := make(map[*yaml.Node]bool)

	var walk func(node *yaml.Node) error
	walk = func(node *yaml.Node) error {
		node = aliasTarget(node)
		// A mapping is walked once, so one that merges itself, which
		// decoding rejects, cannot loop.
		if node == nil || node.Kind != yaml.MappingNode || walked[node] {
			return nil
		}
		walked[node] = true

		written := make(map[string]*yaml.Node, len(node.Content)/2)
		var merge *yaml.Node
		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			key, value := node.Content[idx], node.Content[idx+1]
			if isMergeKey(key) {
				merge = value
				continue
			}
			name, ok := keyName(key)
			if !ok {
				continue // Decoding skips a key that is no string, or fails on it.
			}
			if first, twice := written[name]; twice {
				return &yaml.TypeError{Errors: []string{fmt.Sprintf(
					"line %d: %q and %q both set option %q; keep one of them",
					key.Line, prefix+writtenKey(first), prefix+writtenKey(key), name)}}
			}
			written[name] = key
			if !taken[name] {
				taken[name] = true
				entries = append(entries, mappingEntry{name: name, key: key, value: value})
			}
		}
		if merge == nil {
			return nil
		}
		sources := []*yaml.Node{merge}
		if merge.Kind == yaml.SequenceNode {
			sources = merge.Content
		}
		for _, source := range sources {
			if err := walk(source); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(node); err != nil {
		return nil, err
	}
	return entries, nil
}

// aliasTarget returns the node that node, possibly an alias, stands for.
func aliasTarget(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

// plainMapping reports whether node, or the node it is an alias of, is a
// mapping whose keys are all strings written in it, with no merge key.
// YAML rejects a mapping that writes one key twice, comparing keys by
// their text, which for a string is the name that decoding reads, so no
// two keys of such a mapping read as one name, and nothing else brings
// keys in.
func plainMapping(node *yaml.Node) bool {
	node = aliasTarget(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for idx := 0; idx < len(node.Content); idx += 2 {
		if key := node.Content[idx]; key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
			return false
		}
	}
	return true
}

// isMergeKey reports whether key is a merge key, as decoding tells one:
// the plain scalar <<, or << tagged !!merge. A quoted "<<", like every key
// of a JSON config, is an ordinary key.
func isMergeKey(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && key.Value == "<<" && key.ShortTag() == "!!merge"
}

// keyName returns the name that decoding reads from key: the text of a
// string, and otherwise what the key decodes to as a string, such as the
// scalar that an alias names. It reports false for a key that does not
// decode to a string.
func keyName(key *yaml.Node) (string, bool) {
	if key.Kind == yaml.ScalarNode && key.ShortTag() == "!!str" {
		return key.Value, true
	}
	var name string
	if err := key.Decode(&name); err != nil {
		return "", false
	}
	return name, true
}

// writtenKey returns key as a file writes it: an alias as *anchor, and any
// other key as its text.
func writtenKey(key *yaml.Node) string {
	if key.Kind == yaml.AliasNode {
		return "*" + key.Value
	}
	return key.Value
}

// describeNode names the value of a YAML node for an error message.
func describeNode(node *yaml.Node) string {
	if node.Kind == yaml.ScalarNode {
		return fmt.Sprintf("%s %q", node.ShortTag(), node.Value)
	}
	return node.ShortTag()
}

// FromYAML parses a configuration from YAML bytes. It does not know which
// options the rules declare, so it reads every key written directly under a
// rule key, other than the settings, as an option (see ResolveFlatOptions).
// A configuration loader that knows the rules reads only their options.
func FromYAML(data []byte) (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := cfg.readAllFlatOptions(); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// Ensure Rules map is initialized
	if cfg.Rules == nil {
		cfg.Rules = make(map[string]RuleConfig)
	}

	return cfg, nil
}

// readAllFlatOptions reads every key written directly under a rule key,
// other than the settings, into the rule's options, in rules and
// rule_defaults, rule by rule in order of key, so the first error is the
// same every time.
func (c *Config) readAllFlatOptions() error {
	for _, section := range []struct {
		name  string
		rules map[string]RuleConfig
	}{{"rules", c.Rules}, {"rule_defaults", c.RuleDefaults}} {
		var keys []string
		for key, rule := range section.rules {
			if len(rule.flat) > 0 {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		for _, key := range keys {
			rule := section.rules[key]
			if _, err := rule.ResolveFlatOptions(func(string) bool { return true }); err != nil {
				return fmt.Errorf("%s.%s: %w", section.name, key, err)
			}
			section.rules[key] = rule
		}
	}
	return nil
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

	// Keys not read yet stay unread in the copy. Reading them replaces the
	// slice and never changes the nodes, so the copy can share the nodes.
	clone.flat = slices.Clone(rc.flat)

	return clone
}

// YAMLIndent returns the default YAML indentation.
func YAMLIndent() int {
	return 2
}
