package configloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"

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

	return loadProjectFile(t, ".gomdlint.yml", content)
}

// loadProjectFile loads a project whose config file, named name, holds
// content.
func loadProjectFile(t *testing.T, name, content string) (*LoadResult, error) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
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
		`:4: "format" cannot be set in a config file; use the --format flag or the GOMDLINT_FORMAT environment variable`,
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

// TestLoad_CommandLineOnlyKeysPointAtTheirSettings covers settings that
// config files cannot hold. The warning names the flag, and the environment
// variable that sets a lasting default where there is one.
func TestLoad_CommandLineOnlyKeysPointAtTheirSettings(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, "rule_format: id\nenable_rules: [MD001]\n")
	if err != nil {
		t.Fatalf("command-line-only keys must not fail loading: %v", err)
	}

	want := []string{
		`:1: "rule_format" cannot be set in a config file; use the --rule-format flag or the GOMDLINT_RULE_FORMAT environment variable`,
		`:2: "enable_rules" cannot be set in a config file; use the --enable flag` + "\n",
	}
	joined := strings.Join(result.Warnings, "\n") + "\n"
	for _, fragment := range want {
		if !strings.Contains(joined, fragment) {
			t.Errorf("warnings do not contain %q:\n%s", fragment, joined)
		}
	}
	if result.Config.RuleFormat != config.RuleFormatName {
		t.Errorf("rule format = %q, want the default %q", result.Config.RuleFormat, config.RuleFormatName)
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

	// Each template loads under the name init writes it to.
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

		name := ".gomdlint.yml"
		if opts.Format == "json" {
			name = ".gomdlint.json"
		}
		result, err := loadProjectFile(t, name, string(content))
		if err != nil {
			t.Fatalf("template %+v does not load: %v", opts, err)
		}
		if len(result.LoadedFrom) != 1 || filepath.Base(result.LoadedFrom[0]) != name {
			t.Errorf("template %+v: loaded from %v, want %s", opts, result.LoadedFrom, name)
		}
		if len(result.Warnings) != 0 {
			t.Errorf("template %+v loads with warnings: %v", opts, result.Warnings)
		}
	}
}

// escapeUTF16 writes char as JSON escapes of its UTF-16 code units, the way
// serializers that escape every non-ASCII character write characters
// outside the Basic Multilingual Plane.
func escapeUTF16(char rune) string {
	high, low := utf16.EncodeRune(char)
	return fmt.Sprintf(`\u%04x\u%04x`, high, low)
}

// TestLoad_JSONProjectConfig covers .gomdlint.json, which "gomdlint init
// --format json" writes. It is read as JSON, including what the YAML
// decoder rejects or reads differently.
func TestLoad_JSONProjectConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		content    string
		wantIgnore []string
	}{
		{
			name: "tabs, escaped slashes and surrogate pairs",
			content: "{\n" +
				"\t\"flavor\": \"commonmark\",\n" +
				"\t\"ignore\": [\"docs\\/drafts\\/**\", \"" + escapeUTF16('😀') + "/**\"],\n" +
				"\t\"rules\": {\"MD009\": {\"enabled\": false}}\n" +
				"}\n",
			wantIgnore: []string{"docs/drafts/**", "😀/**"},
		},
		{
			name:       "byte order mark",
			content:    "\xef\xbb\xbf{\"flavor\": \"commonmark\", \"rules\": {\"MD009\": false}, \"ignore\": [\"a\"]}",
			wantIgnore: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := loadProjectFile(t, ".gomdlint.json", tt.content)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if len(result.LoadedFrom) != 1 || filepath.Base(result.LoadedFrom[0]) != ".gomdlint.json" {
				t.Errorf("loaded from %v, want the .gomdlint.json project config", result.LoadedFrom)
			}
			if result.Config.Flavor != config.FlavorCommonMark {
				t.Errorf("flavor = %q, want %q", result.Config.Flavor, config.FlavorCommonMark)
			}
			if !slices.Equal(result.Config.Ignore, tt.wantIgnore) {
				t.Errorf("ignore = %q, want %q", result.Config.Ignore, tt.wantIgnore)
			}
			if md009 := result.Config.Rules["MD009"]; md009.Enabled == nil || *md009.Enabled {
				t.Errorf("MD009 must be disabled, got rules %v", result.Config.Rules)
			}
			if len(result.Warnings) != 0 {
				t.Errorf("unexpected warnings: %v", result.Warnings)
			}
		})
	}
}

