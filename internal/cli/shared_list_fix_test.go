package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
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
