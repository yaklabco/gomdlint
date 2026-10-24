package configloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// The tests in this file cover markdownlint-cli2's options files,
// .markdownlint-cli2.jsonc, .markdownlint-cli2.yaml, .markdownlint-cli2.cjs
// and .markdownlint-cli2.mjs, whose config key holds a markdownlint
// configuration. Their expected values are what markdownlint-cli2 0.23.3,
// with markdownlint 0.41.1, does with each directory.

// ruleEnabled reports whether cfg's settings under rules turn the rule with
// the given ID on, and then whether they set it at all.
func ruleEnabled(cfg *config.Config, id string) (bool, bool) {
	rule, ok := cfg.Rules[id]
	if !ok || rule.Enabled == nil {
		return false, false
	}
	return *rule.Enabled, true
}

// ruleOff reports whether cfg's settings under rules turn the rule with the
// given ID off.
func ruleOff(cfg *config.Config, id string) bool {
	enabled, set := ruleEnabled(cfg, id)
	return set && !enabled
}

// TestDiscoverPaths_MarkdownlintCLI2Files covers the discovery of
// markdownlint-cli2 options files. The nearest directory that holds a
// markdownlint configuration holds the one that applies: its markdownlint
// config file, which markdownlint-cli2 prefers to the options file's config
// key, or else an options file that sets the config key
// (lint.MarkdownlintCLI2Options.ConfigSet), as markdownlint-cli2 0.23.3
// lints a directory whose options files set none with a parent
// directory's markdownlint config file. An options file that gomdlint
// cannot read, such as a JavaScript one, may set it, so it stops the
// search. Past an options file, a parent directory's options file that
// sets the config key wins over the config file beside it, as
// markdownlint-cli2 merges options files before it looks at config files.
// Without any configuration, the nearest options file applies, so that
// markdownlint's defaults apply and its other keys get warnings. A nearer
// options file that the search passed is kept for its warnings.
func TestDiscoverPaths_MarkdownlintCLI2Files(t *testing.T) {
	t.Parallel()

	const withConfig = "{\"config\": {}}\n"
	tests := []struct {
		name               string
		files              map[string]string // relative to the repository, which holds docs
		markdownlint, cli2 string
		nearer             string // the options file that the search passed
	}{
		{name: "options file alone", files: map[string]string{".markdownlint-cli2.jsonc": withConfig},
			markdownlint: ".markdownlint-cli2.jsonc", cli2: ".markdownlint-cli2.jsonc"},
		{name: "options file alone without config", files: map[string]string{".markdownlint-cli2.jsonc": "{}\n"},
			markdownlint: ".markdownlint-cli2.jsonc", cli2: ".markdownlint-cli2.jsonc"},
		{name: "YAML options file", files: map[string]string{".markdownlint-cli2.yaml": "config: {}\n"},
			markdownlint: ".markdownlint-cli2.yaml", cli2: ".markdownlint-cli2.yaml"},
		{name: "both kinds", files: map[string]string{".markdownlint-cli2.yaml": "config: {}\n", ".markdownlint.json": "{}\n"},
			markdownlint: ".markdownlint.json", cli2: ".markdownlint-cli2.yaml"},
		{name: "JSONC before YAML", files: map[string]string{".markdownlint-cli2.yaml": "config: {}\n", ".markdownlint-cli2.jsonc": withConfig},
			markdownlint: ".markdownlint-cli2.jsonc", cli2: ".markdownlint-cli2.jsonc"},
		{name: "nearer options file", files: map[string]string{".markdownlint.json": "{}\n", "docs/.markdownlint-cli2.jsonc": withConfig},
			markdownlint: "docs/.markdownlint-cli2.jsonc", cli2: "docs/.markdownlint-cli2.jsonc"},
		{name: "nearer options file without config", files: map[string]string{
			".markdownlint.json": "{}\n", "docs/.markdownlint-cli2.jsonc": "{\"ignores\": []}\n",
		}, markdownlint: ".markdownlint.json", nearer: "docs/.markdownlint-cli2.jsonc"},
		{name: "nearer options file with a null config", files: map[string]string{
			".markdownlint.json": "{}\n", "docs/.markdownlint-cli2.yaml": "config: ~\n",
		}, markdownlint: ".markdownlint.json", nearer: "docs/.markdownlint-cli2.yaml"},
		{name: "options files, the nearer without config", files: map[string]string{
			".markdownlint-cli2.jsonc": withConfig, "docs/.markdownlint-cli2.jsonc": "{}\n",
		}, markdownlint: ".markdownlint-cli2.jsonc", cli2: ".markdownlint-cli2.jsonc", nearer: "docs/.markdownlint-cli2.jsonc"},
		{
			// Past a nearer options file, markdownlint-cli2 merges a parent's
			// options file, whose config then wins over the parent's
			// markdownlint config file.
			name: "both kinds above a nearer options file without config", files: map[string]string{
				".markdownlint.json": "{}\n", ".markdownlint-cli2.jsonc": withConfig, "docs/.markdownlint-cli2.jsonc": "{}\n",
			}, markdownlint: ".markdownlint-cli2.jsonc", cli2: ".markdownlint-cli2.jsonc", nearer: "docs/.markdownlint-cli2.jsonc",
		},
		{
			name: "both kinds above a nearer options file, without config", files: map[string]string{
				".markdownlint.json": "{}\n", ".markdownlint-cli2.jsonc": "{}\n", "docs/.markdownlint-cli2.jsonc": "{}\n",
			}, markdownlint: ".markdownlint.json", cli2: ".markdownlint-cli2.jsonc", nearer: "docs/.markdownlint-cli2.jsonc",
		},
		{name: "nearer JavaScript options file", files: map[string]string{
			".markdownlint.json": "{}\n", "docs/.markdownlint-cli2.cjs": "module.exports = {};\n",
		}, markdownlint: "docs/.markdownlint-cli2.cjs", cli2: "docs/.markdownlint-cli2.cjs"},
		{name: "nearer options file that cannot be read", files: map[string]string{
			".markdownlint.json": "{}\n", "docs/.markdownlint-cli2.jsonc": "config:\n",
		}, markdownlint: "docs/.markdownlint-cli2.jsonc", cli2: "docs/.markdownlint-cli2.jsonc"},
		{name: "config file alone", files: map[string]string{".markdownlint.yaml": "{}\n"}, markdownlint: ".markdownlint.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
			docs := filepath.Join(repo, "docs")
			require.NoError(t, os.MkdirAll(docs, 0o755))
			for name, content := range tt.files {
				writeMarkdownlint(t, repo, name, content)
			}

			paths, err := DiscoverPaths(context.Background(), docs)
			require.NoError(t, err)

			assert.Equal(t, filepath.Join(repo, tt.markdownlint), paths.Markdownlint)
			want := ""
			if tt.cli2 != "" {
				want = filepath.Join(repo, tt.cli2)
			}
			assert.Equal(t, want, paths.MarkdownlintCLI2)
			wantNearer := ""
			if tt.nearer != "" {
				wantNearer = filepath.Join(repo, tt.nearer)
			}
			assert.Equal(t, wantNearer, paths.MarkdownlintCLI2Nearer)
		})
	}
}

