package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

const (
	trailingDoc = "# Title  \n\nText.\n"
	fixedDoc    = "# Title\n\nText.\n"
)

// inProject runs the test from a fresh directory holding files, with no user
// configuration and no GOMDLINT_* settings from the environment.
func inProject(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".xdg"))
	for _, entry := range os.Environ() {
		if name, _, ok := strings.Cut(entry, "="); ok && strings.HasPrefix(name, "GOMDLINT_") {
			t.Setenv(name, "")
		}
	}
	t.Chdir(dir)

	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

// backupFiles lists the backup files in dir.
func backupFiles(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var backups []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".bak") {
			backups = append(backups, entry.Name())
		}
	}
	return backups
}

func TestLintAppliesSeverityDefault(t *testing.T) {
	inProject(t, map[string]string{
		"doc.md":        trailingDoc,
		".gomdlint.yml": "severity_default: error\n",
	})

	got := execute(t, "lint", "--color", "never", "--no-context", "doc.md")

	assert.Equal(t, cli.ExitLintErrors, got.code, got.stdout)
	assert.NotContains(t, got.stdout, "warning")
}

func TestLintFixCreatesBackupsByDefault(t *testing.T) {
	dir := inProject(t, map[string]string{"doc.md": trailingDoc})

	got := execute(t, "lint", "--fix", "doc.md")

	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Equal(t, fixedDoc, readFile(t, filepath.Join(dir, "doc.md")))
	assert.NotEmpty(t, backupFiles(t, dir))
}

func TestLintFixHonoursDisabledBackups(t *testing.T) {
	dir := inProject(t, map[string]string{
		"doc.md":        trailingDoc,
		".gomdlint.yml": "backups:\n  enabled: false\n",
	})

	got := execute(t, "lint", "--fix", "doc.md")

	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Equal(t, fixedDoc, readFile(t, filepath.Join(dir, "doc.md")))
	assert.Empty(t, backupFiles(t, dir))
}

func TestLintFixFromEnvironment(t *testing.T) {
	dir := inProject(t, map[string]string{"doc.md": trailingDoc})
	t.Setenv("GOMDLINT_FIX", "true")

	got := execute(t, "lint", "doc.md")

	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Equal(t, fixedDoc, readFile(t, filepath.Join(dir, "doc.md")))
}

// TestLintExplicitFalseFlagsOverrideEnvironment covers boolean flags set to
// false explicitly, which must override true values from the environment.
func TestLintExplicitFalseFlagsOverrideEnvironment(t *testing.T) {
	t.Run("fix", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": trailingDoc})
		t.Setenv("GOMDLINT_FIX", "true")

		got := execute(t, "lint", "--fix=false", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Equal(t, trailingDoc, readFile(t, filepath.Join(dir, "doc.md")))
	})

	t.Run("no-backups", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": trailingDoc})
		t.Setenv("GOMDLINT_NO_BACKUPS", "true")

		got := execute(t, "lint", "--fix", "--no-backups=false", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.NotEmpty(t, backupFiles(t, dir))
	})

	t.Run("dry-run", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": trailingDoc})
		t.Setenv("GOMDLINT_DRY_RUN", "true")

		got := execute(t, "lint", "--fix", "--dry-run=false", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Equal(t, fixedDoc, readFile(t, filepath.Join(dir, "doc.md")))
	})
}
