package ruledocs_test

import (
	"bytes"
	"encoding/json"
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
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
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

// TestPageURL covers the published address of each rule's page: the page
// that ReferenceFiles writes, on main of the repository, as the JSON
// Schema's $id is. A rule without a generated page has no address.
func TestPageURL(t *testing.T) {
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)
	files, err := ruledocs.ReferenceFiles(entries)
	require.NoError(t, err)

	const base = "https://github.com/yaklabco/gomdlint/blob/main/"
	assert.Equal(t, base+"docs/rules/", ruledocs.RulePagesURL)
	for _, entry := range entries {
		url := ruledocs.PageURL(entry)
		path, found := strings.CutPrefix(url, base)
		if assert.True(t, found, "%s: %q", entry.ID, url) {
			assert.Contains(t, files, path, "%s: a page that ReferenceFiles writes", entry.ID)
		}
	}

	custom := lint.NewBaseRule("ZZ001", "custom-rule", "A custom rule.", nil, false)
	impostor := lint.NewBaseRule("MD009", "my-trailing-spaces", "Another rule under MD009.", nil, false)
	assert.Empty(t, ruledocs.PageURL(ruledocs.NewEntry(&custom, lint.RuleMeta{ID: "ZZ001", Name: "custom-rule"})),
		"a custom rule")
	assert.Empty(t, ruledocs.PageURL(ruledocs.NewEntry(&impostor, lint.RuleMeta{})),
		"a custom rule under a built-in rule's ID")
}

// TestPagesOfACustomRuleUnderABuiltinID covers a custom rule that takes a
// built-in rule's ID under another name: its pages show none of the
// built-in rule's example, and say that markdownlint does not have it.
func TestPagesOfACustomRuleUnderABuiltinID(t *testing.T) {
	impostor := lint.NewBaseRule("MD009", "my-trailing-spaces", "Another rule under MD009.", nil, false)
	entry := ruledocs.NewEntry(&impostor, lint.RuleMeta{})

	var text, markdown bytes.Buffer
	require.NoError(t, ruledocs.WriteTextPage(&text, entry, ""))
	require.NoError(t, ruledocs.WriteMarkdownPage(&markdown, entry))
	for _, page := range []string{text.String(), markdown.String()} {
		assert.Contains(t, page, "markdownlint does not have this rule.")
		assert.NotContains(t, page, "md009.md")
		assert.NotContains(t, page, "This document breaks the rule")
	}
}

// replacementRule is a custom rule that replaces a built-in one.
type replacementRule struct {
	lint.BaseRule
}

// TestPagesOfARuleThatReplacesABuiltinOne covers a registry with the
// built-in rules, in which a custom rule then replaces MD009: under another
// name, its pages describe it, not the built-in rule, and under the
// built-in rule's own name they keep the catalog's words but show no
// example and no published page, as the rule is not the built-in one.
func TestPagesOfARuleThatReplacesABuiltinOne(t *testing.T) {
	for _, tt := range []struct {
		name     string
		ruleName string
		keepMeta bool
	}{
		{name: "another name", ruleName: "company-policy"},
		{name: "the built-in name", ruleName: "no-trailing-spaces", keepMeta: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			registry := lint.NewRegistry()
			rules.RegisterAll(registry)
			registry.Register(&replacementRule{
				BaseRule: lint.NewBaseRule("MD009", tt.ruleName, "Company policy.", []string{"policy"}, false),
			})

			var entry ruledocs.Entry
			for _, candidate := range ruledocs.Entries(registry, nil) {
				if candidate.ID == "MD009" {
					entry = candidate
				}
			}
			require.Equal(t, tt.ruleName, entry.Name)
			assert.Empty(t, ruledocs.PageURL(entry), "not the built-in rule")

			var text, markdown bytes.Buffer
			require.NoError(t, ruledocs.WriteTextPage(&text, entry, ""))
			require.NoError(t, ruledocs.WriteMarkdownPage(&markdown, entry))
			for _, page := range []string{text.String(), markdown.String()} {
				assert.NotContains(t, page, "This document breaks the rule", "no built-in example")
				assert.NotContains(t, page, "md009.md", "no markdownlint link")
				if tt.keepMeta {
					assert.Contains(t, page, "br_spaces", "the catalog's options")
					assert.Contains(t, page, "whitespace", "the catalog's tags and words")
					continue
				}
				assert.Contains(t, page, "Company policy.")
				assert.NotContains(t, page, "br_spaces", "no built-in options")
				assert.NotContains(t, page, "whitespace", "no built-in tags or words")
			}
		})
	}
}

