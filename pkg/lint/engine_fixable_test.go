package lint_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// lintWithConfig lints content with rules under cfg.
func lintWithConfig(t *testing.T, content string, cfg *config.Config, rules ...lint.Rule) *lint.FileResult {
	t.Helper()

	registry := lint.NewRegistry()
	for _, rule := range rules {
		registry.Register(rule)
	}
	result, err := lint.NewEngine(&mockParser{}, registry).LintFile(context.Background(), "test.md", []byte(content), cfg)
	require.NoError(t, err)
	return result
}

// TestEngine_OnlyFixesThatFixAppliesAreFixable covers rules whose fixes
// --fix would not apply: a rule with auto_fix: false, a rule left out of
// --fix-rules, and a rule that does not declare fixes. Their findings are
// reported without a fix, so they do not count as fixable, with or
// without --fix.
func TestEngine_OnlyFixesThatFixAppliesAreFixable(t *testing.T) {
	t.Parallel()

	// TEST001 upper-cases "hello" and TEST002 upper-cases "there".
	excludable := fix.TextEdit{StartOffset: 0, EndOffset: 5, NewText: "HELLO"}
	fixing := func(id string, edit fix.TextEdit, canFix bool) *funcRule {
		rule := newFuncRule(id, func(*lint.RuleContext) ([]lint.Diagnostic, error) {
			return []lint.Diagnostic{fixDiag(id, edit)}, nil
		})
		rule.BaseRule = lint.NewBaseRule(id, "rule-"+id, "", nil, canFix)
		return rule
	}

	tests := []struct {
		name      string
		configure func(cfg *config.Config)
		canFix    bool
		fixable   bool
	}{
		{name: "fixable by default", configure: func(*config.Config) {}, canFix: true, fixable: true},
		{
			name:      "auto_fix false",
			configure: func(cfg *config.Config) { cfg.Rules["TEST001"] = config.RuleConfig{AutoFix: config.Bool(false)} },
			canFix:    true,
		},
		{
			name: "auto_fix false in rule_defaults",
			configure: func(cfg *config.Config) {
				cfg.RuleDefaults = map[string]config.RuleConfig{"TEST001": {AutoFix: config.Bool(false)}}
			},
			canFix: true,
		},
		{name: "left out of fix-rules", configure: func(cfg *config.Config) { cfg.FixRules = []string{"TEST002"} }, canFix: true},
		{
			name:      "named in fix-rules",
			configure: func(cfg *config.Config) { cfg.FixRules = []string{"rule-TEST001", "TEST002"} },
			canFix:    true,
			fixable:   true,
		},
		{name: "rule without fixes", configure: func(*config.Config) {}, canFix: false},
	}

	for _, tt := range tests {
		for _, fixMode := range []bool{false, true} {
			name := tt.name
			if fixMode {
				name += " with fix"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				cfg := config.NewConfig()
				cfg.Fix = fixMode
				tt.configure(cfg)

				result := lintWithConfig(t, "hello there", cfg,
					fixing("TEST001", excludable, tt.canFix),
					fixing("TEST002", fix.TextEdit{StartOffset: 6, EndOffset: 11, NewText: "THERE"}, true))

				require.Len(t, result.Diagnostics, 2, "every finding is reported")
				first := result.Diagnostics[0]
				require.Equal(t, "TEST001", first.RuleID)
				assert.Equal(t, tt.fixable, first.HasFix(), "TEST001 fixable")
				assert.True(t, result.Diagnostics[1].HasFix(), "TEST002 keeps its fix")
				wantFixable := 1
				if tt.fixable {
					wantFixable = 2
				}
				assert.Equal(t, wantFixable, result.FixableCount())
				switch {
				case !fixMode:
					assert.Empty(t, result.Edits, "fixes are applied only with --fix")
				case tt.fixable:
					assert.Contains(t, result.Edits, excludable)
				default:
					assert.NotContains(t, result.Edits, excludable, "--fix must not apply the excluded fix")
				}
				assert.Empty(t, result.RuleErrors)
			})
		}
	}
}

// TestEngine_InvalidFixOfExcludedRuleIsNoRuleError covers a rule whose
// fixes are excluded: its fix is never offered, so a broken fix is no
// failure of the run.
func TestEngine_InvalidFixOfExcludedRuleIsNoRuleError(t *testing.T) {
	t.Parallel()

	bad := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: 3, EndOffset: 2})}, nil
	})
	cfg := config.NewConfig()
	cfg.Rules["TEST001"] = config.RuleConfig{AutoFix: config.Bool(false)}

	result := lintWithConfig(t, "hello", cfg, bad)

	require.Len(t, result.Diagnostics, 1)
	assert.False(t, result.Diagnostics[0].HasFix())
	assert.Empty(t, result.RuleErrors)
}

func TestResolveRules_Fixable(t *testing.T) {
	t.Parallel()

	rule := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) { return nil, nil })
	registry := lint.NewRegistry()
	registry.Register(rule)

	resolve := func(configure func(cfg *config.Config)) lint.ResolvedRule {
		cfg := config.NewConfig()
		configure(cfg)
		resolved := lint.ResolveRules(registry, cfg)
		require.Len(t, resolved, 1)
		return resolved[0]
	}

	withoutFix := resolve(func(*config.Config) {})
	assert.True(t, withoutFix.Fixable, "--fix would apply the rule's fixes")
	assert.False(t, withoutFix.AutoFix, "fixes are applied only with --fix")

	withFix := resolve(func(cfg *config.Config) { cfg.Fix = true })
	assert.True(t, withFix.Fixable)
	assert.True(t, withFix.AutoFix)

	excluded := resolve(func(cfg *config.Config) {
		cfg.Fix = true
		cfg.Rules["TEST001"] = config.RuleConfig{AutoFix: config.Bool(false)}
	})
	assert.False(t, excluded.Fixable)
	assert.False(t, excluded.AutoFix)

	listed := resolve(func(cfg *config.Config) {
		cfg.Rules["TEST001"] = config.RuleConfig{AutoFix: config.Bool(false)}
		cfg.FixRules = []string{"TEST001"}
	})
	assert.True(t, listed.Fixable, "--fix-rules overrides the config file")
	assert.False(t, listed.AutoFix)
}
