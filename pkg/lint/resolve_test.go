package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

const (
	testRuleID1 = "MD001"
	testRuleID2 = "MD002"
)

// testRule is a simple rule implementation for testing.
type testRule struct {
	lint.BaseRule
}

func newTestRule(id string, canFix bool) *testRule {
	return &testRule{
		BaseRule: lint.NewBaseRule(id, id+"-name", "", nil, canFix),
	}
}

func TestResolveRules_Empty(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	cfg := config.NewConfig()

	resolved := lint.ResolveRules(registry, cfg)

	if len(resolved) != 0 {
		t.Errorf("expected 0 rules, got %d", len(resolved))
	}
}

func TestResolveRules_DefaultEnabled(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(newTestRule(testRuleID1, false))
	registry.Register(newTestRule(testRuleID2, false))

	cfg := config.NewConfig()

	resolved := lint.ResolveRules(registry, cfg)

	// Both rules should be enabled by default (BaseRule.DefaultEnabled returns true).
	if len(resolved) != 2 {
		t.Errorf("expected 2 rules, got %d", len(resolved))
	}
}

func TestResolveRules_DisableViaConfig(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(newTestRule(testRuleID1, false))
	registry.Register(newTestRule(testRuleID2, false))

	cfg := config.NewConfig()
	enabled := false
	cfg.Rules[testRuleID1] = config.RuleConfig{Enabled: &enabled}

	resolved := lint.ResolveRules(registry, cfg)

	if len(resolved) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resolved))
	}

	if resolved[0].Rule.ID() != testRuleID2 {
		t.Errorf("expected %s to be enabled, got %s", testRuleID2, resolved[0].Rule.ID())
	}
}

func TestResolveRules_EnableViaConfig(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(newTestRule(testRuleID1, false))

	cfg := config.NewConfig()
	cfg.RuleDefaults = map[string]config.RuleConfig{testRuleID1: {Enabled: config.Bool(false)}}
	cfg.Rules[testRuleID1] = config.RuleConfig{Enabled: config.Bool(true)}

	resolved := lint.ResolveRules(registry, cfg)

	// Per-rule config overrides rule_defaults.
	if len(resolved) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resolved))
	}
}

// TestResolveRules_CLIListsOverrideConfig covers --enable and --disable,
// which apply after rule_defaults and rules from configuration files.
func TestResolveRules_CLIListsOverrideConfig(t *testing.T) {
	t.Parallel()

	t.Run("enable beats a config disable", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.Rules[testRuleID1] = config.RuleConfig{Enabled: config.Bool(false)}
		cfg.EnableRules = []string{testRuleID1}

		assert.True(t, resolveOne(t, newTestRule(testRuleID1, false), cfg).Enabled)
	})

	t.Run("enable beats a rule_defaults disable", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.RuleDefaults = map[string]config.RuleConfig{testRuleID1: {Enabled: config.Bool(false)}}
		cfg.EnableRules = []string{testRuleID1}

		assert.True(t, resolveOne(t, newTestRule(testRuleID1, false), cfg).Enabled)
	})

	t.Run("disable beats a config enable", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.Rules[testRuleID1] = config.RuleConfig{Enabled: config.Bool(true)}
		cfg.DisableRules = []string{testRuleID1}

		registry := lint.NewRegistry()
		registry.Register(newTestRule(testRuleID1, false))
		assert.Empty(t, lint.ResolveRules(registry, cfg))
	})

	t.Run("disable beats enable", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.EnableRules = []string{testRuleID1}
		cfg.DisableRules = []string{testRuleID1}

		registry := lint.NewRegistry()
		registry.Register(newTestRule(testRuleID1, false))
		assert.Empty(t, lint.ResolveRules(registry, cfg))
	})
}

// TestResolveRules_CLIListsMatchNames covers rule names and case-insensitive
// IDs in --enable, --disable and --fix-rules.
func TestResolveRules_CLIListsMatchNames(t *testing.T) {
	t.Parallel()

	rule := newTestRule(testRuleID1, true) // Name is "MD001-name".

	t.Run("disable by name", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.DisableRules = []string{"md001-name"}

		registry := lint.NewRegistry()
		registry.Register(rule)
		assert.Empty(t, lint.ResolveRules(registry, cfg))
	})

	t.Run("enable by lowercase ID", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.Rules[testRuleID1] = config.RuleConfig{Enabled: config.Bool(false)}
		cfg.EnableRules = []string{"md001"}

		assert.True(t, resolveOne(t, rule, cfg).Enabled)
	})

	t.Run("fix-rules by name", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.Fix = true
		cfg.FixRules = []string{"MD001-name"}

		assert.True(t, resolveOne(t, rule, cfg).AutoFix)
	})
}