// TestLoad_WarnsAboutAPassedOptionsFile covers an options file without the
// config key below a directory with a markdownlint config file. The search
// passes it, as markdownlint-cli2 does, but its other keys still configure
// markdownlint-cli2 for its directory, so they get warnings as they would
// if the search had stopped there.
func TestLoad_WarnsAboutAPassedOptionsFile(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	writeMarkdownlint(t, repo, ".markdownlint.json", `{"MD013": false}`)
	options := writeMarkdownlint(t, repo, "docs/.markdownlint-cli2.jsonc", "{\n  \"ignores\": [\"gen/**\"]\n}\n")

	result, err := Load(context.Background(), markdownlintOptions(filepath.Join(repo, "docs"), false, "", nil))
	require.NoError(t, err)
	assert.True(t, ruleOff(result.Config, "MD013"), "the parent's markdownlint config applies")
	assert.Equal(t, []string{
		options + `:2: unsupported markdownlint-cli2 option "ignores" (set ignore in a gomdlint config instead); it is ignored`,
	}, result.Warnings)
}

// TestLoad_MarkdownlintCLI2Config covers a project whose markdownlint
// configuration is the config key of an options file: it applies to the
// run, as markdownlint-cli2 applies it, in JSON with comments and in YAML.
// The options file's other keys configure markdownlint-cli2 itself, which
// gomdlint does not read, so each gets a warning with its line, and
// $schema, which editors read, gets none.
func TestLoad_MarkdownlintCLI2Config(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, file, content string
		warnings            []string
	}{
		{
			name: "JSONC", file: ".markdownlint-cli2.jsonc",
			content: "{\n  \"$schema\": \"https://example.com/schema.json\",\n  // Rules.\n" +
				"  \"config\": {\"MD013\": false, \"whitespace\": false,},\n  \"ignores\": [\"drafts/**\"],\n}\n",
			warnings: []string{
				`:5: unsupported markdownlint-cli2 option "ignores" (set ignore in a gomdlint config instead); it is ignored`,
			},
		},
		{
			name: "YAML", file: ".markdownlint-cli2.yaml",
			content: "globs:\n  - \"**/*.md\"\nconfig:\n  MD013: false\n  whitespace: false\nconfg: 1\n",
			warnings: []string{
				`:1: unsupported markdownlint-cli2 option "globs" (name the files on the command line instead); it is ignored`,
				`:6: unknown key "confg" (did you mean "config"?); it is ignored`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := writeMarkdownlint(t, dir, tt.file, tt.content)

			result, err := Load(context.Background(), markdownlintOptions(dir, false, "", nil))
			require.NoError(t, err)

			for _, id := range []string{"MD013", "MD009"} {
				enabled, set := ruleEnabled(result.Config, id)
				assert.True(t, set, id)
				assert.False(t, enabled, id)
			}
			assert.Contains(t, result.LoadedFrom, source)
			want := make([]string, len(tt.warnings))
			for idx, warning := range tt.warnings {
				want[idx] = source + warning
			}
			assert.Equal(t, want, result.Warnings)
		})
	}
}

