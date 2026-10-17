package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/internal/configloader"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// initTemplate runs "gomdlint init" with args in a fresh directory and
// returns the path and content of the file it wrote.
func initTemplate(t *testing.T, name string, args ...string) (string, string) {
	t.Helper()

	dir := inProject(t, nil)
	path := filepath.Join(dir, name)
	got := execute(t, append([]string{"init", "--output", path}, args...)...)
	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	return path, readFile(t, path)
}

// loadTemplate loads the config file at path the way the lint command
// does, without user, system or environment settings.
func loadTemplate(t *testing.T, path string) *config.Config {
	t.Helper()

	result, err := configloader.Load(context.Background(), configloader.LoadOptions{
		WorkingDir:         filepath.Dir(path),
		ExplicitPath:       path,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		IgnoreEnv:          true,
		NonInteractive:     true,
	})
	require.NoError(t, err)
	assert.Empty(t, result.Warnings, "a generated config loads without warnings")
	return result.Config
}

// requireDefaultResolution checks that cfg resolves every built-in rule
// exactly as a run without a config file does: the same rules enabled, with
// the same severities and fixes, and no options.
func requireDefaultResolution(t *testing.T, cfg *config.Config) {
	t.Helper()

	resolve := func(cfg *config.Config) map[string]lint.ResolvedRule {
		resolved := make(map[string]lint.ResolvedRule)
		for _, rr := range lint.ResolveRules(lint.DefaultRegistry, cfg) {
			if rr.Config != nil {
				assert.Empty(t, rr.Config.Options, "%s: a generated config sets no options", rr.Rule.ID())
			}
			resolved[rr.Rule.ID()] = lint.ResolvedRule{
				Enabled: rr.Enabled, Severity: rr.Severity, Fixable: rr.Fixable, AutoFix: rr.AutoFix,
			}
		}
		return resolved
	}

	defaults := config.NewConfig()
	assert.Equal(t, resolve(defaults), resolve(cfg), "the enabled rules must resolve as without a config")

	// Rules that are off by default only resolve when enabled.
	defaults.EnableRules = lint.DefaultRegistry.IDs()
	cfg.EnableRules = lint.DefaultRegistry.IDs()
	defer func() { cfg.EnableRules = nil }()
	assert.Equal(t, resolve(defaults), resolve(cfg), "every rule must resolve as without a config")
}

