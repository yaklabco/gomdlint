package config

import (
	"bytes"
	"cmp"
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

// UnmarshalYAML decodes the settings of one rule: a mapping of the settings
// that RuleSettingKeys names, or markdownlint's shorthand of a bare
// boolean, which enables (true) or disables (false) the rule and leaves its
// other settings alone. Strings such as "yes" are not booleans here, as in
// markdownlint.
//
// A mapping may also write options directly under the rule key, the flat
// shape that markdownlint uses. Only the options that the rule declares
// tell which of those keys are options, so decoding leaves them alone and
// never decodes their values: ReadFlatOptions reads them from the parsed
// document, as FromYAML and the configuration loader do. A key that is a
// setting is always the setting, so an option spelled like one can be set
// only under options.
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
		if err := checkRepeatedKeys(value); err != nil {
			return err
		}
		*rc = RuleConfig(decoded)
		return nil
	default:
		return &yaml.TypeError{Errors: []string{fmt.Sprintf(
			"line %d: rule settings must be true, false or a mapping, not %s", value.Line, describeNode(value))}}
	}
}

// checkRepeatedKeys returns a *yaml.TypeError for two keys that read as one
// name in mapping, the settings of a rule, or in the options mapping that
// decoding takes from it (see repeatedKey). A mapping whose keys are all
// strings, with no merge key, needs no check (see plainMapping).
func checkRepeatedKeys(mapping *yaml.Node) error {
	if !plainMapping(mapping) {
		if err := repeatedKey(mapping, ""); err != nil {
			return err
		}
	}
	options, found, err := takenValue(mapping, "options")
	if err != nil || !found || plainMapping(options) {
		return err
	}
	return repeatedKey(options, "options.")
}

// FlatOptionConflictError is the error of an option that a rule's settings
// write both directly under the rule key and under options, under the same
// name: either value could win.
type FlatOptionConflictError struct {
	// Section is rules or rule_defaults.
	Section string

	// Rule is the rule key as the file writes it.
	Rule string

	// Name is the name that both write.
	Name string

	// Line is the line of the key written directly under the rule key.
	Line int
}

// Error describes the conflict in the manner of the error for two
// spellings of one option.
func (e *FlatOptionConflictError) Error() string {
	return fmt.Sprintf("%s.%s: line %d: %q and %q both set option %q; keep one of them",
		e.Section, e.Rule, e.Line, e.Name, "options."+e.Name, e.Name)
}

// ReadFlatOptions reads into cfg the options that doc, the parsed YAML
// document that cfg was decoded from, writes directly under rule keys,
// under rules and rule_defaults: for each rule key, the keys of its
// settings other than the settings themselves, as decoding takes them,
// after merge keys and aliases. reads tells, by the rule key as the file
// writes it and the key's name, which of those keys are options of the
// rule. The value of such a key is decoded and joins the rule's options.
// The value of any other key is never decoded, so that one which cannot
// be decoded, such as a value that contains itself, does no harm. An
// option that reads accepts and that the rule's options already set under
// the same name is a *FlatOptionConflictError. Rules are read in order of
// key, and the keys of each rule in order of name, so the first error is
// the same every time.
func ReadFlatOptions(doc *yaml.Node, cfg *Config, reads func(rule, option string) bool) error {
	root := doc
	if root != nil && root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil
		}
		root = root.Content[0]
	}

	for _, section := range []struct {
		name  string
		rules map[string]RuleConfig
	}{{"rules", cfg.Rules}, {"rule_defaults", cfg.RuleDefaults}} {
		node, found, err := takenValue(root, section.name)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		entries, err := takenEntries(node)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			rule, ok := section.rules[entry.name]
			if !ok {
				continue
			}
			key := entry.name
			if err := rule.readFlatOptions(section.name, key, entry.value, func(option string) bool {
				return reads(key, option)
			}); err != nil {
				return err
			}
			section.rules[key] = rule
		}
	}
	return nil
}

