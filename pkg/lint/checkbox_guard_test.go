package lint

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// guardFor builds the checkbox guard of input as the engine does, from the
// document goldmark parses.
func guardFor(t *testing.T, input string) *checkboxGuard {
	t.Helper()
	file, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	return newCheckboxGuard(file.Content, file.Root)
}

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
			regions: []string{"- \n  [x] task\nnext\n"},
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
		{name: "carriage returns", input: "- [x] a\r\n- b\r\n", regions: []string{"- [x] a\r\n- b\r\n"}},
		{name: "carriage returns alone", input: "-\r  [x] a\r\rb\r", regions: []string{"-\r  [x] a\r\r"}},
		{name: "byte order mark", input: "\ufeff- [x] a\n", regions: []string{"\ufeff- [x] a\n"}},
		{name: "marker without a space", input: "-[x] a\n"},
		// micromark reads a checkbox in a list that starts a footnote
		// definition's content on its first line.
		{name: "list in a footnote", input: "[^a]: - [x] task\n", regions: []string{"[^a]: - [x] task\n"}},
		{name: "quoted footnote", input: "> [^a]: 1. [ ] task\n", regions: []string{"> [^a]: 1. [ ] task\n"}},
		{
			name: "footnote in a list item", input: "- a\n  [^b]: - [x] c\n",
			regions: []string{"  [^b]: - [x] c\n"},
		},
		{name: "footnote without a list", input: "[^a]: [x] text\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := guardFor(t, tt.input)
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
		{
			name: "space after an empty marker in a footnote", input: "[^a]: - \n    [x] task\n",
			old: "- \n", new: "-\n", want: true,
		},
		{
			name: "carriage returns alone end lines", input: "- \r  [x] task\r",
			old: "- \r", new: "-\r", want: true,
		},
		{name: "space after the brackets", input: "- [x] \n", old: "] \n", new: "]\n", want: true},
		// The same changes on a plain candidate are let through (see
		// TestCheckboxGuardPlainCarveOut).
		{name: "marker spacing", input: "-  [\t] task\n", old: "-  [", new: "- [", want: true},
		{name: "marker style", input: "* [x] \n", old: "* [", new: "- [", want: true},
		{name: "indentation", input: "- a\n  - [\t] x\n", old: "\n  - [", new: "\n    - [", want: true},
		{name: "tab between the brackets", input: "- [\t] task\n", old: "[\t]", new: "[ ]", want: true},
		{name: "text after the brackets", input: "- [x] task  \n", old: "task  \n", new: "Task  \n", want: true},
		{name: "line after a bare checkbox", input: "- [x]\n  task  \n", old: "task  \n", new: "task\n", want: true},
		{name: "joining the marker line to the line before", input: "a\n- [x] b\n", old: "a\n", new: "a ", want: true},
		{name: "text before the marker", input: "a\n- [x] b\n", old: "\n- [x]", new: "\nz- [x]", want: true},
		{
			name: "blank line before a task list", input: "a\n- [x] b\n", old: "a\n", new: "a\n\n",
			want: false,
		},
		{name: "line after the brackets' line", input: "- [\t] b\n  c  \n", old: "c  \n", new: "c\n", want: true},
		{name: "line after the region", input: "- [x] b\n  c\n  d  \n", old: "d  \n", new: "d\n", want: false},
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
			guard := guardFor(t, tt.input)
			assert.Equal(t, tt.want, guard.withholds([]fix.TextEdit{edit}))
		})
	}
}

// TestCheckboxGuardInsertions checks insertions at the edges of a region:
// a whole line inserted before the marker line keeps the item as it is,
// while text that joins the marker line does not, and an insertion after
// the region's last line ending touches nothing.
func TestCheckboxGuardInsertions(t *testing.T) {
	input := "a\n- [x] b\nc\nd\n"
	regionStart := strings.Index(input, "- [x]")
	regionEnd := strings.Index(input, "d\n")
	guard := guardFor(t, input)
	insert := func(offset int, text string) bool {
		return guard.withholds([]fix.TextEdit{{StartOffset: offset, EndOffset: offset, NewText: text}})
	}
	assert.False(t, insert(regionStart, "\n"), "a blank line before the marker line")
	assert.True(t, insert(regionStart, "z"), "text joined to the marker line")
	assert.True(t, insert(regionStart+3, " "), "a space inside the brackets' line")
	assert.False(t, insert(regionEnd, "z"), "text on the next line")
	assert.False(t, insert(len(input), "\n"), "a final line ending")

	last := guardFor(t, "- [x] a")
	assert.True(t, last.withholds([]fix.TextEdit{{StartOffset: 7, EndOffset: 7, NewText: "\n"}}),
		"a line ending added to the region's last line")

	unterminated := guardFor(t, "a\n- [x] b")
	assert.True(t, unterminated.withholds([]fix.TextEdit{{StartOffset: 2, EndOffset: 2, NewText: "z"}}),
		"text joined to the marker line of a region at the end of the content")
	assert.False(t, unterminated.withholds([]fix.TextEdit{{StartOffset: 2, EndOffset: 2, NewText: "\n"}}),
		"a blank line before the marker line of a region at the end of the content")
}

