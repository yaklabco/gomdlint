package cli_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
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

func readBytes(t *testing.T, path string) []byte {
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

	fixedLF := readBytes(t, filepath.Join(lfDir, "doc.md"))
	fixedCRLF := readBytes(t, filepath.Join(crlfDir, "doc.md"))

	require.NotEqual(t, crlfDocument, string(fixedLF), "the document must have fixable findings")
	assert.Zero(t, bareLineFeeds(fixedCRLF), "fixing a CRLF file must not add bare LF line endings:\n%q", fixedCRLF)
	assert.Equal(t, string(toCRLF(fixedLF)), string(fixedCRLF),
		"a CRLF file must get the same fixes as its LF version")
	assert.True(t, bytes.HasSuffix(fixedCRLF, []byte("End\r\n")), "the final newline must be CRLF")
}

// sarifInsertedTexts returns every replacement.insertedContent.text in a
// SARIF document, wherever it sits.
func sarifInsertedTexts(node any) []string {
	var texts []string
	switch value := node.(type) {
	case map[string]any:
		if inserted, ok := value["insertedContent"].(map[string]any); ok {
			if text, ok := inserted["text"].(string); ok {
				texts = append(texts, text)
			}
		}
		for _, child := range value {
			texts = append(texts, sarifInsertedTexts(child)...)
		}
	case []any:
		for _, child := range value {
			texts = append(texts, sarifInsertedTexts(child)...)
		}
	}
	return texts
}