// TestNewEntry covers the entry of one rule in its built-in state, which
// matches the entry that Entries gives it without a configuration, except
// for the config key, which takes a registry.
func TestNewEntry(t *testing.T) {
	for _, want := range ruledocs.Entries(lint.DefaultRegistry, nil) {
		rule, found := lint.DefaultRegistry.GetByID(want.ID)
		require.True(t, found)
		meta, found := lint.DefaultRegistry.Meta(want.ID)
		require.True(t, found)
		want.ConfigKey = ""
		assert.Equal(t, want, ruledocs.NewEntry(rule, meta), want.ID)
	}

	custom := lint.NewBaseRule("ZZ001", "custom-rule", "The rule's own words.", []string{"own"}, true)
	assert.Equal(t, ruledocs.Entry{
		RuleMeta: lint.RuleMeta{ID: "ZZ001", Name: "custom-rule", Fixable: true, Description: "The rule's own words."},
		Default:  ruledocs.State{Enabled: true, Severity: config.SeverityWarning},
		State:    ruledocs.State{Enabled: true, Severity: config.SeverityWarning},
	}, ruledocs.NewEntry(&custom, lint.RuleMeta{}), "a rule without a catalog entry")
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
	before := "# Project\n\nIntro.\n\n" + ruledocs.ReadmeBlockBegin + "\nSTALE-BLOCK\n" + ruledocs.ReadmeBlockEnd + "\n\nAfter.\n"

	updated, err := ruledocs.UpdateReadme([]byte(before), entries)
	require.NoError(t, err)

	text := string(updated)
	assert.True(t, strings.HasPrefix(text, "# Project\n\nIntro.\n\n"+ruledocs.ReadmeBlockBegin+"\n"), text)
	assert.True(t, strings.HasSuffix(text, "\n"+ruledocs.ReadmeBlockEnd+"\n\nAfter.\n"), text)
	assert.NotContains(t, text, "STALE-BLOCK")
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
		ruledocs.ReadmeBlockBegin + "\nSTALE-BLOCK\n" + ruledocs.ReadmeBlockEnd + "\n\nAfter.\n"

	updated, err := ruledocs.UpdateReadme([]byte(readme), entries)
	require.NoError(t, err)
	text := string(updated)
	assert.True(t, strings.HasPrefix(text, "# Project\n\n"+sample+"\n"+tildes+"\n"+ruledocs.ReadmeBlockBegin+"\n\n"), text)
	assert.NotContains(t, text, "STALE-BLOCK")
	assert.Contains(t, text, "(docs/rules/heading-increment.md)")

	_, err = ruledocs.UpdateReadme([]byte("# Project\n\n"+sample), entries)
	require.Error(t, err, "markers only in code mark no block")

	unclosed := "# Project\n\n```\n" + ruledocs.ReadmeBlockBegin + "\n" + ruledocs.ReadmeBlockEnd + "\n"
	_, err = ruledocs.UpdateReadme([]byte(unclosed), entries)
	assert.Error(t, err, "a fence that is never closed runs to the end of the README")
}

// TestUpdateReadmeReadsTheREADMEAsMarkdown covers fence lines and markers
// that Markdown does not read as fences or markers: a fence line inside an
// HTML comment opens no code block, so the real markers after it count,
// and markers inside a list item or a blockquote mark nothing.
func TestUpdateReadmeReadsTheREADMEAsMarkdown(t *testing.T) {
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)[:2]
	block := ruledocs.ReadmeBlockBegin + "\nSTALE-BLOCK\n" + ruledocs.ReadmeBlockEnd + "\n"
	sample := "```\n" + ruledocs.ReadmeBlockBegin + "\nSAMPLE\n" + ruledocs.ReadmeBlockEnd + "\n```\n"

	for name, readme := range map[string]string{
		"a fence line in a comment": "<!-- example:\n```\n-->\n\n" + block + "\n" + sample,
		"markers in a list item":    "- item\n\n  " + ruledocs.ReadmeBlockBegin + "\n  " + ruledocs.ReadmeBlockEnd + "\n\n" + block,
		"markers in a blockquote":   "> " + ruledocs.ReadmeBlockBegin + "\n> " + ruledocs.ReadmeBlockEnd + "\n\n" + block,
		"a fence in a list item":    "- item\n\n  ```\n  code\n  ```\n\n" + block + "\n" + sample,
	} {
		t.Run(name, func(t *testing.T) {
			updated, err := ruledocs.UpdateReadme([]byte(readme), entries)
			require.NoError(t, err)
			text := string(updated)
			assert.NotContains(t, text, "STALE-BLOCK")
			if strings.Contains(readme, "SAMPLE") {
				assert.Contains(t, text, sample, "the sample stays as it is")
			}
			assert.Contains(t, text, "(docs/rules/heading-increment.md)")
		})
	}
}