// TestLoad_JSONNumbers covers numbers in a JSON config file, which load as
// the same numbers in a YAML file do: integers as int, so rules read them
// as they read YAML options, and numbers too large for a float64 as the
// YAML decoder reads them, without failing the file.
func TestLoad_JSONNumbers(t *testing.T) {
	t.Parallel()

	const options = `{"line_length": 100, "ratio": 0.5, "huge": 1e400}`
	jsonResult, err := loadProjectFile(t, ".gomdlint.json", `{"rules": {"MD013": {"options": `+options+`}}}`)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	yamlResult, err := loadProject(t, "rules:\n  MD013:\n    options: "+options+"\n")
	if err != nil {
		t.Fatalf("Load() of the YAML equivalent error = %v", err)
	}

	got := jsonResult.Config.Rules["MD013"].Options
	if lineLength, ok := got["line_length"].(int); !ok || lineLength != 100 {
		t.Errorf("line_length = %#v, want int 100", got["line_length"])
	}
	if want := yamlResult.Config.Rules["MD013"].Options; fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", want) {
		t.Errorf("options = %#v, want %#v as from YAML", got, want)
	}
}

// TestLoad_JSONExplicitConfig covers --config with a .json file, which is
// read as JSON like a discovered one.
func TestLoad_JSONExplicitConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "lint-settings.json")
	if err := os.WriteFile(path, []byte(`{"ignore": ["build\/**"]}`), 0o644); err != nil {
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
	if want := []string{"build/**"}; !slices.Equal(result.Config.Ignore, want) {
		t.Errorf("ignore = %q, want %q", result.Config.Ignore, want)
	}
}

// TestLoad_RejectsMalformedJSON covers JSON config files that are not JSON.
// The error names the line and column. The YAML decoder would load some of
// them, such as a missing value, which it reads as null.
func TestLoad_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "missing value", content: "{\n  \"flavor\": \n}\n", want: []string{"line 3, column 1"}},
		{name: "trailing comma", content: "{\n  \"flavor\": \"gfm\",\n}\n", want: []string{"line 3, column 1"}},
		{name: "YAML", content: "flavor: gfm\n", want: []string{"line 1, column 2"}},
		{name: "empty", content: "", want: []string{"unexpected end of JSON input"}},
		{name: "duplicate key", content: `{"flavor": "gfm", "flavor": "commonmark"}`, want: []string{`"flavor" already defined`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := loadProjectFile(t, ".gomdlint.json", tt.content)
			if err == nil {
				t.Fatal("Load() succeeded, want an error")
			}
			for _, fragment := range append([]string{".gomdlint.json: parse JSON: "}, tt.want...) {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("error %q does not contain %q", err, fragment)
				}
			}
		})
	}
}

// TestLoad_JSONFileHoldingYAML covers a .json config that holds YAML, as
// "gomdlint init --output x.json" and "gomdlint migrate --output x.json"
// wrote before they wrote JSON for such a name. Loading fails, and the error
// says what to do. JSON that is merely malformed gets no such advice.
func TestLoad_JSONFileHoldingYAML(t *testing.T) {
	t.Parallel()

	const hint = "it looks like YAML; rename it to .yml, or rewrite it as JSON"

	_, err := loadProjectFile(t, ".gomdlint.json", "# gomdlint configuration\nflavor: gfm\nrules:\n  MD009: false\n")
	if err == nil || !strings.Contains(err.Error(), "parse JSON: ") || !strings.Contains(err.Error(), hint) {
		t.Errorf("error = %v, want a JSON parse error with the hint %q", err, hint)
	}

	_, err = loadProjectFile(t, ".gomdlint.json", "{\n  \"flavor\": \n}\n")
	if err == nil || strings.Contains(err.Error(), hint) {
		t.Errorf("error = %v, want a JSON parse error without the hint", err)
	}
}

