package configloader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jamesainslie/gomdlint/pkg/config"
)

// MigrationResult contains the result of converting a markdownlint config.
type MigrationResult struct {
	// Config is the converted gomdlint configuration.
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

	cfg := config.NewConfig()

	// Handle special keys
	processSpecialKeys(raw, result)

	// Process remaining keys as rules
	for key, value := range raw {
		processRuleKey(cfg, key, value, result)
	}

	result.Config = cfg
	return result, nil
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
	if defaultVal, ok := raw["default"].(bool); ok {
		if !defaultVal {
			result.Warnings = append(result.Warnings,
				"'default: false' means all rules are disabled by default; "+
					"in gomdlint, rules are enabled by default and must be explicitly disabled")
		}
		delete(raw, "default")
	}

	// Handle "extends" key
	if extends, ok := raw["extends"].(string); ok {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("'extends: %q' is not yet supported; you may need to merge configs manually", extends))
		delete(raw, "extends")
	}

	// Remove $schema
	delete(raw, "$schema")
}

// processRuleKey processes a single key from the markdownlint config.
func processRuleKey(cfg *config.Config, key string, value any, result *MigrationResult) {
	// Try to normalize as a rule ID
	ruleID := NormalizeRuleID(key)
	if ruleID != "" {
		ruleCfg := convertRuleValue(value)
		cfg.Rules[ruleID] = ruleCfg
		return
	}

	// Check if it's a tag
	if IsTag(key) {
		ruleIDs := GetTagRules(key)
		enabled := valueToBool(value)
		for _, rid := range ruleIDs {
			cfg.Rules[rid] = config.RuleConfig{
				Enabled: &enabled,
			}
		}
		return
	}

	// Unknown key
	result.Warnings = append(result.Warnings,
		fmt.Sprintf("unknown key %q; skipping", key))
}

// convertRuleValue converts a markdownlint rule value to our RuleConfig.
func convertRuleValue(value any) config.RuleConfig {
	cfg := config.RuleConfig{}

	switch typedVal := value.(type) {
	case bool:
		cfg.Enabled = &typedVal
	case map[string]any:
		enabled := true
		cfg.Enabled = &enabled
		cfg.Options = make(map[string]any)
		for key, optVal := range typedVal {
			// Apply option mappings if needed
			mappedKey := mapOptionName(key)
			cfg.Options[mappedKey] = optVal
		}
	case nil:
		// Explicitly null means disabled
		enabled := false
		cfg.Enabled = &enabled
	default:
		// For other types (numbers, strings), enable with default options
		enabled := true
		cfg.Enabled = &enabled
	}

	return cfg
}

// valueToBool converts various value types to a boolean.
func valueToBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case nil:
		return false
	case map[string]any:
		return true // Object means enabled with options
	default:
		return true
	}
}

// mapOptionName maps markdownlint option names to gomdlint option names.
// Most options have the same name, but some may differ.
func mapOptionName(name string) string {
	// Currently, we keep the same names for compatibility
	// Add mappings here if we diverge from markdownlint option names
	return name
}

// GenerateMigrationHeader returns a header comment for migrated configs.
func GenerateMigrationHeader(sourcePath string) string {
	return fmt.Sprintf(`# gomdlint configuration
# Migrated from: %s
# See: https://github.com/jamesainslie/gomdlint
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
