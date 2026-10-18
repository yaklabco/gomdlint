package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

const (
	trailingDoc = "# Title \n\nText.\n"
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

// TestLintFixesEveryHardLink covers a file with two names: --fix fixes it
// under both, so a strict lint afterwards passes.
func TestLintFixesEveryHardLink(t *testing.T) {
	dir := inProject(t, map[string]string{"doc.md": trailingDoc})
	if err := os.Link(filepath.Join(dir, "doc.md"), filepath.Join(dir, "same.md")); err != nil {
		t.Skipf("hard links not supported: %v", err)
	}

	fixed := execute(t, "lint", "--strict", "--fix", "--no-backups", ".")
	require.Equal(t, cli.ExitSuccess, fixed.code, fixed.stderr)

	got := execute(t, "lint", "--strict", ".")
	assert.Equal(t, cli.ExitSuccess, got.code, got.stdout)
	for _, name := range []string{"doc.md", "same.md"} {
		assert.Equal(t, fixedDoc, readFile(t, filepath.Join(dir, name)), name)
	}
}

// TestLintFixLeavesFilesIgnoredDespiteSymlinkedGitignore covers a nested
// .gitignore that is a symlink to a file re-including keep.md. Git does not
// read it, so keep.md stays ignored by the root pattern and --fix must not
// touch it.
func TestLintFixLeavesFilesIgnoredDespiteSymlinkedGitignore(t *testing.T) {
	dir := inProject(t, map[string]string{
		".git/HEAD":    "ref: refs/heads/main\n",
		".gitignore":   "*.md\n",
		"policy.txt":   "!keep.md\n",
		"docs/keep.md": trailingDoc,
	})
	if err := os.Symlink(filepath.Join("..", "policy.txt"), filepath.Join(dir, "docs", ".gitignore")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	got := execute(t, "lint", "--fix", "--no-backups", ".")

	assert.Equal(t, trailingDoc, readFile(t, filepath.Join(dir, "docs", "keep.md")), got.stdout)
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

// TestLintIgnoreAbsoluteDirectoryPattern covers an absolute --ignore pattern
// with a trailing slash: it skips matching directories, not files.
func TestLintIgnoreAbsoluteDirectoryPattern(t *testing.T) {
	dir := inProject(t, map[string]string{
		"guide.md":        trailingDoc,
		"notes.md/wip.md": trailingDoc,
	})
	pattern := filepath.Join(dir, "*.md") + string(filepath.Separator)

	got := execute(t, "lint", "--color", "never", "--no-context", "--ignore", pattern)

	assert.Contains(t, got.stdout, "guide.md")
	assert.NotContains(t, got.stdout, "wip.md")
}

// TestLintAppliesLoneMarkdownlintConfig covers a project configured only
// for markdownlint: its settings apply, with a notice on stderr, and
// nothing prompts or writes when the streams are not terminals.
func TestLintAppliesLoneMarkdownlintConfig(t *testing.T) {
	dir := inProject(t, map[string]string{
		"doc.md":             trailingDoc,
		".markdownlint.json": `{"no-trailing-spaces": false}`,
	})

	got := execute(t, "lint", "--format", "json", "doc.md")

	assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.NotContains(t, got.stdout, "MD009")
	assert.True(t, strings.HasPrefix(strings.TrimSpace(got.stdout), "{"), "stdout must stay pure JSON:\n%s", got.stdout)
	assert.Contains(t, got.stderr, ".markdownlint.json")
	assert.Contains(t, got.stderr, "gomdlint migrate")
	assert.NoFileExists(t, filepath.Join(dir, ".gomdlint.yml"))
}

// tableDoc has a GFM table row with an extra cell, which MD056 reports when
// tables are parsed.
const tableDoc = "# Title\n\n| a | b |\n| - | - |\n| 1 | 2 | 3 |\n"

func TestLintDefaultsToGFM(t *testing.T) {
	t.Run("tables are checked without --flavor", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": tableDoc})

		got := execute(t, "lint", "--color", "never", "--no-context", "doc.md")

		assert.Contains(t, got.stdout, "table-column-count")
	})

	t.Run("commonmark remains selectable", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": tableDoc})

		got := execute(t, "lint", "--color", "never", "--no-context", "--flavor", "commonmark", "doc.md")

		assert.NotContains(t, got.stdout, "table-column-count")
	})

	t.Run("help names the default", func(t *testing.T) {
		got := execute(t, "lint", "--help")

		assert.Contains(t, got.stdout, `(default "gfm")`)
	})
}

func TestInitTemplateUsesGFM(t *testing.T) {
	dir := inProject(t, nil)

	got := execute(t, "init")

	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Contains(t, readFile(t, filepath.Join(dir, ".gomdlint.yml")), "\nflavor: gfm\n")
}

// TestLintDiffFormatPreviewsFixes covers --format diff without --fix: it
// shows the diff --fix would apply and leaves the file alone.
func TestLintDiffFormatPreviewsFixes(t *testing.T) {
	dir := inProject(t, map[string]string{"doc.md": trailingDoc})

	got := execute(t, "lint", "--format", "diff", "--color", "never", "doc.md")

	assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Contains(t, got.stdout, "-# Title ")
	assert.Contains(t, got.stdout, "+# Title")
	assert.Equal(t, trailingDoc, readFile(t, filepath.Join(dir, "doc.md")))
	assert.Empty(t, backupFiles(t, dir))

	// The exit code still describes the file as it is.
	got = execute(t, "lint", "--format", "diff", "--strict", "doc.md")
	assert.Equal(t, cli.ExitLintWarnings, got.code)
}

func TestLintReportsUnreadableFileOnce(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("permission bits do not stop root or Windows from reading files")
	}

	dir := inProject(t, map[string]string{"locked.md": fixedDoc})
	locked := filepath.Join(dir, "locked.md")
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { assert.NoError(t, os.Chmod(locked, 0o600)) })

	got := execute(t, "lint", "--color", "never", "locked.md")

	assert.Equal(t, cli.ExitIOError, got.code)
	assert.Contains(t, got.stdout, "permission denied")
	assert.NotContains(t, got.stdout, "permission denied: permission denied")
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

