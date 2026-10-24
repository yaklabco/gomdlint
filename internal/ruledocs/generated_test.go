package ruledocs_test

import (
	"bytes"
	"errors"
	"flag"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// update regenerates the rule reference instead of comparing it:
//
//	go test ./internal/ruledocs -update
//
// The flag goes after the package path: go test stops reading package
// paths at the first flag it does not know.
var update = flag.Bool("update", false, "regenerate the rule reference")

// repoRoot returns the root of the repository, two directories above this
// file's.
func repoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

// TestGeneratedDocsAreUpToDate checks that the rule reference in the
// repository is what the catalog and the examples generate: a page for
// each rule under docs/rules, their index, the configuration's JSON
// Schema under docs/schema, and the block of the README that lists the
// rules. With -update it writes them instead, and removes generated files
// that no rule needs any more.
func TestGeneratedDocsAreUpToDate(t *testing.T) {
	root := repoRoot(t)
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)

	files, err := ruledocs.ReferenceFiles(entries)
	require.NoError(t, err)

	readmePath := filepath.Join(root, "README.md")
	readme, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	files["README.md"], err = ruledocs.UpdateReadme(readme, entries)
	require.NoError(t, err)

	for _, name := range slices.Sorted(maps.Keys(files)) {
		path := filepath.Join(root, filepath.FromSlash(name))
		if *update {
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, files[name], 0o644))
			continue
		}
		onDisk, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s is missing. Run: go test ./internal/ruledocs -update", name)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, string(files[name]), string(onDisk), "%s is out of date. Run: go test ./internal/ruledocs -update", name)
	}

	for _, dir := range ruledocs.ReferenceDirs() {
		listed, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		require.NoError(t, err)
		for _, file := range listed {
			name := dir + "/" + file.Name()
			if _, generated := files[name]; generated {
				continue
			}
			path := filepath.Join(root, filepath.FromSlash(name))
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			if *update && ruledocs.IsGenerated(content) {
				require.NoError(t, os.Remove(path))
				continue
			}
			t.Errorf("%s is not part of the rule reference. Remove it, or run: go test ./internal/ruledocs -update", name)
		}
	}
}

// readmeCount matches a count of rules in the README's prose, such as "55
// lint rules", "37 rules" or "37 of 55 rules", with the share before it, as
// in "67% (37 of 55 rules)".
var readmeCount = regexp.MustCompile(`(?:\b(\d+)% \()?\b(\d+)(?: of (\d+))?(?: lint)? rules\b`)

// TestReadmeCountsMatchTheCatalog checks the counts of rules that the
// README's prose gives outside the generated block: each count is the
// number of rules, of fixable rules or of markdownlint's rules, and "N of
// M rules" gives the fixable rules of all of them, with the share rounded.
func TestReadmeCountsMatchTheCatalog(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	require.NoError(t, err)
	before, _, found := strings.Cut(string(readme), ruledocs.ReadmeBlockBegin)
	require.True(t, found)
	_, after, found := strings.Cut(string(readme), ruledocs.ReadmeBlockEnd)
	require.True(t, found)

	entries := ruledocs.Entries(lint.DefaultRegistry, nil)
	total, fixable, markdownlint := len(entries), 0, 0
	for _, entry := range entries {
		if entry.Fixable {
			fixable++
		}
		if lint.IsMarkdownlintRule(entry.ID) {
			markdownlint++
		}
	}
	number := func(text string) int {
		value, err := strconv.Atoi(text)
		require.NoError(t, err)
		return value
	}

	matches := readmeCount.FindAllStringSubmatch(before+after, -1)
	require.NotEmpty(t, matches)
	for _, match := range matches {
		count := number(match[2])
		if match[3] == "" {
			assert.Contains(t, []int{total, fixable, markdownlint}, count, "%q", match[0])
			continue
		}
		assert.Equal(t, []int{fixable, total}, []int{count, number(match[3])}, "%q", match[0])
		if match[1] != "" {
			assert.Equal(t, (100*fixable+total/2)/total, number(match[1]), "%q", match[0])
		}
	}
}

