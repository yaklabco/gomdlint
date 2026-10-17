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
			assert.Contains(t, text.stdout, "1 issue (1 warning), in 1 file\n", "summary: %s", text.stdout)
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

		assert.Contains(t, got.stdout, "1 issue (1 warning), in 1 file, 1 fixable\n")
	})
}

// TestLint_FixWithDiffFormatShowsTheWrittenChanges covers --fix --format
// diff: the fixes are written, and the report is the diff of what was
// written, the same diff that --format diff previews and that --fix
// --dry-run computes without writing.
func TestLint_FixWithDiffFormatShowsTheWrittenChanges(t *testing.T) {
	// Two passes: the style fix and the spacing fix of the closed heading
	// overlap, so one of them waits for the second pass, and the diff
	// covers both.
	const (
		doc   = "# Title \n\n##  Spaced  ##\n\nText.\n"
		fixed = "# Title\n\n## Spaced\n\nText.\n"
	)
	dir := inProject(t, map[string]string{"doc.md": doc, "clean.md": fixedDoc})
	path := filepath.Join(dir, "doc.md")

	preview := execute(t, "lint", "--format", "diff", "--color", "never", "doc.md", "clean.md")
	require.Contains(t, preview.stdout, "diff --git a/doc.md b/doc.md", preview.stderr)
	require.Equal(t, doc, readFile(t, path), "a preview writes nothing")

	dryRun := execute(t, "lint", "--fix", "--dry-run", "--format", "diff", "--color", "never", "doc.md", "clean.md")
	assert.Equal(t, preview.stdout, dryRun.stdout, "a dry run shows the previewed diff")
	require.Equal(t, doc, readFile(t, path), "a dry run writes nothing")

	got := execute(t, "lint", "--fix", "--no-backups", "--format", "diff", "--color", "never", "doc.md", "clean.md")

	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Equal(t, preview.stdout, got.stdout, "--fix shows the diff of what it wrote")
	assert.NotContains(t, got.stdout, "clean.md", "a file without fixes has no diff")
	assert.Contains(t, got.stdout, "1 file changed")
	assert.Equal(t, fixed, readFile(t, path), "the fixes are written")
}

// TestLint_FixWithDiffFormatShowsNothingWhenNothingChanges covers --fix
// --format diff on files without fixes: no diff and no summary.
func TestLint_FixWithDiffFormatShowsNothingWhenNothingChanges(t *testing.T) {
	inProject(t, map[string]string{"clean.md": fixedDoc})

	got := execute(t, "lint", "--fix", "--no-backups", "--format", "diff", "--color", "never", "clean.md")

	assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Empty(t, got.stdout)
}
