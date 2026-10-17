package lint

import (
	"maps"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
)

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

// LookupRuleID resolves a rule ID, name, or alias to the canonical rule ID.
// Matching ignores case and surrounding whitespace, so "md009" and
// "No-Trailing-Spaces" both resolve to "MD009".
func LookupRuleID(registry *Registry, key string) (string, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	for _, candidate := range []string{key, strings.ToUpper(key), strings.ToLower(key)} {
		if id, _, ok := registry.Resolve(candidate); ok {
			return id, true
		}
	}
	return "", false
}

// ruleListed reports whether list names rule by ID or name, ignoring case.
func ruleListed(list []string, rule Rule) bool {
	for _, key := range list {
		key = strings.TrimSpace(key)
		if strings.EqualFold(key, rule.ID()) || strings.EqualFold(key, rule.Name()) {
			return true
		}
	}
	return false
}

// resolveRule resolves the configuration for a single rule.
//
// Settings apply in increasing precedence: the rule's built-in defaults,
// severity_default, rule_defaults, rules, then the command-line lists
// (EnableRules, DisableRules and FixRules), which name rules by ID or name.
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

	// severity_default replaces the built-in severity of every rule that
	// rule_defaults or rules do not give an explicit severity.
	if cfg.SeverityDefault != "" {
		rr.Severity = config.Severity(cfg.SeverityDefault)
	}

	if baseline, ok := cfg.RuleDefaults[rule.ID()]; ok {
		rr.Config = &baseline
		if baseline.Enabled != nil {
			rr.Enabled = *baseline.Enabled
		}
		if baseline.Severity != nil {
			rr.Severity = config.Severity(*baseline.Severity)
		}
		if baseline.AutoFix != nil {
			rr.AutoFix = *baseline.AutoFix && rule.CanFix()
		}
	}

	// Apply rule-specific config.
	if ruleCfg, ok := cfg.Rules[rule.ID()]; ok {
		if rr.Config != nil && rr.Config.Options != nil {
			options := maps.Clone(rr.Config.Options)
			maps.Copy(options, ruleCfg.Options)
			ruleCfg.Options = options
		}
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

	// Explicit enable/disable from the command line override configuration.
	// A rule in both lists stays disabled.
	if ruleListed(cfg.EnableRules, rule) {
		rr.Enabled = true
	}
	if ruleListed(cfg.DisableRules, rule) {
		rr.Enabled = false
	}

	// Apply fix-rules filter from CLI.
	if len(cfg.FixRules) > 0 {
		rr.AutoFix = rule.CanFix() && ruleListed(cfg.FixRules, rule)
	}

	// Disable auto-fix if --fix is not set.
	if !cfg.Fix {
		rr.AutoFix = false
	}

	return rr
}