// TestReferenceFiles covers the files of the rule reference: a page for
// every rule, named after it, the index and the schema, each starting
// with a notice of how to regenerate it.
func TestReferenceFiles(t *testing.T) {
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)
	files, err := ruledocs.ReferenceFiles(entries)
	require.NoError(t, err)

	assert.Len(t, files, len(entries)+2, "a page for each rule, the index and the schema")
	for _, entry := range entries {
		page, found := files["docs/rules/"+entry.Name+".md"]
		if assert.True(t, found, "%s has a page", entry.ID) {
			assert.True(t, ruledocs.IsGenerated(page), "%s", entry.ID)
			assert.Contains(t, string(page), "\n# "+entry.Name+"\n")
		}
	}

	index := string(files["docs/rules/README.md"])
	assert.True(t, ruledocs.IsGenerated([]byte(index)))
	assert.Contains(t, index, "\n| [`heading-increment`](heading-increment.md) | MD001 | no | ")
	assert.Contains(t, index, "\ngomdlint has 55 rules, and 37 of them can fix what they find.")
	assert.True(t, ruledocs.IsGenerated(files["docs/schema/gomdlint.schema.json"]))
}

// TestGeneratedMarkdownLintsClean lints the index and the pages of the
// rule reference, and the README block, as CI lints the repository's
// documentation: with gomdlint's own rules and the repository's
// configuration.
func TestGeneratedMarkdownLintsClean(t *testing.T) {
	root := repoRoot(t)
	cfg := loadRepoConfig(t, filepath.Join(root, ".gomdlint.yml"))
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)

	files, err := ruledocs.ReferenceFiles(entries)
	require.NoError(t, err)
	for name, content := range files {
		if strings.HasSuffix(name, ".md") {
			assertLintsClean(t, name, content, cfg)
		}
	}

	readme := "# Project\n\n## Rules\n\n" + ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockEnd + "\n"
	updated, err := ruledocs.UpdateReadme([]byte(readme), entries)
	require.NoError(t, err)
	assertLintsClean(t, "README.md", updated, cfg)
}

// loadRepoConfig reads the repository's own configuration.
func loadRepoConfig(t *testing.T, path string) *config.Config {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	cfg, err := config.FromYAML(data)
	require.NoError(t, err)
	return cfg
}

// TestUpdateReadme covers the replacement of the README block: only the
// text between the markers changes, and the markers must each appear
// once, in order.
func TestUpdateReadme(t *testing.T) {
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)[:2]
	before := "# Project\n\nIntro.\n\n" + ruledocs.ReadmeBlockBegin + "\nold text\n" + ruledocs.ReadmeBlockEnd + "\n\nAfter.\n"

	updated, err := ruledocs.UpdateReadme([]byte(before), entries)
	require.NoError(t, err)

	text := string(updated)
	assert.True(t, strings.HasPrefix(text, "# Project\n\nIntro.\n\n"+ruledocs.ReadmeBlockBegin+"\n"), text)
	assert.True(t, strings.HasSuffix(text, "\n"+ruledocs.ReadmeBlockEnd+"\n\nAfter.\n"), text)
	assert.NotContains(t, text, "old text")
	assert.Contains(t, text, "(docs/rules/heading-increment.md)")

	again, err := ruledocs.UpdateReadme(updated, entries)
	require.NoError(t, err)
	assert.Equal(t, text, string(again), "updating twice changes nothing")

	for _, broken := range []string{
		"# Project\n",
		ruledocs.ReadmeBlockBegin + "\n",
		ruledocs.ReadmeBlockEnd + "\n" + ruledocs.ReadmeBlockBegin + "\n",
		ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockEnd + "\n" + ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockEnd + "\n",
		ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockEnd + "\n" + ruledocs.ReadmeBlockEnd + "\n",
		ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockEnd + "\n",
		"Intro " + ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockEnd + "\n",
	} {
		_, err := ruledocs.UpdateReadme([]byte(broken), entries)
		assert.Error(t, err, "%q", broken)
	}
}

