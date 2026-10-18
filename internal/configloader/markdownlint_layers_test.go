package configloader

import (
	"context"
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

// TestLoad_MarkdownlintSettingReplacesLowerLayerOptions covers a project's
// markdownlint config, translated for the run, over a user config that sets
// options of a rule that the markdownlint config names. As #42 did, the
// translation writes out markdownlint's defaults for some options of some
// rules, so those options replace the user config's, while the others keep
// the user config's values.
func TestLoad_MarkdownlintSettingReplacesLowerLayerOptions(t *testing.T) {
	tests := []struct {
		name, user, markdownlint string
		options                  map[string]any // of MD013, as the rule reads them
	}{
		{
			name:         "default key and rule key over rule_defaults",
			user:         "rule_defaults:\n  MD013:\n    options:\n      max: 20\n",
			markdownlint: `{"default": false, "MD013": true}`,
			options:      map[string]any{"max": 80, "ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true},
		},
		{
			name:         "rule key over rules",
			user:         "rules:\n  MD013:\n    options:\n      max: 20\n      ignore_urls: true\n      strict: true\n",
			markdownlint: `{"MD013": true}`,
			options: map[string]any{
				"max": 80, "ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true, "strict": true,
			},
		},
		{
			name:         "rule key with options over rules",
			user:         "rules:\n  MD013:\n    options:\n      max: 20\n",
			markdownlint: `{"MD013": {"line_length": 60}}`,
			options: map[string]any{
				"max": float64(60), "ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			writeMarkdownlint(t, dir, "xdg/gomdlint/config.yaml", tt.user)
			project := filepath.Join(dir, "project")
			writeMarkdownlint(t, project, ".markdownlint.json", tt.markdownlint)

			result, err := Load(context.Background(), LoadOptions{
				WorkingDir: project, IgnoreSystemConfig: true, IgnoreEnv: true, NonInteractive: true,
			})
			require.NoError(t, err)

			registry := lint.NewRegistry()
			rules.RegisterAll(registry)
			var got map[string]any
			for _, resolved := range lint.ResolveRules(registry, result.Config) {
				if resolved.Rule.ID() == lineLengthID {
					got = resolved.Config.Options
				}
			}
			assert.Equal(t, tt.options, got)

			reported := lintWith(t, registry, result.Config, layeredParagraph)
			if tt.options["max"] == 80 {
				assert.Empty(t, reported, "54 characters are within 80")
			}
		})
	}
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
