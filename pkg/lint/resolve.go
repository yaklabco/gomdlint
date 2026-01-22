package lint

import "github.com/yaklabco/gomdlint/pkg/config"

// ResolvedRule pairs a Rule with its resolved configuration.
type ResolvedRule struct {
	// Rule is the underlying rule implementation.
	Rule Rule

	// Enabled indicates whether the rule should be run.
	Enabled bool

	// Severity is the resolved severity for diagnostics from this rule.
	Severity config.Severity

	// AutoFix indicates whether auto-fix is enabled for this rule.
	AutoFix bool

	// Config is the rule-specific configuration (may be nil).
	Config *config.RuleConfig
}

// ResolveRules determines which rules to run based on registry and config.
// Returns only enabled rules with their resolved configuration.
func ResolveRules(registry *Registry, cfg *config.Config) []ResolvedRule {
	var resolved []ResolvedRule

	for _, rule := range registry.Rules() {
		rr := resolveRule(rule, cfg)
		if rr.Enabled {
			resolved = append(resolved, rr)
		}
	}

	return resolved
}

// resolveRule resolves the configuration for a single rule.
func resolveRule(rule Rule, cfg *config.Config) ResolvedRule {
	rr := ResolvedRule{
		Rule:     rule,
		Enabled:  rule.DefaultEnabled(),
		Severity: rule.DefaultSeverity(),
		AutoFix:  rule.CanFix(),
		Config:   nil,
	}

	if cfg == nil {
		return rr
	}

	// Check for explicit enable/disable from CLI.
	for _, id := range cfg.EnableRules {
		if id == rule.ID() {
			rr.Enabled = true
			break
		}
	}
	for _, id := range cfg.DisableRules {
		if id == rule.ID() {
			rr.Enabled = false
			break
		}
	}

	// Apply rule-specific config.
	if ruleCfg, ok := cfg.Rules[rule.ID()]; ok {
		rr.Config = &ruleCfg

		if ruleCfg.Enabled != nil {
			rr.Enabled = *ruleCfg.Enabled
		}
		if ruleCfg.Severity != nil {
			rr.Severity = config.Severity(*ruleCfg.Severity)
		}
		if ruleCfg.AutoFix != nil {
			rr.AutoFix = *ruleCfg.AutoFix && rule.CanFix()
		}
	}

	// Apply fix-rules filter from CLI.
	if len(cfg.FixRules) > 0 {
		rr.AutoFix = false
		for _, id := range cfg.FixRules {
			if id == rule.ID() && rule.CanFix() {
				rr.AutoFix = true
				break
			}
		}
	}

	// Disable auto-fix if --fix is not set.
	if !cfg.Fix {
		rr.AutoFix = false
	}

	return rr
}