// TestUpdateReadmeSkipsCodeBlocks covers a README that shows the markers in
// fenced code blocks: they mark nothing, so the block between the real
// markers is replaced and the samples stay as they are, and a README with
// markers only in code has no block.
func TestUpdateReadmeSkipsCodeBlocks(t *testing.T) {
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)[:2]
	sample := "```markdown\n" + ruledocs.ReadmeBlockBegin + "\nsample\n" + ruledocs.ReadmeBlockEnd + "\n```\n"
	tildes := "  ~~~~\n" + ruledocs.ReadmeBlockBegin + "\n```\n" + ruledocs.ReadmeBlockEnd + "\n~~~~~\n"
	readme := "# Project\n\n" + sample + "\n" + tildes + "\n" +
		ruledocs.ReadmeBlockBegin + "\nold\n" + ruledocs.ReadmeBlockEnd + "\n\nAfter.\n"

	updated, err := ruledocs.UpdateReadme([]byte(readme), entries)
	require.NoError(t, err)
	text := string(updated)
	assert.True(t, strings.HasPrefix(text, "# Project\n\n"+sample+"\n"+tildes+"\n"+ruledocs.ReadmeBlockBegin+"\n\n"), text)
	assert.NotContains(t, text, "old")
	assert.Contains(t, text, "(docs/rules/heading-increment.md)")

	_, err = ruledocs.UpdateReadme([]byte("# Project\n\n"+sample), entries)
	assert.Error(t, err, "markers only in code mark no block")

	unclosed := "# Project\n\n```\n" + ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockEnd + "\n"
	_, err = ruledocs.UpdateReadme([]byte(unclosed), entries)
	assert.Error(t, err, "a fence that is never closed runs to the end of the README")
}

// TestUpdateReadmeKeepsLineEndings covers a README with CRLF line endings
// and one whose end marker is its last line, without a newline: the block
// takes the README's line endings, and the rest stays as it was.
func TestUpdateReadmeKeepsLineEndings(t *testing.T) {
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)[:2]

	crlf := "# Project\r\n\r\n" + ruledocs.ReadmeBlockBegin + "\r\nold\r\n" + ruledocs.ReadmeBlockEnd + "\r\n\r\nAfter.\r\n"
	updated, err := ruledocs.UpdateReadme([]byte(crlf), entries)
	require.NoError(t, err)
	text := string(updated)
	assert.True(t, strings.HasPrefix(text, "# Project\r\n\r\n"+ruledocs.ReadmeBlockBegin+"\r\n\r\n"), text)
	assert.True(t, strings.HasSuffix(text, "|\r\n\r\n"+ruledocs.ReadmeBlockEnd+"\r\n\r\nAfter.\r\n"), text)
	assert.Equal(t, strings.Count(text, "\n"), strings.Count(text, "\r\n"), "every line ends in CRLF")
	assert.NotContains(t, text, "old")

	unterminated := ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockEnd
	updated, err = ruledocs.UpdateReadme([]byte(unterminated), entries)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(updated), "|\n\n"+ruledocs.ReadmeBlockEnd), string(updated))
}

// TestIsGenerated covers the notice that marks a generated file.
func TestIsGenerated(t *testing.T) {
	assert.False(t, ruledocs.IsGenerated([]byte("# Notes\n")))
	assert.False(t, ruledocs.IsGenerated(nil))

	var page bytes.Buffer
	require.NoError(t, ruledocs.WriteMarkdownPage(&page, ruledocs.Entries(lint.DefaultRegistry, nil)[0]))
	assert.False(t, ruledocs.IsGenerated(page.Bytes()), "explain prints pages without the notice")
}
