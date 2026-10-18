package configloader

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// MigrationResult contains the result of converting a markdownlint config.
type MigrationResult struct {
	// Config is the converted gomdlint configuration. It holds rule
	// settings only, under Rules and RuleDefaults, because that is all a
	// markdownlint config sets. Every other setting is unset, so that the
	// layers below it, such as a user config, keep theirs.
	Config *config.Config

	// Warnings contains non-fatal issues encountered during conversion.
	// WarningLine gives the line of the source file that each is about.
	Warnings []string

	// SourcePath is the path to the original markdownlint config.
	SourcePath string

	// warningLines maps the index in Warnings of each warning about one
	// line of the source file to that line.
	warningLines map[int]int
}

// WarningLine returns the line of the source file that Warnings[idx] is
// about, such as the line of an option that its rule does not read, or 0
// for a warning about no single line.
func (r *MigrationResult) WarningLine(idx int) int {
	return r.warningLines[idx]
}

// addWarning records message, a warning about the given line of the source
// file, or about no single line when line is 0.
func (r *MigrationResult) addWarning(line int, message string) {
	if line > 0 {
		if r.warningLines == nil {
			r.warningLines = make(map[int]int)
		}
		r.warningLines[len(r.Warnings)] = line
	}
	r.Warnings = append(r.Warnings, message)
}

// markdownlintTranslator converts the settings of rules in a markdownlint
// configuration to gomdlint's settings.
type markdownlintTranslator struct {
	// rule converts the setting of the rule with the given ID under a key
	// that names it, and reports the problems that its options hold.
	rule func(registry *lint.Registry, ruleID string, value any) (config.RuleConfig, []lint.OptionIssue)

	// defaultRule converts the value of the default key for the rule with
	// the given ID.
	defaultRule func(registry *lint.Registry, ruleID string, value any) config.RuleConfig
}

// ConvertMarkdownlintConfig converts a markdownlint config file to gomdlint
// format. As in markdownlint 0.41.1, the default key, the first key named
// default in any case, gives every rule its baseline (see
// config.MarkdownlintDefaultRuleConfig), and the other keys apply in the
// order they are written, each replacing what an earlier key set for the
// rules it names (see lint.MarkdownlintSettings). It returns the converted
// config with any warnings, or an error if the file cannot be read or
// parsed.
//
// Rule options are translated through the rule catalog: markdownlint's
// spellings become gomdlint's canonical names, such as max for
// line_length, and options that the rule does not read, or whose values
// have the wrong type, are left out with a warning, as markdownlint ignores
// them. Every setting of some rules also writes out markdownlint's
// defaults for some of their options (see lint.OptionSpec.MarkdownlintPinned),
// so that it replaces them in a lower configuration layer, as gomdlint's
// translation always has. Two spellings of one option are an error.
func ConvertMarkdownlintConfig(path string) (*MigrationResult, error) {
	return convertMarkdownlintConfig(path, markdownlintTranslator{
		rule:        (*lint.Registry).MarkdownlintRuleConfig,
		defaultRule: (*lint.Registry).MarkdownlintDefaultRuleConfig,
	})
}

