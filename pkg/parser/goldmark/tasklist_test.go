package goldmark

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// TestTaskCheckboxOnlyStartsAParagraph checks that a task checkbox is read
// only where GFM reads one: at the start of the paragraph that is a list
// item's first block, followed by a space, a tab or a line ending and then
// more of the paragraph. goldmark's own parser also reads one at the start
// of a heading and right before other text, which hides a link such as
// "[ ](/url)" from the rules.
func TestTaskCheckboxOnlyStartsAParagraph(t *testing.T) {
	tests := []struct {
		name  string
		input string
		boxes []string // Source of each checkbox, in order.
		links int      // Links the document holds.
	}{
		{name: "paragraph", input: "- [ ] task\n", boxes: []string{"[ ]"}},
		{name: "checked", input: "- [x] done\n- [X] also\n", boxes: []string{"[x]", "[X]"}},
		{name: "ordered", input: "1. [ ] ordered\n", boxes: []string{"[ ]"}},
		{name: "loose list", input: "- [ ] a\n\n- [x] b\n", boxes: []string{"[ ]", "[x]"}},
		{name: "tab after", input: "- [x]\tfoo\n", boxes: []string{"[x]"}},
		{name: "tab inside", input: "- [\t] tab\n", boxes: []string{"[\t]"}},
		{name: "line ending then text", input: "- [x]\n  next\n", boxes: []string{"[x]"}},
		{name: "spaces and line ending then text", input: "- [x]   \n  next\n", boxes: []string{"[x]"}},
		{name: "carriage return and line feed", input: "- [x]\r\n  next\r\n", boxes: []string{"[x]"}},
		{name: "table after", input: "- [x] | a | b |\n  | - | - |\n", boxes: []string{"[x]"}},
		{name: "atx heading", input: "- ## [ ](/url).\n", links: 1},
		{name: "checked atx heading", input: "- ## [x] head\n"},
		{name: "setext heading", input: "- [ ] foo\n  ---\n"},
		{name: "equals setext heading", input: "- [x] bar\n  ===\n"},
		{name: "text right after", input: "- [x]foo\n"},
		{name: "link right after", input: "- [ ](https://example.org)\n", links: 1},
		{name: "checked link right after", input: "- [x](https://example.org)\n", links: 1},
		{name: "nothing after", input: "- [ ]\n"},
		{name: "only spaces after", input: "- [x]   \n"},
		{name: "blockquote in item", input: "- > [ ] quote\n"},
		{name: "second block of item", input: "- a\n\n  [ ] b\n"},
		{name: "not a list", input: "[ ] text\n"},
		// micromark expands a tab to the next multiple of four columns, so
		// "[\t]" is a checkbox only where the tab is one column wide.
		{name: "tab four columns wide", input: "-  [\t] task\n"},
		{name: "tab three columns wide", input: "-   [\t] task\n"},
		{name: "tab two columns wide", input: "-    [\t] task\n"},
		{name: "tab in an ordered item", input: "1. [\t] task\n"},
		{name: "tab in a quoted item", input: "> - [\t] task\n"},
		{name: "tab in a nested item", input: "- a\n  - [\t] task\n"},
		{name: "tabs after the marker and inside", input: "-\t[\t] task\n"},
		{name: "tab after the marker", input: "-\t[x] foo\n", boxes: []string{"[x]"}},
		{name: "wide ordered item", input: "1.  [x] wide\n", boxes: []string{"[x]"}},
		{name: "form feed inside", input: "- [\f] done\n"},
		// A line ending inside the brackets counts as their space when the
		// next line of the paragraph starts with "]".
		{name: "line ending inside", input: "- [\n  ] task\n", boxes: []string{"[\n  ]"}},
		{name: "carriage return and line feed inside", input: "- [\r\n  ] task\r\n", boxes: []string{"[\r\n  ]"}},
		{name: "line ending inside and a lazy bracket", input: "- [\n] task\n", boxes: []string{"[\n]"}},
		{name: "line ending inside in a quoted item", input: "> - [\n>   ] task\n", boxes: []string{"[\n>   ]"}},
		{name: "line ending inside and a line ending after", input: "- [\n  ]\n  task\n", boxes: []string{"[\n  ]"}},
		{name: "line ending inside and a space before the bracket", input: "- [\n   ] task\n"},
		{name: "line ending inside and a tab before the bracket", input: "- [\n\t] task\n"},
		{name: "line ending inside and nothing after", input: "- [\n  ]\n"},
		{name: "letter and line ending inside", input: "- [x\n  ] task\n"},
		{name: "tab after the brackets", input: "- [ ]\ttask\n", boxes: []string{"[ ]"}},
		{name: "blank first line", input: "-\n  [x] done\n", boxes: []string{"[x]"}},
		{name: "code before the brackets", input: "- `x` [x] a\n"},
		// After link reference definitions, which goldmark makes the item's
		// first children, no checkbox is read, as goldmark's own parser
		// reads none. markdownlint reads no checkboxes and reads "[x]" there
		// as a reference.
		{name: "after a definition", input: "- [c]: /url\n  [x] done\n"},
		{name: "unchecked after a definition", input: "- [c]: /url\n  [ ] done\n"},
		{name: "after two definitions", input: "- [c]: /url\n  [d]: /other\n  [x] done\n"},
		{name: "after a definition and a blank line", input: "- [c]: /url\n\n  [x] done\n"},
		{name: "after a blank first line and a definition", input: "-\n  [c]: /url\n  [x] done\n"},
		{name: "defined label after a definition", input: "- [c]: /url\n  [x] done\n\n[x]: /x\n", links: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseForSpans(t, FlavorGFM, tt.input)
			boxes := mdast.FindByKind(file.Root, mdast.NodeTaskCheckbox)
			got := make([]string, 0, len(boxes))
			for _, box := range boxes {
				got = append(got, string(box.Text()))
				require.Equal(t, mdast.NodeParagraph, box.Parent.Kind, "a checkbox starts a paragraph")
				require.Same(t, box, box.Parent.FirstChild, "a checkbox starts its paragraph")
			}
			want := tt.boxes
			if want == nil {
				want = []string{}
			}
			require.Equal(t, want, got)
			require.Len(t, mdast.FindByKind(file.Root, mdast.NodeLink), tt.links)

			commonMark := parseForSpans(t, FlavorCommonMark, tt.input)
			require.Empty(t, mdast.FindByKind(commonMark.Root, mdast.NodeTaskCheckbox), "CommonMark has no task lists")
		})
	}
}

