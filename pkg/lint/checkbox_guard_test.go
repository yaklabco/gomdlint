package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

// TestCheckboxCandidateRegions checks which text the guard takes for a
// possible task list checkbox, written as the text of each region.
func TestCheckboxCandidateRegions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		regions []string
	}{
		{name: "plain task", input: "- [ ] task\n", regions: []string{"- [ ] task\n"}},
		{name: "checked and ordered", input: "1. [x] a\n2) [X] b\n", regions: []string{"1. [x] a\n2) [X] b\n"}},
		{name: "any one character", input: "+ [\t] tab\n* [y] why\n", regions: []string{"+ [\t] tab\n* [y] why\n"}},
		{name: "one multibyte character", input: "- [é] e\n", regions: []string{"- [é] e\n"}},
		{name: "two characters", input: "- [xx] no\n"},
		{name: "not first", input: "- a [x] b\n"},
		{name: "no list item", input: "[x] text\n\n> [ ] quote\n"},
		{
			// Nothing but spaces after the brackets: micromark reads a
			// checkbox only if the paragraph goes on, so the next line counts.
			name: "spaces after the brackets", input: "- [x] \n  more\n\nafter\n",
			regions: []string{"- [x] \n  more\n"},
		},
		{name: "last line", input: "- [x]", regions: []string{"- [x]"}},
		{
			name: "blank first line", input: "text\n- \n  [x] task\nnext\n",
			regions: []string{"- \n  [x] task\n"},
		},
		{
			name: "after a definition", input: "- [cc]: /url\n  [x] done\n",
			regions: []string{"- [cc]: /url\n  [x] done\n"},
		},
		{name: "after a blank line", input: "- a\n\n  [x] b\n"},
		{name: "blank quoted line ends the chunk", input: "> - a\n>\n>   [x] b\n"},
		{
			name: "line ending between the brackets", input: "- [\n  ] task\n\nend\n",
			regions: []string{"- [\n  ] task\n\n"},
		},
		{name: "quoted and nested", input: "> - - [x] a\n", regions: []string{"> - - [x] a\n"}},
		{name: "tab after the marker", input: "-\t[x] a\n", regions: []string{"-\t[x] a\n"}},
		{
			name: "next item ends the chunk", input: "- a\n- [x] b\n",
			regions: []string{"- [x] b\n"},
		},
		{name: "ten digits are no marker", input: "1234567890. [x] a\n"},
		{name: "carriage returns", input: "- [x] a\r\n- b\r\n", regions: []string{"- [x] a\r\n"}},
		{name: "byte order mark", input: "\ufeff- [x] a\n", regions: []string{"\ufeff- [x] a\n"}},
		{name: "marker without a space", input: "-[x] a\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := newCheckboxGuard([]byte(tt.input))
			got := make([]string, 0, len(guard.regions))
			for _, region := range guard.regions {
				got = append(got, tt.input[region.start:region.end])
			}
			want := tt.regions
			if want == nil {
				want = []string{}
			}
			assert.Equal(t, want, got)
		})
	}
}

