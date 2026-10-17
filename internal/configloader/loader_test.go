package configloader

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	_ "github.com/yaklabco/gomdlint/pkg/lint/rules" // Register rules
)

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	// Create temp directory with no config files
	tmpDir := t.TempDir()

	ctx := context.Background()
	opts := LoadOptions{
		WorkingDir:         tmpDir,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		NonInteractive:     true,
	}

	result, err := opts.load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if result.Config == nil {
		t.Fatal("Load() returned nil config")
	}

	// Check defaults are applied. The default flavor matches markdownlint,
	// which always parses GFM tables and autolink literals.
	if result.Config.Flavor != config.FlavorGFM {
		t.Errorf("expected flavor %q, got %q", config.FlavorGFM, result.Config.Flavor)
	}
}

func (o LoadOptions) load(ctx context.Context) (*LoadResult, error) {
	return Load(ctx, o)
}

func TestLoad_ProjectConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a project config
	// Note: jobs is a CLI-only option (yaml:"-"), so it won't be loaded from file
	configContent := `
flavor: gfm
rules:
  MD001:
    enabled: false
`
	configPath := filepath.Join(tmpDir, ".gomdlint.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx := context.Background()
	opts := LoadOptions{
		WorkingDir:         tmpDir,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		NonInteractive:     true,
	}

	result, err := Load(ctx, opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if result.Config.Flavor != config.FlavorGFM {
		t.Errorf("expected flavor %q, got %q", config.FlavorGFM, result.Config.Flavor)
	}

	// Check that the rule config was loaded
	md001, ok := result.Config.Rules["MD001"]
	if !ok {
		t.Fatal("MD001 rule not found in config")
	}
	if md001.Enabled == nil || *md001.Enabled {
		t.Error("expected MD001 to be disabled")
	}

	if len(result.LoadedFrom) != 1 {
		t.Errorf("expected 1 loaded file, got %d", len(result.LoadedFrom))
	}
}

func TestLoad_ExplicitConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a custom config
	// Note: format is a CLI-only option (yaml:"-"), so we test flavor instead
	configContent := `
flavor: gfm
severity_default: warning
`
	customPath := filepath.Join(tmpDir, "custom-config.yml")
	if err := os.WriteFile(customPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx := context.Background()
	opts := LoadOptions{
		WorkingDir:         tmpDir,
		ExplicitPath:       customPath,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		NonInteractive:     true,
	}

	result, err := Load(ctx, opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if result.Config.Flavor != config.FlavorGFM {
		t.Errorf("expected flavor %q, got %q", config.FlavorGFM, result.Config.Flavor)
	}

	if result.Config.SeverityDefault != "warning" {
		t.Errorf("expected severity_default %q, got %q", "warning", result.Config.SeverityDefault)
	}
}

func TestLoad_CLIOverrides(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a project config
	configContent := `
flavor: commonmark
jobs: 2
`
	configPath := filepath.Join(tmpDir, ".gomdlint.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx := context.Background()
	cliCfg := &config.Config{
		Flavor: config.FlavorGFM,
		Jobs:   8,
		Fix:    true,
	}
	opts := LoadOptions{
		WorkingDir:         tmpDir,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		NonInteractive:     true,
		CLIConfig:          cliCfg,
	}

	result, err := Load(ctx, opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// CLI should override project config
	if result.Config.Flavor != config.FlavorGFM {
		t.Errorf("expected flavor %q (CLI override), got %q", config.FlavorGFM, result.Config.Flavor)
	}

	if result.Config.Jobs != 8 {
		t.Errorf("expected jobs 8 (CLI override), got %d", result.Config.Jobs)
	}

	if !result.Config.Fix {
		t.Error("expected fix true (CLI override)")
	}
}

func TestLoad_InvalidConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create an invalid config
	configContent := `
flavor: invalid-flavor
`
	configPath := filepath.Join(tmpDir, ".gomdlint.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx := context.Background()
	opts := LoadOptions{
		WorkingDir:         tmpDir,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		NonInteractive:     true,
	}

	_, err := Load(ctx, opts)
	if err == nil {
		t.Fatal("expected validation error for invalid flavor")
	}
}

func TestLoad_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := LoadOptions{
		WorkingDir:         t.TempDir(),
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		NonInteractive:     true,
	}

	_, err := Load(ctx, opts)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestLoader_NormalizesRuleKeys(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create temp config file using rule names instead of IDs
	content := `
rules:
  no-trailing-spaces:
    enabled: false
  no-hard-tabs:
    enabled: true
    severity: error
`
	configPath := filepath.Join(tmpDir, ".gomdlint.yml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx := context.Background()
	opts := LoadOptions{
		WorkingDir:         tmpDir,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		NonInteractive:     true,
	}

	result, err := Load(ctx, opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should be normalized to IDs internally
	// MD009 is no-trailing-spaces, MD010 is no-hard-tabs
	_, hasID := result.Config.Rules["MD009"]
	_, hasName := result.Config.Rules["no-trailing-spaces"]

	if !hasID {
		t.Error("expected MD009 to be present after normalization")
	}
	if hasName {
		t.Error("expected no-trailing-spaces to be removed after normalization")
	}

	// Check MD010 (no-hard-tabs)
	md010, hasMD010 := result.Config.Rules["MD010"]
	if !hasMD010 {
		t.Error("expected MD010 to be present after normalization")
	} else {
		if md010.Enabled == nil || !*md010.Enabled {
			t.Error("expected MD010 to be enabled")
		}
		if md010.Severity == nil || *md010.Severity != "error" {
			t.Error("expected MD010 severity to be error")
		}
	}
}

// loadProject writes content as the project config in a fresh directory and
// loads it without machine-level configuration.
func loadProject(t *testing.T, content string) (*LoadResult, error) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gomdlint.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return Load(context.Background(), LoadOptions{
		WorkingDir:         dir,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		IgnoreEnv:          true,
		NonInteractive:     true,
	})
}

func TestLoad_WarnsAboutUnknownKeys(t *testing.T) {
	t.Parallel()

	content := `flavour: gfm
ignor:
  - "docs/**"
format: json
backups:
  enable: false
rules:
  MD013:
    line_length: 120
    enabled: true
  MD009:
    severity: error
unknown_section: 1
`
	result, err := loadProject(t, content)
	if err != nil {
		t.Fatalf("unknown keys must not fail loading: %v", err)
	}

	want := []string{
		`:1: unknown key "flavour" (did you mean "flavor"?)`,
		`:2: unknown key "ignor" (did you mean "ignore"?)`,
		`:4: "format" cannot be set in a config file; use the --format flag`,
		`:6: unknown key "backups.enable" (did you mean "backups.enabled"?)`,
		`:9: unknown key "rules.MD013.line_length"; rule options belong under "rules.MD013.options"`,
		`:13: unknown key "unknown_section"`,
	}
	joined := strings.Join(result.Warnings, "\n")
	for _, fragment := range want {
		if !strings.Contains(joined, fragment) {
			t.Errorf("warnings do not contain %q:\n%s", fragment, joined)
		}
	}
	if len(result.Warnings) != len(want) {
		t.Errorf("expected %d warnings, got %d:\n%s", len(want), len(result.Warnings), joined)
	}
}

func TestLoad_KnownKeysDoNotWarn(t *testing.T) {
	t.Parallel()

	content := `flavor: gfm
severity_default: warning
ignore:
  - "docs/**"
backups:
  enabled: false
  mode: sidecar
rules:
  MD013:
    enabled: true
    severity: error
    auto_fix: false
    options:
      max: 120
rule_defaults:
  MD009:
    enabled: true
`
	result, err := loadProject(t, content)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

// TestGeneratedTemplatesLoadCleanly keeps `gomdlint init` output free of
// settings the loader ignores.
func TestGeneratedTemplatesLoadCleanly(t *testing.T) {
	t.Parallel()

	for _, opts := range []config.TemplateOptions{
		{Format: "yaml"},
		{Format: "yaml", Full: true},
		{Format: "json"},
		{Format: "json", Full: true},
	} {
		content, err := config.GenerateTemplate(opts)
		if err != nil {
			t.Fatalf("GenerateTemplate(%+v) error = %v", opts, err)
		}

		result, err := loadProject(t, string(content))
		if err != nil {
			t.Fatalf("template %+v does not load: %v", opts, err)
		}
		if len(result.Warnings) != 0 {
			t.Errorf("template %+v loads with warnings: %v", opts, result.Warnings)
		}
	}
}

// TestLoad_IgnoreBaseDir covers the directory ignore patterns are relative
// to: the config file's own directory for project and explicit configs, and
// the working directory otherwise.
func TestLoad_IgnoreBaseDir(t *testing.T) {
	t.Parallel()

	t.Run("project config", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		sub := filepath.Join(dir, "docs", "guide")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".gomdlint.yml"), []byte("ignore:\n  - \"drafts/**\"\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		result, err := Load(context.Background(), isolatedOptions(sub))
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result.IgnoreBaseDir != dir {
			t.Errorf("IgnoreBaseDir = %q, want the config's directory %q", result.IgnoreBaseDir, dir)
		}
	})

	t.Run("explicit config", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		cfgDir := filepath.Join(dir, "config")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		explicit := filepath.Join(cfgDir, "lint.yml")
		if err := os.WriteFile(explicit, []byte("ignore:\n  - \"drafts/**\"\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		opts := isolatedOptions(dir)
		opts.ExplicitPath = explicit
		result, err := Load(context.Background(), opts)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result.IgnoreBaseDir != cfgDir {
			t.Errorf("IgnoreBaseDir = %q, want %q", result.IgnoreBaseDir, cfgDir)
		}
	})

	t.Run("command line", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".gomdlint.yml"), []byte("ignore:\n  - \"drafts/**\"\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		sub := filepath.Join(dir, "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		opts := isolatedOptions(sub)
		opts.CLIConfig = &config.Config{Ignore: []string{"tmp/**"}}
		result, err := Load(context.Background(), opts)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result.IgnoreBaseDir != sub {
			t.Errorf("IgnoreBaseDir = %q, want the working directory %q", result.IgnoreBaseDir, sub)
		}
	})

	t.Run("no patterns", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		result, err := Load(context.Background(), isolatedOptions(dir))
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result.IgnoreBaseDir != dir {
			t.Errorf("IgnoreBaseDir = %q, want %q", result.IgnoreBaseDir, dir)
		}
	})
}

func TestLoad_EnvIgnoreIsRelativeToWorkingDir(t *testing.T) {
	t.Setenv("GOMDLINT_IGNORE", "tmp/**")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gomdlint.yml"), []byte("ignore:\n  - \"drafts/**\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	opts := isolatedOptions(sub)
	opts.IgnoreEnv = false
	result, err := Load(context.Background(), opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.IgnoreBaseDir != sub {
		t.Errorf("IgnoreBaseDir = %q, want %q", result.IgnoreBaseDir, sub)
	}
}

// TestLoad_ExplicitConfigSkipsProjectDiscovery holds Load to its documented
// contract: --config replaces the project config instead of layering on it.
func TestLoad_ExplicitConfigSkipsProjectDiscovery(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gomdlint.yml"), []byte("flavor: commonmark\nrules:\n  MD013:\n    enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	explicit := filepath.Join(dir, "other.yml")
	if err := os.WriteFile(explicit, []byte("severity_default: error\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	opts := isolatedOptions(dir)
	opts.ExplicitPath = explicit
	result, err := Load(context.Background(), opts)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if _, ok := result.Config.Rules["MD013"]; ok {
		t.Error("the project config must not load alongside --config")
	}
	if !slices.Equal(result.LoadedFrom, []string{explicit}) {
		t.Errorf("LoadedFrom = %v, want only %s", result.LoadedFrom, explicit)
	}
}

func TestLoad_FileDiscoverySettings(t *testing.T) {
	t.Parallel()

	defaults, err := loadProject(t, "flavor: gfm\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !defaults.Config.GitignoreEnabled() || !defaults.Config.DefaultIgnoresEnabled() {
		t.Error("gitignore and default_ignores must default to enabled")
	}

	disabled, err := loadProject(t, "gitignore: false\ndefault_ignores: false\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if disabled.Config.GitignoreEnabled() || disabled.Config.DefaultIgnoresEnabled() {
		t.Error("a config file must be able to disable gitignore and default_ignores")
	}
}

func TestLoad_RejectsMalformedIgnorePatterns(t *testing.T) {
	t.Parallel()

	_, err := loadProject(t, "ignore:\n  - \"docs/[a-\"\n")
	if err == nil || !strings.Contains(err.Error(), "docs/[a-") {
		t.Errorf("expected an error naming the malformed pattern, got %v", err)
	}
}

// isolatedOptions loads from dir with no machine-level configuration.
func isolatedOptions(dir string) LoadOptions {
	return LoadOptions{
		WorkingDir:         dir,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		IgnoreEnv:          true,
		NonInteractive:     true,
	}
}

func TestLoad_ConfigFileDisablesBackups(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, "backups:\n  enabled: false\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if lint.BackupConfigFromConfig(result.Config).Enabled {
		t.Error("backups.enabled: false in a config file must disable backups")
	}
}

func TestLoad_BackupsDefaultToEnabled(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, "backups:\n  mode: sidecar\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !lint.BackupConfigFromConfig(result.Config).Enabled {
		t.Error("backups must stay enabled when no layer disables them")
	}
}

func TestLoad_LaterLayerReenablesBackups(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	explicit := filepath.Join(dir, "explicit.yml")
	if err := os.WriteFile(filepath.Join(dir, ".gomdlint.yml"), []byte("backups:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(explicit, []byte("backups:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := Load(context.Background(), LoadOptions{
		WorkingDir:         dir,
		ExplicitPath:       explicit,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		IgnoreEnv:          true,
		NonInteractive:     true,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !lint.BackupConfigFromConfig(result.Config).Enabled {
		t.Error("an explicit config must be able to re-enable backups")
	}
}

func TestLoad_EnvDisablesBackups(t *testing.T) {
	t.Setenv("GOMDLINT_BACKUPS_ENABLED", "false")

	dir := t.TempDir()
	result, err := Load(context.Background(), LoadOptions{
		WorkingDir:         dir,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		NonInteractive:     true,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if lint.BackupConfigFromConfig(result.Config).Enabled {
		t.Error("GOMDLINT_BACKUPS_ENABLED=false must disable backups")
	}
}

// TestLoader_RejectsDuplicateRules covers one file configuring a rule under
// two keys. Map order would pick a winner at random, so loading fails and
// names both keys.
func TestLoader_RejectsDuplicateRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		keys    []string
	}{
		{
			name:    "ID and name",
			content: "rules:\n  MD009:\n    enabled: false\n  no-trailing-spaces:\n    enabled: true\n",
			keys:    []string{`"MD009"`, `"no-trailing-spaces"`},
		},
		{
			name:    "ID and alias",
			content: "rules:\n  MD004:\n    enabled: false\n  ul-style:\n    enabled: true\n",
			keys:    []string{`"MD004"`, `"ul-style"`},
		},
		{
			name:    "ID in two cases",
			content: "rules:\n  MD009:\n    enabled: false\n  md009:\n    enabled: true\n",
			keys:    []string{`"MD009"`, `"md009"`},
		},
		{
			name:    "rule_defaults",
			content: "rule_defaults:\n  MD009:\n    enabled: false\n  no-trailing-spaces:\n    enabled: true\n",
			keys:    []string{`"MD009"`, `"no-trailing-spaces"`, "rule_defaults"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for range 5 { // Map order varies between runs; the outcome must not.
				_, err := loadProject(t, tt.content)
				if err == nil {
					t.Fatal("expected an error for a rule configured under two keys")
				}
				for _, key := range tt.keys {
					if !strings.Contains(err.Error(), key) {
						t.Errorf("error %q does not name %s", err, key)
					}
				}
			}
		})
	}
}

func TestLoader_NormalizesRuleKeyCase(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, "rules:\n  md009:\n    enabled: false\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	md009, ok := result.Config.Rules["MD009"]
	if !ok || md009.Enabled == nil || *md009.Enabled {
		t.Errorf("expected md009 to configure MD009, got rules %v", result.Config.Rules)
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "md009") {
			t.Errorf("unexpected warning: %s", warning)
		}
	}
}

// TestLoader_LayersMergeAcrossRuleKeys covers different files naming the
// same rule differently: the later layer wins like any other setting.
func TestLoader_LayersMergeAcrossRuleKeys(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gomdlint.yml"), []byte("rules:\n  MD009:\n    enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	severity := string(config.SeverityError)

	for range 5 {
		result, err := Load(context.Background(), LoadOptions{
			WorkingDir:         dir,
			IgnoreSystemConfig: true,
			IgnoreUserConfig:   true,
			IgnoreMarkdownlint: true,
			IgnoreEnv:          true,
			NonInteractive:     true,
			CLIConfig: &config.Config{Rules: map[string]config.RuleConfig{
				"no-trailing-spaces": {Severity: &severity},
			}},
		})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if len(result.Config.Rules) != 1 {
			t.Fatalf("expected one normalized rule, got %v", result.Config.Rules)
		}
		md009 := result.Config.Rules["MD009"]
		if md009.Enabled == nil || *md009.Enabled {
			t.Error("the project layer's enabled: false must survive the merge")
		}
		if md009.Severity == nil || *md009.Severity != severity {
			t.Error("the CLI layer's severity must apply to MD009")
		}
	}
}
