package configloader

import "github.com/yaklabco/gomdlint/pkg/config"

// merge combines two configurations, with override taking precedence over base.
// The merge follows these rules:
//   - Scalar values: override overwrites base if override is non-zero
//   - Maps: deep merge, with override's values taking precedence
//   - Slices: override replaces base entirely if override is non-nil
//   - Nil/unset values in override do not override values in base
func merge(base, override *config.Config) *config.Config {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	// Start with a shallow copy of base
	result := *base

	// Scalars: override overwrites base if set (non-zero value)
	if override.Flavor != "" {
		result.Flavor = override.Flavor
	}
	if override.SeverityDefault != "" {
		result.SeverityDefault = override.SeverityDefault
	}
	if override.Format != "" {
		result.Format = override.Format
	}
	if override.Jobs != 0 {
		result.Jobs = override.Jobs
	}

	// Booleans: these are tricky because false is the zero value.
	// For Fix, DryRun, NoBackups - we check if they're true in override.
	// This means CLI --fix will override, but config file cannot unset.
	if override.Fix {
		result.Fix = override.Fix
	}
	if override.DryRun {
		result.DryRun = override.DryRun
	}
	if override.NoBackups {
		result.NoBackups = override.NoBackups
	}

	// Backups: merge individual fields
	if override.Backups.Mode != "" {
		result.Backups.Mode = override.Backups.Mode
	}
	// For Enabled, we need to handle it specially since false is meaningful
	// The BackupsConfig struct uses bool directly, so we can only detect
	// "true" being set. This is a limitation of the current config structure.
	if override.Backups.Enabled {
		result.Backups.Enabled = override.Backups.Enabled
	}

	// Maps: deep merge
	result.Rules = mergeRules(base.Rules, override.Rules)

	// Slices: override replaces base entirely if non-nil
	if override.Ignore != nil {
		result.Ignore = override.Ignore
	}
	if override.EnableRules != nil {
		result.EnableRules = override.EnableRules
	}
	if override.DisableRules != nil {
		result.DisableRules = override.DisableRules
	}
	if override.FixRules != nil {
		result.FixRules = override.FixRules
	}

	return &result
}

// mergeRules performs deep merge of rule configurations.
// Both maps are iterated, with override's values taking precedence.
func mergeRules(base, override map[string]config.RuleConfig) map[string]config.RuleConfig {
	if base == nil && override == nil {
		return nil
	}
	if base == nil {
		// Return a copy of override
		result := make(map[string]config.RuleConfig, len(override))
		for key, val := range override {
			result[key] = val
		}
		return result
	}
	if override == nil {
		// Return a copy of base
		result := make(map[string]config.RuleConfig, len(base))
		for key, val := range base {
			result[key] = val
		}
		return result
	}

	// Create result with capacity for both
	result := make(map[string]config.RuleConfig, len(base)+len(override))

	// Copy all from base
	for key, val := range base {
		result[key] = val
	}

	// Merge from override (override takes precedence)
	for key, val := range override {
		if existing, ok := result[key]; ok {
			result[key] = mergeRuleConfig(existing, val)
		} else {
			result[key] = val
		}
	}

	return result
}

// mergeRuleConfig merges individual rule configurations.
// override's values take precedence over base's values.
func mergeRuleConfig(base, override config.RuleConfig) config.RuleConfig {
	result := base

	if override.Enabled != nil {
		result.Enabled = override.Enabled
	}
	if override.Severity != nil {
		result.Severity = override.Severity
	}
	if override.AutoFix != nil {
		result.AutoFix = override.AutoFix
	}

	// Options: deep merge
	if override.Options != nil {
		if result.Options == nil {
			result.Options = make(map[string]any)
		}
		for key, val := range override.Options {
			result.Options[key] = val
		}
	}

	return result
}

// MergeAll merges multiple configurations in order, with later configs taking precedence.
func MergeAll(configs ...*config.Config) *config.Config {
	if len(configs) == 0 {
		return nil
	}

	result := configs[0]
	for i := 1; i < len(configs); i++ {
		result = merge(result, configs[i])
	}
	return result
}
