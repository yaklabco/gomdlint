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