// TestLintWarnsAboutUnknownRulesThroughMerges covers a typo that a YAML
// merge key brings into rules, in a config given with --config. The typo
// leaves its rule enabled, so it must warn.
func TestLintWarnsAboutUnknownRulesThroughMerges(t *testing.T) {
	inProject(t, map[string]string{
		"doc.md":     trailingDoc,
		"config.yml": "rules:\n  <<: &rules\n    no-trailling-spaces: false\n",
	})

	got := execute(t, "lint", "--config", "config.yml", "doc.md")

	assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Contains(t, got.stderr, `config.yml:3: unknown rule "no-trailling-spaces" `+
		`(did you mean "no-trailing-spaces"?); it is ignored (through the merge at line 2)`)
}

// TestLintChecksTheRuleKeysThatDecodingReads covers configs, given with
// --config, whose rule keys YAML's merge and alias rules change.
func TestLintChecksTheRuleKeysThatDecodingReads(t *testing.T) {
	t.Run("alias key", func(t *testing.T) {
		inProject(t, map[string]string{
			"doc.md":     trailingDoc,
			"config.yml": "ignore: [&MD013 no-trailling-spaces]\nrules:\n  *MD013 : false\n",
		})

		got := execute(t, "lint", "--config", "config.yml", "doc.md")

		assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Contains(t, got.stderr,
			`config.yml:3: unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?); it is ignored`)
	})

	t.Run("merged rules replaced by written rules", func(t *testing.T) {
		inProject(t, map[string]string{
			"doc.md":     trailingDoc,
			"config.yml": "<<:\n  rules:\n    no-trailling-spaces: false\nrules:\n  MD009: false\n",
		})

		got := execute(t, "lint", "--config", "config.yml", "doc.md")

		assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.NotContains(t, got.stderr, "unknown rule")
	})
}

