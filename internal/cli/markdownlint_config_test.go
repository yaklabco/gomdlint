package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// trailingSpaceDoc has one finding, a trailing space (MD009), whatever
// rules a markdownlint config leaves on.
const trailingSpaceDoc = "# Title\n\ntext \n"

// reportedFiles returns, for each file that a JSON report lists, the IDs of
// the rules that reported it, in order, or nil for a file without findings.
// Paths are written with slashes.
func reportedFiles(t *testing.T, report string) map[string][]string {
	t.Helper()

	var parsed struct {
		Files []struct {
			Path        string `json:"path"`
			Diagnostics []struct {
				RuleID string `json:"ruleId"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed), report)

	files := make(map[string][]string)
	for _, file := range parsed.Files {
		var ids []string
		for _, diag := range file.Diagnostics {
			ids = append(ids, diag.RuleID)
		}
		slices.Sort(ids)
		files[filepath.ToSlash(file.Path)] = slices.Compact(ids)
	}
	return files
}

// TestMigrate_KeepsInheritedSettings covers the settings that a user config
// sets and a markdownlint config cannot: its ignore patterns, flavor,
// gitignore and default_ignores settings and backups. While gomdlint
// translates the markdownlint config for each run they apply, and they
// must still apply once the markdownlint config is migrated. A migrated
// file used to write each of them with its default value, such as
// ignore: [], which hid the user's.
func TestMigrate_KeepsInheritedSettings(t *testing.T) {
	files := map[string]string{
		".xdg/gomdlint/config.yaml": "ignore:\n  - \"drafts/**\"\n" +
			"flavor: commonmark\n" +
			"gitignore: false\n" +
			"default_ignores: false\n" +
			"backups:\n  enabled: false\n",
		".markdownlint.json": `{"MD013": false}` + "\n",
		// The user's pattern skips drafts.
		"drafts/draft.md": trailingSpaceDoc,
		// gitignore: false lints a file that .gitignore lists.
		".gitignore": "ignored.md\n",
		"ignored.md":  trailingSpaceDoc,
		// default_ignores: false lints node_modules.
		"node_modules/dep.md": trailingSpaceDoc,
		// Under gfm, blanks-around-tables (MD058) reports the table that
		// follows the paragraph; under commonmark there is no table.
		"table.md": "# Title\n\nText\n| a | b |\n| - | - |\n| 1 | 2 |\n",
	}
	want := map[string][]string{
		"ignored.md":          {"MD009"},
		"node_modules/dep.md": {"MD009"},
		"table.md":            nil,
	}

	dir := inProject(t, files)

	translated := execute(t, "lint", "--format", "json", ".")
	require.Contains(t, translated.stderr, "translated to gomdlint settings for this run")
	require.Equal(t, want, reportedFiles(t, translated.stdout), "the markdownlint config translated for the run")

	migrated := execute(t, "migrate", ".markdownlint.json", "--output", ".gomdlint.yml")
	require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)
	// As migrate advises, the markdownlint config goes.
	require.NoError(t, os.Remove(filepath.Join(dir, ".markdownlint.json")))

	got := execute(t, "lint", "--format", "json", ".")
	require.Empty(t, got.stderr)
	assert.Equal(t, want, reportedFiles(t, got.stdout), "the migrated config")

	fixed := execute(t, "lint", "--fix", "ignored.md")
	require.Equal(t, cli.ExitSuccess, fixed.code, fixed.stderr+fixed.stdout)
	assert.Empty(t, backupFiles(t, dir), "the user config turns backups off")
}