// TestLoad_WarnsAboutUnknownKeysInJSON covers the unknown key warnings of a
// JSON config file, which name the line of the key like those of a YAML one.
func TestLoad_WarnsAboutUnknownKeysInJSON(t *testing.T) {
	t.Parallel()

	content := "{\n" +
		"  \"flavour\": \"gfm\",\n" +
		"  \"rules\": {\n" +
		"    \"MD013\": {\"line_length\": 100, \"note\": \"a\\/b\"}\n" +
		"  }\n" +
		"}\n"
	result, err := loadProjectFile(t, ".gomdlint.json", content)
	if err != nil {
		t.Fatalf("unknown keys must not fail loading: %v", err)
	}

	want := []string{
		`.gomdlint.json:2: unknown key "flavour" (did you mean "flavor"?)`,
		`.gomdlint.json:4: unknown key "rules.MD013.line_length"; rule options belong under "rules.MD013.options"`,
		`.gomdlint.json:4: unknown key "rules.MD013.note"; rule options belong under "rules.MD013.options"`,
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

// relativeWarnings returns warnings with the directory of the config file,
// the part before name, cut off.
func relativeWarnings(warnings []string, name string) []string {
	relative := make([]string, len(warnings))
	for idx, warning := range warnings {
		relative[idx] = warning[max(strings.Index(warning, name), 0):]
	}
	return relative
}

// TestLoad_WarnsAboutUnknownRules covers keys under rules and rule_defaults
// that name no rule. The warning names the file and line of the key and the
// closest rule. Those keys take rules only, so a typo is never offered a
// tag, and a tag gets the names of its rules.
func TestLoad_WarnsAboutUnknownRules(t *testing.T) {
	t.Parallel()

	content := `rules:
  no-trailling-spaces:
    enabled: false
  blockquote: false
  tables: false
  MD009:
    severity: error
rule_defaults:
  linelength:
    enabled: false
`
	result, err := loadProject(t, content)
	if err != nil {
		t.Fatalf("unknown rules must not fail loading: %v", err)
	}

	want := []string{
		`.gomdlint.yml:2: unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?); it is ignored`,
		`.gomdlint.yml:4: unknown rule "blockquote" (it is a tag; configure its rules instead: ` +
			`no-multiple-space-blockquote, no-blanks-blockquote); it is ignored`,
		`.gomdlint.yml:5: unknown rule "tables" (did you mean "blanks-around-tables"?); it is ignored`,
		`.gomdlint.yml:9: unknown rule "linelength" (did you mean "line-length"?); it is ignored`,
	}
	assert.Equal(t, want, relativeWarnings(result.Warnings, ".gomdlint.yml"))
}

// TestLoad_RuleKeysFromYAMLMergeKeys covers YAML merge keys, which bring in
// the keys of an anchored mapping: under rules, the rules of another
// section, and under a rule, the settings of another rule. A merge key is
// not a rule or a setting, so it must not be reported as unknown, and the
// merged keys apply.
func TestLoad_RuleKeysFromYAMLMergeKeys(t *testing.T) {
	t.Parallel()

	content := `rule_defaults: &base
  MD013:
    enabled: false
rules:
  <<: *base
  MD009: &off
    enabled: false
  MD010:
    <<: *off
    severity: error
`
	result, err := loadProject(t, content)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	assert.Empty(t, result.Warnings)
	for _, ruleID := range []string{"MD009", "MD010", "MD013"} {
		rule, ok := result.Config.Rules[ruleID]
		if assert.True(t, ok, "%s is configured: %v", ruleID, result.Config.Rules) && assert.NotNil(t, rule.Enabled, ruleID) {
			assert.False(t, *rule.Enabled, ruleID)
		}
	}
}

func TestLoad_WarnsAboutUnknownRulesInJSON(t *testing.T) {
	t.Parallel()

	content := "{\n  \"rules\": {\n    \"no-trailling-spaces\": false\n  }\n}\n"
	result, err := loadProjectFile(t, ".gomdlint.json", content)
	if err != nil {
		t.Fatalf("unknown rules must not fail loading: %v", err)
	}

	assert.Equal(t, []string{
		`.gomdlint.json:3: unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?); it is ignored`,
	}, relativeWarnings(result.Warnings, ".gomdlint.json"))
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

// TestLoad_EnvRuleFormat covers GOMDLINT_RULE_FORMAT, which sets how output
// names rules unless the command line sets it. Config files cannot set it.
func TestLoad_EnvRuleFormat(t *testing.T) {
	load := func(t *testing.T, cli *config.Config) (*LoadResult, error) {
		t.Helper()

		return Load(context.Background(), LoadOptions{
			WorkingDir:         t.TempDir(),
			IgnoreSystemConfig: true,
			IgnoreUserConfig:   true,
			IgnoreMarkdownlint: true,
			NonInteractive:     true,
			CLIConfig:          cli,
		})
	}

	t.Run("defaults to name", func(t *testing.T) {
		t.Setenv("GOMDLINT_RULE_FORMAT", "")

		result, err := load(t, nil)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result.Config.RuleFormat != config.RuleFormatName {
			t.Errorf("rule format = %q, want %q", result.Config.RuleFormat, config.RuleFormatName)
		}
	})

	t.Run("selects the format", func(t *testing.T) {
		t.Setenv("GOMDLINT_RULE_FORMAT", "id")

		result, err := load(t, nil)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result.Config.RuleFormat != config.RuleFormatID {
			t.Errorf("rule format = %q, want %q", result.Config.RuleFormat, config.RuleFormatID)
		}
	})

	t.Run("loses to the command line", func(t *testing.T) {
		t.Setenv("GOMDLINT_RULE_FORMAT", "id")

		result, err := load(t, &config.Config{RuleFormat: config.RuleFormatCombined})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if result.Config.RuleFormat != config.RuleFormatCombined {
			t.Errorf("rule format = %q, want %q", result.Config.RuleFormat, config.RuleFormatCombined)
		}
	})

	t.Run("rejects an invalid value", func(t *testing.T) {
		t.Setenv("GOMDLINT_RULE_FORMAT", "ids")

		_, err := load(t, nil)
		if err == nil {
			t.Fatal("Load() succeeded with an invalid rule format")
		}
		for _, want := range []string{`invalid rule format "ids"`, "name, id, combined"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
		}
	})
}

// TestLoader_RejectsDuplicateRules covers one file configuring a rule under
// two keys. Map order would pick a winner at random, so loading fails and
// names both keys.
func TestLoader_RejectsDuplicateRules(t *testing.T) {
	t.Parallel()

	// The message names the rule by name, then ID.
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "ID and name",
			content: "rules:\n  MD009:\n    enabled: false\n  no-trailing-spaces:\n    enabled: true\n",
			want:    `rules: "MD009" and "no-trailing-spaces" both configure no-trailing-spaces (MD009); keep one of them`,
		},
		{
			name:    "ID and alias",
			content: "rules:\n  MD004:\n    enabled: false\n  ul-style:\n    enabled: true\n",
			want:    `rules: "MD004" and "ul-style" both configure unordered-list-style (MD004); keep one of them`,
		},
		{
			name:    "ID in two cases",
			content: "rules:\n  MD009:\n    enabled: false\n  md009:\n    enabled: true\n",
			want:    `rules: "MD009" and "md009" both configure no-trailing-spaces (MD009); keep one of them`,
		},
		{
			name:    "rule_defaults",
			content: "rule_defaults:\n  MD009:\n    enabled: false\n  no-trailing-spaces:\n    enabled: true\n",
			want:    `rule_defaults: "MD009" and "no-trailing-spaces" both configure no-trailing-spaces (MD009); keep one of them`,
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
				if !strings.HasSuffix(err.Error(), ".gomdlint.yml: "+tt.want) {
					t.Errorf("error = %q, want it to end with %q", err, ".gomdlint.yml: "+tt.want)
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
