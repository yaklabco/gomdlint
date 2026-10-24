package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// TestLint_SummaryCountsFixedFindingsNotEdits covers the fix count of the
// summary, which counts the findings fixed, not the edits. MD022 reports the
// missing blank line below "# Title" and the one above "## Sub", which one
// edit fixes, and MD049 reports both markers of "_two_", which one fix of
// two edits fixes. So three edits fix four findings.
func TestLint_SummaryCountsFixedFindingsNotEdits(t *testing.T) {
	const doc = "# Title\n## Sub\n\nText *one* and _two_ here.\n"

	t.Run("dry run", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": doc})

		got := execute(t, "lint", "--fix", "--dry-run", "--color", "never", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Contains(t, got.stdout, "4 fixes would be applied in 1 file")
	})

	t.Run("fix", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": doc})

		got := execute(t, "lint", "--fix", "--no-backups", "--color", "never", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Contains(t, got.stdout, "4 fixes applied in 1 file")
		assert.Equal(t, "# Title\n\n## Sub\n\nText *one* and *two* here.\n", readFile(t, filepath.Join(dir, "doc.md")))
	})
}
