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

// repoFiles is a Git working tree with a gitignored build directory and
// third-party directories, each holding a file with a fixable issue.
func repoFiles() map[string]string {
	return map[string]string{
		".git/HEAD":                  "ref: refs/heads/main\n",
		".gitignore":                 "build/\n",
		"doc.md":                     trailingDoc,
		"build/out.md":               trailingDoc,
		"node_modules/pkg/readme.md": trailingDoc,
		"vendor/mod/doc.md":          trailingDoc,
	}
}

func TestLintFixSkipsVendoredAndGitignoredFiles(t *testing.T) {
	dir := inProject(t, repoFiles())

	got := execute(t, "lint", "--fix", "--no-backups")

	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Equal(t, fixedDoc, readFile(t, filepath.Join(dir, "doc.md")))
	for _, untouched := range []string{"build/out.md", "node_modules/pkg/readme.md", "vendor/mod/doc.md"} {
		assert.Equal(t, trailingDoc, readFile(t, filepath.Join(dir, filepath.FromSlash(untouched))), untouched)
	}
}

func TestLintIgnoreDefaultsCanBeTurnedOff(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		config string
		want   string
	}{
		{name: "--no-gitignore flag", args: []string{"--no-gitignore"}, want: "build/out.md"},
		{name: "gitignore config key", config: "gitignore: false\n", want: "build/out.md"},
		{name: "--no-default-ignores flag", args: []string{"--no-default-ignores"}, want: "node_modules/pkg/readme.md"},
		{name: "default_ignores config key", config: "default_ignores: false\n", want: "vendor/mod/doc.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := repoFiles()
			if tt.config != "" {
				files[".gomdlint.yml"] = tt.config
			}
			inProject(t, files)

			got := execute(t, append([]string{"lint", "--color", "never", "--no-context"}, tt.args...)...)

			assert.Contains(t, got.stdout, filepath.FromSlash(tt.want))
		})
	}
}

// TestLintIgnoreRelativeToConfigFile covers running from a subdirectory: the
// project config's patterns stay relative to the config file.
func TestLintIgnoreRelativeToConfigFile(t *testing.T) {
	dir := inProject(t, map[string]string{
		".gomdlint.yml":      "ignore:\n  - \"docs/drafts/**\"\n",
		"docs/guide.md":      trailingDoc,
		"docs/drafts/wip.md": trailingDoc,
	})
	t.Chdir(filepath.Join(dir, "docs"))

	got := execute(t, "lint", "--color", "never", "--no-context")

	assert.Contains(t, got.stdout, "guide.md")
	assert.NotContains(t, got.stdout, "wip.md")
}

func TestLintFormatFromEnvironment(t *testing.T) {
	t.Run("selects the format", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": trailingDoc})
		t.Setenv("GOMDLINT_FORMAT", "json")

		got := execute(t, "lint", "doc.md")

		assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.True(t, strings.HasPrefix(strings.TrimSpace(got.stdout), "{"), "want JSON, got:\n%s", got.stdout)
	})

	t.Run("loses to the flag", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": trailingDoc})
		t.Setenv("GOMDLINT_FORMAT", "json")

		got := execute(t, "lint", "--format", "text", "--color", "never", "doc.md")

		assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Contains(t, got.stdout, "1 issue")
		assert.NotContains(t, got.stdout, "{")
	})

	t.Run("invalid value is a configuration error", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": trailingDoc})
		t.Setenv("GOMDLINT_FORMAT", "bogus")

		got := execute(t, "lint", "doc.md")

		assert.Equal(t, cli.ExitConfigError, got.code)
		assert.Contains(t, got.stderr, `invalid format "bogus"`)
	})
}

func TestLintWarnsAboutUnknownConfigKeys(t *testing.T) {
	inProject(t, map[string]string{
		"doc.md":        fixedDoc,
		".gomdlint.yml": "flavour: gfm\n",
	})

	got := execute(t, "lint", "doc.md")

	assert.Equal(t, cli.ExitSuccess, got.code)
	assert.Contains(t, got.stderr, `unknown key "flavour" (did you mean "flavor"?)`)
	assert.NotContains(t, got.stdout, "flavour")
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

func TestLintRuleListFlags(t *testing.T) {
	t.Run("enable overrides a config disable", func(t *testing.T) {
		inProject(t, map[string]string{
			"doc.md":        trailingDoc,
			".gomdlint.yml": "rules:\n  MD009:\n    enabled: false\n",
		})

		got := execute(t, "lint", "--color", "never", "--enable", "MD009", "doc.md")

		assert.Contains(t, got.stdout, "no-trailing-spaces")
	})

	t.Run("disable by name", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": trailingDoc})

		got := execute(t, "lint", "--color", "never", "--disable", "no-trailing-spaces", "doc.md")

		assert.Equal(t, cli.ExitSuccess, got.code)
		assert.NotContains(t, got.stdout, "no-trailing-spaces")
	})

	t.Run("fix-rules by name", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": trailingDoc})

		got := execute(t, "lint", "--fix", "--no-backups", "--fix-rules", "no-trailing-spaces", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Equal(t, fixedDoc, readFile(t, filepath.Join(dir, "doc.md")))
	})

	for _, flag := range []string{"--enable", "--disable", "--fix-rules"} {
		t.Run(flag+" rejects unknown rules", func(t *testing.T) {
			inProject(t, map[string]string{"doc.md": trailingDoc})

			got := execute(t, "lint", flag, "MD009,MD999", "doc.md")

			assert.Equal(t, cli.ExitInvalidUsage, got.code)
			assert.Contains(t, got.stderr, `"MD999"`)
			assert.Empty(t, got.stdout)
		})
	}
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