func TestFix_ReportedFixTextMatchesTheWrittenBytesInCRLFFiles(t *testing.T) {
	t.Parallel()

	// One finding: the heading needs a blank line below it.
	const original = "# Title\r\nSome text\r\n"
	path := filepath.Join(t.TempDir(), "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	var report struct {
		Files []struct {
			Diagnostics []struct {
				Fixes []struct {
					StartOffset int    `json:"startOffset"`
					EndOffset   int    `json:"endOffset"`
					NewText     string `json:"newText"`
				} `json:"fixes"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(runLint(t, "--format", "json", path)), &report))
	var reported []fix.TextEdit
	for _, file := range report.Files {
		for _, diag := range file.Diagnostics {
			for _, edit := range diag.Fixes {
				reported = append(reported, fix.TextEdit{StartOffset: edit.StartOffset, EndOffset: edit.EndOffset, NewText: edit.NewText})
			}
		}
	}
	require.Len(t, reported, 1, "the fixture must have exactly one fix")

	var sarif any
	require.NoError(t, json.Unmarshal([]byte(runLint(t, "--format", "sarif", path)), &sarif))

	runLint(t, "--fix", path)
	written := readBytes(t, path)

	prepared, err := fix.PrepareEdits(reported, len(original))
	require.NoError(t, err)
	assert.Equal(t, string(written), string(fix.ApplyEdits([]byte(original), prepared)),
		"the fix reported in JSON must produce exactly the bytes --fix writes")
	assert.Equal(t, []string{reported[0].NewText}, sarifInsertedTexts(sarif),
		"SARIF must report the same inserted text as JSON")
	assert.Equal(t, "\r\n", reported[0].NewText, "reported fix text uses the file's line ending")
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
		fixedLF := readBytes(t, filepath.Join(lfDir, name))
		fixedCRLF := readBytes(t, filepath.Join(crlfDir, name))
		assert.Zero(t, bareLineFeeds(fixedCRLF), "%s: fixing a CRLF file added bare LF line endings", name)
		assert.Equal(t, string(toCRLF(fixedLF)), string(fixedCRLF), "%s: CRLF and LF fixes differ", name)
	}
}

// toCR returns content with every line feed written as a carriage return,
// the line ending of a classic Mac OS file. content must hold no carriage
// return.
func toCR(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\n"), []byte("\r"))
}

// toMixed returns content with its line feeds written, line by line, as CR
// and CRLF in turn. content must hold no carriage return.
//
// No line ends with a line feed alone, so no fix can make one CRLF line
// ending of a carriage return and a line feed, and every fix of the LF
// version applies. Where lines end with LF too, the engine withholds a fix
// that would leave a carriage return right before a line feed
// (TestFix_MixedFileKeepsEachLineEnding).
func toMixed(content []byte) []byte {
	endings := []string{"\r", "\r\n"}
	lines := strings.Split(string(content), "\n")
	var out strings.Builder
	for idx, line := range lines {
		out.WriteString(line)
		if idx < len(lines)-1 {
			out.WriteString(endings[idx%len(endings)])
		}
	}
	return []byte(out.String())
}

// toLF returns content with every line ending written as a line feed.
func toLF(content []byte) []byte {
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(content, []byte("\r"), []byte("\n"))
}

func TestFix_CRFileKeepsItsLineEndings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lfPath := filepath.Join(dir, "lf.md")
	crPath := filepath.Join(dir, "cr.md")
	require.NoError(t, os.WriteFile(lfPath, []byte(crlfDocument), 0o644))
	require.NoError(t, os.WriteFile(crPath, toCR([]byte(crlfDocument)), 0o644))

	runLint(t, "--fix", lfPath)
	runLint(t, "--fix", crPath)

	fixedLF := readBytes(t, lfPath)
	fixedCR := readBytes(t, crPath)

	require.NotEqual(t, crlfDocument, string(fixedLF), "the document must have fixable findings")
	assert.NotContains(t, string(fixedCR), "\n", "fixing a CR file must not add line feeds")
	assert.Equal(t, string(toCR(fixedLF)), string(fixedCR), "a CR file must get the same fixes as its LF version")
	assert.True(t, bytes.HasSuffix(fixedCR, []byte("End\r")), "the final newline must be CR")
}

// TestFix_MixedFileKeepsEachLineEnding fixes a file whose lines end with LF,
// CR and CRLF. Each line keeps its own ending; the lines that fixes add take
// the dominant ending, here CR, the first of three tied endings.
func TestFix_MixedFileKeepsEachLineEnding(t *testing.T) {
	t.Parallel()

	const original = "# Title\r" + // Needs a blank line below it.
		"Some text   \n" + // Trailing spaces.
		"\r\n" + // Three blank lines, one too many twice.
		"\r" +
		"\r\n" +
		"Text *one* and _two_.\n" + // Two emphasis styles.
		"End" // No final newline.
	path := filepath.Join(t.TempDir(), "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	runLint(t, "--fix", path)

	assert.Equal(t, "# Title\r"+
		"\r"+
		"Some text\n"+
		"\r\n"+
		"Text *one* and *two*.\n"+
		"End\r", string(readBytes(t, path)))
}

func TestFix_CRAndMixedRuleFixturesMatchTheirLFFixes(t *testing.T) {
	t.Parallel()

	fixtures := filepath.Join("..", "..", "pkg", "lint", "rules", "testdata")
	dir := t.TempDir()
	variants := map[string]func([]byte) []byte{
		"lf": func(content []byte) []byte { return content }, "cr": toCR, "mixed": toMixed, "lfcr": toLFCR,
	}
	for name := range variants {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, name), 0o755))
	}

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
		for variant, convert := range variants {
			if err := os.WriteFile(filepath.Join(dir, variant, name), convert(content), 0o644); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)
	if len(names) == 0 {
		t.Skip("no rule fixtures found")
	}

	for variant := range variants {
		runLint(t, "--fix", filepath.Join(dir, variant))
	}

	for _, name := range names {
		fixedLF := readBytes(t, filepath.Join(dir, "lf", name))
		fixedCR := readBytes(t, filepath.Join(dir, "cr", name))
		fixedMixed := readBytes(t, filepath.Join(dir, "mixed", name))
		assert.NotContains(t, string(fixedCR), "\n", "%s: fixing a CR file added line feeds", name)
		assert.Equal(t, string(toCR(fixedLF)), string(fixedCR), "%s: CR and LF fixes differ", name)
		assert.Zero(t, bareLineFeeds(fixedMixed), "%s: fixing a CR and CRLF file added bare LF line endings", name)
		assert.Equal(t, string(fixedLF), string(toLF(fixedMixed)), "%s: mixed and LF fixes differ", name)

		// Where lines end with LF and CR, some fixes are withheld, so only
		// the endings of the lines --fix keeps are compared.
		original := uniqueLineEndings(toLFCR(readBytes(t, filepath.Join(fixtures, strings.ReplaceAll(name, "__", "/")))))
		for text, ending := range uniqueLineEndings(readBytes(t, filepath.Join(dir, "lfcr", name))) {
			if want, kept := original[text]; kept && want != "" {
				assert.Equal(t, want, ending, "%s: the line %q took another ending", name, text)
			}
		}
	}
}

// toLFCR returns content with its line feeds written, line by line, as LF
// and CR in turn. A carriage return right before a line feed would make one
// CRLF line ending of the two, so such a line ends with LF instead. content
// must hold no carriage return.
func toLFCR(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var out strings.Builder
	for idx, line := range lines {
		out.WriteString(line)
		if idx == len(lines)-1 {
			break
		}
		if idx%2 == 1 && (lines[idx+1] != "" || idx+1 == len(lines)-1) {
			out.WriteString("\r")
		} else {
			out.WriteString("\n")
		}
	}
	return []byte(out.String())
}

// uniqueLineEndings returns, for each non-blank line of content whose text
// no other line has, its line ending, empty for a last line without one.
func uniqueLineEndings(content []byte) map[string]string {
	endings := make(map[string]string)
	counts := make(map[string]int)
	for _, line := range mdast.BuildLines(content) {
		text := string(content[line.StartOffset:line.NewlineStart])
		if strings.TrimSpace(text) == "" {
			continue
		}
		counts[text]++
		endings[text] = string(content[line.NewlineStart:line.EndOffset])
	}
	for text, count := range counts {
		if count > 1 {
			delete(endings, text)
		}
	}
	return endings
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
	assert.Equal(t, "# Hello World\n\nSome text.\n", string(readBytes(t, target)), "the symlink target must be fixed")
}

func TestFix_DoesNotFollowSymlinksOutOfTheLintedPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.md")
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.WriteFile(target, []byte(testMarkdownWithTrailingSpaces), 0o644))
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	runLint(t, "--fix", dir)

	assert.Equal(t, testMarkdownWithTrailingSpaces, string(readBytes(t, target)),
		"fixing a directory must not write a file outside it through a symlink")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the symlink must be left alone")
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

	assert.Equal(t, "# Hello World\n\nSome text.\n", string(readBytes(t, second)), "every hard link must see the fix")
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
	assert.Equal(t, "# Title\n\nSome text", string(readBytes(t, path)), "a dry run must not write")
}