// readFlatOptions reads into rc's options the keys of settings, the node
// of the rule's settings, other than the settings themselves, that reads
// accepts, in order of name (see ReadFlatOptions). section and rule name
// the rule in errors.
func (rc *RuleConfig) readFlatOptions(section, rule string, settings *yaml.Node, reads func(option string) bool) error {
	if !mayHoldFlatOptions(settings) {
		return nil
	}
	entries, err := takenEntries(settings)
	if err != nil {
		return err
	}

	var options map[string]any
	for _, entry := range entries {
		if isRuleSettingKey(entry.name) || !reads(entry.name) {
			continue
		}
		if _, nested := rc.Options[entry.name]; nested {
			return &FlatOptionConflictError{Section: section, Rule: rule, Name: entry.name, Line: entry.line()}
		}
		var value any
		if err := entry.value.Decode(&value); err != nil {
			return fmt.Errorf("%s.%s: line %d: option %q: %w", section, rule, entry.line(), entry.name, err)
		}
		if options == nil {
			// Copies of the settings share their options, so the flat
			// options join a copy.
			options = make(map[string]any, len(rc.Options)+len(entries))
			maps.Copy(options, rc.Options)
		}
		options[entry.name] = value
	}
	if options != nil {
		rc.Options = options
	}
	return nil
}

// mayHoldFlatOptions reports whether node, the settings of a rule, may
// hold a key other than the settings: whether it is a mapping, or an alias
// of one, that writes such a key, or whose keys only decoding can name.
// The settings that gomdlint writes, which Clone reads back, hold none.
func mayHoldFlatOptions(node *yaml.Node) bool {
	node = aliasTarget(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	if !plainMapping(node) {
		return true
	}
	for idx := 0; idx < len(node.Content); idx += 2 {
		if !isRuleSettingKey(node.Content[idx].Value) {
			return true
		}
	}
	return false
}

// mappingEntry is an entry that decoding takes from a mapping: its key's
// name as decoding reads it, the node of its value, and the key that
// writes it, which is nil if no written key can be told.
type mappingEntry struct {
	name       string
	key, value *yaml.Node
}

// line returns the line of the entry's key, or of its value when the key
// is not known.
func (e mappingEntry) line() int {
	if e.key != nil {
		return e.key.Line
	}
	return e.value.Line
}

// takenEntries returns the entries that decoding takes from node, a
// mapping or an alias of one, in order of name, without decoding their
// values. Decoding the mapping into a map of nodes lets yaml.v3 itself
// select them, after merge keys and aliases, as it selects the entries of
// options: a written key replaces a merged one, an earlier merged mapping
// replaces a later one, and a null key is skipped. Each entry is then
// matched with the key that writes it, in the mapping or in a mapping
// that its merge key brings in. A node that is no mapping has none.
func takenEntries(node *yaml.Node) ([]mappingEntry, error) {
	node = aliasTarget(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, nil
	}

	var entries []mappingEntry
	if plainMapping(node) {
		// Every key is written in the mapping as a string, once.
		entries = make([]mappingEntry, 0, len(node.Content)/2)
		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			key := node.Content[idx]
			entries = append(entries, mappingEntry{name: key.Value, key: key, value: node.Content[idx+1]})
		}
	} else {
		var taken map[string]yaml.Node
		if err := node.Decode(&taken); err != nil {
			return nil, err //nolint:wrapcheck // yaml.v3 reports a *yaml.TypeError with the document's other errors only when it is returned unwrapped.
		}
		written := writtenEntries(node)
		entries = make([]mappingEntry, 0, len(taken))
		for name, value := range taken {
			entry := mappingEntry{name: name, value: &value}
			for idx := range written {
				if written[idx].name == name && copyOf(&value, written[idx].value) {
					entry.key, entry.value = written[idx].key, written[idx].value
					break
				}
			}
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, func(left, right mappingEntry) int { return cmp.Compare(left.name, right.name) })
	return entries, nil
}

// takenValue returns the node of the value that decoding takes for the key
// name from node, a mapping or an alias of one, and whether it takes one
// (see takenEntries).
func takenValue(node *yaml.Node, name string) (*yaml.Node, bool, error) {
	node = aliasTarget(node)
	if plainMapping(node) {
		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			if node.Content[idx].Value == name {
				return node.Content[idx+1], true, nil
			}
		}
		return nil, false, nil
	}
	entries, err := takenEntries(node)
	if err != nil {
		return nil, false, err
	}
	for _, entry := range entries {
		if entry.name == name {
			return entry.value, true, nil
		}
	}
	return nil, false, nil
}

// writtenEntry is a key written in a mapping, or in a mapping that one of
// its merge keys brings in: its name as decoding reads it, its key and
// value nodes, and the mapping that writes it.
type writtenEntry struct {
	name              string
	key, value, owner *yaml.Node
}

