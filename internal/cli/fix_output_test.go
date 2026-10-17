package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// jsonFixable returns, for each diagnostic of a JSON report, its rule ID and
// whether it is fixable, and how many fixes it carries.
func jsonFixable(t *testing.T, report string) map[string][2]int {
	t.Helper()

	var parsed struct {
		Files []struct {
			Diagnostics []struct {
				RuleID  string            `json:"ruleId"`
				Fixable bool              `json:"fixable"`
				Fixes   []json.RawMessage `json:"fixes"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed), report)

	found := make(map[string][2]int)
	for _, file := range parsed.Files {
		for _, diag := range file.Diagnostics {
			fixable := 0
			if diag.Fixable {
				fixable = 1
			}
			found[diag.RuleID] = [2]int{fixable, len(diag.Fixes)}
		}
	}
	return found
}

// TestLint_FindingsWhoseFixesAreExcludedAreNotFixable covers findings of
// rules whose fixes --fix would not apply, because of --fix-rules or
// auto_fix: false. The summary, the JSON report and the fix itself all
// agree that they are not fixable.
func TestLint_FindingsWhoseFixesAreExcludedAreNotFixable(t *testing.T) {
	const doc = trailingDoc // One no-trailing-spaces (MD009) finding.

	tests := []struct {
		name   string
		config string
		args   []string
	}{
		{name: "fix-rules names another rule", args: []string{"--fix-rules", "MD010"}},
		{name: "auto_fix false", config: "rules:\n  MD009:\n    auto_fix: false\n"},
		{name: "auto_fix false by rule name", config: "rules:\n  no-trailing-spaces:\n    auto_fix: false\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"doc.md": doc}
			if tt.config != "" {
				files[".gomdlint.yml"] = tt.config
			}
			dir := inProject(t, files)

			text := execute(t, append([]string{"lint", "--color", "never", "--no-context", "doc.md"}, tt.args...)...)
			assert.Contains(t, text.stdout, "1 issue (1 warnings), in 1 file\n", "summary: %s", text.stdout)
			assert.NotContains(t, text.stdout, "fixable")

			report := execute(t, append([]string{"lint", "--format", "json", "doc.md"}, tt.args...)...)
			assert.Equal(t, map[string][2]int{"MD009": {0, 0}}, jsonFixable(t, report.stdout),
				"the finding is reported without a fix")

			fixed := execute(t, append([]string{"lint", "--fix", "--no-backups", "--color", "never", "doc.md"}, tt.args...)...)
			assert.Equal(t, cli.ExitSuccess, fixed.code, fixed.stderr)
			assert.Equal(t, doc, readFile(t, filepath.Join(dir, "doc.md")), "--fix leaves the finding alone")
		})
	}

	t.Run("rules that --fix-rules names stay fixable", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": doc})

		got := execute(t, "lint", "--color", "never", "--no-context", "--fix-rules", "no-trailing-spaces", "doc.md")

		assert.Contains(t, got.stdout, "1 issue (1 warnings), in 1 file, 1 fixable\n")
	})
}
