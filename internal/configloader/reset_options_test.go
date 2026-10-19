package configloader

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestMerge_ResetOptions covers a layer whose settings of a rule reset its
// options: its options replace those of the layers below instead of joining
// them, and the reset stays with the rule while higher layers add options.
func TestMerge_ResetOptions(t *testing.T) {
	t.Parallel()

	lower := &config.Config{
		Rules:        map[string]config.RuleConfig{"MD013": {Options: map[string]any{"max": 20, "strict": true}}},
		RuleDefaults: map[string]config.RuleConfig{"MD013": {Options: map[string]any{"heading_max": 30}}},
	}
	reset := &config.Config{
		Rules:        map[string]config.RuleConfig{"MD013": {Enabled: config.Bool(true), ResetOptions: true}},
		RuleDefaults: map[string]config.RuleConfig{"MD013": {ResetOptions: true, Options: map[string]any{"stern": true}}},
	}
	higher := &config.Config{
		Rules: map[string]config.RuleConfig{"MD013": {Options: map[string]any{"ignore_tables": true}}},
	}

	merged := merge(lower, reset)
	assert.True(t, merged.Rules["MD013"].ResetOptions)
	assert.Empty(t, merged.Rules["MD013"].Options, "the reset drops the options below")
	assert.True(t, *merged.Rules["MD013"].Enabled)
	assert.True(t, merged.RuleDefaults["MD013"].ResetOptions)
	assert.Equal(t, map[string]any{"stern": true}, merged.RuleDefaults["MD013"].Options)
	assert.Equal(t, map[string]any{"max": 20, "strict": true}, lower.Rules["MD013"].Options, "the lower layer is left alone")

	merged = merge(merged, higher)
	assert.True(t, merged.Rules["MD013"].ResetOptions, "a higher layer keeps the reset")
	assert.Equal(t, map[string]any{"ignore_tables": true}, merged.Rules["MD013"].Options)

	kept := merge(lower, higher)
	assert.False(t, kept.Rules["MD013"].ResetOptions)
	assert.Equal(t, map[string]any{"max": 20, "strict": true, "ignore_tables": true}, kept.Rules["MD013"].Options)
}

// TestLoad_ResetOptionsAcrossLayers covers reset_options in gomdlint
// config files of two layers, a user config below a project config. Under
// rules, it drops the options under rule_defaults of every layer, higher
// ones too, as a rule's settings under rules in any layer override those
// under rule_defaults in every layer. Under rule_defaults, it drops only
// the rule_defaults options of lower layers, so their options under rules
// still apply.
func TestLoad_ResetOptionsAcrossLayers(t *testing.T) {
	tests := []struct {
		name, user, project string
		options             map[string]any
		reports             bool
	}{
		{
			name:    "rules over a higher rule_defaults",
			user:    "rules:\n  MD013:\n    enabled: true\n    reset_options: true\n",
			project: "rule_defaults:\n  MD013:\n    options:\n      max: 20\n",
		},
		{
			name:    "rule_defaults under lower rules",
			user:    "rules:\n  MD013:\n    options:\n      max: 20\n",
			project: "rule_defaults:\n  MD013:\n    reset_options: true\n",
			options: map[string]any{"max": 20}, reports: true,
		},
		{
			name:    "rule_defaults over lower rule_defaults",
			user:    "rule_defaults:\n  MD013:\n    options:\n      max: 20\n",
			project: "rule_defaults:\n  MD013:\n    reset_options: true\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			writeMarkdownlint(t, dir, "xdg/gomdlint/config.yaml", tt.user)
			project := filepath.Join(dir, "project")
			writeMarkdownlint(t, project, ".gomdlint.yml", tt.project)

			result, err := Load(context.Background(), LoadOptions{
				WorkingDir: project, IgnoreSystemConfig: true, IgnoreEnv: true, NonInteractive: true,
			})
			require.NoError(t, err)
			assertLineLength(t, result.Config, tt.options, tt.reports)
		})
	}
}

// TestLoad_ReadsResetOptions covers reset_options in a gomdlint config
// file: a rule setting, which gets no warning, and whose misspelling gets
// one that suggests it.
func TestLoad_ReadsResetOptions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMarkdownlint(t, dir, ".gomdlint.yml",
		"rules:\n  MD013:\n    reset_options: true\n  MD009:\n    reset_option: true\n")

	result, err := Load(context.Background(), LoadOptions{
		WorkingDir: dir, IgnoreUserConfig: true, IgnoreSystemConfig: true, IgnoreEnv: true, NonInteractive: true,
	})
	require.NoError(t, err)

	assert.True(t, result.Config.Rules["MD013"].ResetOptions)
	require.Len(t, result.Warnings, 1, strings.Join(result.Warnings, "\n"))
	assert.Equal(t, filepath.Join(dir, ".gomdlint.yml")+
		`:5: unknown key "rules.MD009.reset_option" (did you mean "rules.MD009.reset_options"?); it is ignored`,
		result.Warnings[0])
}