// TestCheckboxGuardPlainCarveOut checks the fixes the guard lets through on
// a plain candidate: "[ ]", "[x]" or "[X]" on its item's marker line, after
// a single list marker and one tab or one to four spaces, with a space or
// tab and text after the "]", in a list item whose first content is the
// paragraph that starts with it, above any footnote definition. A fix may
// change the item's marker for one of its kind, the space between the
// marker and the "[" to one to four spaces, the indentation and the
// trailing whitespace, and the trailing whitespace of the next line, or the
// next line by the same rules when it is a plain candidate too. Every other
// change keeps the full guard.
func TestCheckboxGuardPlainCarveOut(t *testing.T) {
	tests := []struct {
		name  string
		input string
		old   string // Text the fix replaces, found in input.
		new   string
		want  bool // Whether the guard withholds the fix.
	}{
		// Allowed changes.
		{name: "bullet for a bullet", input: "* [ ] todo\n", old: "* [", new: "- [", want: false},
		{name: "ordered number", input: "9. [ ] todo\n", old: "9.", new: "10.", want: false},
		{name: "ordered delimiter", input: "1. [x] done\n", old: "1.", new: "1)", want: false},
		{name: "narrower marker space", input: "-  [x] done\n", old: "-  [", new: "- [", want: false},
		{name: "wider marker space", input: "- [x] done\n", old: "- [", new: "-    [", want: false},
		{name: "indentation", input: "- a\n  - [x] b\n", old: "  - [x]", new: "    - [x]", want: false},
		{name: "indentation inserted", input: "- a\n- [x] b\n", old: "- [x] b", new: "  - [x] b", want: false},
		{name: "trailing spaces", input: "- [x] done   \n", old: "done   ", new: "done", want: false},
		{name: "tab in the indentation", input: "- a\n\t- [x] b\n", old: "\t- [x]", new: "    - [x]", want: false},
		{name: "tab after the marker", input: "-\t[x] done\n", old: "-\t[", new: "-   [", want: false},
		{name: "quoted item", input: "> - [x] a\n", old: "> - [", new: "> * [", want: false},
		{name: "quoted indentation", input: "> - a\n>   - [x] b\n", old: ">   - [x]", new: ">     - [x]", want: false},
		{name: "next line's trailing spaces", input: "- [x] done\n  more  \n", old: "more  ", new: "more", want: false},
		{name: "next line is a plain candidate", input: "- [x] a\n* [ ] b\n", old: "* [ ] b", new: "- [ ] b", want: false},
		{
			// MD007 moves an item and the item nested in it in one edit.
			name: "two lines in one edit", input: "- [ ] a\n   - [x] b\n      - [ ] c\n",
			old: "   - [x] b\n      -", new: "  - [x] b\n    -", want: false,
		},
		{name: "changed line ending", input: "- [x] a\n- [ ] b\n", old: "a\n- [", new: "a\r\n- [", want: true},
		// Changes outside the carve-out.
		{name: "space after the brackets", input: "- [x] done\n", old: "] done", new: "]  done", want: true},
		{name: "tab after the brackets", input: "- [x]\tdone\n", old: "]\t", new: "] ", want: true},
		{name: "marker space made a tab", input: "- [x] done\n", old: "- [", new: "-\t[", want: true},
		{name: "five spaces after the marker", input: "- [x] done\n", old: "- [", new: "-     [", want: true},
		{name: "bullet for a number", input: "- [x] done\n", old: "- [", new: "1. [", want: true},
		{name: "text", input: "- [x] done\n", old: "done", new: "Done", want: true},
		{name: "added line ending", input: "- [x] done\n", old: "done\n", new: "done\n\n", want: true},
		{name: "blockquote marker removed", input: "> - [x] a\n", old: "> - [x]", new: "- [x]", want: true},
		{name: "next line's indentation", input: "- [x] a\n      ---\n", old: "      ---", new: "  ---", want: true},
		{name: "next line's text", input: "- [x] a\n  b\n", old: "  b", new: "  c", want: true},
		{name: "joined to the line before", input: "a\n- [x] b\n", old: "a\n", new: "a ", want: true},
		// Candidates that are not plain keep the full guard.
		{name: "tab between the brackets", input: "- [\t] a  \n", old: "a  ", new: "a", want: true},
		{name: "nothing after the brackets", input: "- [x]   \n  task\n", old: "]   \n", new: "]\n", want: true},
		{name: "next-line form", input: "- \n  [x] task\n", old: "- \n", new: "-\n", want: true},
		{name: "footnote definition above", input: "[^n]: note\n\n- [x] a  \n", old: "a  ", new: "a", want: true},
		{name: "two markers", input: "- - [x] a  \n", old: "a  ", new: "a", want: true},
		{name: "indented code", input: "    - [x] a  \n", old: "a  ", new: "a", want: true},
		{name: "setext heading", input: "- [x] a  \n  ---\n", old: "a  ", new: "a", want: true},
		{name: "two tabs after the marker", input: "-\t\t[x] a  \n", old: "a  ", new: "a", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := strings.Index(tt.input, tt.old)
			if start < 0 {
				t.Fatalf("%q not in %q", tt.old, tt.input)
			}
			edit := fix.TextEdit{StartOffset: start, EndOffset: start + len(tt.old), NewText: tt.new}
			assert.Equal(t, tt.want, guardFor(t, tt.input).withholds([]fix.TextEdit{edit}))
		})
	}
}
