package goldmark

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// TestTaskCheckboxesAsGFM checks where the GFM flavor reads a task list
// checkbox: at the start of a list item's first paragraph, followed by a
// space or tab and more of the paragraph, or by a line ending inside it, as
// micromark-extension-gfm-task-list-item reads one. micromark is the number
// of checkboxes that micromark with that extension renders, and cmark the
// number GitHub's cmark-gfm 0.29.0.gfm.0 renders; both were counted when the
// test was written. Where they differ, the parser follows micromark, except
// after a link reference definition, where it keeps goldmark's reading, and
// with a tab inside the brackets, which micromark reads and cmark-gfm does
// not. goldmark's own extension also reads a checkbox at the start of a
// heading, before other text, before the end of the paragraph and with a
// form feed inside the brackets.
func TestTaskCheckboxesAsGFM(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		boxes     int
		micromark int
		cmark     int
	}{
		{name: "unchecked", input: "- [ ] todo\n", boxes: 1, micromark: 1, cmark: 1},
		{name: "checked", input: "- [x] done\n", boxes: 1, micromark: 1, cmark: 1},
		{name: "checked with a capital", input: "- [X] also\n", boxes: 1, micromark: 1, cmark: 1},
		{name: "tab inside", input: "- [\t] tab\n", boxes: 1, micromark: 1, cmark: 0},
		{name: "ordered", input: "1. [x] a\n", boxes: 1, micromark: 1, cmark: 1},
		{name: "tab after", input: "- [x]\tfoo\n", boxes: 1, micromark: 1, cmark: 1},
		{name: "spaces and a tab before a line ending", input: "- [x] \t \n  b\n", boxes: 1, micromark: 1, cmark: 1},
		{name: "line ending after", input: "- [x]\n  more\n", boxes: 1, micromark: 1, cmark: 0},
		{name: "spaces before a line ending", input: "- [ ]   \n  task\n", boxes: 1, micromark: 1, cmark: 1},
		{name: "CRLF line ending after", input: "- [x]\r\n  more\r\n", boxes: 1, micromark: 1, cmark: 0},
		{name: "CR line ending after", input: "- [x]\r  more\r", boxes: 1, micromark: 1, cmark: 0},
		{name: "CRLF", input: "- [x] a\r\n", boxes: 1, micromark: 1, cmark: 1},
		{name: "CR", input: "- [x] a\r", boxes: 1, micromark: 1, cmark: 1},
		{name: "item opened by an empty line", input: "-\n  [x] a\n", boxes: 1, micromark: 1, cmark: 0},
		{name: "before a table", input: "- [x] a\n  | b | c |\n  | - | - |\n", boxes: 1, micromark: 1, cmark: 1},
		{name: "ATX heading", input: "- ## [ ](/url).\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "ATX heading before text", input: "- # [x] done\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "setext heading", input: "- [x] foo\n  ---\n", boxes: 0, micromark: 0, cmark: 1},
		{name: "destination after", input: "- [x](https://example.com)\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "space and a destination", input: "- [ ](/url)\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "text after", input: "- [x]foo\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "backslash after", input: "- [x]\\\n  b\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "no-break space after", input: "- [x]\u00a0foo\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "full reference", input: "- [x][y]\n\n[y]: /u\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "full reference with a space as its text", input: "- [ ][y]\n\n[y]: /u\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "form feed inside", input: "- [\f] done\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "vertical tab inside", input: "- [\v] done\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "two spaces inside", input: "- [  ] a\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "unchecked alone", input: "- [ ]\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "checked alone", input: "- [x]\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "checked before spaces alone", input: "- [x]   \n", boxes: 0, micromark: 0, cmark: 1},
		{name: "after a definition", input: "- [c]: /url\n  [x] done\n", boxes: 0, micromark: 1, cmark: 0},
		{name: "second paragraph", input: "- a\n\n  [x] b\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "blockquote in an item", input: "- > [x] a\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "code block in an item", input: "- ```\n  [x] a\n  ```\n", boxes: 0, micromark: 0, cmark: 0},
		{name: "paragraph outside a list", input: "Use [x] here\n", boxes: 0, micromark: 0, cmark: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseForSpans(t, FlavorGFM, tt.input)
			boxes := mdast.FindByKind(file.Root, mdast.NodeTaskCheckbox)
			assert.Len(t, boxes, tt.boxes)
			for _, box := range boxes {
				require.Equal(t, mdast.NodeParagraph, box.Parent.Kind)
			}
			commonMark := parseForSpans(t, FlavorCommonMark, tt.input)
			assert.Empty(t, mdast.FindByKind(commonMark.Root, mdast.NodeTaskCheckbox), "CommonMark")
		})
	}
}

// TestBracketsLeftByTheCheckboxReader checks that brackets the GFM flavor
// does not read as a checkbox are read as what follows makes them: the link
// of item 64, whose text is a space, in a heading, and links after a list
// marker.
func TestBracketsLeftByTheCheckboxReader(t *testing.T) {
	tests := []struct {
		input string
		text  string
		dest  string
	}{
		{input: "- ## [ ](/url).\n", text: " ", dest: "/url"},
		{input: "> - ## [ ](/url).\n", text: " ", dest: "/url"},
		{input: "- # [x](#)\n", text: "x", dest: "#"},
		{input: "- [x](https://example.com)\n", text: "x", dest: "https://example.com"},
		{input: "- [ ](/url)\n", text: " ", dest: "/url"},
		{input: "- [X][y]\n\n[y]: /u\n", text: "X", dest: "/u"},
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input), func(t *testing.T) {
			file := parseForSpans(t, FlavorGFM, tt.input)
			links := mdast.FindByKind(file.Root, mdast.NodeLink)
			require.Len(t, links, 1)
			assert.Equal(t, tt.text, string(file.Content[links[0].ContentRange().StartOffset:links[0].ContentRange().EndOffset]))
			assert.Equal(t, tt.dest, links[0].Inline.Link.Destination)
		})
	}
}
