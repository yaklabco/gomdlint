package configloader

import (
	"encoding/json"
	"fmt"
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

// ConvertMarkdownlintConfig converts a markdownlint config file to gomdlint format.
// Returns the converted config, any warnings, and an error if conversion failed.
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

	// Parse as generic map first
	var raw map[string]any
	if IsJSONConfig(path) {
		if err := parseJSONC(content, &raw); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(content, &raw); err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
	}

	cfg := &config.Config{Rules: make(map[string]config.RuleConfig)}
	var defaultValue any = true
	if value, ok := raw["default"]; ok {
		defaultValue = value
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

	// Handle special keys
	processSpecialKeys(raw, result)

	// Apply tags before explicit rules, deterministically.
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if isMarkdownlintTag(registry, key) {
			processRuleKey(registry, cfg, key, raw[key], result)
		}
	}
	for _, key := range keys {
		if !isMarkdownlintTag(registry, key) {
			processRuleKey(registry, cfg, key, raw[key], result)
		}
	}

	result.Config = cfg
	return result, nil
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

// parseJSONC parses JSON with comments (JSONC format).
// It strips comments before parsing.
func parseJSONC(content []byte, target any) error {
	// Simple approach: try parsing as JSON first
	// JSON with comments will fail, but many .jsonc files are valid JSON
	if err := json.Unmarshal(content, target); err == nil {
		return nil
	}

	// Strip comments and try again
	stripped := stripJSONComments(content)
	if err := json.Unmarshal(stripped, target); err != nil {
		return fmt.Errorf("unmarshal stripped JSON: %w", err)
	}
	return nil
}

// stripJSONComments removes JavaScript-style comments from JSON content.
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

// processSpecialKeys handles markdownlint special configuration keys.
func processSpecialKeys(raw map[string]any, result *MigrationResult) {
	// Handle "default" key
	delete(raw, "default")

	// Handle "extends" key
	if extends, ok := raw["extends"].(string); ok {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("'extends: %q' is not yet supported; you may need to merge configs manually", extends))
		delete(raw, "extends")
	}

	// Remove $schema
	delete(raw, "$schema")
}

// processRuleKey processes a single key from the markdownlint config: a rule
// ID, name or alias, or a markdownlint tag, in any case. Like markdownlint,
// it reads the key whole, so a key that is empty or holds whitespace names
// nothing.
func processRuleKey(registry *lint.Registry, cfg *config.Config, key string, value any, result *MigrationResult) {
	if rule, ok := registry.Lookup(key); ok && lint.IsSingleWord(key) {
		cfg.Rules[rule.ID()] = config.MarkdownlintRuleConfig(rule.ID(), value)
		return
	}

	if ruleIDs := markdownlintTagRuleIDs(registry, key); len(ruleIDs) > 0 {
		for _, rid := range ruleIDs {
			cfg.Rules[rid] = config.MarkdownlintRuleConfig(rid, value)
		}
		return
	}

	// markdownlint ignores keys it does not know.
	result.Warnings = append(result.Warnings, registry.UnknownKeyText(key)+"; it is ignored")
}

// isMarkdownlintTag reports whether key is a markdownlint tag.
func isMarkdownlintTag(registry *lint.Registry, key string) bool {
	return len(markdownlintTagRuleIDs(registry, key)) > 0
}

// markdownlintTagRuleIDs returns the IDs of the markdownlint rules that the
// markdownlint tag covers, in ID order. gomdlint files its own rules under
// markdownlint's tags too, but in a markdownlint config a tag means only
// markdownlint's rules, because markdownlint never runs gomdlint's. A tag
// with surrounding whitespace is no tag there.
func markdownlintTagRuleIDs(registry *lint.Registry, tag string) []string {
	if !lint.IsSingleWord(tag) {
		return nil
	}
	var ruleIDs []string
	for _, rule := range registry.RulesByTag(tag) {
		if !lint.IsGomdlintOnlyRule(rule.ID()) {
			ruleIDs = append(ruleIDs, rule.ID())
		}
	}
	return ruleIDs
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
