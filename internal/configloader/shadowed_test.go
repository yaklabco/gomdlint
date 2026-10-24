package configloader

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoad_WarnsAboutShadowedConfigFiles covers a directory that holds two
// or more config files that gomdlint looks for there: it reads the first
// name in its order and ignores the others, which used to happen without a
// word, as after `gomdlint init --format json` beside a YAML config. Each
// ignored file now gets a warning that names the file that is read.
func TestLoad_WarnsAboutShadowedConfigFiles(t *testing.T) {
	tests := []struct {
		name     string
		files    []string // relative to the project, or to xdg/gomdlint
		user     bool
		used     string
		warnings int
	}{
		{name: "YAML over JSON", files: []string{".gomdlint.yml", ".gomdlint.json"}, used: ".gomdlint.yml", warnings: 1},
		{name: ".yml over .yaml", files: []string{".gomdlint.yaml", ".gomdlint.yml"}, used: ".gomdlint.yml", warnings: 1},
		{
			name: "three names", files: []string{"gomdlint.json", "gomdlint.yaml", ".gomdlint.yaml"},
			used: ".gomdlint.yaml", warnings: 2,
		},
		{name: "one name", files: []string{"gomdlint.json"}, used: "gomdlint.json"},
		{name: "user config", files: []string{"config.json", "config.yaml"}, user: true, used: "config.yaml", warnings: 1},
		{
			name: "markdownlint config", files: []string{".markdownlint.yaml", ".markdownlint.json"},
			used: ".markdownlint.json", warnings: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			project := filepath.Join(dir, "project")
			base := project
			if tt.user {
				base = filepath.Join(dir, "xdg", "gomdlint")
			}
			for _, name := range tt.files {
				writeMarkdownlint(t, base, name, "{}\n")
			}
			writeMarkdownlint(t, project, "doc.md", "# Title\n")

			result, err := Load(t.Context(), LoadOptions{
				WorkingDir: project, IgnoreSystemConfig: true, IgnoreEnv: true, NonInteractive: true,
			})
			require.NoError(t, err)

			used := filepath.Join(base, tt.used)
			assert.Contains(t, result.LoadedFrom, used)
			var shadowed []string
			for _, warning := range result.Warnings {
				for _, name := range tt.files {
					ignored := filepath.Join(base, name)
					if ignored != used && warning == "both "+used+" and "+ignored+" exist; using "+used {
						shadowed = append(shadowed, ignored)
					}
				}
			}
			assert.Len(t, shadowed, tt.warnings, "%v", result.Warnings)
		})
	}
}

// TestLoad_WarnsOnlyAboutConfigFilesItReads covers shadowed config files in
// a directory whose config gomdlint does not read: markdownlint configs
// beside a project config, which wins, and a user config that the options
// leave out. They get no warning about each other.
func TestLoad_WarnsOnlyAboutConfigFilesItReads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	user := filepath.Join(dir, "xdg", "gomdlint")
	writeMarkdownlint(t, user, "config.yaml", "{}\n")
	writeMarkdownlint(t, user, "config.json", "{}\n")
	project := filepath.Join(dir, "project")
	writeMarkdownlint(t, project, ".gomdlint.yml", "{}\n")
	writeMarkdownlint(t, project, ".markdownlint.json", "{}\n")
	writeMarkdownlint(t, project, ".markdownlint.yaml", "{}\n")

	result, err := Load(t.Context(), LoadOptions{
		WorkingDir: project, IgnoreSystemConfig: true, IgnoreUserConfig: true, IgnoreEnv: true, NonInteractive: true,
	})
	require.NoError(t, err)

	gomdlintConfig, markdownlintConfig := filepath.Join(project, ".gomdlint.yml"), filepath.Join(project, ".markdownlint.json")
	assert.Equal(t, []string{
		"both " + gomdlintConfig + " and " + markdownlintConfig + " exist; using " + gomdlintConfig,
	}, result.Warnings)
}

// TestLoad_ExplicitConfigShadowsNothing covers a config file named with
// --config in a directory that holds another config file. gomdlint reads
// the named file because the option names it, not because its name comes
// first there, so neither file shadows the other and nothing warns about
// it, whichever of the two the option names.
func TestLoad_ExplicitConfigShadowsNothing(t *testing.T) {
	for _, name := range []string{".gomdlint.yml", ".gomdlint.json"} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			writeMarkdownlint(t, project, ".gomdlint.yml", "{}\n")
			writeMarkdownlint(t, project, ".gomdlint.json", "{}\n")

			explicit := filepath.Join(project, name)
			result, err := Load(t.Context(), LoadOptions{
				WorkingDir: project, ExplicitPath: explicit, IgnoreSystemConfig: true,
				IgnoreUserConfig: true, IgnoreEnv: true, NonInteractive: true,
			})
			require.NoError(t, err)
			assert.Equal(t, []string{explicit}, result.LoadedFrom)
			assert.Empty(t, result.Warnings)
		})
	}
}

// TestLoad_ProjectConfigBesideOptionsFile covers a gomdlint config beside a
// markdownlint-cli2 options file. The gomdlint config wins over a
// markdownlint configuration, so an options file that sets the config key
// gets a warning that names the key. One without it holds no markdownlint
// configuration, only settings of markdownlint-cli2 itself, as the file
// that migrate advises keeping does, so it gets no warning.
func TestLoad_ProjectConfigBesideOptionsFile(t *testing.T) {
	tests := []struct {
		name, options string
		warns         bool
	}{
		{name: "config key", options: "{\"config\": {\"MD013\": false}}\n", warns: true},
		{name: "no config key", options: "{\"globs\": [\"**/*.md\"]}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			gomdlintConfig := writeMarkdownlint(t, project, ".gomdlint.yml", "{}\n")
			options := writeMarkdownlint(t, project, ".markdownlint-cli2.jsonc", tt.options)

			result, err := Load(t.Context(), LoadOptions{
				WorkingDir: project, IgnoreSystemConfig: true, IgnoreUserConfig: true, IgnoreEnv: true, NonInteractive: true,
			})
			require.NoError(t, err)

			if !tt.warns {
				assert.Empty(t, result.Warnings)
				return
			}
			assert.Equal(t, []string{
				"both " + gomdlintConfig + " and the config key of " + options + " exist; using " + gomdlintConfig,
			}, result.Warnings)
		})
	}
}
