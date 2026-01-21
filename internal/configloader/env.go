package configloader

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jamesainslie/gomdlint/pkg/config"
)

// envVarPrefix is the prefix for all gomdlint environment variables.
const envVarPrefix = "GOMDLINT_"

// envFieldType represents the type of a configuration field.
type envFieldType int

const (
	envTypeString envFieldType = iota
	envTypeBool
	envTypeInt
	envTypeSlice
)

// envMapping defines environment variable to config field mappings.
type envMapping struct {
	field string
	typ   envFieldType
}

// envMappings maps environment variable names (without prefix) to config fields.
//
//nolint:gochecknoglobals // Read-only lookup table.
var envMappings = map[string]envMapping{
	"FLAVOR":           {field: "flavor", typ: envTypeString},
	"SEVERITY_DEFAULT": {field: "severity_default", typ: envTypeString},
	"FIX":              {field: "fix", typ: envTypeBool},
	"DRY_RUN":          {field: "dry_run", typ: envTypeBool},
	"JOBS":             {field: "jobs", typ: envTypeInt},
	"FORMAT":           {field: "format", typ: envTypeString},
	"BACKUPS_ENABLED":  {field: "backups.enabled", typ: envTypeBool},
	"BACKUPS_MODE":     {field: "backups.mode", typ: envTypeString},
	"IGNORE":           {field: "ignore", typ: envTypeSlice},
	"NO_BACKUPS":       {field: "no_backups", typ: envTypeBool},
}

// LoadFromEnv applies environment variable overrides to the configuration.
// Environment variables are prefixed with GOMDLINT_ (e.g., GOMDLINT_FLAVOR).
func LoadFromEnv(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}

	for envSuffix, mapping := range envMappings {
		envVar := envVarPrefix + envSuffix
		value := os.Getenv(envVar)
		if value == "" {
			continue
		}

		if err := applyEnvValue(cfg, mapping, value, envVar); err != nil {
			return err
		}
	}

	return nil
}

// applyEnvValue applies a single environment variable value to the config.
func applyEnvValue(cfg *config.Config, mapping envMapping, value, envVar string) error {
	switch mapping.typ {
	case envTypeString:
		return setStringField(cfg, mapping.field, value)
	case envTypeBool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for %s: %q (expected true/false/1/0)", envVar, value)
		}
		return setBoolField(cfg, mapping.field, b)
	case envTypeInt:
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer for %s: %q", envVar, value)
		}
		return setIntField(cfg, mapping.field, i)
	case envTypeSlice:
		parts := parseSliceValue(value)
		return setSliceField(cfg, mapping.field, parts)
	default:
		return fmt.Errorf("unknown field type for %s", envVar)
	}
}

// parseSliceValue parses a comma-separated string into a slice.
// Each element is trimmed of whitespace.
func parseSliceValue(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// setStringField sets a string field on the config by field path.
func setStringField(cfg *config.Config, field, value string) error {
	switch field {
	case "flavor":
		cfg.Flavor = config.Flavor(value)
	case "severity_default":
		cfg.SeverityDefault = value
	case "format":
		cfg.Format = config.OutputFormat(value)
	case "backups.mode":
		cfg.Backups.Mode = value
	default:
		return fmt.Errorf("unknown string field: %s", field)
	}
	return nil
}

// setBoolField sets a boolean field on the config by field path.
func setBoolField(cfg *config.Config, field string, value bool) error {
	switch field {
	case "fix":
		cfg.Fix = value
	case "dry_run":
		cfg.DryRun = value
	case "backups.enabled":
		cfg.Backups.Enabled = value
	case "no_backups":
		cfg.NoBackups = value
	default:
		return fmt.Errorf("unknown boolean field: %s", field)
	}
	return nil
}

// setIntField sets an integer field on the config by field path.
func setIntField(cfg *config.Config, field string, value int) error {
	switch field {
	case "jobs":
		cfg.Jobs = value
	default:
		return fmt.Errorf("unknown integer field: %s", field)
	}
	return nil
}

// setSliceField sets a slice field on the config by field path.
func setSliceField(cfg *config.Config, field string, value []string) error {
	switch field {
	case "ignore":
		cfg.Ignore = value
	default:
		return fmt.Errorf("unknown slice field: %s", field)
	}
	return nil
}

// GetEnvVarName returns the full environment variable name for a config field.
func GetEnvVarName(field string) string {
	for suffix, mapping := range envMappings {
		if mapping.field == field {
			return envVarPrefix + suffix
		}
	}
	return ""
}

// ListEnvVars returns a list of all supported environment variables with their descriptions.
func ListEnvVars() map[string]string {
	return map[string]string{
		"GOMDLINT_FLAVOR":           "Markdown flavor: commonmark or gfm",
		"GOMDLINT_SEVERITY_DEFAULT": "Default severity: error, warning, or info",
		"GOMDLINT_FIX":              "Enable auto-fix: true or false",
		"GOMDLINT_DRY_RUN":          "Dry-run mode: true or false",
		"GOMDLINT_JOBS":             "Number of parallel workers (0 = auto)",
		"GOMDLINT_FORMAT":           "Output format: text, json, sarif, or diff",
		"GOMDLINT_BACKUPS_ENABLED":  "Enable backups when fixing: true or false",
		"GOMDLINT_BACKUPS_MODE":     "Backup mode: sidecar or none",
		"GOMDLINT_IGNORE":           "Comma-separated list of ignore patterns",
		"GOMDLINT_NO_BACKUPS":       "Disable backups: true or false",
	}
}
