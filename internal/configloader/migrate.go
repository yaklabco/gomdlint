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

	// registry holds the rules that the conversion knows, which name the
	// rules in the file that FileContent writes.
	registry *lint.Registry

	// sources maps the ID of each rule under Config.Rules to the key of
	// the source file that set it last.
	sources map[string]ruleSource
}

// ruleSource is the key of a markdownlint config that set a rule.
type ruleSource struct {
	// key is the key as the source file writes it.
	key string

	// order is the place of the key among the keys of the source file
	// that set rules, counted from 0.
	order int

	// tag reports that the key is a markdownlint tag, which sets every
	// rule it covers.
	tag bool
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
// line_length, and options that markdownlint does not read for the rule,
// gomdlint's own names and options among them, or whose values have the
// wrong type, are left out with a warning, as markdownlint ignores them.
// The warning for a name of gomdlint's offers markdownlint's spelling. A
// rule key that turns a rule on resets the options that a lower
// configuration layer, such as a user config, gives the rule (see
// config.RuleConfig.ResetOptions), as markdownlint gives the rule the
// options of the key's value and no others. The default key, and a rule
// key that turns a rule off, write out markdownlint's defaults for some
// options of some rules instead (see lint.OptionSpec.MarkdownlintPinned),
// so that they replace those of a lower layer, as gomdlint's translation
// always has.
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
	read := readMarkdownlintConfigFile
	if isMarkdownlintCLI2File(path) {
		read = readMarkdownlintCLI2File
	}
	settings, locate, err := read(path, content, result)
	if err != nil {
		return nil, err
	}

	cfg := &config.Config{Rules: make(map[string]config.RuleConfig)}
	defaultValue, found := settings.Default()
	if !found {
		defaultValue = true // Without a default key, markdownlint runs every rule.
	}
	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	result.registry = registry
	result.sources = make(map[string]ruleSource)
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
		case markdownlintSchemaKey:
			continue
		case "extends":
			if extends, ok := setting.Value.(string); ok {
				result.addWarning(0,
					fmt.Sprintf("'extends: %q' is not yet supported; you may need to merge configs manually", extends))
				continue
			}
		}
		conversion.processRuleKey(setting.Key, setting.Value)
	}

	result.Config = cfg
	return result, nil
}

// readMarkdownlintConfigFile reads content, the content of the markdownlint
// config file at path, as markdownlint-cli2 0.23.3 reads a config file that
// it finds (see lint.ReadMarkdownlintConfig), and returns its settings with
// a locator for the lines of its keys. A value other than an object holds
// no settings, as markdownlint reads it, and gets a warning in result.
func readMarkdownlintConfigFile(path string, content []byte, result *MigrationResult) (
	*lint.MarkdownlintSettings, *sourceLocator, error,
) {
	// markdownlint-cli2 loads a config that starts with a byte order mark
	// in every format, so the mark is skipped, and lines count from the
	// text after it.
	content = bytes.TrimPrefix(content, utf8BOM)

	read, err := lint.ReadMarkdownlintConfig(content)
	if err != nil {
		return nil, nil, markdownlintParseError(path, err)
	}
	if read.NotObject != "" {
		// markdownlint reads the value as an object without keys, so every
		// rule keeps its defaults.
		result.addWarning(0, lint.NotObjectText(read.NotObject))
	}
	return read.Settings, newSourceLocator(read, content), nil
}

// readMarkdownlintCLI2File reads content, the content of the
// markdownlint-cli2 options file at path, such as .markdownlint-cli2.jsonc,
// as markdownlint-cli2 0.23.3 reads it (see lint.ReadMarkdownlintCLI2Options),
// and returns the settings of the markdownlint configuration under its
// config key, with a locator for the lines of their keys. A file without a
// config key, or whose config key holds no object, holds no settings, so
// that every rule keeps markdownlint's defaults, as markdownlint-cli2 then
// lints. The file's other keys configure markdownlint-cli2 itself, which
// gomdlint does not read, so each gets a warning in result (see
// markdownlintCLI2Warnings).
func readMarkdownlintCLI2File(path string, content []byte, result *MigrationResult) (
	*lint.MarkdownlintSettings, *sourceLocator, error,
) {
	options, root, err := readMarkdownlintCLI2Options(path, content)
	if err != nil {
		return nil, nil, err
	}
	for _, warning := range markdownlintCLI2Warnings(options, root) {
		result.addWarning(warning.line, warning.message)
	}
	if options.Config == nil {
		return &lint.MarkdownlintSettings{}, &sourceLocator{}, nil
	}
	return options.Config.Settings, &sourceLocator{root: root.Key(lint.MarkdownlintCLI2ConfigKey)}, nil
}

