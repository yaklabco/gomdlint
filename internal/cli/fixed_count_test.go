package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// TestLint_SummaryCountsFixedFindingsNotEdits covers the fix count of the
// summary: each emphasis and strong style fix rewrites two delimiters, so
// the two findings take four edits, and the summary counts two fixes.
func TestLint_SummaryCountsFixedFindingsNotEdits(t *testing.T) {
	const doc = "# Title\n\nText *one* and _two_ here.\n\nMore **bold** and __strong__.\n"

	t.Run("dry run", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": doc})

		got := execute(t, "lint", "--fix", "--dry-run", "--color", "never", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Contains(t, got.stdout, "2 fixes would be applied in 1 file")
	})

	t.Run("fix", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": doc})

		got := execute(t, "lint", "--fix", "--no-backups", "--color", "never", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Contains(t, got.stdout, "2 fixes applied in 1 file")
		assert.Equal(t, "# Title\n\nText *one* and *two* here.\n\nMore **bold** and **strong**.\n",
			readFile(t, filepath.Join(dir, "doc.md")))
	})
}
