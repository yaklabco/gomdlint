package configloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/config"
	_ "github.com/jamesainslie/gomdlint/pkg/lint/rules" // Register rules
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

	// Check defaults are applied
	if result.Config.Flavor != config.FlavorCommonMark {
		t.Errorf("expected flavor %q, got %q", config.FlavorCommonMark, result.Config.Flavor)
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

func TestLoader_WarnsDuplicateRules(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create config with both ID and name for same rule
	content := `
rules:
  MD009:
    enabled: false
  no-trailing-spaces:
    enabled: true
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

	// Should have a warning about duplicate rule
	foundWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "duplicate") && strings.Contains(w, "MD009") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("expected warning about duplicate rule, got warnings: %v", result.Warnings)
	}

	// Verify the rule is normalized to canonical ID and has a value
	// Note: which value "wins" is undefined since Go map iteration order is non-deterministic
	md009, ok := result.Config.Rules["MD009"]
	if !ok {
		t.Fatal("expected MD009 in config")
	}
	if md009.Enabled == nil {
		t.Error("expected MD009.Enabled to be set")
	}
}
