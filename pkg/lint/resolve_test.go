package lint_test

import (
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/lint"
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
	// First disable via CLI, then enable via config.
	cfg.DisableRules = []string{testRuleID1}
	enabled := true
	cfg.Rules[testRuleID1] = config.RuleConfig{Enabled: &enabled}

	resolved := lint.ResolveRules(registry, cfg)

	// Config should override CLI disable.
	if len(resolved) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resolved))
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