// TestUpdateReadmeKeepsLineEndings covers a README with CRLF line endings
// and one whose end marker is its last line, without a newline: the block
// takes the README's line endings, and the rest stays as it was.
func TestUpdateReadmeKeepsLineEndings(t *testing.T) {
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)[:2]

	crlf := "# Project\r\n\r\n" + ruledocs.ReadmeBlockBegin + "\r\nSTALE-BLOCK\r\n" + ruledocs.ReadmeBlockEnd + "\r\n\r\nAfter.\r\n"
	updated, err := ruledocs.UpdateReadme([]byte(crlf), entries)
	require.NoError(t, err)
	text := string(updated)
	assert.True(t, strings.HasPrefix(text, "# Project\r\n\r\n"+ruledocs.ReadmeBlockBegin+"\r\n\r\n"), text)
	assert.True(t, strings.HasSuffix(text, "|\r\n\r\n"+ruledocs.ReadmeBlockEnd+"\r\n\r\nAfter.\r\n"), text)
	assert.Equal(t, strings.Count(text, "\n"), strings.Count(text, "\r\n"), "every line ends in CRLF")
	assert.NotContains(t, text, "STALE-BLOCK")

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

	files, err := ruledocs.ReferenceFiles(ruledocs.Entries(lint.DefaultRegistry, nil))
	require.NoError(t, err)
	for name, content := range files {
		assert.True(t, ruledocs.IsGenerated(content), "%s", name)
	}
}

// TestIsGeneratedKeepsHandWrittenFiles covers hand-written files that quote
// the notices, which -update must not remove from the reference's
// directories: Markdown that quotes a page's notice after its first line or
// the schema's $comment in an example, and JSON whose $comment equal to
// the schema's sits below the top level.
func TestIsGeneratedKeepsHandWrittenFiles(t *testing.T) {
	files, err := ruledocs.ReferenceFiles(ruledocs.Entries(lint.DefaultRegistry, nil))
	require.NoError(t, err)
	notice, _, _ := strings.Cut(string(files["docs/rules/README.md"]), "\n")
	require.True(t, strings.HasPrefix(notice, "<!-- Generated by: "), notice)
	var schema struct {
		Comment string `json:"$comment"`
	}
	require.NoError(t, json.Unmarshal(files["docs/schema/gomdlint.schema.json"], &schema))
	require.NotEmpty(t, schema.Comment)
	quoted, err := json.Marshal(schema.Comment)
	require.NoError(t, err)

	for name, content := range map[string]string{
		"Markdown quoting a page's notice": "# Notes\n\n" + notice + "\n",
		"Markdown quoting the schema's $comment": "# Schema notes\n\n```json\n{\n  \"$comment\": " +
			string(quoted) + "\n}\n```\n",
		"Markdown with a nested JSON example": "# Schema notes\n\n```json\n{\"definitions\": {\"x\": {\"$comment\": " +
			string(quoted) + "}}}\n```\n",
		"JSON with a nested $comment": "{\n  \"title\": \"notes\",\n  \"definitions\": {\n    \"x\": {\"$comment\": " +
			string(quoted) + "}\n  }\n}\n",
		"JSON with a nested $comment first":    "{\"x\": {\"$comment\": " + string(quoted) + "}}",
		"JSON with another top-level $comment": "{\"$comment\": \"Hand-written. " + schema.Comment + "\"}",
		"JSON with the key in another case":    "{\"$COMMENT\": " + string(quoted) + "}",
		"a JSON array":                         "[{\"$comment\": " + string(quoted) + "}]",
		"JSON with trailing data":              "{\"$comment\": " + string(quoted) + "}\n{\"x\": 1}\n",
		"JSON after a byte order mark":         "\xef\xbb\xbf{\"$comment\": " + string(quoted) + "}",
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, ruledocs.IsGenerated([]byte(content)))
		})
	}
}