// TestLoad_MarkdownlintCLI2Precedence covers a directory that holds an
// options file beside a markdownlint config file, or another options file.
// markdownlint-cli2 lints with the markdownlint config file, and reads the
// first options file only, .markdownlint-cli2.jsonc before
// .markdownlint-cli2.yaml. So the options file's config key changes
// nothing, which a warning says, as does a warning about each shadowed
// file. An options file that holds no config key leaves every rule at
// markdownlint's defaults.
func TestLoad_MarkdownlintCLI2Precedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		files     map[string]string
		lineOff   bool // whether MD013 is off
		spaceOff  bool // whether MD009 is off
		warnings  []string
		loadedTop string
	}{
		{
			name: "config file wins",
			files: map[string]string{
				".markdownlint.json": `{"MD009": false}`, ".markdownlint-cli2.jsonc": `{"config": {"MD013": false}}`,
			},
			spaceOff: true, loadedTop: ".markdownlint.json",
			warnings: []string{"both {dir}/.markdownlint.json and the config key of {dir}/.markdownlint-cli2.jsonc exist; " +
				"using {dir}/.markdownlint.json"},
		},
		{
			name: "options file without config beside a config file",
			files: map[string]string{
				".markdownlint.json": `{"MD009": false}`, ".markdownlint-cli2.yaml": "noInlineConfig: true\n",
			},
			spaceOff: true, loadedTop: ".markdownlint.json",
			warnings: []string{`{dir}/.markdownlint-cli2.yaml:1: unsupported markdownlint-cli2 option "noInlineConfig"; it is ignored`},
		},
		{
			name: "JSONC options file first",
			files: map[string]string{
				".markdownlint-cli2.jsonc": `{"config": {"MD013": false}}`, ".markdownlint-cli2.yaml": "config:\n  MD009: false\n",
			},
			lineOff: true, loadedTop: ".markdownlint-cli2.jsonc",
			warnings: []string{"both {dir}/.markdownlint-cli2.jsonc and {dir}/.markdownlint-cli2.yaml exist; " +
				"using {dir}/.markdownlint-cli2.jsonc"},
		},
		{
			name:      "options file without config",
			files:     map[string]string{".markdownlint-cli2.jsonc": `{"fix": true}`},
			loadedTop: ".markdownlint-cli2.jsonc",
			warnings:  []string{`{dir}/.markdownlint-cli2.jsonc:1: unsupported markdownlint-cli2 option "fix" (use --fix instead); it is ignored`},
		},
		{
			name:      "config key that holds no object",
			files:     map[string]string{".markdownlint-cli2.jsonc": `{"config": [1]}`},
			loadedTop: ".markdownlint-cli2.jsonc",
			warnings:  []string{`{dir}/.markdownlint-cli2.jsonc:1: config holds a list instead of an object of settings; it is ignored`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for name, content := range tt.files {
				writeMarkdownlint(t, dir, name, content)
			}

			result, err := Load(context.Background(), markdownlintOptions(dir, false, "", nil))
			require.NoError(t, err)

			assert.Equal(t, tt.lineOff, ruleOff(result.Config, "MD013"), "MD013")
			assert.Equal(t, tt.spaceOff, ruleOff(result.Config, "MD009"), "MD009")
			assert.Equal(t, []string{filepath.Join(dir, tt.loadedTop)}, result.LoadedFrom)
			want := make([]string, len(tt.warnings))
			for idx, warning := range tt.warnings {
				want[idx] = strings.ReplaceAll(warning, "{dir}", dir)
			}
			assert.Equal(t, want, result.Warnings)
		})
	}
}

