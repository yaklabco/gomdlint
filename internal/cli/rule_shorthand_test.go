package cli_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// shorthandDoc has findings for line-length (MD013), no-inline-html (MD033)
// and no-trailing-spaces (MD009), which are all on by default.
const shorthandDoc = "# Title\n\n" +
	"This line is long enough to go past the default line length limit, whether that limit is eighty " +
	"or one hundred and twenty characters.\n\n" +
	"Some <b>bold</b> text. \n"

// userConfigWithoutHTML is a user config, at the path where inProject's
// XDG_CONFIG_HOME puts it, that turns no-inline-html off, so that "true"
// in a project config has a rule to turn back on.
var userConfigWithoutHTML = map[string]string{".xdg/gomdlint/config.yaml": "rules:\n  no-inline-html: false\n"}

// reportedRules returns the rule IDs that a JSON report names.
func reportedRules(t *testing.T, report string) map[string]bool {
	t.Helper()

	var parsed struct {
		Files []struct {
			Diagnostics []struct {
				RuleID string `json:"ruleId"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed), report)

	rules := make(map[string]bool)
	for _, file := range parsed.Files {
		for _, diag := range file.Diagnostics {
			rules[diag.RuleID] = true
		}
	}
	return rules
}

// TestLint_RuleBooleanShorthand covers rules set to true or false in a
// gomdlint config, as markdownlint allows: false turns a rule off, true
// turns one on, in YAML and in JSON. Every rule these tests use is on by
// default, so a user config turns no-inline-html off for the project
// config to turn back on.
func TestLint_RuleBooleanShorthand(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		config string
	}{
		{name: "YAML by name", file: ".gomdlint.yml", config: "rules:\n  line-length: false\n  no-inline-html: true\n"},
		{name: "YAML by ID", file: ".gomdlint.yml", config: "rules:\n  MD013: false\n  MD033: true\n"},
		{name: "JSON", file: "gomdlint.json", config: `{"rules": {"line-length": false, "MD033": true}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"doc.md": shorthandDoc, tt.file: tt.config}
			maps.Copy(files, userConfigWithoutHTML)
			inProject(t, files)

			got := execute(t, "lint", "--config", tt.file, "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, map[string]bool{"MD009": true, "MD033": true}, reportedRules(t, got.stdout))
		})
	}

	t.Run("without the config the defaults apply", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": shorthandDoc})

		got := execute(t, "lint", "--format", "json", "doc.md")

		assert.Equal(t, map[string]bool{"MD009": true, "MD013": true, "MD033": true}, reportedRules(t, got.stdout))
	})

	t.Run("the user config alone turns no-inline-html off", func(t *testing.T) {
		files := map[string]string{"doc.md": shorthandDoc}
		maps.Copy(files, userConfigWithoutHTML)
		inProject(t, files)

		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr)
		assert.Equal(t, map[string]bool{"MD009": true, "MD013": true}, reportedRules(t, got.stdout))
	})
}

// TestLint_RuleShorthandAgreesWithMigration covers the same shorthand in a
// markdownlint config: applied for the run, migrated with "gomdlint
// migrate", or written in a gomdlint config, it turns the same rules on and
// off.
func TestLint_RuleShorthandAgreesWithMigration(t *testing.T) {
	const shorthand = "line-length: false\nno-inline-html: true\n"
	ruleStates := func(rules map[string]bool) [2]bool { return [2]bool{rules["MD013"], rules["MD033"]} }
	want := [2]bool{false, true}

	t.Run("markdownlint config applied for the run", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": shorthandDoc, ".markdownlint.yaml": shorthand})

		got := execute(t, "lint", "--format", "json", "doc.md")

		assert.Equal(t, want, ruleStates(reportedRules(t, got.stdout)))
	})

	t.Run("migrated config", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": shorthandDoc, ".markdownlint.yaml": shorthand})

		migrated := execute(t, "migrate", ".markdownlint.yaml", "--output", "migrated.yml")
		require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)
		got := execute(t, "lint", "--config", "migrated.yml", "--format", "json", "doc.md")

		assert.Equal(t, want, ruleStates(reportedRules(t, got.stdout)))
	})

	// A .json config must hold JSON, so migrate writes JSON for one.
	t.Run("config migrated to a JSON file", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": shorthandDoc, ".markdownlint.yaml": shorthand})

		migrated := execute(t, "migrate", ".markdownlint.yaml", "--output", ".gomdlint.json")
		require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)
		assert.True(t, json.Valid([]byte(readFile(t, filepath.Join(dir, ".gomdlint.json")))), "migrate writes JSON")
		// As migrate advises, the markdownlint file goes, so only the
		// JSON file can set the rules.
		require.NoError(t, os.Remove(filepath.Join(dir, ".markdownlint.yaml")))
		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr, "lint reads the JSON file without complaint")
		assert.Equal(t, want, ruleStates(reportedRules(t, got.stdout)))
	})

	t.Run("gomdlint config", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": shorthandDoc, ".gomdlint.yml": "rules:\n  line-length: false\n  no-inline-html: true\n"})

		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr)
		assert.Equal(t, want, ruleStates(reportedRules(t, got.stdout)))
	})
}
