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

	// Fixable reports whether --fix would apply this rule's fixes: the rule
	// declares fixes (Rule.CanFix), and neither auto_fix nor --fix-rules
	// excludes it. It does not depend on whether this run fixes files.
	Fixable bool

	// AutoFix indicates whether this run applies the rule's fixes: the rule
	// is Fixable and the configuration has Fix set.
	AutoFix bool

	// Config is the rule-specific configuration (may be nil).
	Config *config.RuleConfig
}

// ResolveRules determines which rules to run based on registry and config.
// Returns only enabled rules with their resolved configuration.
func ResolveRules(registry *Registry, cfg *config.Config) []ResolvedRule {
	var resolved []ResolvedRule

	for _, rr := range ResolveAllRules(registry, cfg) {
		if rr.Enabled {
			resolved = append(resolved, rr)
		}
	}

	return resolved
}

// ResolveAllRules resolves the configuration of every rule in registry, in
// rule ID order, as ResolveRules does, whether or not the configuration
// enables it. A disabled rule keeps the severity it would report at. A nil
// configuration gives each rule its built-in settings.
func ResolveAllRules(registry *Registry, cfg *config.Config) []ResolvedRule {
	rules := registry.Rules()
	resolved := make([]ResolvedRule, 0, len(rules))

	declared := registry.lookups().options
	for _, rule := range rules {
		resolved = append(resolved, resolveRule(rule, cfg, declared[rule.ID()]))
	}

	return resolved
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
//
// declared holds the options that the rule's catalog entry declares, or is
// nil. With them, the options of rule_defaults and of rules are each
// rewritten to canonical names (see NormalizeOptions) before rules
// override rule_defaults option by option, so that the rule reads every
// option under its canonical name. Settings under rules that reset the
// rule's options (config.RuleConfig.ResetOptions) replace those of
// rule_defaults instead. The configuration is never changed.
func resolveRule(rule Rule, cfg *config.Config, declared *optionTable) ResolvedRule {
	rr := ResolvedRule{
		Rule:     rule,
		Enabled:  rule.DefaultEnabled(),
		Severity: rule.DefaultSeverity(),
		Fixable:  rule.CanFix(),
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
		baseline.Options = canonicalOptions(declared, baseline.Options)
		rr.Config = &baseline
		if baseline.Enabled != nil {
			rr.Enabled = *baseline.Enabled
		}
		if baseline.Severity != nil {
			rr.Severity = config.Severity(*baseline.Severity)
		}
		if baseline.AutoFix != nil {
			rr.Fixable = *baseline.AutoFix && rule.CanFix()
		}
	}

	// Apply rule-specific config. Settings that reset the rule's options
	// leave out those of rule_defaults.
	if ruleCfg, ok := cfg.Rules[rule.ID()]; ok {
		ruleCfg.Options = canonicalOptions(declared, ruleCfg.Options)
		if rr.Config != nil && rr.Config.Options != nil && !ruleCfg.ResetOptions {
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
			rr.Fixable = *ruleCfg.AutoFix && rule.CanFix()
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
		rr.Fixable = rule.CanFix() && ruleListed(cfg.FixRules, rule)
	}

	// Fixes are only applied with --fix.
	rr.AutoFix = rr.Fixable && cfg.Fix

	return rr
}

// canonicalOptions returns values, the options of one configuration
// section for a rule, rewritten to the canonical names of declared, the
// rule's declared options, or values itself when declared is nil. Of two
// spellings of one option, the canonical one wins: configuration files
// reject both, but a configuration built in code may hold them.
func canonicalOptions(declared *optionTable, values map[string]any) map[string]any {
	if declared == nil {
		return values
	}
	normalized, _ := declared.normalize(values)
	return normalized
}