// TestLoad_MarkdownlintCLI2Errors covers options files that
// markdownlint-cli2 0.23.3 cannot use: one that its parser rejects fails
// the run when it holds the markdownlint configuration, as it fails
// markdownlint-cli2, and gets a warning when a markdownlint config file
// beside it holds the configuration, since gomdlint takes nothing from it.
// A JavaScript options file cannot be read: with nothing beside it, every
// rule keeps markdownlint's defaults, as for a JavaScript config file, and
// beside a config file it gets a warning.
func TestLoad_MarkdownlintCLI2Errors(t *testing.T) {
	t.Parallel()

	t.Run("unreadable options file alone", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeMarkdownlint(t, dir, ".markdownlint-cli2.jsonc", "config:\n  MD013: false\n")

		_, err := Load(context.Background(), markdownlintOptions(dir, false, "", nil))
		require.Error(t, err)
		assert.Contains(t, err.Error(), filepath.Join(dir, ".markdownlint-cli2.jsonc"))
		assert.Contains(t, err.Error(), "parse JSON")
	})

	t.Run("unreadable options file beside a config file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeMarkdownlint(t, dir, ".markdownlint.json", `{"MD009": false}`)
		options := writeMarkdownlint(t, dir, ".markdownlint-cli2.yaml", "")

		result, err := Load(context.Background(), markdownlintOptions(dir, false, "", nil))
		require.NoError(t, err)
		enabled, set := ruleEnabled(result.Config, "MD009")
		assert.True(t, set)
		assert.False(t, enabled)
		require.Len(t, result.Warnings, 1)
		assert.True(t, strings.HasPrefix(result.Warnings[0], options+": cannot read it (parse YAML: "), result.Warnings[0])
		assert.True(t, strings.HasSuffix(result.Warnings[0], "); it is ignored"), result.Warnings[0])
	})

	t.Run("options file that cannot be opened beside a config file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		configFile := writeMarkdownlint(t, dir, ".markdownlint.json", `{"MD009": false}`)
		options := filepath.Join(dir, ".markdownlint-cli2.jsonc")

		// The warning names the file once, not again in the error.
		warnings := sideOptionsWarnings(options, configFile)
		require.Len(t, warnings, 1)
		assert.True(t, strings.HasPrefix(warnings[0], options+": cannot read it ("), warnings[0])
		assert.True(t, strings.HasSuffix(warnings[0], "); it is ignored"), warnings[0])
		assert.Equal(t, 1, strings.Count(warnings[0], options), warnings[0])
	})

	t.Run("JavaScript options file alone", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeMarkdownlint(t, dir, ".markdownlint-cli2.cjs", "module.exports = {config: {MD013: false}};\n")

		result, err := Load(context.Background(), markdownlintOptions(dir, false, "", nil))
		require.NoError(t, err)
		_, set := ruleEnabled(result.Config, "MD013")
		assert.False(t, set)
		assert.Equal(t, []string{GetMigrationWarning(filepath.Join(dir, ".markdownlint-cli2.cjs"))}, result.Warnings)
	})

	t.Run("JavaScript options file beside a config file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeMarkdownlint(t, dir, ".markdownlint.json", `{"MD009": false}`)
		options := writeMarkdownlint(t, dir, ".markdownlint-cli2.mjs", "export default {};\n")

		result, err := Load(context.Background(), markdownlintOptions(dir, false, "", nil))
		require.NoError(t, err)
		assert.Equal(t, []string{
			options + ": JavaScript, which gomdlint cannot read; its markdownlint-cli2 options are ignored",
		}, result.Warnings)
	})
}

// TestConvertMarkdownlintConfig_MarkdownlintCLI2File covers the migration
// of an options file: its config key converts as a markdownlint config
// does, with each warning at its line of the options file, and its other
// keys get warnings too.
func TestConvertMarkdownlintConfig_MarkdownlintCLI2File(t *testing.T) {
	t.Parallel()

	result := convertFile(t, ".markdownlint-cli2.yaml",
		"customRules:\n  - ./rule.js\nconfig:\n  default: false\n  MD013:\n    line_length: 100\n    lin_length: 1\n")

	enabled, set := ruleEnabled(result.Config, "MD013")
	assert.True(t, set)
	assert.True(t, enabled)
	assert.Equal(t, map[string]any{"max": float64(100)}, result.Config.Rules["MD013"].Options)
	assert.False(t, *result.Config.RuleDefaults["MD009"].Enabled, "the config key's default key applies")
	assert.Equal(t, []string{
		`unsupported markdownlint-cli2 option "customRules"; it is ignored`,
		`unknown option "lin_length" for line-length (did you mean "line_length"?); it is ignored`,
	}, result.Warnings)
	assert.Equal(t, []int{1, 7}, []int{result.WarningLine(0), result.WarningLine(1)})

	content, err := result.FileContent(filepath.Join(t.TempDir(), ".gomdlint.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "Migrated from: .markdownlint-cli2.yaml")
}