// readMarkdownlintCLI2Options reads content, the content of the
// markdownlint-cli2 options file at path, with the parser that its name
// selects, JSON with comments for a name that ends in .jsonc and YAML
// otherwise, and returns it with the positions of its keys.
func readMarkdownlintCLI2Options(path string, content []byte) (
	*lint.MarkdownlintCLI2Options, *lint.MarkdownlintPosition, error,
) {
	syntax, format := lint.MarkdownlintYAML, "YAML"
	if IsJSONConfig(path) {
		syntax, format = lint.MarkdownlintJSONC, "JSON"
	}
	options, err := lint.ReadMarkdownlintCLI2Options(content, syntax)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", format, err)
	}
	root := options.Positions
	if syntax == lint.MarkdownlintJSONC {
		root = jsonRootPositions(content)
	}
	return options, root, nil
}

// lineWarning is a warning about one line of a file, or about no single
// line when line is 0.
type lineWarning struct {
	line    int
	message string
}

// markdownlintCLI2Warnings describes the problems of options, a
// markdownlint-cli2 options file whose keys root places, other than those of
// the markdownlint configuration under its config key: holding no object,
// a config key that holds no object, and each other key but $schema, which
// editors read. Those keys configure markdownlint-cli2 itself, such as its
// globs and ignores, and gomdlint does not read them.
func markdownlintCLI2Warnings(options *lint.MarkdownlintCLI2Options, root *lint.MarkdownlintPosition) []lineWarning {
	var warnings []lineWarning
	if options.NotObject != "" {
		warnings = append(warnings, lineWarning{message: lint.NotObjectText(options.NotObject)})
	}
	for _, key := range options.Keys {
		line := positionLine(root.Key(key))
		switch key {
		case markdownlintSchemaKey:
		case lint.MarkdownlintCLI2ConfigKey:
			if options.Config != nil && options.Config.NotObject != "" {
				warnings = append(warnings, lineWarning{
					line: line, message: key + " holds " + lint.NotObjectText(options.Config.NotObject),
				})
			}
		default:
			warnings = append(warnings, lineWarning{line: line, message: markdownlintCLI2KeyText(key)})
		}
	}
	return warnings
}

// markdownlintSchemaKey is the key of a markdownlint or markdownlint-cli2
// configuration that names its JSON schema, which editors read.
const markdownlintSchemaKey = "$schema"

// markdownlintCLI2Options maps each option of a markdownlint-cli2 options
// file but config, which holds the markdownlint configuration
// (cli2SchemaKeys in markdownlint-cli2 0.23.3's constants.mjs), to what to
// use in gomdlint instead, where gomdlint has a counterpart.
//
//nolint:gochecknoglobals // Read-only lookup table.
var markdownlintCLI2Options = map[string]string{
	"customRules":       "",
	"fix":               "use --fix instead",
	"frontMatter":       "",
	"gitignore":         "set gitignore in a gomdlint config instead",
	"globs":             "name the files on the command line instead",
	"ignores":           "set ignore in a gomdlint config instead",
	"markdownItPlugins": "",
	"modulePaths":       "",
	"noBanner":          "",
	"noInlineConfig":    "",
	"noProgress":        "",
	"outputFormatters":  "",
	"overrides":         "",
	"showFound":         "",
}

// markdownlintCLI2KeyText describes key, a top-level key of a
// markdownlint-cli2 options file other than config and $schema, for a
// warning: an option of markdownlint-cli2, which gomdlint does not read,
// with what to use instead where gomdlint has a counterpart, or a key that
// markdownlint-cli2 does not know, with the closest key that it knows.
func markdownlintCLI2KeyText(key string) string {
	hint, known := markdownlintCLI2Options[key]
	if !known {
		keys := append(slices.Collect(maps.Keys(markdownlintCLI2Options)), lint.MarkdownlintCLI2ConfigKey, markdownlintSchemaKey)
		slices.Sort(keys)
		return unknownKeyMessage("", key, keys)
	}
	text := fmt.Sprintf("unsupported markdownlint-cli2 option %q", key)
	if hint != "" {
		text += " (" + hint + ")"
	}
	return text + "; it is ignored"
}

