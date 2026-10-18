package configloader

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/jsonc"
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
// format. It reads the file as markdownlint-cli2 0.23.3 reads a config that
// it finds, whatever its name says (see lint.ReadMarkdownlintConfig): as
// JSON with comments and trailing commas, or failing that as YAML, after a
// byte order mark. A file that holds a value other than an object holds no
// settings, as markdownlint reads it, with a warning. As in markdownlint
// 0.41.1, the default key, the first key named default in any case, gives
// every rule its baseline (see config.MarkdownlintDefaultRuleConfig), and
// the other keys apply in the order they are written, each replacing what
// an earlier key set for the rules it names (see lint.MarkdownlintSettings).
// It returns the converted config with any warnings, or an error if the
// file cannot be read or parsed.
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
	// markdownlint-cli2 loads a config that starts with a byte order mark
	// in every format, so the mark is skipped, and lines count from the
	// text after it.
	content = bytes.TrimPrefix(content, utf8BOM)

	read, err := lint.ReadMarkdownlintConfig(content)
	if err != nil {
		return nil, markdownlintParseError(path, err)
	}
	settings := read.Settings
	locate := newSourceLocator(read.Syntax, content)
	if read.NotObject != "" {
		// markdownlint reads the value as an object without keys, so every
		// rule keeps its defaults.
		result.addWarning(0, lint.NotObjectText(read.NotObject))
	}

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

// markdownlintParseError describes err, the error that reading the
// markdownlint config at path gave. Text that is neither JSON nor YAML (see
// lint.MarkdownlintSyntaxError) gets the error of the syntax that the
// file's name suggests: JSON for a name that ends in .json or .jsonc, and
// YAML otherwise.
func markdownlintParseError(path string, err error) error {
	var syntaxErr *lint.MarkdownlintSyntaxError
	switch {
	case !errors.As(err, &syntaxErr):
		return fmt.Errorf("parse JSON: %w", err)
	case IsJSONConfig(path):
		return fmt.Errorf("parse JSON: %w", syntaxErr.JSON)
	default:
		return fmt.Errorf("parse YAML: %w", syntaxErr.YAML)
	}
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

// markdownlintConversion converts the rule keys of one markdownlint
// configuration into cfg, recording warnings in result.
type markdownlintConversion struct {
	registry  *lint.Registry
	translate markdownlintTranslator
	cfg       *config.Config
	result    *MigrationResult
	locate    *sourceLocator
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

	// taken indexes, for each mapping that a lookup has searched, the
	// entries whose values the config takes, by name (see
	// indexTakenEntries), so that each mapping is read once, however many
	// warnings name its keys.
	taken map[*yaml.Node]map[string]*writtenEntry

	// reads counts the written entries that indexing has read.
	reads int
}

// newSourceLocator reads content, a markdownlint config that
// lint.ReadMarkdownlintConfig read in syntax, into a tree of nodes that
// record their lines: as JSON with comments and trailing commas, which
// jsonDocument reads once jsonc.Strip has blanked them, or as YAML.
// Content that it cannot read has no lines to find.
func newSourceLocator(syntax lint.MarkdownlintSyntax, content []byte) *sourceLocator {
	var doc *yaml.Node
	if syntax == lint.MarkdownlintJSONC {
		parsed, err := jsonDocument(jsonc.Strip(content))
		if err != nil {
			return &sourceLocator{}
		}
		doc = parsed
	} else {
		doc = &yaml.Node{}
		if err := yaml.Unmarshal(content, doc); err != nil {
			return &sourceLocator{}
		}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return &sourceLocator{}
	}
	return &sourceLocator{root: doc.Content[0]}
}

// optionLine returns the line of option, a key under the config's key key,
// or, when item is not 0, the line of the option's item at that position,
// counted from 1. It returns 0 when the source does not show it.
func (l *sourceLocator) optionLine(key, option string, item int) int {
	setting := l.entry(l.root, key)
	if setting == nil {
		return 0
	}
	entry := l.entry(setting.value, option)
	if entry == nil {
		return 0
	}
	if line := itemLine(entry.value, item); line > 0 {
		return line
	}
	return entry.key.Line
}

// entry returns the entry for name of mapping, a mapping or an alias to
// one, whose value the config takes (see indexTakenEntries), or nil when
// there is none. The first lookup in a mapping indexes its entries.
func (l *sourceLocator) entry(mapping *yaml.Node, name string) *writtenEntry {
	for mapping != nil && mapping.Kind == yaml.AliasNode {
		mapping = mapping.Alias
	}
	if mapping == nil {
		return nil
	}
	index, indexed := l.taken[mapping]
	if !indexed {
		entries := markdownlintEntries(mapping)
		l.reads += len(entries)
		index = indexTakenEntries(entries)
		if l.taken == nil {
			l.taken = make(map[*yaml.Node]map[string]*writtenEntry)
		}
		l.taken[mapping] = index
	}
	return index[name]
}

// markdownlintEntries lists the entries written in mapping itself, each
// named by the text of its key. js-yaml 5, which markdownlint-cli2 reads
// YAML with, reads a merge key (<<) as an ordinary key, so a merged mapping
// brings in no entries, and JSON has no merge keys. A key that js-yaml
// reads as another string, such as 0x10 as "16", is not found by that
// string, which leaves a warning about it without a line.
func markdownlintEntries(mapping *yaml.Node) []writtenEntry {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	entries := make([]writtenEntry, 0, len(mapping.Content)/2)
	for idx := 0; idx+1 < len(mapping.Content); idx += 2 {
		key := mapping.Content[idx]
		named := key
		for named.Kind == yaml.AliasNode && named.Alias != nil {
			named = named.Alias
		}
		if named.Kind == yaml.ScalarNode {
			entries = append(entries, writtenEntry{name: named.Value, key: key, value: mapping.Content[idx+1]})
		}
	}
	return entries
}

// indexTakenEntries indexes entries, the entries of a mapping as
// markdownlintEntries lists them, by name, keeping for each name the entry
// whose value the config takes: the last one written, since JSON keeps the
// last of a key written twice, and YAML has no key twice.
func indexTakenEntries(entries []writtenEntry) map[string]*writtenEntry {
	index := make(map[string]*writtenEntry, len(entries))
	for idx := range entries {
		index[entries[idx].name] = &entries[idx]
	}
	return index
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
			c.warn(c.locate.optionLine(key, option, 0), lint.UnknownTagOptionText(option, key))
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
