package goldmark

import (
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