func TestRegistryLookupResolvesRuleKeys(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(&testRule{BaseRule: lint.NewBaseRule("MD009", "no-trailing-spaces", "", nil, true)})
	registry.Describe(lint.RuleMeta{ID: "MD009", Name: "no-trailing-spaces", Aliases: []string{"trailing-spaces"}})

	for _, key := range []string{"MD009", "md009", "no-trailing-spaces", "No-Trailing-Spaces", "trailing-spaces", " MD009 "} {
		rule, ok := registry.Lookup(key)
		if assert.True(t, ok, key) {
			assert.Equal(t, "MD009", rule.ID(), key)
		}
	}

	for _, key := range []string{"MD999", "", "no-such-rule"} {
		_, ok := registry.Lookup(key)
		assert.False(t, ok, key)
	}
}

func TestResolveRules_CLIEnable(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(newTestRule(testRuleID1, false))

	cfg := config.NewConfig()
	cfg.EnableRules = []string{testRuleID1}

	resolved := lint.ResolveRules(registry, cfg)

	if len(resolved) != 1 {
		t.Errorf("expected 1 rule, got %d", len(resolved))
	}
}

func TestResolveRules_CLIDisable(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(newTestRule(testRuleID1, false))
	registry.Register(newTestRule(testRuleID2, false))

	cfg := config.NewConfig()
	cfg.DisableRules = []string{testRuleID1}

	resolved := lint.ResolveRules(registry, cfg)

	if len(resolved) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resolved))
	}

	if resolved[0].Rule.ID() != testRuleID2 {
		t.Errorf("expected %s, got %s", testRuleID2, resolved[0].Rule.ID())
	}
}

func TestResolveRules_SeverityOverride(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(newTestRule(testRuleID1, false))

	cfg := config.NewConfig()
	severity := string(config.SeverityError)
	cfg.Rules[testRuleID1] = config.RuleConfig{Severity: &severity}

	resolved := lint.ResolveRules(registry, cfg)

	if len(resolved) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resolved))
	}

	if resolved[0].Severity != config.SeverityError {
		t.Errorf("expected error severity, got %v", resolved[0].Severity)
	}
}

// infoRule is a rule whose built-in default severity is info.
type infoRule struct {
	lint.BaseRule
}

func (r *infoRule) DefaultSeverity() config.Severity {
	return config.SeverityInfo
}

func resolveOne(t *testing.T, rule lint.Rule, cfg *config.Config) lint.ResolvedRule {
	t.Helper()

	registry := lint.NewRegistry()
	registry.Register(rule)

	resolved := lint.ResolveRules(registry, cfg)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resolved))
	}
	return resolved[0]
}

func TestResolveRules_SeverityDefault(t *testing.T) {
	t.Parallel()

	errorSeverity := string(config.SeverityError)
	infoSeverity := string(config.SeverityInfo)

	tests := []struct {
		name  string
		rule  lint.Rule
		setup func(cfg *config.Config)
		want  config.Severity
	}{
		{
			name: "applies to rules without an explicit severity",
			rule: newTestRule(testRuleID1, false),
			setup: func(cfg *config.Config) {
				cfg.SeverityDefault = errorSeverity
			},
			want: config.SeverityError,
		},
		{
			name: "replaces a rule's built-in default",
			rule: &infoRule{BaseRule: lint.NewBaseRule(testRuleID1, "info-rule", "", nil, false)},
			setup: func(cfg *config.Config) {
				cfg.SeverityDefault = errorSeverity
			},
			want: config.SeverityError,
		},
		{
			name: "loses to a rule severity",
			rule: newTestRule(testRuleID1, false),
			setup: func(cfg *config.Config) {
				cfg.SeverityDefault = errorSeverity
				cfg.Rules[testRuleID1] = config.RuleConfig{Severity: &infoSeverity}
			},
			want: config.SeverityInfo,
		},
		{
			name: "loses to a rule_defaults severity",
			rule: newTestRule(testRuleID1, false),
			setup: func(cfg *config.Config) {
				cfg.SeverityDefault = errorSeverity
				cfg.RuleDefaults = map[string]config.RuleConfig{testRuleID1: {Severity: &infoSeverity}}
			},
			want: config.SeverityInfo,
		},
		{
			name:  "unset keeps the built-in default",
			rule:  &infoRule{BaseRule: lint.NewBaseRule(testRuleID1, "info-rule", "", nil, false)},
			setup: func(*config.Config) {},
			want:  config.SeverityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := config.NewConfig()
			tt.setup(cfg)

			assert.Equal(t, tt.want, resolveOne(t, tt.rule, cfg).Severity)
		})
	}
}