// TestCheckboxGuardWithholds checks which fixes the guard withholds: those
// that change the text of a candidate region, join its first line to the
// line before, or leave a window around the fix with a candidate it did
// not have.
func TestCheckboxGuardWithholds(t *testing.T) {
	tests := []struct {
		name  string
		input string
		old   string // Text the fix replaces, found in input.
		new   string
		want  bool
	}{
		// Codex's repros: micromark reads text in "- \n  [x] task" and a
		// checkbox in "-\n  [x] task"; cmark-gfm reads a checkbox in
		// "- [x] \n" and text in "- [x]\n".
		{name: "space after an empty marker", input: "- \n  [x] task\n", old: "- \n", new: "-\n", want: true},
		{name: "space after the brackets", input: "- [x] \n", old: "] \n", new: "]\n", want: true},
		{name: "marker spacing", input: "-  [x] task\n", old: "-  [", new: "- [", want: true},
		{name: "marker style", input: "* [x] task\n", old: "* [", new: "- [", want: true},
		{name: "indentation", input: "- a\n  - [\t] x\n", old: "\n  - [", new: "\n    - [", want: true},
		{name: "tab between the brackets", input: "- [\t] task\n", old: "[\t]", new: "[ ]", want: true},
		{name: "text after the brackets", input: "- [x] task  \n", old: "task  \n", new: "task\n", want: true},
		{name: "line after a bare checkbox", input: "- [x]\n  task  \n", old: "task  \n", new: "task\n", want: true},
		{name: "joining the marker line to the line before", input: "a\n- [x] b\n", old: "a\n", new: "a ", want: true},
		{name: "text before the marker", input: "a\n- [x] b\n", old: "\n- [x]", new: "\nz- [x]", want: true},
		{
			name: "blank line before a task list", input: "a\n- [x] b\n", old: "a\n", new: "a\n\n",
			want: false,
		},
		{name: "line after the region", input: "- [x] b\n  c  \n", old: "c  \n", new: "c\n", want: false},
		{name: "unrelated line", input: "text  \n\n- [x] b\n", old: "text  \n", new: "text\n", want: false},
		{name: "ordinary list item", input: "-  a\n", old: "-  a", new: "- a", want: false},
		{
			name: "deleting a preceding blank line", input: "a\n\n\n- [x] b\n", old: "\n\n\n", new: "\n\n",
			want: false,
		},
		// Fixes that make a candidate.
		{name: "link text spacing", input: "- [ x](u)\n", old: "[ x]", new: "[x]", want: true},
		{name: "reversed link", input: "- (x)[y]\n", old: "(x)[y]", new: "[x](y)", want: true},
		{name: "removing a blank line before content", input: "- [x]\n\n  task\n", old: "]\n\n", new: "]\n", want: true},
		{name: "making a blank line hold text", input: "-\n\n  [x] task\n", old: "-\n\n", new: "-\nz\n", want: true},
		{name: "marker made by the fix", input: "a\n\n[x] b\n", old: "\n[x]", new: "\n- [x]", want: true},
		{name: "ordinary text", input: "- a  \n", old: "a  \n", new: "a\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := strings.Index(tt.input, tt.old)
			if start < 0 {
				t.Fatalf("%q not in %q", tt.old, tt.input)
			}
			edit := fix.TextEdit{StartOffset: start, EndOffset: start + len(tt.old), NewText: tt.new}
			guard := newCheckboxGuard([]byte(tt.input))
			assert.Equal(t, tt.want, guard.withholds([]fix.TextEdit{edit}))
		})
	}
}

// TestCheckboxGuardInsertions checks insertions at the edges of a region:
// a whole line inserted before the marker line keeps the item as it is,
// while text that joins the marker line does not, and an insertion after
// the region's last line ending touches nothing.
func TestCheckboxGuardInsertions(t *testing.T) {
	input := "a\n- [x] b\nc\n"
	regionStart := strings.Index(input, "- [x]")
	regionEnd := strings.Index(input, "c\n")
	guard := newCheckboxGuard([]byte(input))
	insert := func(offset int, text string) bool {
		return guard.withholds([]fix.TextEdit{{StartOffset: offset, EndOffset: offset, NewText: text}})
	}
	assert.False(t, insert(regionStart, "\n"), "a blank line before the marker line")
	assert.True(t, insert(regionStart, "z"), "text joined to the marker line")
	assert.True(t, insert(regionStart+3, " "), "a space inside the brackets' line")
	assert.False(t, insert(regionEnd, "z"), "text on the next line")
	assert.False(t, insert(len(input), "\n"), "a final line ending")

	last := newCheckboxGuard([]byte("- [x] a"))
	assert.True(t, last.withholds([]fix.TextEdit{{StartOffset: 7, EndOffset: 7, NewText: "\n"}}),
		"a line ending added to the region's last line")

	unterminated := newCheckboxGuard([]byte("a\n- [x] b"))
	assert.True(t, unterminated.withholds([]fix.TextEdit{{StartOffset: 2, EndOffset: 2, NewText: "z"}}),
		"text joined to the marker line of a region at the end of the content")
	assert.False(t, unterminated.withholds([]fix.TextEdit{{StartOffset: 2, EndOffset: 2, NewText: "\n"}}),
		"a blank line before the marker line of a region at the end of the content")
}
