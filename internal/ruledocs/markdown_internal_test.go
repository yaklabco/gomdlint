package ruledocs

import (
	"bytes"
	"html"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// renderGFM renders source as GitHub Flavored Markdown, as GitHub shows a
// docs page: with tables, strikethrough, task lists and links for bare
// URLs.
func renderGFM(t *testing.T, source string) string {
	t.Helper()

	var out bytes.Buffer
	require.NoError(t, goldmark.New(goldmark.WithExtensions(extension.GFM)).Convert([]byte(source), &out))
	return out.String()
}

// catalogTexts returns every text of the catalog that Markdown output
// shows, keyed by where it comes from.
func catalogTexts() map[string]string {
	texts := make(map[string]string)
	for _, meta := range rules.Catalog() {
		texts[meta.ID+" description"] = meta.Description
		texts[meta.ID+" rationale"] = meta.Rationale
		texts[meta.ID+" fix"] = meta.Fix
		for _, option := range meta.Options {
			texts[meta.ID+" option "+option.Name] = option.Description
		}
	}
	// Words that could start a block at the start of a wrapped line, and a
	// URL, which GitHub links without brackets.
	texts["block starts"] = "Text - with + list = markers, 1. and 2) numbers, --- rules, " +
		"a #heading and ### hashes, > quotes, | pipes |, ~~~ fences and ``` code."
	texts["links"] = "See https://example.com, www.example.com/path, (www.example.com) and <mailto:a@example.com>, " +
		"&amp; or &#42; *text* _text_ [text](url) ![alt](src) [label] $x$ ~strike~."
	return texts
}

// TestMarkdownTextShowsTheCatalogTextLiterally renders each catalog text as
// Markdown output writes it, in a paragraph wrapped at 80 columns, and
// checks that it renders as exactly the plain text: Markdown punctuation
// in the text, such as **bold** or [text](url), shows as written and makes
// no emphasis, link or other markup.
func TestMarkdownTextShowsTheCatalogTextLiterally(t *testing.T) {
	for source, text := range catalogTexts() {
		if text == "" {
			continue
		}
		paragraph := strings.Join(wrap(markdownText(text), lineWidth, "", ""), "\n")

		got := renderGFM(t, paragraph+"\n")

		inner, isParagraph := strings.CutPrefix(got, "<p>")
		inner, closed := strings.CutSuffix(inner, "</p>\n")
		if assert.True(t, isParagraph && closed, "%s renders as one paragraph: %q", source, got) {
			assert.NotContains(t, inner, "<", "%s makes no markup: %q", source, got)
			assert.Equal(t, text, html.UnescapeString(strings.ReplaceAll(inner, "\n", " ")), "%s", source)
		}
	}
}

// TestMarkdownTextAtTheStartOfEveryLine wraps each catalog text at width
// 1, so that each word starts a line, in a paragraph and in a list item,
// and checks that it still renders as exactly the plain text: no word that
// starts a line opens a list, a heading, a quote, a thematic break, a code
// block or a table.
func TestMarkdownTextAtTheStartOfEveryLine(t *testing.T) {
	for source, text := range catalogTexts() {
		if text == "" {
			continue
		}
		escaped := markdownText(text)

		paragraph := renderGFM(t, strings.Join(wrap(escaped, 1, "", ""), "\n")+"\n")
		inner, isParagraph := strings.CutPrefix(paragraph, "<p>")
		inner, closed := strings.CutSuffix(inner, "</p>\n")
		if assert.True(t, isParagraph && closed, "%s renders as one paragraph: %q", source, paragraph) {
			assert.NotContains(t, inner, "<", "%s makes no markup: %q", source, paragraph)
			assert.Equal(t, text, html.UnescapeString(strings.ReplaceAll(inner, "\n", " ")), "%s", source)
		}

		item := renderGFM(t, strings.Join(wrap(escaped, 1, "- ", "  "), "\n")+"\n")
		inner, isList := strings.CutPrefix(item, "<ul>\n<li>")
		inner, closed = strings.CutSuffix(inner, "</li>\n</ul>\n")
		if assert.True(t, isList && closed, "%s renders as one list item: %q", source, item) {
			assert.NotContains(t, inner, "<", "%s makes no markup: %q", source, item)
			assert.Equal(t, text, html.UnescapeString(strings.ReplaceAll(inner, "\n", " ")), "%s", source)
		}
	}
}

// TestMarkdownTextInTableCells covers a description in a cell of the rules
// table, where a pipe would end the cell.
func TestMarkdownTextInTableCells(t *testing.T) {
	for source, text := range catalogTexts() {
		if text == "" {
			continue
		}
		table := "| Description |\n| --- |\n| " + markdownText(text) + " |\n"

		got := renderGFM(t, table)

		_, cell, found := strings.Cut(got, "<td>")
		cell, _, closed := strings.Cut(cell, "</td>")
		if assert.True(t, found && closed, "%s renders in one cell: %q", source, got) {
			assert.NotContains(t, cell, "<", "%s makes no markup: %q", source, got)
			assert.Equal(t, text, html.UnescapeString(cell), "%s", source)
		}
	}
}

// TestWrap covers the line breaking of text and Markdown output.
func TestWrap(t *testing.T) {
	text := "Lines fit in 80 characters, with exemptions such as lone links and definitions, " +
		"and a word that is far too long to fit: " + strings.Repeat("x", 90) + " which stands alone."

	lines := wrap(text, lineWidth, "Why: ", "  ")

	assert.Equal(t, []string{
		"Why: Lines fit in 80 characters, with exemptions such as lone links and",
		"  definitions, and a word that is far too long to fit:",
		"  " + strings.Repeat("x", 90),
		"  which stands alone.",
	}, lines)
	for _, line := range lines {
		if !strings.Contains(line, "xxx") {
			assert.LessOrEqual(t, utf8.RuneCountInString(line), lineWidth, "%q", line)
		}
	}

	assert.Equal(t, []string{"Why:"}, wrap("", lineWidth, "Why: ", ""), "an empty text keeps its label")
	assert.Equal(t, []string{"- `a code span with spaces` and", "  more"},
		wrap("`a code span with spaces` and more", 32, "- ", "  "), "code spans do not break")
	assert.Equal(t, []string{"\\`not a code span\\`"}, wrap("\\`not a code span\\`", 80, "", ""),
		"escaped backticks open no code span")
	assert.Equal(t, []string{"\\`not", "a", "code", "span\\`"}, wrap("\\`not a code span\\`", 5, "", ""))
}