// TestLintReadsRuleOptionsOutsideOptions covers rule options written
// directly in the rule entry, as markdownlint writes them, in YAML and in
// JSON, under markdownlint's names: they apply, without a warning.
func TestLintReadsRuleOptionsOutsideOptions(t *testing.T) {
	const doc = "# Title\n\nA line of more than twenty characters.\n"
	for name, content := range map[string]string{
		".gomdlint.yml":  "rules:\n  line-length:\n    line_length: 20\n",
		".gomdlint.json": `{"rules": {"MD013": {"line_length": 20, "severity": "error"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			inProject(t, map[string]string{"doc.md": doc, name: content})

			got := execute(t, "lint", "--color", "never", "--no-context", "doc.md")

			assert.Empty(t, got.stderr)
			assert.Contains(t, got.stdout, "doc.md:3:21")
			assert.Contains(t, got.stdout, "line-length")
		})
	}

	// Linting copies the configuration for a file with a
	// markdownlint-configure-file comment, and the copy keeps the option.
	t.Run("file with a configure-file comment", func(t *testing.T) {
		inProject(t, map[string]string{
			"doc.md":        "# Title\n\n<!-- markdownlint-configure-file {\"MD009\": false} -->\n\nA line of more than twenty characters.\n",
			".gomdlint.yml": "rules:\n  line-length:\n    line_length: 20\n",
		})

		got := execute(t, "lint", "--color", "never", "--no-context", "doc.md")

		assert.Empty(t, got.stderr)
		assert.Contains(t, got.stdout, "doc.md:5:21")
		assert.Contains(t, got.stdout, "line-length")
	})
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

	// A markdownlint tag stands for every rule it covers, in any case.
	t.Run("disable by tag", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": trailingDoc})

		got := execute(t, "lint", "--color", "never", "--disable", "WhiteSpace", "doc.md")

		assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.NotContains(t, got.stdout, "no-trailing-spaces")
	})

	t.Run("enable by tag overrides a config disable", func(t *testing.T) {
		inProject(t, map[string]string{
			"doc.md":        trailingDoc,
			".gomdlint.yml": "rules:\n  MD009:\n    enabled: false\n",
		})

		got := execute(t, "lint", "--color", "never", "--enable", "whitespace", "doc.md")

		assert.Contains(t, got.stdout, "no-trailing-spaces")
	})

	t.Run("fix-rules by tag", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": trailingDoc})

		got := execute(t, "lint", "--fix", "--no-backups", "--fix-rules", "whitespace", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Equal(t, fixedDoc, readFile(t, filepath.Join(dir, "doc.md")))
	})

	t.Run("fix-rules leaves out rules outside the tag", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": trailingDoc})

		got := execute(t, "lint", "--fix", "--no-backups", "--fix-rules", "headings", "doc.md")

		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.Equal(t, trailingDoc, readFile(t, filepath.Join(dir, "doc.md")))
	})

	// Inline directives still accept gomdlint's former tags for now, but
	// the flags take only the tags markdownlint knows.
	t.Run("former gomdlint tags are unknown", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": trailingDoc})

		got := execute(t, "lint", "--disable", "style", "doc.md")

		assert.Equal(t, cli.ExitInvalidUsage, got.code)
		assert.Contains(t, got.stderr, `unknown rule "style"`)
	})
}

// TestLintDryRunSummary covers --fix --dry-run, which writes nothing: the
// summary says what would be fixed, not that anything was.
func TestLintDryRunSummary(t *testing.T) {
	dir := inProject(t, map[string]string{"a.md": "# T\n\ntext"})

	got := execute(t, "lint", "--fix", "--dry-run", "--color", "never", "a.md")

	assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Contains(t, got.stdout, "1 fix would be applied in 1 file")
	assert.NotContains(t, got.stdout, "fixed in 0 files")
	assert.Equal(t, "# T\n\ntext", readFile(t, filepath.Join(dir, "a.md")))
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