// TestInit_FullTemplateDocumentsEveryRule covers "init --full": every
// registered rule is listed, keyed by ID, with its real default enabled
// state, and its default severity as a comment, so the file changes
// nothing until it is edited and severity_default still works.
func TestInit_FullTemplateDocumentsEveryRule(t *testing.T) {
	path, content := initTemplate(t, ".gomdlint.yml", "--full")

	var parsed struct {
		Rules map[string]map[string]any `yaml:"rules"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(content), &parsed))

	ids := lint.DefaultRegistry.IDs()
	require.NotEmpty(t, ids)
	assert.Len(t, parsed.Rules, len(ids), "every registered rule is listed")
	for _, rule := range lint.DefaultRegistry.Rules() {
		settings, ok := parsed.Rules[rule.ID()]
		if !assert.True(t, ok, "%s is missing", rule.ID()) {
			continue
		}
		assert.Equal(t, map[string]any{"enabled": rule.DefaultEnabled()}, settings, rule.ID())
		for _, want := range []string{
			"  # " + rule.ID() + ": " + rule.Name() + "\n",
			"  " + rule.ID() + ":\n    enabled: " + strconv.FormatBool(rule.DefaultEnabled()) +
				"\n    # severity: " + string(rule.DefaultSeverity()) + "\n",
		} {
			assert.Contains(t, content, want)
		}
	}

	requireDefaultResolution(t, loadTemplate(t, path))

	t.Run("severity_default applies to every rule", func(t *testing.T) {
		require.NoError(t, os.WriteFile(path, []byte(content+"severity_default: error\n"), 0o644))

		for _, rr := range lint.ResolveRules(lint.DefaultRegistry, loadTemplate(t, path)) {
			assert.Equal(t, config.SeverityError, rr.Severity, rr.Rule.ID())
		}
	})
}

// TestInit_JSONTemplates covers "init --format json": the minimal template
// sets what the minimal YAML template sets and lists no rules, and the full
// one lists every rule with its default enabled state. Neither sets a
// severity or an option.
func TestInit_JSONTemplates(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		path, content := initTemplate(t, ".gomdlint.json", "--format", "json")

		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(content), &parsed), content)
		assert.Equal(t, map[string]any{"flavor": "gfm"}, parsed)

		requireDefaultResolution(t, loadTemplate(t, path))
	})

	t.Run("full", func(t *testing.T) {
		path, content := initTemplate(t, ".gomdlint.json", "--format", "json", "--full")

		var parsed struct {
			Rules map[string]map[string]any `json:"rules"`
		}
		require.NoError(t, json.Unmarshal([]byte(content), &parsed), content)
		assert.Len(t, parsed.Rules, len(lint.DefaultRegistry.IDs()), "every registered rule is listed")
		for _, rule := range lint.DefaultRegistry.Rules() {
			assert.Equal(t, map[string]any{"enabled": rule.DefaultEnabled()}, parsed.Rules[rule.ID()], rule.ID())
		}

		requireDefaultResolution(t, loadTemplate(t, path))
	})
}

// TestInit_MinimalYAMLTemplateChangesNothing covers plain "init".
func TestInit_MinimalYAMLTemplateChangesNothing(t *testing.T) {
	path, _ := initTemplate(t, ".gomdlint.yml")

	requireDefaultResolution(t, loadTemplate(t, path))
}

// TestInit_LintReadsTheWrittenFile covers "gomdlint init" followed by
// "gomdlint lint" without --config, in each format: lint reads the file
// that init wrote. Disabling a rule in that file shows whether it did.
func TestInit_LintReadsTheWrittenFile(t *testing.T) {
	tests := []struct {
		format  string
		name    string
		disable func(t *testing.T, content string) string
	}{
		{
			format: "yaml",
			name:   ".gomdlint.yml",
			disable: func(_ *testing.T, content string) string {
				return content + "rules:\n  no-trailing-spaces: false\n"
			},
		},
		{
			format: "json",
			name:   ".gomdlint.json",
			disable: func(t *testing.T, content string) string {
				t.Helper()
				var settings map[string]any
				require.NoError(t, json.Unmarshal([]byte(content), &settings), content)
				settings["rules"] = map[string]any{"no-trailing-spaces": false}
				edited, err := json.MarshalIndent(settings, "", "  ")
				require.NoError(t, err)
				return string(edited) + "\n"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			dir := inProject(t, map[string]string{"doc.md": trailingDoc})
			got := execute(t, "init", "--format", tt.format)
			require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
			path := filepath.Join(dir, tt.name)
			content := readFile(t, path)

			// The template changes no rule, so the finding remains.
			got = execute(t, "lint", "--color", "never", "doc.md")
			assert.Contains(t, got.stdout, "no-trailing-spaces", "stderr: %s", got.stderr)

			require.NoError(t, os.WriteFile(path, []byte(tt.disable(t, content)), 0o644))
			got = execute(t, "lint", "--color", "never", "doc.md")
			assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
			assert.Equal(t, "No issues found (1 file checked)\n", got.stdout)
			assert.Empty(t, got.stderr)
		})
	}
}

// TestInit_OutputNameChoosesJSON covers "init --output" with a .json name.
// A .json config must hold JSON, so without --format, init writes JSON, and
// --format yaml is refused rather than writing a file lint cannot read.
func TestInit_OutputNameChoosesJSON(t *testing.T) {
	t.Run("without --format", func(t *testing.T) {
		for _, name := range []string{".gomdlint.json", "lint.JSON"} {
			path, content := initTemplate(t, name)

			assert.True(t, json.Valid([]byte(content)), "%s holds JSON: %s", name, content)
			requireDefaultResolution(t, loadTemplate(t, path))
		}
	})

	t.Run("with --format yaml", func(t *testing.T) {
		dir := inProject(t, nil)
		path := filepath.Join(dir, ".gomdlint.json")

		got := execute(t, "init", "--format", "yaml", "--output", path)

		assert.Equal(t, cli.ExitInvalidUsage, got.code)
		assert.Contains(t, got.stderr, "a .json file must hold JSON")
		assert.NoFileExists(t, path)
	})

	t.Run("with --format json and another name", func(t *testing.T) {
		path, content := initTemplate(t, "lint.yml", "--format", "json")

		assert.True(t, json.Valid([]byte(content)), content)
		requireDefaultResolution(t, loadTemplate(t, path))
	})
}

// TestLint_YAMLConfigWinsOverJSON covers a directory that holds both
// .gomdlint.yml and .gomdlint.json: lint reads the YAML file and ignores
// the JSON one, as it prefers .gomdlint.yml over .gomdlint.yaml.
func TestLint_YAMLConfigWinsOverJSON(t *testing.T) {
	inProject(t, map[string]string{
		"doc.md":         trailingDoc,
		".gomdlint.yml":  "flavor: gfm\n",
		".gomdlint.json": `{"rules": {"no-trailing-spaces": false}}` + "\n",
	})

	got := execute(t, "lint", "--color", "never", "doc.md")

	assert.Contains(t, got.stdout, "no-trailing-spaces", "the JSON file must be ignored; stderr: %s", got.stderr)
}
