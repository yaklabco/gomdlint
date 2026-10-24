package configloader

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// layeredParagraph is 54 characters long: over a limit of 20, under 80.
const layeredParagraph = "This paragraph is longer than twenty characters, yes.\n"

// lineLengthID is the ID of line-length, the rule that the layered tests
// configure.
const lineLengthID = "MD013"

// lowerLineLengthRules is a user config that gives line-length a limit of
// 20 under rules, with options that markdownlint's translation does not
// write out.
const lowerLineLengthRules = "rules:\n  MD013:\n    options:\n      max: 20\n      ignore_urls: true\n      strict: true\n"

// TestLoad_MarkdownlintSettingReplacesLowerLayerOptions covers a project's
// markdownlint config, translated for the run, over a user config that sets
// options of a rule that the markdownlint config names. In markdownlint
// 0.41.1 a rule key that turns the rule on, with true, a severity or an
// object, leaves every option that it does not set at its default, so the
// translation resets the rule's options: the user config's options do not
// apply. A key that turns the rule off resets nothing, and the options that
// #42 wrote out for it still replace the user config's, so a rule that
// --enable turns on again keeps the user config's other options.
// gomdlint's own config files keep their meaning: there, true turns the
// rule on and keeps the options of lower layers. The config that migrate
// writes behaves as the markdownlint config does.
func TestLoad_MarkdownlintSettingReplacesLowerLayerOptions(t *testing.T) {
	tests := []struct {
		name, user, markdownlint, project string
		enable                            bool
		options                           map[string]any // of MD013, as the rule reads them
		reports                           bool           // whether MD013 reports layeredParagraph
	}{
		{
			name:         "rule key over rule_defaults",
			user:         "rule_defaults:\n  MD013:\n    options:\n      max: 20\n      strict: true\n",
			markdownlint: `{"default": false, "MD013": true}`,
		},
		{name: "true over rules", user: lowerLineLengthRules, markdownlint: `{"MD013": true}`},
		{name: "a severity over rules", user: lowerLineLengthRules, markdownlint: `{"MD013": "warning"}`},
		{name: "a tag over rules", user: lowerLineLengthRules, markdownlint: `{"line_length": true}`},
		{name: "a name over rules", user: lowerLineLengthRules, markdownlint: `{"line-length": {}}`},
		{
			name: "an object over rules", user: lowerLineLengthRules, markdownlint: `{"MD013": {"line_length": 60}}`,
			options: map[string]any{"max": float64(60)},
		},
		{
			name:         "an object over rules and rule_defaults",
			user:         lowerLineLengthRules + "rule_defaults:\n  MD013:\n    options:\n      heading_max: 30\n",
			markdownlint: `{"MD013": {"line_length": 30}}`, options: map[string]any{"max": float64(30)}, reports: true,
		},
		{
			name: "false over rules", user: lowerLineLengthRules, markdownlint: `{"MD013": false}`, enable: true,
			options: map[string]any{
				"max": 80, "ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true, "strict": true,
			},
		},
		{
			name: "the default key over rules", user: lowerLineLengthRules, markdownlint: `{"default": true}`,
			options: map[string]any{
				"max": 20, "ignore_code_blocks": false, "ignore_urls": true, "markdownlint_compatible": true, "strict": true,
			},
			reports: true,
		},
		{
			name: "gomdlint's true over rules", user: lowerLineLengthRules, project: "rules:\n  MD013: true\n",
			options: map[string]any{"max": 20, "ignore_urls": true, "strict": true}, reports: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			writeMarkdownlint(t, dir, "xdg/gomdlint/config.yaml", tt.user)
			project := filepath.Join(dir, "project")
			if tt.markdownlint != "" {
				writeMarkdownlint(t, project, ".markdownlint.json", tt.markdownlint)
			}
			if tt.project != "" {
				writeMarkdownlint(t, project, ".gomdlint.yml", tt.project)
			}
			var cli *config.Config
			if tt.enable {
				cli = &config.Config{EnableRules: []string{lineLengthID}}
			}

			result, err := Load(context.Background(), LoadOptions{
				WorkingDir: project, IgnoreSystemConfig: true, IgnoreEnv: true, NonInteractive: true, CLIConfig: cli,
			})
			require.NoError(t, err)
			assertLineLength(t, result.Config, tt.options, tt.reports)

			if tt.markdownlint == "" {
				return
			}
			converted, err := ConvertMarkdownlintConfig(filepath.Join(project, ".markdownlint.json"))
			require.NoError(t, err)
			migratedProject := filepath.Join(dir, "migrated")
			require.NoError(t, os.MkdirAll(migratedProject, 0o755))
			migrated := filepath.Join(migratedProject, ".gomdlint.yml")
			content, err := converted.FileContent(migrated)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(migrated, content, 0o600))
			result, err = Load(context.Background(), LoadOptions{
				WorkingDir: migratedProject, IgnoreSystemConfig: true, IgnoreEnv: true, NonInteractive: true, CLIConfig: cli,
			})
			require.NoError(t, err)
			assertLineLength(t, result.Config, normalizedNumbers(tt.options), tt.reports)
		})
	}
}

// assertLineLength checks the options with which line-length runs under
// cfg, and whether it reports layeredParagraph.
func assertLineLength(t *testing.T, cfg *config.Config, options map[string]any, reports bool) {
	t.Helper()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	var got map[string]any
	runs := false
	for _, resolved := range lint.ResolveRules(registry, cfg) {
		if resolved.Rule.ID() != lineLengthID {
			continue
		}
		runs = true
		if resolved.Config != nil {
			got = resolved.Config.Options
		}
	}
	require.True(t, runs, "line-length runs")
	if len(options) == 0 {
		assert.Empty(t, got, "every option keeps its default")
	} else {
		assert.Equal(t, options, got)
	}
	assert.Equal(t, reports, len(lintWith(t, registry, cfg, layeredParagraph)) > 0, "reports")
}

// normalizedNumbers returns options with each float64 that holds a whole
// number as an int, as a gomdlint config file that migrate writes reads it.
func normalizedNumbers(options map[string]any) map[string]any {
	if options == nil {
		return nil
	}
	normalized := make(map[string]any, len(options))
	for key, value := range options {
		if number, ok := value.(float64); ok && number == float64(int(number)) {
			value = int(number)
		}
		normalized[key] = value
	}
	return normalized
}

// lintWith lints content with cfg and returns the lines that MD013 reports.
func lintWith(t *testing.T, registry *lint.Registry, cfg *config.Config, content string) []int {
	t.Helper()

	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
		context.Background(), "doc.md", []byte("# Title\n\n"+content), cfg)
	require.NoError(t, err)
	var lines []int
	for _, diag := range result.Diagnostics {
		if diag.RuleID == lineLengthID {
			lines = append(lines, diag.StartLine)
		}
	}
	return lines
}
