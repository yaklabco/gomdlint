package cli_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/fix"
)

// runLint runs "gomdlint lint" with a minimal config and no backups and
// returns stdout. The exit status depends on the findings left, which these
// tests do not check.
func runLint(t *testing.T, args ...string) string {
	t.Helper()

	cfgFile := filepath.Join(t.TempDir(), ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0o644))

	cmd := cli.NewRootCommand(cli.BuildInfo{Version: "test", Commit: "test", Date: "test"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{
		"lint", "--config", cfgFile, "--no-backups", "--color", "never", "--no-context",
	}, args...))
	_ = cmd.Execute() //nolint:errcheck // The exit status reflects remaining findings, not the behaviour under test.

	return stdout.String()
}

func toCRLF(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\n"), []byte("\r\n"))
}

func bareLineFeeds(content []byte) int {
	return bytes.Count(content, []byte("\n")) - bytes.Count(content, []byte("\r\n"))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return content
}

// crlfDocument has findings for several fixable rules that insert line
// breaks (blank lines around headings, lists and fences), remove them
// (multiple blank lines), and add the missing final newline.
const crlfDocument = "# Title\nSome text   \n- item one\n- item two\n\n## Section\nText before fence\n" +
	"```js\ncode\n```\nText after fence\n\n\n\nEnd"

func TestFix_CRLFFileKeepsItsLineEndings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lfDir := filepath.Join(dir, "lf")
	crlfDir := filepath.Join(dir, "crlf")
	require.NoError(t, os.MkdirAll(lfDir, 0o755))
	require.NoError(t, os.MkdirAll(crlfDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lfDir, "doc.md"), []byte(crlfDocument), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(crlfDir, "doc.md"), toCRLF([]byte(crlfDocument)), 0o644))

	runLint(t, "--fix", lfDir)
	runLint(t, "--fix", crlfDir)

	fixedLF := readFile(t, filepath.Join(lfDir, "doc.md"))
	fixedCRLF := readFile(t, filepath.Join(crlfDir, "doc.md"))

	require.NotEqual(t, crlfDocument, string(fixedLF), "the document must have fixable findings")
	assert.Zero(t, bareLineFeeds(fixedCRLF), "fixing a CRLF file must not add bare LF line endings:\n%q", fixedCRLF)
	assert.Equal(t, string(toCRLF(fixedLF)), string(fixedCRLF),
		"a CRLF file must get the same fixes as its LF version")
	assert.True(t, bytes.HasSuffix(fixedCRLF, []byte("End\r\n")), "the final newline must be CRLF")
}

func TestFix_CRLFRuleFixturesMatchTheirLFFixes(t *testing.T) {
	t.Parallel()

	fixtures := filepath.Join("..", "..", "pkg", "lint", "rules", "testdata")
	dir := t.TempDir()
	lfDir := filepath.Join(dir, "lf")
	crlfDir := filepath.Join(dir, "crlf")
	require.NoError(t, os.MkdirAll(lfDir, 0o755))
	require.NoError(t, os.MkdirAll(crlfDir, 0o755))

	var names []string
	err := filepath.WalkDir(fixtures, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".input.md") {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil || bytes.Contains(content, []byte("\r")) {
			return err
		}
		rel, err := filepath.Rel(fixtures, path)
		if err != nil {
			return err
		}
		name := strings.ReplaceAll(filepath.ToSlash(rel), "/", "__")
		names = append(names, name)
		if err := os.WriteFile(filepath.Join(lfDir, name), content, 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(crlfDir, name), toCRLF(content), 0o644)
	})
	require.NoError(t, err)
	if len(names) == 0 {
		t.Skip("no rule fixtures found")
	}

	runLint(t, "--fix", lfDir)
	runLint(t, "--fix", crlfDir)

	for _, name := range names {
		fixedLF := readFile(t, filepath.Join(lfDir, name))
		fixedCRLF := readFile(t, filepath.Join(crlfDir, name))
		assert.Zero(t, bareLineFeeds(fixedCRLF), "%s: fixing a CRLF file added bare LF line endings", name)
		assert.Equal(t, string(toCRLF(fixedLF)), string(fixedCRLF), "%s: CRLF and LF fixes differ", name)
	}
}

func TestFix_WritesThroughSymlinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.WriteFile(target, []byte(testMarkdownWithTrailingSpaces), 0o644))
	if err := os.Symlink("target.md", link); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	runLint(t, "--fix", link)

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "fixing through a symlink must keep the symlink")
	assert.Equal(t, "# Hello World\n\nSome text.\n", string(readFile(t, target)), "the symlink target must be fixed")
}

func TestFix_KeepsHardLinksTogether(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "second.md")
	require.NoError(t, os.WriteFile(first, []byte(testMarkdownWithTrailingSpaces), 0o644))
	if err := os.Link(first, second); err != nil {
		t.Skipf("hard links not supported here: %v", err)
	}

	runLint(t, "--fix", first)

	assert.Equal(t, "# Hello World\n\nSome text.\n", string(readFile(t, second)), "every hard link must see the fix")
	firstInfo, err := os.Stat(first)
	require.NoError(t, err)
	secondInfo, err := os.Stat(second)
	require.NoError(t, err)
	assert.True(t, os.SameFile(firstInfo, secondInfo), "fixing must not split hard links")
}

func TestFix_DryRunDiffShowsAddedFinalNewline(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# Title\n\nSome text"), 0o644))

	out := runLint(t, "--fix", "--dry-run", "--format", "diff", path)

	assert.Contains(t, out, "-Some text\n"+fix.NoNewlineMarker+"\n+Some text\n",
		"a fix that only adds the final newline must show in the diff")
	assert.Equal(t, "# Title\n\nSome text", string(readFile(t, path)), "a dry run must not write")
}