// writtenEntries lists the keys written in mapping and in the mappings
// that its merge keys bring in, each mapping's own keys before those of
// the mappings it merges, in turn. A key that decoding skips, such as a
// null, is left out. Each mapping is listed once, so a mapping that
// merges itself, which decoding rejects, cannot loop.
func writtenEntries(mapping *yaml.Node) []writtenEntry {
	var entries []writtenEntry
	walked := make(map[*yaml.Node]bool)
	var walk func(node *yaml.Node)
	walk = func(node *yaml.Node) {
		node = aliasTarget(node)
		if node == nil || node.Kind != yaml.MappingNode || walked[node] {
			return
		}
		walked[node] = true

		var merge *yaml.Node
		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			key, value := node.Content[idx], node.Content[idx+1]
			if isMergeKey(key) {
				merge = value
				continue
			}
			if name, ok := keyName(key); ok {
				entries = append(entries, writtenEntry{name: name, key: key, value: value, owner: node})
			}
		}
		if merge == nil {
			return
		}
		if merge.Kind == yaml.SequenceNode {
			for _, source := range merge.Content {
				walk(source)
			}
			return
		}
		walk(merge)
	}
	walk(mapping)
	return entries
}

// repeatedKey returns a *yaml.TypeError for the first two keys written in
// one mapping, node or a mapping that its merge keys bring in, that read
// as one name, such as a key and an alias of it. The error names both
// keys, after prefix, such as "options.", at the line of the second.
func repeatedKey(node *yaml.Node, prefix string) error {
	first := make(map[*yaml.Node]map[string]*yaml.Node)
	for _, entry := range writtenEntries(node) {
		names := first[entry.owner]
		if names == nil {
			names = make(map[string]*yaml.Node)
			first[entry.owner] = names
		}
		if earlier, twice := names[entry.name]; twice {
			return &yaml.TypeError{Errors: []string{fmt.Sprintf(
				"line %d: %q and %q both set option %q; keep one of them",
				entry.key.Line, prefix+writtenKey(earlier), prefix+writtenKey(entry.key), entry.name)}}
		}
		names[entry.name] = entry.key
	}
	return nil
}

// copyOf reports whether decoded is decoding's copy of node. Decoding into
// a yaml.Node copies the node it reads, so the copy keeps its position, its
// tag and value, the target of an alias and the content of a collection.
func copyOf(decoded, node *yaml.Node) bool {
	return decoded.Kind == node.Kind && decoded.Line == node.Line && decoded.Column == node.Column &&
		decoded.Tag == node.Tag && decoded.Value == node.Value && decoded.Alias == node.Alias &&
		len(decoded.Content) == len(node.Content) && (len(node.Content) == 0 || &decoded.Content[0] == &node.Content[0])
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

// keyName returns the name that decoding into a map of strings reads from
// key: the text of a scalar, or of the scalar that an alias names, the
// decoded text of a binary scalar, and nothing for a null, which decoding
// skips, or for a collection, which it rejects.
func keyName(key *yaml.Node) (string, bool) {
	target := aliasTarget(key)
	if target == nil || target.Kind != yaml.ScalarNode {
		return "", false
	}
	switch target.ShortTag() {
	case "!!null":
		return "", false
	case "!!binary":
		var name string
		if err := target.Decode(&name); err != nil {
			return "", false
		}
		return name, true
	}
	return target.Value, true
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

// FromYAML parses a configuration from YAML bytes, with the options that it
// writes directly under rule keys. It does not know which options the rules
// declare, so it reads every key under a rule other than the settings as an
// option (see ReadFlatOptions). A configuration loader that knows the
// rules reads only their options.
func FromYAML(data []byte) (*Config, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	cfg, err := decodeConfig(&doc)
	if err != nil {
		return nil, err
	}
	if doc.Kind != 0 {
		if err := ReadFlatOptions(&doc, cfg, func(string, string) bool { return true }); err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
	}
	return cfg, nil
}

// decodeConfig decodes the configuration that doc, a parsed YAML document,
// holds, without the options that it writes directly under rule keys.
// Empty content holds no document and sets nothing.
func decodeConfig(doc *yaml.Node) (*Config, error) {
	cfg := &Config{}
	if doc.Kind != 0 {
		if err := doc.Decode(cfg); err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
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

	// ToYAML writes every option under options, so reading it back needs no
	// pass over keys written directly under rule keys. Linting clones the
	// configuration for each file with a markdownlint-configure-file comment.
	var doc yaml.Node
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return c.deepCopy()
	}
	clone, err := decodeConfig(&doc)
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
