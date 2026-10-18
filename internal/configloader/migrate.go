package configloader

import (
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
	Warnings []string

	// SourcePath is the path to the original markdownlint config.
	SourcePath string
}

// ConvertMarkdownlintConfig converts a markdownlint config file to gomdlint
// format. As in markdownlint 0.41.1, the default key gives every rule its
// baseline, and the other keys apply in the order they are written, each
// replacing what an earlier key set for the rules it names (see
// lint.MarkdownlintSettings). It returns the converted config with any
// warnings, or an error if the file cannot be read or parsed.
func ConvertMarkdownlintConfig(path string) (*MigrationResult, error) {
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

	cfg := &config.Config{Rules: make(map[string]config.RuleConfig)}
	var defaultValue any = true
	for _, setting := range settings.Settings() {
		if setting.Key == "default" {
			defaultValue = setting.Value
		}
	}
	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	cfg.RuleDefaults = make(map[string]config.RuleConfig)
	for _, id := range registry.IDs() {
		value := defaultValue
		if lint.IsGomdlintOnlyRule(id) {
			value = false // markdownlint does not run gomdlint's own rules.
		}
		cfg.RuleDefaults[id] = config.MarkdownlintRuleConfig(id, value)
	}

	for _, setting := range settings.Settings() {
		switch setting.Key {
		case "default", "$schema":
			continue
		case "extends":
			if extends, ok := setting.Value.(string); ok {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("'extends: %q' is not yet supported; you may need to merge configs manually", extends))
				continue
			}
		}
		processRuleKey(registry, cfg, setting.Key, setting.Value, result)
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
func yamlKeyOrder(node *yaml.Node) []string {
	var keys []string
	seen := make(map[string]bool)
	visiting := make(map[*yaml.Node]bool) // Guards against a mapping that merges itself.
	var walk func(node *yaml.Node)
	walk = func(node *yaml.Node) {
		for node != nil && node.Kind == yaml.AliasNode {
			node = node.Alias
		}
		if node == nil || node.Kind != yaml.MappingNode || visiting[node] {
			return
		}
		visiting[node] = true
		defer delete(visiting, node)

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

// stripJSONComments removes JavaScript-style comments from JSON content.
// Valid JSON, which holds no comments, comes back as it is.
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

// processRuleKey applies key, one key of the markdownlint config, and its
// value to the rules that the key names as markdownlint reads it (see
// lint.Registry.MarkdownlintConfigRuleIDs): a rule ID, name or alias, or a
// markdownlint tag, which covers markdownlint's rules only, in any spelling
// whose JavaScript upper case is the same. Each replaces what an earlier
// key set for the rule. markdownlint ignores a key that names nothing,
// which gets a warning.
func processRuleKey(registry *lint.Registry, cfg *config.Config, key string, value any, result *MigrationResult) {
	ruleIDs, known := registry.MarkdownlintConfigRuleIDs(key)
	if !known {
		result.Warnings = append(result.Warnings, registry.UnknownKeyText(key)+"; it is ignored")
		return
	}
	for _, id := range ruleIDs {
		cfg.Rules[id] = config.MarkdownlintRuleConfig(id, value)
	}
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
