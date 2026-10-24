package rules

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	gmext "github.com/yuin/goldmark/extension"
	gmextast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"
)

// TestFootnoteStartCandidates checks the loose test for where a footnote
// definition may start: a "[^" that only whitespace, container markers and
// the "]:" of another definition precede on its line, with "]:" after it
// before the next blank line.
func TestFootnoteStartCandidates(t *testing.T) {
	tests := []struct {
		input   string
		offsets []int
	}{
		{input: "[^1]: Note.\n", offsets: []int{0}},
		{input: "Text.\n\n   [^a`[`]: Note.\n", offsets: []int{10}},
		{input: "> - 1. [^a]: Note.\n", offsets: []int{7}},
		{input: "1) [^a]: Note.\n", offsets: []int{3}},
		{input: "\t[^a]: Note.\n", offsets: []int{1}},
		{input: "[^a\nb]: Note.\n", offsets: []int{0}},
		{input: "[^x]: [^a`[`]: Note.\n", offsets: []int{0, 6}},
		{input: "[^x]: - [^y]: Note.\n", offsets: []int{0, 8}},
		{input: "[^1]: Note.\r\n[^2]: More.\r\n", offsets: []int{0, 13}},
		{input: "Text.\r[^1]: Note.\r", offsets: []int{6}},
		{input: "[^1] starts.\r\n\r\n[^1]: Note.\r\n", offsets: []int{16}},
		{input: "[^1] starts.\r\r[^1]: Note.\r", offsets: []int{14}},
		{input: "[^1] starts this line.\n"},
		{input: "[^a\n\nb]: Note.\n"},
		{input: "Text [^a]: mid line.\n"},
		// No renderer reads a definition here, but the test may find more.
		{input: "-[^a]: Note.\n", offsets: []int{1}},
		{input: "x[^a]: Note.\n"},
		{input: "12x [^a]: Note.\n"},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.input), func(t *testing.T) {
			assert.Equal(t, tt.offsets, footnoteStartCandidates([]byte(tt.input)))
		})
	}
}

// TestParseFootnotesGivesUpOnDeepNesting checks that a line with many
// nested footnote definitions, which goldmark parses in quadratic time,
// counts as unread without the footnote parse.
func TestParseFootnotesGivesUpOnDeepNesting(t *testing.T) {
	source := []byte(strings.Repeat("[^a]: ", maxFootnoteNesting+1) + "x <b>\n")
	doc, unread := parseFootnotes(source)
	assert.Nil(t, doc)
	assert.True(t, unread)
	doc, _ = parseFootnotes([]byte(strings.Repeat("[^a]: ", maxFootnoteNesting) + "x\n"))
	assert.NotNil(t, doc, "the footnote parse runs up to the limit")
}

// TestBlockAfterLabel checks which text after a footnote label may read
// otherwise for goldmark, which keeps the spaces before it, than for
// cmark-gfm, which skips them: a tab, four spaces, a blockquote and a list
// marker, and nothing else.
func TestBlockAfterLabel(t *testing.T) {
	tests := map[string]bool{
		"[^a]: text":            false,
		"[^a]:   text":          false,
		"[^a]:":                 false,
		"[^a]: <div>":           false,
		"[^a]: [x](y)":          false,
		"[^a]: 2019 study":      false,
		"[^a]: 2019.5 percent":  false,
		"[^a]: 1234567890. x":   false,
		"[^a]: -x":              false,
		"[^a]: ***":             false,
		"[^a]: # Heading":       false,
		"[^a]: ```go":           false,
		"[^a]: x\n- y":          false,
		"[^a]: [^b]: - x":       false,
		"[^a]:    > x":          true,
		"[^a]:     x":           true,
		"[^a]:\tx":              true,
		"[^a]:  \tx":            true,
		"[^a]: > x":             true,
		"[^a]: - x":             true,
		"[^a]: -":               true,
		"[^a]: -\tx":            true,
		"[^a]: + x":             true,
		"[^a]: * x":             true,
		"[^a]: * * *":           true,
		"[^a]: 1. x":            true,
		"[^a]: 1) x":            true,
		"[^a]: 123456789. x":    true,
		"[^a]: 10.":             true,
		"[^a]: 10.\r\n    text": true,
	}
	for input, want := range tests {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			assert.Equal(t, want, blockAfterLabel([]byte(input), 0))
		})
	}
}

// TestHasTagStart checks which "<" may start raw HTML.
func TestHasTagStart(t *testing.T) {
	tests := map[string]bool{
		"":               false,
		"1 < 2":          false,
		"a <= b, c <- d": false,
		"{{< x >}}":      false,
		"ends with <":    false,
		"<3 and <<":      false,
		"<b>":            true,
		"x</b>":          true,
		"<!-- c -->":     true,
		"<?x?>":          true,
		"<https://x>":    true,
		"1 < 2 <Z":       true,
	}
	for input, want := range tests {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			assert.Equal(t, want, hasTagStart([]byte(input)))
		})
	}
}

// TestFootnoteMarkdownReadsDefinitionsAsTheExtension checks that the footnote
// parse reads a referenced definition where goldmark's footnote extension
// does, so that a goldmark upgrade that changes the extension's block
// parser, or its priority, which footnoteBlockPriority copies, shows here.
func TestFootnoteMarkdownReadsDefinitionsAsTheExtension(t *testing.T) {
	stock := goldmark.New(goldmark.WithExtensions(gmext.GFM, gmext.Footnote))
	inputs := []string{
		"See [^1].\n\n[^1]: Note <b>x</b>.\n",
		"See [^1] and [x].\n\n[x]: /url\n[^1]: Note.\n",
		"See [^1].\n\n- a\n\n  [^1]: Note.\n\n      Code\n",
		"See [^1].\n\n> [^1]: Note.\n> More.\n",
		"See [^1].\n\nText\n[^1]: Note.\n",
	}
	count := func(doc gmast.Node) int {
		footnotes := 0
		//nolint:errcheck // The visitor never returns an error.
		_ = gmast.Walk(doc, func(node gmast.Node, entering bool) (gmast.WalkStatus, error) {
			if _, ok := node.(*gmextast.Footnote); ok && entering {
				footnotes++
			}
			return gmast.WalkContinue, nil
		})
		return footnotes
	}
	for _, input := range inputs {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			source := []byte(input)
			want := count(stock.Parser().Parse(gmtext.NewReader(source)))
			require.Positive(t, want)
			assert.Equal(t, want, count(footnoteMarkdown().Parser().Parse(gmtext.NewReader(source))))
		})
	}
}
