package goldmark

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// checkboxPlace is where a task checkbox starts and whether it is checked.
type checkboxPlace struct {
	offset  int
	checked bool
}

// goldmarkCheckboxPlaces returns the checkboxes that goldmark's own GFM
// extension, which main used, reads in source, leaving out those that start
// a heading.
func goldmarkCheckboxPlaces(t *testing.T, source []byte) []checkboxPlace {
	t.Helper()
	root := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(source))
	places := []checkboxPlace{}
	err := gmast.Walk(root, func(node gmast.Node, entering bool) (gmast.WalkStatus, error) {
		box, ok := node.(*east.TaskCheckBox)
		if !ok || !entering {
			return gmast.WalkContinue, nil
		}
		if _, heading := box.Parent().(*gmast.Heading); !heading {
			places = append(places, checkboxPlace{offset: box.Parent().Lines().At(0).Start, checked: box.IsChecked})
		}
		return gmast.WalkContinue, nil
	})
	require.NoError(t, err)
	return places
}

// TestTaskCheckboxes checks the task checkboxes the parser reads: those
// goldmark's own task list extension reads, as main did, with tabs, form
// feeds and text right after the brackets as goldmark reads them, except
// that it reads none in a heading, so that the rules see a link such as
// "[ ](/url)" in "- ## [ ](/url).".
func TestTaskCheckboxes(t *testing.T) {
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
		// No checkbox in a heading, where goldmark reads one.
		{name: "atx heading", input: "- ## [ ](/url).\n", links: 1},
		{name: "checked atx heading", input: "- ## [x] head\n"},
		{name: "atx heading with a reference", input: "- ## [ ][ref].\n\n[ref]: /url\n", links: 1},
		{name: "quoted atx heading", input: "> - ## [ ](/url).\n", links: 1},
		{name: "setext heading", input: "- [ ] foo\n  ---\n"},
		{name: "equals setext heading", input: "- [x] bar\n  ===\n"},
		// goldmark reads a checkbox before other text, with nothing after
		// it, with a tab of any width or a form feed inside.
		{name: "text right after", input: "- [x]foo\n", boxes: []string{"[x]"}},
		// The address after the checkbox is then an autolink.
		{name: "link right after", input: "- [ ](https://example.org)\n", boxes: []string{"[ ]"}, links: 1},
		{name: "checked link right after", input: "- [x](https://example.org)\n", boxes: []string{"[x]"}, links: 1},
		{name: "reference right after", input: "- [ ][y] done\n", boxes: []string{"[ ]"}},
		{name: "nothing after", input: "- [ ]\n", boxes: []string{"[ ]"}},
		{name: "only spaces after", input: "- [x]   \n", boxes: []string{"[x]"}},
		{name: "tab four columns wide", input: "-  [\t] task\n", boxes: []string{"[\t]"}},
		{name: "tab in an ordered item", input: "1. [\t] task\n", boxes: []string{"[\t]"}},
		{name: "tab in a quoted item", input: "> - [\t] task\n", boxes: []string{"[\t]"}},
		{name: "tabs after the marker and inside", input: "-\t[\t] task\n", boxes: []string{"[\t]"}},
		{name: "tab after the marker", input: "-\t[x] foo\n", boxes: []string{"[x]"}},
		{name: "wide ordered item", input: "1.  [x] wide\n", boxes: []string{"[x]"}},
		{name: "form feed inside", input: "- [\f] done\n", boxes: []string{"[\f]"}},
		{name: "form feed inside before a reference", input: "- [\f][y] done\n", boxes: []string{"[\f]"}},
		{name: "vertical tab inside", input: "- [\v] done\n"},
		// It reads none over a line ending, or anywhere but at the start of
		// the item's first block.
		{name: "line ending inside", input: "- [\n  ] task\n"},
		{name: "letter and line ending inside", input: "- [x\n  ] task\n"},
		{name: "blockquote in item", input: "- > [ ] quote\n"},
		{name: "second block of item", input: "- a\n\n  [ ] b\n"},
		{name: "not a list", input: "[ ] text\n"},
		{name: "blank first line", input: "-\n  [x] done\n", boxes: []string{"[x]"}},
		{name: "code before the brackets", input: "- `x` [x] a\n"},
		{name: "after a definition", input: "- [c]: /url\n  [x] done\n"},
		{name: "after a definition and a blank line", input: "- [c]: /url\n\n  [x] done\n"},
		{name: "defined label after a definition", input: "- [c]: /url\n  [x] done\n\n[x]: /x\n", links: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseForSpans(t, FlavorGFM, tt.input)
			boxes := mdast.FindByKind(file.Root, mdast.NodeTaskCheckbox)
			got := make([]string, 0, len(boxes))
			places := make([]checkboxPlace, 0, len(boxes))
			for _, box := range boxes {
				got = append(got, string(box.Text()))
				places = append(places, checkboxPlace{offset: box.SourceRange().StartOffset, checked: box.Inline.Checked})
				require.Equal(t, mdast.NodeParagraph, box.Parent.Kind, "a checkbox starts a paragraph")
				require.Same(t, box, box.Parent.FirstChild, "a checkbox starts its paragraph")
			}
			want := tt.boxes
			if want == nil {
				want = []string{}
			}
			require.Equal(t, want, got)
			require.Equal(t, goldmarkCheckboxPlaces(t, []byte(tt.input)), places, "goldmark's own extension")
			require.Len(t, mdast.FindByKind(file.Root, mdast.NodeLink), tt.links)

			commonMark := parseForSpans(t, FlavorCommonMark, tt.input)
			require.Empty(t, mdast.FindByKind(commonMark.Root, mdast.NodeTaskCheckbox), "CommonMark has no task lists")
		})
	}
}

// TestTaskCheckboxTakesTheSpaceAfterIt checks that a checkbox takes the
// spaces and tabs after it, and a line ending after them, as goldmark's
// parser does, so no hard line break follows a checkbox and its text starts
// after the white space.
func TestTaskCheckboxTakesTheSpaceAfterIt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		text  string
	}{
		{name: "three spaces", input: "- [ ]   \n  task\n", text: "task"},
		{name: "two spaces", input: "- [x]  \n  task\n", text: "task"},
		{name: "one space", input: "- [ ] \n  task\n", text: "task"},
		{name: "spaces and a tab", input: "- [ ]  \t\n  task\n", text: "task"},
		{name: "text after", input: "- [ ]   todo\n", text: "todo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseForSpans(t, FlavorGFM, tt.input)
			require.Len(t, mdast.FindByKind(file.Root, mdast.NodeTaskCheckbox), 1)
			require.Empty(t, mdast.FindByKind(file.Root, mdast.NodeHardBreak))
			var texts []string
			for _, node := range mdast.FindByKind(file.Root, mdast.NodeText) {
				if value := strings.TrimSpace(string(node.Text())); value != "" {
					texts = append(texts, value)
				}
			}
			require.Equal(t, []string{tt.text}, texts)
		})
	}
}
