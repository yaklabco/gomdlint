package ruledocs_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// builtInEntry returns the entry of the built-in rule with the given ID.
func builtInEntry(t *testing.T, id string) ruledocs.Entry {
	t.Helper()

	for _, entry := range ruledocs.Entries(lint.DefaultRegistry, nil) {
		if entry.ID == id {
			return entry
		}
	}
	require.Failf(t, "no such rule", "%s", id)
	return ruledocs.Entry{}
}

// TestMarkdownPageShowsTheExample covers the example on the Markdown page
// of a fixing rule whose example needs options: the configuration, keyed
// by the rule's name, the document that breaks the rule, what gomdlint
// reports and the fixed document.
func TestMarkdownPageShowsTheExample(t *testing.T) {
	var page bytes.Buffer
	require.NoError(t, ruledocs.WriteMarkdownPage(&page, builtInEntry(t, "MD044")))

	assert.Contains(t, page.String(), "\n## Example\n\n"+
		"With this configuration:\n\n"+
		"```yaml\n"+
		"rules:\n"+
		"  proper-names:\n"+
		"    options:\n"+
		"      names:\n"+
		"        - JavaScript\n"+
		"        - GitHub\n"+
		"```\n\n"+
		"This document breaks the rule:\n\n"+
		"```markdown\n"+
		"# Guide\n\n"+
		"Install the javascript client from github.\n"+
		"```\n\n"+
		"gomdlint reports:\n\n"+
		"```text\n"+
		"3:13 Proper name \"javascript\" should be \"JavaScript\"\n"+
		"3:36 Proper name \"github\" should be \"GitHub\"\n"+
		"```\n\n"+
		"The fix changes it to:\n\n"+
		"```markdown\n"+
		"# Guide\n\n"+
		"Install the JavaScript client from GitHub.\n"+
		"```\n\n"+
		"## Configure\n")
}

// TestTextPageShowsTheExample covers the example on the text page of a
// rule that cannot fix what it finds: the document that breaks the rule,
// what gomdlint reports and a document that follows the rule.
func TestTextPageShowsTheExample(t *testing.T) {
	var page bytes.Buffer
	entry := builtInEntry(t, "MD001")
	require.NoError(t, ruledocs.WriteTextPage(&page, entry, entry.Name))

	assert.Contains(t, page.String(), "\n\n"+
		"This document breaks the rule:\n"+
		"  | # Release notes\n"+
		"  |\n"+
		"  | ### Fixed\n"+
		"  |\n"+
		"  | The parser keeps the spaces at the end of a line.\n"+
		"\n"+
		"gomdlint reports:\n"+
		"  3:1 Heading level jumped from H1 to H3\n"+
		"\n"+
		"This document follows the rule:\n"+
		"  | # Release notes\n"+
		"  |\n"+
		"  | ## Fixed\n"+
		"  |\n"+
		"  | The parser keeps the spaces at the end of a line.\n"+
		"\n"+
		"Configure:\n")
}

// TestPagesSayThatTheExampleEndsWithoutANewline covers an example document
// whose last line has no newline, which a code block cannot show: the
// label of the document says so, on both pages.
func TestPagesSayThatTheExampleEndsWithoutANewline(t *testing.T) {
	entry := builtInEntry(t, "MD047")
	var markdown, text bytes.Buffer
	require.NoError(t, ruledocs.WriteMarkdownPage(&markdown, entry))
	require.NoError(t, ruledocs.WriteTextPage(&text, entry, entry.Name))

	label := "This document breaks the rule. Its last line has no newline:"
	assert.Contains(t, markdown.String(), "\n\n"+label+"\n\n```markdown\n# Guide\n")
	assert.Contains(t, text.String(), "\n\n"+label+"\n  | # Guide\n")
	assert.Equal(t, 1, strings.Count(markdown.String(), label), "only the document that breaks the rule")
	assert.Contains(t, markdown.String(), "\n\nThe fix changes it to:\n\n")
}

// TestPagesWithoutAnExample covers a rule without an example: its pages
// have no example section.
func TestPagesWithoutAnExample(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(&quietRule{lint.NewBaseRule("XX001", "quiet-rule", "", nil, false)})
	entry := ruledocs.Entries(registry, nil)[0]

	var markdown, text bytes.Buffer
	require.NoError(t, ruledocs.WriteMarkdownPage(&markdown, entry))
	require.NoError(t, ruledocs.WriteTextPage(&text, entry, entry.Name))

	assert.NotContains(t, markdown.String(), "## Example")
	assert.NotContains(t, text.String(), "breaks the rule")
}