// TestTaskCheckboxKeepsAHardBreak checks that two or more spaces between a
// checkbox and the end of its line make a hard line break, as micromark
// reads them: micromark renders "- [ ]   \n  task" with a <br /> after the
// checkbox. The checkbox takes the spaces before text on its own line, so
// the text starts after them.
func TestTaskCheckboxKeepsAHardBreak(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		breaks int // Hard breaks micromark renders.
		text   string
	}{
		{name: "three spaces", input: "- [ ]   \n  task\n", breaks: 1, text: "task"},
		{name: "two spaces", input: "- [x]  \n  task\n", breaks: 1, text: "task"},
		{name: "one space", input: "- [ ] \n  task\n", text: "task"},
		{name: "spaces and a tab", input: "- [ ]  \t\n  task\n", text: "task"},
		{name: "line ending inside and three spaces", input: "- [\n  ]   \n  task\n", breaks: 1, text: "task"},
		{name: "text after", input: "- [ ]   todo\n", text: "todo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseForSpans(t, FlavorGFM, tt.input)
			require.Len(t, mdast.FindByKind(file.Root, mdast.NodeTaskCheckbox), 1)
			require.Len(t, mdast.FindByKind(file.Root, mdast.NodeHardBreak), tt.breaks)
			var texts []string
			for _, text := range mdast.FindByKind(file.Root, mdast.NodeText) {
				if value := strings.TrimSpace(string(text.Text())); value != "" {
					texts = append(texts, value)
				}
			}
			require.Equal(t, []string{tt.text}, texts)
		})
	}
}
