package cli_test

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// update makes the golden tests write the output they get to their golden
// files instead of comparing it:
//
//	go test ./internal/cli/ -run Golden -update
//
// Read every changed golden file before committing it.
var update = flag.Bool("update", false, "write the output of golden tests to their golden files")

// testdataPath returns the absolute path of elem under testdata. Tests that
// change into a project directory resolve their testdata paths first.
func testdataPath(t *testing.T, elem ...string) string {
	t.Helper()

	path, err := filepath.Abs(filepath.Join(append([]string{"testdata"}, elem...)...))
	require.NoError(t, err)
	return path
}

// readProject returns the files under dir by slash-separated relative path,
// for inProject.
func readProject(t *testing.T, dir string) map[string]string {
	t.Helper()

	files := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	require.NoError(t, err)
	return files
}

// assertGolden compares got with the golden file at path, or writes got to
// it under -update.
func assertGolden(t *testing.T, path, got string) {
	t.Helper()

	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}

	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden file missing; run go test ./internal/cli/ -run Golden -update")
	assert.Equal(t, string(want), got)
}

// TestLintOutputGolden lints the fixture project under testdata/project in
// each human output format with each rule identifier format, and compares
// the output with testdata/golden/lint-<format>-<rule format>.golden.
func TestLintOutputGolden(t *testing.T) {
	project := readProject(t, testdataPath(t, "project"))
	goldenDir := testdataPath(t, "golden")

	for _, format := range []string{"text"} {
		for _, ruleFormat := range []string{"name", "id", "combined"} {
			name := "lint-" + format + "-" + ruleFormat
			t.Run(name, func(t *testing.T) {
				inProject(t, project)

				got := execute(t, "lint", "--color", "never", "--format", format, "--rule-format", ruleFormat)

				assert.Equal(t, cli.ExitLintErrors, got.code, got.stderr)
				assert.Empty(t, got.stderr)
				assertGolden(t, filepath.Join(goldenDir, name+".golden"), got.stdout)
			})
		}
	}
}

// TestLintRuleFormatDefaultsToName covers lint without --rule-format, which
// names rules as --rule-format name does.
func TestLintRuleFormatDefaultsToName(t *testing.T) {
	project := readProject(t, testdataPath(t, "project"))
	want := readFile(t, testdataPath(t, "golden", "lint-text-name.golden"))
	inProject(t, project)

	got := execute(t, "lint", "--color", "never")

	assert.Equal(t, cli.ExitLintErrors, got.code, got.stderr)
	assert.Equal(t, want, got.stdout)
}
