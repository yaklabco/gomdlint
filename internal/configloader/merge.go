package configloader

import (
	"maps"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// merge combines two configurations, with override taking precedence over base.
// The merge follows these rules:
//   - Scalar values: override overwrites base if override is non-zero
//   - Maps: deep merge, with override's values taking precedence, except
//     that a rule's options that reset those below them replace them (see
//     config.RuleConfig.ResetOptions)
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
	if override.RuleFormat != "" {
		result.RuleFormat = override.RuleFormat
	}
	if override.Jobs != 0 {
		result.Jobs = override.Jobs
	}

	// Fix, DryRun and NoBackups are command-line settings that config files
	// cannot hold, so only true values from the environment or flags reach
	// this point. The CLI applies flags set explicitly to false after loading.
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
	// A nil Enabled means the override leaves the setting alone, so false
	// from a config file disables backups.
	if override.Backups.Enabled != nil {
		result.Backups.Enabled = config.Bool(*override.Backups.Enabled)
	}
	if override.Gitignore != nil {
		result.Gitignore = config.Bool(*override.Gitignore)
	}
	if override.DefaultIgnores != nil {
		result.DefaultIgnores = config.Bool(*override.DefaultIgnores)
	}

	// Maps: deep merge
	result.Rules = mergeRules(base.Rules, override.Rules)
	result.RuleDefaults = mergeRules(base.RuleDefaults, override.RuleDefaults)

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
// override's values take precedence over base's values. Settings that
// reset the rule's options (config.RuleConfig.ResetOptions) replace
// base's options with their own, and the result keeps the reset.
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

	// Options: replaced by settings that reset them, else deep merged.
	switch {
	case override.ResetOptions:
		result.ResetOptions = true
		result.Options = maps.Clone(override.Options)
	case override.Options != nil:
		if result.Options == nil {
			result.Options = make(map[string]any)
		} else {
			result.Options = maps.Clone(result.Options)
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