func TestResolveRules_AutoFix(t *testing.T) {
	t.Parallel()

	t.Run("disabled when fix flag not set", func(t *testing.T) {
		t.Parallel()

		registry := lint.NewRegistry()
		registry.Register(newTestRule(testRuleID1, true))

		cfg := config.NewConfig()
		cfg.Fix = false

		resolved := lint.ResolveRules(registry, cfg)

		if len(resolved) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(resolved))
		}

		if resolved[0].AutoFix {
			t.Error("AutoFix should be false when Fix flag is not set")
		}
	})

	t.Run("enabled when fix flag set", func(t *testing.T) {
		t.Parallel()

		registry := lint.NewRegistry()
		registry.Register(newTestRule(testRuleID1, true))

		cfg := config.NewConfig()
		cfg.Fix = true

		resolved := lint.ResolveRules(registry, cfg)

		if len(resolved) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(resolved))
		}

		if !resolved[0].AutoFix {
			t.Error("AutoFix should be true when Fix flag is set")
		}
	})

	t.Run("disabled via config even with fix flag", func(t *testing.T) {
		t.Parallel()

		registry := lint.NewRegistry()
		registry.Register(newTestRule(testRuleID1, true))

		cfg := config.NewConfig()
		cfg.Fix = true
		autoFix := false
		cfg.Rules[testRuleID1] = config.RuleConfig{AutoFix: &autoFix}

		resolved := lint.ResolveRules(registry, cfg)

		if len(resolved) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(resolved))
		}

		if resolved[0].AutoFix {
			t.Error("AutoFix should be false when disabled via config")
		}
	})
}

func TestResolveRules_FixRulesFilter(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(newTestRule(testRuleID1, true))
	registry.Register(newTestRule(testRuleID2, true))

	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.FixRules = []string{testRuleID1}

	resolved := lint.ResolveRules(registry, cfg)

	if len(resolved) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(resolved))
	}

	var rule1, rule2 *lint.ResolvedRule
	for idx := range resolved {
		if resolved[idx].Rule.ID() == testRuleID1 {
			rule1 = &resolved[idx]
		} else if resolved[idx].Rule.ID() == testRuleID2 {
			rule2 = &resolved[idx]
		}
	}

	if rule1 == nil || rule2 == nil {
		t.Fatal("expected both rules to be resolved")
	}

	if !rule1.AutoFix {
		t.Errorf("%s should have AutoFix enabled", testRuleID1)
	}
	if rule2.AutoFix {
		t.Errorf("%s should have AutoFix disabled due to FixRules filter", testRuleID2)
	}
}

// TestResolveRules_SeverityShorthandTurnsRulesOn covers a severity written
// as a rule's value, which markdownlint reads as the rule turned on at
// that severity: a rule that is off by default runs, and so does a rule
// that rule_defaults turns off. The severity setting alone leaves both
// off.
func TestResolveRules_SeverityShorthandTurnsRulesOn(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(&offByDefaultRule{BaseRule: lint.NewBaseRule("TEST002", "second-rule", "Second", nil, false)})
	registry.Register(newTestRule(testRuleID1, false))

	resolve := func(content string) map[string]config.Severity {
		t.Helper()

		cfg, err := config.FromYAML([]byte(content))
		if err != nil {
			t.Fatalf("FromYAML(%q): %v", content, err)
		}
		severs := make(map[string]config.Severity)
		for _, rule := range lint.ResolveRules(registry, cfg) {
			severs[rule.Rule.ID()] = rule.Severity
		}
		return severs
	}

	assert.Equal(t, map[string]config.Severity{"TEST002": config.SeverityWarning, testRuleID1: config.SeverityWarning},
		resolve("rules:\n  TEST002: warning\n"), "a rule that is off by default runs")
	assert.Equal(t, map[string]config.Severity{testRuleID1: config.SeverityError},
		resolve("rule_defaults:\n  MD001: false\nrules:\n  MD001: error\n"), "a rule that rule_defaults turns off runs")

	assert.Equal(t, map[string]config.Severity{testRuleID1: config.SeverityWarning},
		resolve("rules:\n  TEST002:\n    severity: warning\n"), "the setting alone leaves the rule off")
	assert.Empty(t, resolve("rule_defaults:\n  MD001: false\nrules:\n  MD001:\n    severity: error\n"),
		"the setting alone keeps a rule off that rule_defaults turns off")
}

func TestResolveRules_NilConfig(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(newTestRule(testRuleID1, true))

	resolved := lint.ResolveRules(registry, nil)

	if len(resolved) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resolved))
	}

	// With nil config, defaults should apply.
	if resolved[0].Severity != config.SeverityWarning {
		t.Errorf("expected warning severity, got %v", resolved[0].Severity)
	}
}

func TestResolvedRule_ConfigPresent(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(newTestRule(testRuleID1, false))

	cfg := config.NewConfig()
	cfg.Rules[testRuleID1] = config.RuleConfig{
		Options: map[string]any{"max_length": 80},
	}

	resolved := lint.ResolveRules(registry, cfg)

	if len(resolved) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resolved))
	}

	if resolved[0].Config == nil {
		t.Fatal("expected Config to be set")
	}

	if resolved[0].Config.Options["max_length"] != 80 {
		t.Errorf("expected max_length option to be 80")
	}
}
