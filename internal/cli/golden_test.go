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

// testdataDir is the absolute path of testdata, resolved when the package
// loads, before any test changes into a project directory.
var testdataDir = func() string {
	dir, err := filepath.Abs("testdata")
	if err != nil {
		panic(err)
	}
	return dir
}()

// testdataPath returns the absolute path of elem under testdata, whatever
// the current directory.
func testdataPath(elem ...string) string {
	return filepath.Join(append([]string{testdataDir}, elem...)...)
}

// skipUnlessSlashPaths skips a test whose golden output holds
// slash-separated paths on systems that print paths with another
// separator, such as Windows.
func skipUnlessSlashPaths(t *testing.T) {
	t.Helper()

	if filepath.Separator != '/' {
		t.Skip("golden files hold slash-separated paths")
	}
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
	skipUnlessSlashPaths(t)
	project := readProject(t, testdataPath("project"))

	for _, format := range []string{"text", "table", "summary"} {
		for _, ruleFormat := range []string{"name", "id", "combined"} {
			name := "lint-" + format + "-" + ruleFormat
			t.Run(name, func(t *testing.T) {
				inProject(t, project)

				got := execute(t, "lint", "--color", "never", "--format", format, "--rule-format", ruleFormat)

				assert.Equal(t, cli.ExitLintErrors, got.code, got.stderr)
				assert.Empty(t, got.stderr)
				assertGolden(t, testdataPath("golden", name+".golden"), got.stdout)
			})
		}
	}
}

// TestLintTableOutputGolden lints the fixture project in table format,
// combined and per file, with and without color, and compares the output
// with testdata/golden/lint-table[-per-file]-name[-color].golden. The
// tables show the same text either way: severity markers lead the rows,
// and the legend explains the markers the rows show. Color uses basic ANSI,
// which --color always uses without TERM.
func TestLintTableOutputGolden(t *testing.T) {
	skipUnlessSlashPaths(t)
	project := readProject(t, testdataPath("project"))

	for _, layout := range []struct{ name, flag string }{{name: "table"}, {name: "table-per-file", flag: "--per-file"}} {
		for _, color := range []struct{ suffix, mode string }{{mode: "never"}, {suffix: "-color", mode: "always"}} {
			name := "lint-" + layout.name + "-name" + color.suffix
			t.Run(name, func(t *testing.T) {
				inProject(t, project)
				for _, env := range []string{"TERM", "COLORTERM", "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "CI"} {
					t.Setenv(env, "")
				}
				args := []string{"lint", "--color", color.mode, "--format", "table"}
				if layout.flag != "" {
					args = append(args, layout.flag)
				}

				got := execute(t, args...)

				assert.Equal(t, cli.ExitLintErrors, got.code, got.stderr)
				assert.Empty(t, got.stderr)
				assertGolden(t, testdataPath("golden", name+".golden"), got.stdout)
			})
		}
	}
}

// TestLintRuleFormatDefaultsToName covers lint without --rule-format, which
// names rules as --rule-format name does.
func TestLintRuleFormatDefaultsToName(t *testing.T) {
	skipUnlessSlashPaths(t)
	inProject(t, readProject(t, testdataPath("project")))

	got := execute(t, "lint", "--color", "never")

	assert.Equal(t, cli.ExitLintErrors, got.code, got.stderr)
	assert.Equal(t, readFile(t, testdataPath("golden", "lint-text-name.golden")), got.stdout)
}

// TestLintRuleFormatFromEnvironment covers GOMDLINT_RULE_FORMAT, which sets
// the rule format of every human output format when --rule-format is not
// given. Like GOMDLINT_FORMAT, an invalid value is a configuration error,
// and the flag wins over the environment, even over an invalid value.
func TestLintRuleFormatFromEnvironment(t *testing.T) {
	skipUnlessSlashPaths(t)
	project := readProject(t, testdataPath("project"))
	golden := func(t *testing.T, name string) string {
		t.Helper()
		return readFile(t, testdataPath("golden", name+".golden"))
	}

	for _, format := range []string{"text", "table", "summary"} {
		t.Run("selects the format of "+format+" output", func(t *testing.T) {
			want := golden(t, "lint-"+format+"-id")
			inProject(t, project)
			t.Setenv("GOMDLINT_RULE_FORMAT", "id")

			got := execute(t, "lint", "--color", "never", "--format", format)

			assert.Equal(t, cli.ExitLintErrors, got.code, got.stderr)
			assert.Equal(t, want, got.stdout)
		})
	}

	t.Run("loses to the flag", func(t *testing.T) {
		want := golden(t, "lint-text-combined")
		inProject(t, project)
		t.Setenv("GOMDLINT_RULE_FORMAT", "id")

		got := execute(t, "lint", "--color", "never", "--rule-format", "combined")

		assert.Equal(t, cli.ExitLintErrors, got.code, got.stderr)
		assert.Equal(t, want, got.stdout)
	})

	t.Run("invalid value is a configuration error", func(t *testing.T) {
		inProject(t, project)
		t.Setenv("GOMDLINT_RULE_FORMAT", "ids")

		got := execute(t, "lint", "--color", "never")

		assert.Equal(t, cli.ExitConfigError, got.code)
		assert.Contains(t, got.stderr, `invalid rule format "ids"; must be one of: name, id, combined`)
		assert.Empty(t, got.stdout, "no file is linted with an invalid configuration")
	})

	t.Run("flag wins over an invalid value", func(t *testing.T) {
		want := golden(t, "lint-text-name")
		inProject(t, project)
		t.Setenv("GOMDLINT_RULE_FORMAT", "ids")

		got := execute(t, "lint", "--color", "never", "--rule-format", "name")

		assert.Equal(t, cli.ExitLintErrors, got.code, got.stderr)
		assert.Equal(t, want, got.stdout)
	})
}

// TestLintMachineFormatsIgnoreRuleFormat covers JSON, SARIF and diff
// output, which do not change with --rule-format or GOMDLINT_RULE_FORMAT:
// JSON and SARIF always carry both the rule ID and the rule name, and a
// diff names no rules.
func TestLintMachineFormatsIgnoreRuleFormat(t *testing.T) {
	project := readProject(t, testdataPath("project"))

	for _, format := range []string{"json", "sarif", "diff"} {
		t.Run(format, func(t *testing.T) {
			inProject(t, project)
			want := execute(t, "lint", "--color", "never", "--format", format)
			require.NotEmpty(t, want.stdout)

			for _, ruleFormat := range []string{"name", "id", "combined"} {
				got := execute(t, "lint", "--color", "never", "--format", format, "--rule-format", ruleFormat)
				assert.Equal(t, want, got, "--rule-format %s", ruleFormat)
			}

			t.Setenv("GOMDLINT_RULE_FORMAT", "combined")
			assert.Equal(t, want, execute(t, "lint", "--color", "never", "--format", format), "GOMDLINT_RULE_FORMAT=combined")
		})
	}
}
