package lint

import (
	"slices"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// RuleInfos returns the metadata that configuration templates document for
// each rule in registry, in rule ID order: its name, description and tags,
// whether it can fix findings, and its default enabled state and severity.
func RuleInfos(registry *Registry) []config.RuleInfo {
	rules := registry.Rules()
	infos := make([]config.RuleInfo, 0, len(rules))
	for _, rule := range rules {
		infos = append(infos, config.RuleInfo{
			ID:          rule.ID(),
			Name:        rule.Name(),
			Description: rule.Description(),
			Enabled:     rule.DefaultEnabled(),
			Severity:    rule.DefaultSeverity(),
			Tags:        slices.Clone(rule.Tags()),
			CanFix:      rule.CanFix(),
		})
	}
	return infos
}

// init makes config.GenerateTemplate document the rules of DefaultRegistry.
// The config package cannot import this one, so it asks for them through
// config.DefaultRuleInfoProvider. The provider reads the registry when a
// template is generated, after every rule has registered.
//
//nolint:gochecknoinits // The config package documents its provider as set by this package during init.
func init() {
	config.DefaultRuleInfoProvider = func() []config.RuleInfo {
		return RuleInfos(DefaultRegistry)
	}
}