// positionLine returns the line of position, or 0 when it is nil.
func positionLine(position *lint.MarkdownlintPosition) int {
	if position == nil {
		return 0
	}
	return position.Line
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

// markdownlintConversion converts the rule keys of one markdownlint
// configuration into cfg, recording warnings in result.
type markdownlintConversion struct {
	registry  *lint.Registry
	translate markdownlintTranslator
	cfg       *config.Config
	result    *MigrationResult
	locate    *sourceLocator

	// keys counts the keys that have named rules so far.
	keys int
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
	// root holds the places of the config's keys, or is nil when the
	// source shows none.
	root *lint.MarkdownlintPosition
}

// newSourceLocator returns the locator for content, a markdownlint config
// that lint.ReadMarkdownlintConfig read as read. YAML comes with the places
// of its keys from that reading, so that loading and placing a warning
// read each key alike, whatever directives, aliases or spellings of keys
// the text holds. JSON with comments and trailing commas is read again by
// jsonDocument, once jsonc.Strip has blanked them, which reads each key as
// the JSON decoder does. Content that it cannot read has no lines to find.
func newSourceLocator(read *lint.MarkdownlintSource, content []byte) *sourceLocator {
	if read.Syntax != lint.MarkdownlintJSONC {
		return &sourceLocator{root: read.Positions}
	}
	return &sourceLocator{root: jsonRootPositions(content)}
}

// jsonRootPositions returns the places of the keys of content, JSON with
// comments and trailing commas, which jsonDocument reads once jsonc.Strip
// has blanked them, as the JSON decoder reads each key, or nil for content
// that it cannot read.
func jsonRootPositions(content []byte) *lint.MarkdownlintPosition {
	doc, err := jsonDocument(jsonc.Strip(content))
	if err != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	return jsonPositions(doc.Content[0])
}

// jsonPositions returns the places of node, a JSON value that jsonDocument
// read, and of its keys and items. A key written twice takes the place of
// its last entry, whose value JSON takes.
func jsonPositions(node *yaml.Node) *lint.MarkdownlintPosition {
	place := &lint.MarkdownlintPosition{Line: node.Line}
	switch node.Kind {
	case yaml.MappingNode:
		place.Keys = make(map[string]*lint.MarkdownlintPosition, len(node.Content)/2)
		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			key := node.Content[idx]
			entry := jsonPositions(node.Content[idx+1])
			entry.Line = key.Line
			place.Keys[key.Value] = entry
		}
	case yaml.SequenceNode:
		place.Items = make([]*lint.MarkdownlintPosition, len(node.Content))
		for idx, item := range node.Content {
			place.Items[idx] = jsonPositions(item)
		}
	}
	return place
}

// optionLine returns the line of option, a key under the config's key key,
// or, when item is not 0, the line of the option's item at that position,
// counted from 1. It returns 0 when the source does not show it.
func (l *sourceLocator) optionLine(key, option string, item int) int {
	entry := l.root.Key(key).Key(option)
	if entry == nil {
		return 0
	}
	if at := entry.Item(item); at != nil {
		return at.Line
	}
	return entry.Line
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
// that a rule cannot use, gomdlint's own spellings among them, which
// markdownlint does not read. A tag gives its options to every rule it
// covers, as in markdownlint, so an option that one of them does not read
// is no mistake: only an option that none of them reads gets a warning.
func (c *markdownlintConversion) processRuleKey(key string, value any) {
	ruleIDs, known := c.registry.MarkdownlintConfigRuleIDs(key)
	if !known {
		c.warn(0, c.registry.UnknownKeyText(key)+"; it is ignored")
		return
	}
	source := ruleSource{key: key, order: c.keys, tag: c.isTag(key, ruleIDs)}
	c.keys++
	unread := make(map[string]int) // option key -> the rules that do not read it
	for _, id := range ruleIDs {
		converted, issues := c.translate.rule(c.registry, id, value)
		for _, issue := range issues {
			if issue.Kind == lint.OptionIssueUnknown && len(ruleIDs) > 1 {
				unread[issue.Key]++
				continue
			}
			c.warn(c.locate.optionLine(key, issue.Key, issue.Item), issue.Text(c.ruleName(id)))
		}
		c.cfg.Rules[id] = converted
		c.result.sources[id] = source
	}
	for _, option := range slices.Sorted(maps.Keys(unread)) {
		if unread[option] == len(ruleIDs) {
			c.warn(c.locate.optionLine(key, option, 0), lint.UnknownTagOptionText(option, key))
		}
	}
}

// isTag reports whether key, a key of the markdownlint config that names
// the rules with the given IDs, is a markdownlint tag rather than one
// rule's ID, name or alias.
func (c *markdownlintConversion) isTag(key string, ruleIDs []string) bool {
	rule, isRule := c.registry.Lookup(key)
	return !isRule || len(ruleIDs) != 1 || rule.ID() != ruleIDs[0]
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
