package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// offByDefaultRule is a rule that is disabled by default with error
// severity.
type offByDefaultRule struct {
	lint.BaseRule
}

func (r *offByDefaultRule) DefaultEnabled() bool { return false }

func (r *offByDefaultRule) DefaultSeverity() config.Severity { return config.SeverityError }

func TestRuleInfos(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(&offByDefaultRule{BaseRule: lint.NewBaseRule("TEST002", "second-rule", "Second", []string{"b"}, true)})
	registry.Register(newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) { return nil, nil }))

	assert.Equal(t, []config.RuleInfo{
		{ID: "TEST001", Name: "rule-TEST001", Enabled: true, Severity: config.SeverityWarning, CanFix: true},
		{
			ID: "TEST002", Name: "second-rule", Description: "Second", Enabled: false,
			Severity: config.SeverityError, Tags: []string{"b"}, CanFix: true,
		},
	}, lint.RuleInfos(registry), "rules in ID order with their defaults")
}

// TestDefaultRuleInfoProviderIsSet covers the hook that lets config
// templates document the rules of the default registry.
func TestDefaultRuleInfoProviderIsSet(t *testing.T) {
	t.Parallel()

	require.NotNil(t, config.DefaultRuleInfoProvider)
	assert.Equal(t, lint.RuleInfos(lint.DefaultRegistry), config.DefaultRuleInfoProvider())
}
