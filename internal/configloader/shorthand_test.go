package configloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestLoad_RuleBooleanShorthand covers rules set to true or false, as
// markdownlint allows, by name, alias or ID. They load without warnings,
// and false disables a rule while true enables one.
func TestLoad_RuleBooleanShorthand(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, "rules:\n  line-length: false\n  MD033: true\n  ul-style: false\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}

	for id, want := range map[string]bool{"MD013": false, "MD033": true, "MD004": false} {
		rule, ok := result.Config.Rules[id]
		if !ok || rule.Enabled == nil {
			t.Errorf("%s: not configured: %+v", id, result.Config.Rules)
			continue
		}
		if *rule.Enabled != want {
			t.Errorf("%s: enabled = %v, want %v", id, *rule.Enabled, want)
		}
	}
}

// TestLoad_RuleBooleanShorthandValidatesLikeTheMapping covers validation of
// the shorthand: an unknown rule is a warning either way, and a value that
// is neither a boolean nor a mapping is an error that names its line.
func TestLoad_RuleBooleanShorthandValidatesLikeTheMapping(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"rules:\n  no-such-rule: false\n",
		"rules:\n  no-such-rule:\n    enabled: false\n",
	} {
		result, err := loadProject(t, content)
		if err != nil {
			t.Fatalf("Load(%q) error = %v", content, err)
		}
		if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], `unknown rule "no-such-rule"`) {
			t.Errorf("Load(%q) warnings = %v, want one unknown rule warning", content, result.Warnings)
		}
	}

	for content, line := range map[string]string{
		"rules:\n  MD013: off\n":                 "line 2",
		"rules:\n  MD013:\n    enabled: maybe\n": "line 3",
	} {
		_, err := loadProject(t, content)
		if err == nil || !strings.Contains(err.Error(), line) {
			t.Errorf("Load(%q) error = %v, want an error at %s", content, err, line)
		}
	}
}

// TestLoad_RuleBooleanShorthandInJSON covers a JSON config file given with
// --config.
func TestLoad_RuleBooleanShorthandInJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "gomdlint.json")
	if err := os.WriteFile(path, []byte(`{"rules": {"line-length": false, "MD033": true}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := Load(context.Background(), LoadOptions{
		WorkingDir:         dir,
		ExplicitPath:       path,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		IgnoreEnv:          true,
		NonInteractive:     true,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	if rule := result.Config.Rules["MD013"]; rule.Enabled == nil || *rule.Enabled {
		t.Errorf("MD013 = %+v, want disabled", rule)
	}
	if rule := result.Config.Rules["MD033"]; rule.Enabled == nil || !*rule.Enabled {
		t.Errorf("MD033 = %+v, want enabled", rule)
	}
}

// TestLoad_RuleBooleanShorthandKeepsLowerLayerOptions covers a project
// config that turns on a rule a user config configured: true only sets
// enabled, so the user's options still apply, as with "enabled: true".
func TestLoad_RuleBooleanShorthandKeepsLowerLayerOptions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	user := filepath.Join(dir, "user.yaml")
	project := filepath.Join(dir, "project.yml")
	if err := os.WriteFile(user, []byte("rules:\n  MD013:\n    enabled: false\n    options:\n      max: 120\n"), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}
	if err := os.WriteFile(project, []byte("rules:\n  line-length: true\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	cfg := mergeFiles(t, user, project)

	rule := cfg.Rules["MD013"]
	if rule.Enabled == nil || !*rule.Enabled {
		t.Errorf("MD013 = %+v, want enabled", rule)
	}
	if rule.Options["max"] != 120 {
		t.Errorf("MD013 options = %v, want the user's max of 120", rule.Options)
	}
}

// mergeFiles loads config files in order of increasing precedence, as Load
// merges its layers.
func mergeFiles(t *testing.T, paths ...string) *config.Config {
	t.Helper()

	var cfg *config.Config
	for _, path := range paths {
		fileCfg, warnings, err := loadConfigFile(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if len(warnings) != 0 {
			t.Errorf("%s: unexpected warnings: %v", path, warnings)
		}
		cfg = merge(cfg, fileCfg)
	}
	return cfg
}
