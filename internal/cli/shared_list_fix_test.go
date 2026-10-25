package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/fix"
)

// sharedListFixDocs hold lists whose fixes change several items at once.
// Replacing the bullet of b or c alone would split the nested list, and
// moving a or b alone would take b out of a, so MD004 and MD007 each fix
// their two findings with one fix.
var sharedListFixDocs = map[string]string{
	"bullets.md": "# L\n\n- a\n  * b\n  * c\n- d\n",
	"indent.md":  "# L\n\n   - a\n     - b\n",
}

// TestLintSharedListFixJSONGolden covers the JSON output of findings that
// one list fix resolves together: each of them is fixable and carries the
// edit of its own line, as markdownlint gives each of them a fix. The
// golden file is testdata/golden/lint-shared-list-fix-json.golden.
func TestLintSharedListFixJSONGolden(t *testing.T) {
	skipUnlessSlashPaths(t)
	inProject(t, sharedListFixDocs)

	got := execute(t, "lint", "--format", "json", "bullets.md", "indent.md")

	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Empty(t, got.stderr)
	assertGolden(t, testdataPath("golden", "lint-shared-list-fix-json.golden"), got.stdout)
}

// TestLint_SharedListFixCounts covers the counts of findings that one list
// fix resolves together. The text output counts each of them as fixable,
// and --fix applies the fix once and counts each of them as fixed.
func TestLint_SharedListFixCounts(t *testing.T) {
	const (
		fixedBullets = "# L\n\n- a\n  - b\n  - c\n- d\n"
		fixedIndent  = "# L\n\n- a\n  - b\n"
	)

	t.Run("lint", func(t *testing.T) {
		inProject(t, sharedListFixDocs)

		got := execute(t, "lint", "--color", "never", "bullets.md", "indent.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Contains(t, got.stdout, "4 issues (4 warnings), in 2 files, 4 fixable")
	})

	t.Run("dry run", func(t *testing.T) {
		inProject(t, sharedListFixDocs)

		got := execute(t, "lint", "--fix", "--dry-run", "--color", "never", "bullets.md", "indent.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Contains(t, got.stdout, "4 fixes would be applied in 2 files")
	})

	t.Run("fix", func(t *testing.T) {
		dir := inProject(t, sharedListFixDocs)

		got := execute(t, "lint", "--fix", "--no-backups", "--color", "never", "bullets.md", "indent.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Contains(t, got.stdout, "4 fixes applied in 2 files")
		assert.Equal(t, fixedBullets, readFile(t, filepath.Join(dir, "bullets.md")))
		assert.Equal(t, fixedIndent, readFile(t, filepath.Join(dir, "indent.md")))
	})
}

// reportedEdits returns the distinct fix edits of a JSON report and the
// number of findings in a fix group.
func reportedEdits(t *testing.T, report string) ([]fix.TextEdit, int) {
	t.Helper()

	var output struct {
		Files []struct {
			Diagnostics []struct {
				Fixes []struct {
					StartOffset int    `json:"startOffset"`
					EndOffset   int    `json:"endOffset"`
					NewText     string `json:"newText"`
				} `json:"fixes"`
				FixGroup int `json:"fixGroup"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &output))
	require.Len(t, output.Files, 1)
	var edits []fix.TextEdit
	seen := make(map[fix.TextEdit]bool)
	grouped := 0
	for _, diag := range output.Files[0].Diagnostics {
		if diag.FixGroup != 0 {
			grouped++
		}
		for _, reported := range diag.Fixes {
			edit := fix.TextEdit{StartOffset: reported.StartOffset, EndOffset: reported.EndOffset, NewText: reported.NewText}
			if !seen[edit] {
				seen[edit] = true
				edits = append(edits, edit)
			}
		}
	}
	return edits, grouped
}

// TestLint_SharedListFixReportsWhatFixWrites applies the edits that the JSON
// report gives the findings of each list, as a consumer of the report does,
// and compares the result with what --fix writes, in files with one line
// ending and with mixed ones, where --fix writes the line endings between
// the items of a group as the file's dominant one.
func TestLint_SharedListFixReportsWhatFixWrites(t *testing.T) {
	docs := map[string]string{
		"lf.md":        "# L\n\n- a\n  * b\n  * c\n- d\n",
		"crlf.md":      "# L\r\n\r\n- a\r\n  * b\r\n  * c\r\n- d\r\n",
		"mixed.md":     "# L\r\n\r\n- a\r\n  * b\n  * c\r\n- d\r\n",
		"indent.md":    "# L\r\n\r\n   - a\n     - b\r\n     - c\r\n",
		"mixed-cr.md":  "# L\n\n- a\n  * b\r  * c\n- d\n",
		"blank-lf.md":  "# L\r\n\r\n- a\r\n\r\n  * b\n\n  * c\r\n\r\n- d\r\n",
		"two-lists.md": "# L\r\n\r\n- a\r\n  * b\n  * c\r\n\r\ntext\r\n\r\n- e\r\n  + f\n  + g\r\n",
	}
	for name, content := range docs {
		t.Run(name, func(t *testing.T) {
			dir := inProject(t, map[string]string{name: content})

			report := execute(t, "lint", "--format", "json", name)
			require.Empty(t, report.stderr)
			edits, grouped := reportedEdits(t, report.stdout)
			require.NotZero(t, grouped, "the file should have a fix group")
			prepared, err := fix.PrepareEdits(edits, len(content))
			require.NoError(t, err)
			reported := string(fix.ApplyEdits([]byte(content), prepared))

			got := execute(t, "lint", "--fix", "--no-backups", "--color", "never", name)
			require.Empty(t, got.stderr)

			assert.Equal(t, readFile(t, filepath.Join(dir, name)), reported)
		})
	}
}
