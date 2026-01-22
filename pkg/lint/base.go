package lint

import "github.com/yaklabco/gomdlint/pkg/config"

// BaseRule provides a default implementation of the Rule interface.
// Embed this in rule implementations and override methods as needed.
//
// Fields are unexported to avoid stutter and name collisions with interface methods.
// Use the New* constructors or struct literal with field names.
type BaseRule struct {
	id      string   // Unique identifier (e.g., "MD001")
	name    string   // Human-readable name
	desc    string   // Detailed description
	tags    []string // Categorization tags
	fixable bool     // Whether the rule can auto-fix
}

// NewBaseRule creates a BaseRule with the given properties.
func NewBaseRule(id, name, desc string, tags []string, fixable bool) BaseRule {
	return BaseRule{
		id:      id,
		name:    name,
		desc:    desc,
		tags:    tags,
		fixable: fixable,
	}
}

// ID returns the unique identifier for this rule.
func (r *BaseRule) ID() string {
	return r.id
}

// Name returns the human-readable name of the rule.
func (r *BaseRule) Name() string {
	return r.name
}

// Description returns a detailed description of what the rule checks.
func (r *BaseRule) Description() string {
	return r.desc
}

// DefaultEnabled returns whether the rule is enabled by default.
// Override this method to change the default.
func (r *BaseRule) DefaultEnabled() bool {
	return true
}

// DefaultSeverity returns the default severity for this rule.
// Override this method to change the default.
func (r *BaseRule) DefaultSeverity() config.Severity {
	return config.SeverityWarning
}

// Tags returns categorization tags for this rule.
func (r *BaseRule) Tags() []string {
	return r.tags
}

// CanFix returns whether this rule can auto-fix issues.
func (r *BaseRule) CanFix() bool {
	return r.fixable
}

// Apply must be overridden by concrete rule implementations.
// The default implementation returns no diagnostics.
func (r *BaseRule) Apply(_ *RuleContext) ([]Diagnostic, error) {
	return nil, nil
}
