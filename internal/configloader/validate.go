package configloader

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/lint"
)

// ValidationError represents a configuration validation error.
type ValidationError struct {
	// Field is the path to the invalid field (e.g., "rules.MD001.severity").
	Field string

	// Value is the invalid value.
	Value any

	// Message describes the validation error.
	Message string

	// FilePath is the config file containing the error (if known).
	FilePath string

	// Line is the line number in the config file (if known).
	Line int
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	var parts []string

	if e.FilePath != "" {
		if e.Line > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", e.FilePath, e.Line))
		} else {
			parts = append(parts, e.FilePath)
		}
	}

	if e.Field != "" {
		parts = append(parts, e.Field)
	}

	parts = append(parts, e.Message)

	return strings.Join(parts, ": ")
}

// ValidationResult contains all validation findings.
type ValidationResult struct {
	// Errors are validation failures that prevent loading.
	Errors []ValidationError

	// Warnings are non-fatal issues (e.g., unknown fields).
	Warnings []ValidationError
}

// Valid returns true if there are no errors.
func (r *ValidationResult) Valid() bool {
	return len(r.Errors) == 0
}

// HasWarnings returns true if there are any warnings.
func (r *ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// AllMessages returns all error and warning messages combined.
func (r *ValidationResult) AllMessages() []string {
	messages := make([]string, 0, len(r.Errors)+len(r.Warnings))
	for _, e := range r.Errors {
		messages = append(messages, "error: "+e.Error())
	}
	for _, w := range r.Warnings {
		messages = append(messages, "warning: "+w.Error())
	}
	return messages
}

// knownSeverities lists valid severity values.
//
//nolint:gochecknoglobals // Read-only lookup table.
var knownSeverities = map[string]bool{
	"error":   true,
	"warning": true,
	"info":    true,
}

// knownFlavors lists valid flavor values.
//
//nolint:gochecknoglobals // Read-only lookup table.
var knownFlavors = map[config.Flavor]bool{
	config.FlavorCommonMark: true,
	config.FlavorGFM:        true,
}

// knownFormats lists valid output format values.
//
//nolint:gochecknoglobals // Read-only lookup table.
var knownFormats = map[config.OutputFormat]bool{
	config.FormatText:    true,
	config.FormatTable:   true,
	config.FormatJSON:    true,
	config.FormatSARIF:   true,
	config.FormatDiff:    true,
	config.FormatSummary: true,
}

// knownBackupModes lists valid backup mode values.
//
//nolint:gochecknoglobals // Read-only lookup table.
var knownBackupModes = map[string]bool{
	"sidecar": true,
	"none":    true,
}

// Validate checks a configuration for errors and warnings.
func Validate(cfg *config.Config) *ValidationResult {
	if cfg == nil {
		return &ValidationResult{}
	}

	result := &ValidationResult{}

	// Validate flavor
	if cfg.Flavor != "" && !knownFlavors[cfg.Flavor] {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "flavor",
			Value:   cfg.Flavor,
			Message: fmt.Sprintf("invalid flavor %q; must be one of: commonmark, gfm", cfg.Flavor),
		})
	}

	// Validate severity_default
	if cfg.SeverityDefault != "" && !knownSeverities[cfg.SeverityDefault] {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "severity_default",
			Value:   cfg.SeverityDefault,
			Message: fmt.Sprintf("invalid severity %q; must be one of: error, warning, info", cfg.SeverityDefault),
		})
	}

	// Validate format
	if cfg.Format != "" && !knownFormats[cfg.Format] {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "format",
			Value:   cfg.Format,
			Message: fmt.Sprintf("invalid format %q; must be one of: text, table, json, sarif, diff, summary", cfg.Format),
		})
	}

	// Validate jobs
	if cfg.Jobs < 0 {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "jobs",
			Value:   cfg.Jobs,
			Message: "jobs must be >= 0 (0 means auto)",
		})
	}

	// Validate backups.mode
	if cfg.Backups.Mode != "" && !knownBackupModes[cfg.Backups.Mode] {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "backups.mode",
			Value:   cfg.Backups.Mode,
			Message: fmt.Sprintf("invalid backup mode %q; must be one of: sidecar, none", cfg.Backups.Mode),
		})
	}

	// Validate rules
	validateRules(cfg, result)

	// Validate ignore patterns
	validateIgnorePatterns(cfg, result)

	return result
}

// validateRules checks rule configurations for errors and warnings.
func validateRules(cfg *config.Config, result *ValidationResult) {
	registry := lint.DefaultRegistry

	for ruleID, ruleCfg := range cfg.Rules {
		// Check if rule exists in registry
		if _, exists := registry.Get(ruleID); !exists {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   "rules." + ruleID,
				Value:   ruleID,
				Message: fmt.Sprintf("unknown rule %q; it will be ignored", ruleID),
			})
		}

		// Validate rule severity
		if ruleCfg.Severity != nil && !knownSeverities[*ruleCfg.Severity] {
			result.Errors = append(result.Errors, ValidationError{
				Field:   "rules." + ruleID + ".severity",
				Value:   *ruleCfg.Severity,
				Message: fmt.Sprintf("invalid severity %q; must be one of: error, warning, info", *ruleCfg.Severity),
			})
		}
	}
}

// validateIgnorePatterns checks that ignore patterns are valid globs.
func validateIgnorePatterns(cfg *config.Config, result *ValidationResult) {
	for i, pattern := range cfg.Ignore {
		// filepath.Match returns an error only for malformed patterns
		_, err := filepath.Match(pattern, "")
		if err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("ignore[%d]", i),
				Value:   pattern,
				Message: fmt.Sprintf("invalid glob pattern: %v", err),
			})
		}
	}
}

// ValidateWithFile validates configuration and includes file path in errors.
func ValidateWithFile(cfg *config.Config, filePath string) *ValidationResult {
	result := Validate(cfg)

	// Add file path to all errors and warnings
	for i := range result.Errors {
		result.Errors[i].FilePath = filePath
	}
	for i := range result.Warnings {
		result.Warnings[i].FilePath = filePath
	}

	return result
}

// IsValidSeverity returns true if the severity string is valid.
func IsValidSeverity(s string) bool {
	return knownSeverities[s]
}

// IsValidFlavor returns true if the flavor is valid.
func IsValidFlavor(f config.Flavor) bool {
	return knownFlavors[f]
}

// IsValidFormat returns true if the format is valid.
func IsValidFormat(f config.OutputFormat) bool {
	return knownFormats[f]
}

// IsValidBackupMode returns true if the backup mode is valid.
func IsValidBackupMode(mode string) bool {
	return knownBackupModes[mode]
}
