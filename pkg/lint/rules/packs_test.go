package rules

import (
	"testing"
)

func TestPacks(t *testing.T) {
	packs := Packs()

	// Verify we have the expected number of packs.
	expectedCount := 4
	if len(packs) != expectedCount {
		t.Errorf("got %d packs, want %d", len(packs), expectedCount)
	}

	// Verify each pack has required fields.
	for _, pack := range packs {
		if pack.Name == "" {
			t.Error("pack has empty name")
		}
		if pack.Description == "" {
			t.Errorf("pack %q has empty description", pack.Name)
		}
		if len(pack.Rules) == 0 {
			t.Errorf("pack %q has no rules", pack.Name)
		}

		// Verify each rule config is valid.
		for ruleID, cfg := range pack.Rules {
			if cfg.Enabled == nil {
				t.Errorf("pack %q rule %q has nil Enabled", pack.Name, ruleID)
			}
			if cfg.Severity == nil {
				t.Errorf("pack %q rule %q has nil Severity", pack.Name, ruleID)
			}
		}
	}
}

func TestPackByName(t *testing.T) {
	tests := []struct {
		name  string
		want  bool
		rules int
	}{
		{"core", true, 10},
		{"strict", true, 33},
		{"relaxed", true, 2},
		{"gfm", true, 12},
		{"nonexistent", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pack := PackByName(tt.name)
			if tt.want {
				if pack == nil {
					t.Errorf("PackByName(%q) returned nil, want pack", tt.name)
					return
				}
				if pack.Name != tt.name {
					t.Errorf("pack.Name = %q, want %q", pack.Name, tt.name)
				}
				if len(pack.Rules) != tt.rules {
					t.Errorf("pack %q has %d rules, want %d", tt.name, len(pack.Rules), tt.rules)
				}
			} else if pack != nil {
				t.Errorf("PackByName(%q) returned pack, want nil", tt.name)
			}
		})
	}
}

func TestPackNames(t *testing.T) {
	names := PackNames()

	expected := []string{"core", "strict", "relaxed", "gfm"}
	if len(names) != len(expected) {
		t.Errorf("got %d names, want %d", len(names), len(expected))
	}

	for i, name := range expected {
		if names[i] != name {
			t.Errorf("names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

func TestCorePack(t *testing.T) {
	pack := CorePack()

	// Core pack should have essential rules.
	essentialRules := []string{"MD009", "MD047", "MD012", "MD001"}
	for _, ruleID := range essentialRules {
		if _, ok := pack.Rules[ruleID]; !ok {
			t.Errorf("core pack missing essential rule %q", ruleID)
		}
	}

	// All rules should be enabled with warning or info severity.
	for ruleID, cfg := range pack.Rules {
		if cfg.Enabled == nil || !*cfg.Enabled {
			t.Errorf("core pack rule %q should be enabled", ruleID)
		}
		if cfg.Severity == nil {
			t.Errorf("core pack rule %q has no severity", ruleID)
			continue
		}
		sev := *cfg.Severity
		if sev != "warning" && sev != "info" {
			t.Errorf("core pack rule %q has severity %q, want warning or info", ruleID, sev)
		}
	}
}

func TestStrictPack(t *testing.T) {
	pack := StrictPack()

	// Strict pack should have HTML rule.
	if _, ok := pack.Rules["MD033"]; !ok {
		t.Error("strict pack missing MD033 (no-inline-html)")
	}

	// Most rules should be error severity.
	errorCount := 0
	for _, cfg := range pack.Rules {
		if cfg.Severity != nil && *cfg.Severity == "error" {
			errorCount++
		}
	}

	// At least 10 rules should be errors.
	if errorCount < 10 {
		t.Errorf("strict pack has %d error rules, want at least 10", errorCount)
	}
}

func TestRelaxedPack(t *testing.T) {
	pack := RelaxedPack()

	// Relaxed pack should have very few rules.
	if len(pack.Rules) > 5 {
		t.Errorf("relaxed pack has %d rules, want <= 5", len(pack.Rules))
	}

	// All rules should be info severity.
	for ruleID, cfg := range pack.Rules {
		if cfg.Severity != nil && *cfg.Severity != "info" {
			t.Errorf("relaxed pack rule %q has severity %q, want info", ruleID, *cfg.Severity)
		}
	}
}

func TestGFMAuthoringPack(t *testing.T) {
	pack := GFMAuthoringPack()

	// GFM pack should have table rules.
	tableRules := []string{"MDL002", "MDL003", "MDL004"}
	for _, ruleID := range tableRules {
		if _, ok := pack.Rules[ruleID]; !ok {
			t.Errorf("GFM pack missing table rule %q", ruleID)
		}
	}

	// GFM pack should have link/image rules.
	if _, ok := pack.Rules["MD042"]; !ok {
		t.Error("GFM pack missing MD042 (no-empty-links)")
	}
	if _, ok := pack.Rules["MD045"]; !ok {
		t.Error("GFM pack missing MD045 (image-alt-text)")
	}
}