// convertMarkdownlintConfig implements ConvertMarkdownlintConfig, with
// translate converting the settings of the rules.
func convertMarkdownlintConfig(path string, translate markdownlintTranslator) (*MigrationResult, error) {
	result := &MigrationResult{
		SourcePath: path,
	}

	// Check for JavaScript config files
	if IsJavaScriptConfig(path) {
		return nil, fmt.Errorf("cannot convert JavaScript config file %q; please create a gomdlint config manually", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	settings, err := readMarkdownlintSettings(path, content)
	if err != nil {
		return nil, err
	}
	locate := newSourceLocator(path, content)

	cfg := &config.Config{Rules: make(map[string]config.RuleConfig)}
	defaultValue, found := settings.Default()
	if !found {
		defaultValue = true // Without a default key, markdownlint runs every rule.
	}
	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	cfg.RuleDefaults = make(map[string]config.RuleConfig)
	for _, id := range registry.IDs() {
		value := defaultValue
		if lint.IsGomdlintOnlyRule(id) {
			value = false // markdownlint does not run gomdlint's own rules.
		}
		cfg.RuleDefaults[id] = translate.defaultRule(registry, id, value)
	}

	conversion := &markdownlintConversion{
		registry: registry, translate: translate, cfg: cfg, result: result, locate: locate,
	}
	for _, setting := range settings.Settings() {
		if lint.IsMarkdownlintDefaultKey(setting.Key) {
			continue // The first default key is the baseline, and later ones do nothing.
		}
		switch setting.Key {
		case "$schema":
			continue
		case "extends":
			if extends, ok := setting.Value.(string); ok {
				result.addWarning(0,
					fmt.Sprintf("'extends: %q' is not yet supported; you may need to merge configs manually", extends))
				continue
			}
		}
		if err := conversion.processRuleKey(setting.Key, setting.Value); err != nil {
			return nil, fmt.Errorf("%s: %w", setting.Key, err)
		}
	}

	result.Config = cfg
	return result, nil
}

// readMarkdownlintSettings reads content, the markdownlint config at path,
// into its settings in the order of its keys: as JSON with comments for a
// name that ends in .json or .jsonc, and as YAML otherwise.
func readMarkdownlintSettings(path string, content []byte) (*lint.MarkdownlintSettings, error) {
	if IsJSONConfig(path) {
		settings, err := lint.DecodeMarkdownlintJSON(stripJSONComments(content))
		if err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
		return settings, nil
	}
	settings, err := markdownlintYAMLSettings(content)
	if err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	return settings, nil
}

// markdownlintYAMLSettings reads content, a markdownlint config in YAML,
// into its settings: the keys of its mapping in the order of yamlKeyOrder,
// with the values that decoding gives them. Empty content and null hold no
// settings.
func markdownlintYAMLSettings(content []byte) (*lint.MarkdownlintSettings, error) {
	var values map[string]any
	if err := yaml.Unmarshal(content, &values); err != nil {
		return nil, err //nolint:wrapcheck // The caller names the format.
	}
	settings := &lint.MarkdownlintSettings{}
	if len(values) == 0 {
		return settings, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, err //nolint:wrapcheck // Decoding the same content succeeded.
	}
	for _, key := range yamlKeyOrder(doc.Content[0]) {
		if value, ok := values[key]; ok {
			settings.Set(key, value)
			delete(values, key)
		}
	}
	// The order holds every key that decoding reads. Should one be missing,
	// it still applies, after the others.
	for _, key := range slices.Sorted(maps.Keys(values)) {
		settings.Set(key, values[key])
	}
	return settings, nil
}

// yamlKeyOrder returns the keys of node, a YAML mapping or an alias of one,
// in the order in which js-yaml 4 adds them to an object: each key written
// in the mapping in its place, and the keys of the mappings that a merge key
// (<<) brings in, in their own order, in the place of the merge key. A key
// comes once, in its first place, which keeps it when a later entry
// replaces its value. Decoding gives a key written in the mapping the value
// written with it, and a merged key the value of the first mapping that
// brings it in, as js-yaml 4 does too.
//
// Each mapping is walked once. Its keys, and those of the mappings it
// merges, have their places once it has been walked, so a later merge of it
// adds nothing, and a mapping that merges itself ends the walk there.
func yamlKeyOrder(node *yaml.Node) []string {
	var keys []string
	seen := make(map[string]bool)
	walked := make(map[*yaml.Node]bool)
	var walk func(node *yaml.Node)
	walk = func(node *yaml.Node) {
		for node != nil && node.Kind == yaml.AliasNode {
			node = node.Alias
		}
		if node == nil || node.Kind != yaml.MappingNode || walked[node] {
			return
		}
		walked[node] = true

		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			key, value := node.Content[idx], node.Content[idx+1]
			if isMergeKey(key) {
				sources := []*yaml.Node{value}
				if value.Kind == yaml.SequenceNode {
					sources = value.Content
				}
				for _, source := range sources {
					walk(source)
				}
				continue
			}
			var name string
			if key.Decode(&name) == nil && !seen[name] {
				seen[name] = true
				keys = append(keys, name)
			}
		}
	}
	walk(node)
	return keys
}

// FileContent returns the content of a gomdlint config file at path that
// holds the converted settings: JSON for a name that ends in .json, which
// lint reads as JSON, and otherwise YAML under a header that names the
// source file. The file holds the rule settings alone, under rules and
// rule_defaults, so a user or system config still sets everything else,
// such as ignore patterns and the flavor.
func (r *MigrationResult) FileContent(path string) ([]byte, error) {
	if HoldsJSON(path) {
		content, err := r.Config.RuleSettingsToJSON()
		if err != nil {
			return nil, fmt.Errorf("serialize %s: %w", path, err)
		}
		return content, nil
	}
	content, err := r.Config.RuleSettingsToYAML(GenerateMigrationHeader(r.SourcePath))
	if err != nil {
		return nil, fmt.Errorf("serialize %s: %w", path, err)
	}
	return content, nil
}

// stripJSONComments removes JavaScript-style comments from JSON content,
// keeping their line endings, so that every line keeps its number. Valid
// JSON, which holds no comments, comes back as it is.
func stripJSONComments(content []byte) []byte {
	var result []byte
	inString := false
	inSingleComment := false
	inMultiComment := false

	for idx := 0; idx < len(content); idx++ {
		char := content[idx]

		if inSingleComment {
			if char == '\n' {
				inSingleComment = false
				result = append(result, char)
			}
			continue
		}

		if inMultiComment {
			if char == '*' && idx+1 < len(content) && content[idx+1] == '/' {
				inMultiComment = false
				idx++ // skip the closing /
			} else if char == '\n' {
				result = append(result, char) // Lines keep their numbers, for warnings.
			}
			continue
		}

		if inString {
			result = append(result, char)
			if char == '\\' && idx+1 < len(content) {
				idx++
				result = append(result, content[idx])
			} else if char == '"' {
				inString = false
			}
			continue
		}

		if char == '"' {
			inString = true
			result = append(result, char)
			continue
		}

		if char == '/' && idx+1 < len(content) {
			next := content[idx+1]
			if next == '/' {
				inSingleComment = true
				idx++
				continue
			}
			if next == '*' {
				inMultiComment = true
				idx++
				continue
			}
		}

		result = append(result, char)
	}

	return result
}

// markdownlintConversion converts the rule keys of one markdownlint
// configuration into cfg, recording warnings in result.
type markdownlintConversion struct {
	registry  *lint.Registry
	translate markdownlintTranslator
	cfg       *config.Config
	result    *MigrationResult
	locate    sourceLocator
}

// rule converts value, the setting of the rule with the given ID, and
// returns the problems its options hold, other than two spellings of one
// option, which are an error.
func (c *markdownlintConversion) rule(id string, value any) (config.RuleConfig, []lint.OptionIssue, error) {
	converted, issues := c.translate.rule(c.registry, id, value)
	for _, issue := range issues {
		if issue.Kind == lint.OptionIssueConflict {
			return config.RuleConfig{}, nil, errors.New(issue.Text(c.ruleName(id)))
		}
	}
	return converted, issues, nil
}

// ruleName returns the name of the registered rule with the given ID.
func (c *markdownlintConversion) ruleName(id string) string {
	if rule, ok := c.registry.GetByID(id); ok {
		return rule.Name()
	}
	return id
}

// warn records a warning about the given line of the configuration, or
// about no single line when line is 0.
func (c *markdownlintConversion) warn(line int, message string) {
	c.result.addWarning(line, message)
}

// sourceLocator finds the lines of the keys of a markdownlint config in its
// source, for warnings that name them.
type sourceLocator struct {
	// root is the config's top-level mapping, or nil when the source holds
	// none.
	root *yaml.Node
}

// newSourceLocator reads content, the markdownlint config at path, into a
// tree of nodes that record their lines: as JSON with comments for a name
// that ends in .json or .jsonc, which jsonDocument reads, and as YAML
// otherwise. Content that it cannot read has no lines to find.
func newSourceLocator(path string, content []byte) sourceLocator {
	var doc *yaml.Node
	if IsJSONConfig(path) {
		parsed, err := jsonDocument(stripJSONComments(content))
		if err != nil {
			return sourceLocator{}
		}
		doc = parsed
	} else {
		doc = &yaml.Node{}
		if err := yaml.Unmarshal(content, doc); err != nil {
			return sourceLocator{}
		}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return sourceLocator{}
	}
	return sourceLocator{root: doc.Content[0]}
}

// optionLine returns the line of option, a key under the config's key key,
// or, when item is not 0, the line of the option's item at that position,
// counted from 1. It returns 0 when the source does not show it.
func (l sourceLocator) optionLine(key, option string, item int) int {
	setting := takenSourceEntry(l.root, key)
	if setting == nil {
		return 0
	}
	entry := takenSourceEntry(setting.value, option)
	if entry == nil {
		return 0
	}
	if line := itemLine(entry.value, item); line > 0 {
		return line
	}
	return entry.key.Line
}

// takenSourceEntry returns the entry for name of mapping, a mapping or an
// alias to one, whose value the config takes: the last one written in the
// mapping itself, since JSON keeps the last of a key written twice and
// YAML has no key twice, or else the first that a merge key brings in, as
// YAML takes it. It returns nil when there is none.
func takenSourceEntry(mapping *yaml.Node, name string) *writtenEntry {
	var own, merged *writtenEntry
	entries := writtenEntries(mapping)
	for idx := range entries {
		entry := &entries[idx]
		switch {
		case entry.name != name:
		case entry.merge == nil:
			own = entry
		case merged == nil:
			merged = entry
		}
	}
	if own != nil {
		return own
	}
	return merged
}

// processRuleKey applies key, one key of the markdownlint config, and its
// value to the rules that the key names as markdownlint reads it (see
// lint.Registry.MarkdownlintConfigRuleIDs): a rule ID, name or alias, or a
// markdownlint tag, which covers markdownlint's rules only, in any spelling
// whose JavaScript upper case is the same. Each replaces what an earlier
// key set for the rule. markdownlint ignores a key that names nothing,
// which gets a warning.
//
// The value's options are translated for each rule (see
// lint.Registry.MarkdownlintRuleConfig), with a warning for each option
// that a rule cannot use. A tag gives its options to every rule it covers,
// as in markdownlint, so an option that one of them does not read is no
// mistake: only an option that none of them reads gets a warning.
func (c *markdownlintConversion) processRuleKey(key string, value any) error {
	ruleIDs, known := c.registry.MarkdownlintConfigRuleIDs(key)
	if !known {
		c.warn(0, c.registry.UnknownKeyText(key)+"; it is ignored")
		return nil
	}
	unread := make(map[string]int) // option key -> the rules that do not read it
	for _, id := range ruleIDs {
		converted, issues, err := c.rule(id, value)
		if err != nil {
			return err
		}
		for _, issue := range issues {
			if issue.Kind == lint.OptionIssueUnknown && len(ruleIDs) > 1 {
				unread[issue.Key]++
				continue
			}
			c.warn(c.locate.optionLine(key, issue.Key, issue.Item), issue.Text(c.ruleName(id)))
		}
		c.cfg.Rules[id] = converted
	}
	for _, option := range slices.Sorted(maps.Keys(unread)) {
		if unread[option] == len(ruleIDs) {
			c.warn(c.locate.optionLine(key, option, 0),
				fmt.Sprintf("unknown option %q for the rules of the tag %q; it is ignored", option, key))
		}
	}
	return nil
}

// GenerateMigrationHeader returns a header comment for migrated configs.
func GenerateMigrationHeader(sourcePath string) string {
	return fmt.Sprintf(`# gomdlint configuration
# Migrated from: %s
# See: https://github.com/yaklabco/gomdlint
`, filepath.Base(sourcePath))
}

// CanMigrate returns true if the config file can be migrated.
// JavaScript config files cannot be migrated.
func CanMigrate(path string) bool {
	return !IsJavaScriptConfig(path)
}

// GetMigrationWarning returns a warning message for files that cannot be migrated.
func GetMigrationWarning(path string) string {
	if IsJavaScriptConfig(path) {
		ext := filepath.Ext(path)
		return fmt.Sprintf("JavaScript config file (%s) cannot be converted automatically; "+
			"please create a .gomdlint.yml file manually or run 'gomdlint init'", ext)
	}
	return ""
}

// DetectConfigFormat determines the format of a config file.
func DetectConfigFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json", ".jsonc":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".cjs", ".mjs":
		return "javascript"
	default:
		return "unknown"
	}
}
