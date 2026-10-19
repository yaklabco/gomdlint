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

// TestCheckboxCandidateRegions checks which text the guard keeps fixes away
// from, written as the text of each merged region. A line that holds "[",
// one character and "]", or ends with "[", anywhere, is a candidate; one
// that is not plain guards its paragraph run and the lines on either side
// of it, and a plain one its line and the next.
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
		{name: "not first", input: "- a [x] b\n", regions: []string{"- a [x] b\n"}},
		{name: "no list item", input: "[x] text\n\n> [ ] quote\n", regions: []string{"[x] text\n\n> [ ] quote\n"}},
		{
			name: "spaces after the brackets", input: "- [x] \n  more\n\nafter\n",
			regions: []string{"- [x] \n  more\n\n"},
		},
		{name: "last line", input: "- [x]", regions: []string{"- [x]"}},
		{
			name: "blank first line", input: "text\n- \n  [x] task\nnext\n",
			regions: []string{"text\n- \n  [x] task\nnext\n"},
		},
		{
			name: "after a definition", input: "- [cc]: /url\n  [x] done\n",
			regions: []string{"- [cc]: /url\n  [x] done\n"},
		},
		{name: "after a blank line", input: "- a\n\n  [x] b\n", regions: []string{"\n  [x] b\n"}},
		{name: "quote marker lines are not blank", input: "> - a\n>\n>   [x] b\n", regions: []string{"> - a\n>\n>   [x] b\n"}},
		{
			name: "line ending after the bracket", input: "- [\n  ] task\n\nend\n",
			regions: []string{"- [\n  ] task\n\n"},
		},
		{name: "quoted and nested", input: "> - - [x] a\n", regions: []string{"> - - [x] a\n"}},
		{name: "tab after the marker", input: "-\t[x] a\n", regions: []string{"-\t[x] a\n"}},
		{name: "plain item after an item", input: "- a\n- [x] b\n", regions: []string{"- [x] b\n"}},
		{name: "ten digit marker", input: "1234567890. [x] a\n", regions: []string{"1234567890. [x] a\n"}},
		{name: "carriage returns", input: "- [x] a\r\n- b\r\n", regions: []string{"- [x] a\r\n- b\r\n"}},
		{name: "carriage returns alone", input: "-\r  [x] a\r\rb\r", regions: []string{"-\r  [x] a\r\r"}},
		{name: "byte order mark", input: "\ufeff- [x] a\n", regions: []string{"\ufeff- [x] a\n"}},
		{name: "marker without a space", input: "-[x] a\n", regions: []string{"-[x] a\n"}},
		{name: "list in a footnote", input: "[^a]: - [x] task\n", regions: []string{"[^a]: - [x] task\n"}},
		{name: "quoted footnote", input: "> [^a]: 1. [ ] task\n", regions: []string{"> [^a]: 1. [ ] task\n"}},
		{name: "footnote in a list item", input: "- a\n  [^b]: - [x] c\n", regions: []string{"- a\n  [^b]: - [x] c\n"}},
		{name: "footnote without a list", input: "[^a]: [x] text\n", regions: []string{"[^a]: [x] text\n"}},
		{name: "reference in a paragraph", input: "a\n\nsee [1] and\nmore\n\nb\n", regions: []string{"\nsee [1] and\nmore\n\n"}},
		{
			name: "plain items with one that is not", input: "- [ ] a\n- [\t] b\n- [x] c\n\nd\n",
			regions: []string{"- [ ] a\n- [\t] b\n- [x] c\n\n"},
		},
		{
			name: "frozen lines above an item that cannot follow text", input: "a\n\n#Title\nmore\n> 3. [x] task\n",
			regions: []string{"\n#Title\nmore\n> 3. [x] task\n"},
		},
		{
			name: "frozen lines above every item of a list from 3", input: "# T\n3. [ ] a\n4. [ ] b\n5. [ ] c\n",
			regions: []string{"# T\n3. [ ] a\n4. [ ] b\n5. [ ] c\n"},
		},
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

// TestCheckboxGuardInsertions checks insertions at the edges of a region: a
// blank line may come in before a plain bullet item's marker line, while
// text that joins the marker line may not; an insertion after the region's
// last line ending touches nothing; and no line comes in next to a region
// at either end of the content.
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

	// No line comes in before a region that starts the content or after one
	// that ends it.
	edges := "- [\t] a\n"
	for _, offset := range []int{0, len(edges)} {
		assert.True(t, guardFor(t, edges).withholds([]fix.TextEdit{{StartOffset: offset, EndOffset: offset, NewText: "x\n"}}),
			"a line inserted at offset %d next to a region", offset)
	}
	assert.True(t, guardFor(t, "- [ ] a\n").withholds([]fix.TextEdit{{StartOffset: 0, EndOffset: 0, NewText: "x\n"}}),
		"a line inserted before a plain candidate that starts the content")
	assert.False(t, guardFor(t, "").withholds([]fix.TextEdit{{StartOffset: 0, EndOffset: 0, NewText: "x\n"}}),
		"a line inserted into empty content")
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

// TestCheckboxGuardAnyBracketPair checks the round 3 review's cases: every
// line with a bracket pair holding one character guards its paragraph,
// whatever comes before it, so no fix there changes a checkbox that a
// renderer reads after a prefix outside the grammar, such as cmark-gfm
// 0.29 after "1!" or a ten-digit number, or in a footnote whose label holds
// an escaped bracket.
func TestCheckboxGuardAnyBracketPair(t *testing.T) {
	tests := []struct {
		name  string
		input string
		old   string // Text the fix replaces, found in input.
		new   string
		want  bool // Whether the guard withholds the fix.
	}{
		{name: "escaped footnote label", input: "[^a\\]:b]: - [\t] task\n\nUse [^a\\]:b].\n", old: "[\t]", new: "[ ]", want: true},
		{name: "marker with an exclamation mark", input: "-\n  1! [x] \n", old: "] \n", new: "]\n", want: true},
		{name: "ten digit marker", input: "-\n  1234567890. [x] \n", old: "] \n", new: "]\n", want: true},
		{name: "ten digit marker first", input: "1234567890. [x] a  \n", old: "a  ", new: "a", want: true},
		{name: "marker without a space", input: "-[x] a  \n", old: "a  ", new: "a", want: true},
		{name: "line above in the paragraph", input: "intro  \nsee [1] here\n", old: "intro  ", new: "intro", want: true},
		{name: "blank line before the paragraph", input: "a\n\nsee [1] here\n", old: "a\n\n", new: "a\n", want: true},
		{name: "separate paragraph", input: "a  \n\n\nsee [1] here\n", old: "a  ", new: "a", want: false},
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

// TestCheckboxGuardPlainBoundary checks what fixes may change directly
// before a plain candidate. One blank line may come in before an item that
// can interrupt a paragraph in both renderers, a bullet item or an item of
// an ordered list whose first marker is "1." or "1)", on whose line no
// blockquote starts, as MD032 inserts it; such an item starts after
// paragraph text as after a blank line. Blank lines directly before a
// candidate may go while one stays. Every other line inserted or removed
// directly before or after a candidate is withheld; micromark reads
// "Intro\n> 3. [x] task" as text and "Intro\n>\n> 3. [x] task" as a
// checkbox. Before an item that cannot interrupt a paragraph, the lines up
// to the blank line above it may change only in their trailing whitespace:
// micromark reads "#Title\n> 3. [x] task" as text and
// "# Title\n> 3. [x] task" as a checkbox.
func TestCheckboxGuardPlainBoundary(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		offset string // Text before the edit, found in input, or the end.
		delete int    // Bytes the edit removes there.
		insert string
		want   bool // Whether the guard withholds the fix.
	}{
		// Allowed.
		{name: "blank line before a bullet item", input: "Intro\n- [ ] a\n", offset: "Intro\n", insert: "\n", want: false},
		{name: "blank line at the end of the line before", input: "Intro\n- [ ] a\n", offset: "Intro", insert: "\n", want: false},
		{name: "blank line before an item numbered 1", input: "Intro\n1. [ ] a\n", offset: "Intro\n", insert: "\n", want: false},
		{name: "blank line before an item numbered 1 with a parenthesis", input: "Intro\n1) [ ] a\n", offset: "Intro\n", insert: "\n", want: false},
		{name: "quoted blank line in an open blockquote", input: "> Intro\n> - [ ] a\n", offset: "> Intro\n", insert: ">\n", want: false},
		{name: "blank line after a heading", input: "# Title\n- [ ] a\n", offset: "# Title\n", insert: "\n", want: false},
		{name: "one of two blank lines removed", input: "Intro\n\n\n- [ ] a\n", offset: "Intro\n", delete: 1, want: false},
		{name: "line inserted above the line before", input: "Intro\nmore\n- [ ] a\n", offset: "Intro\n", insert: "x\n", want: false},
		{name: "heading fixed directly before a bullet item", input: "#Title\n- [ ] a\n", offset: "#", insert: " ", want: false},
		{name: "blank line after a blank line before a bullet item", input: "Intro\n\n- [ ] a\n", offset: "Intro\n\n", insert: "\n", want: false},
		{name: "trailing spaces above an item that cannot interrupt", input: "Intro  \n> 3. [x] task\n", offset: "Intro", delete: 2, want: false},
		{name: "blank line removed before an ordered item from 3", input: "# T\n\n\n3. [ ] a\n", offset: "# T\n", delete: 1, want: false},
		{name: "text above the blank line before an item that cannot interrupt", input: "#Title\n\n3. [ ] a\n", offset: "#", insert: " ", want: false},
		{name: "line above the blank line above an item that cannot follow text", input: "a\n\n#Title\n> 3. [x] task\n", offset: "a\n", insert: "x\n", want: false},
		{name: "items of a list from 3", input: "# T\n\n3. [ ] a  \n4. [x] b\n", offset: "a", delete: 2, want: false},
		{name: "trailing spaces on the third item of a list from 3 after a heading", input: "# T\n3. [ ] a\n4. [ ] b\n5. [ ] c  \n", offset: "5. [ ] c", delete: 2, want: false},
		{name: "item after a lazy line in its blockquote", input: "> Intro\ncontinued\n> - [ ] a\n", offset: "continued\n", insert: ">\n", want: false},
		{name: "blank line before a bullet item with CRLF", input: "Intro\r\n- [ ] a\r\n", offset: "Intro\r\n", insert: "\r\n", want: false},
		{name: "trailing spaces on a plain item after a byte order mark", input: "\ufeff- [ ] a  \n", offset: "\ufeff- [ ] a", delete: 2, want: false},
		{name: "blank line before an item after a byte order mark", input: "\ufeffIntro\n- [ ] a\n", offset: "\ufeffIntro\n", insert: "\n", want: false},
		{name: "blank line removed after a first line with only a byte order mark", input: "\ufeff\n\n3. [ ] a\n", offset: "\ufeff\n", delete: 1, want: false},
		// Withheld.
		{name: "zero-padded first item", input: "Intro\n01. [ ] a\n", offset: "Intro\n", insert: "\n", want: true},
		{name: "blank line before a list whose first marker is zero-padded", input: "Intro\n01. a\n2. [x] b\n", offset: "Intro\n", insert: "\n", want: true},
		{name: "heading fixed directly before an item that cannot interrupt", input: "#Title\n> 3. [x] task\n", offset: "#", insert: " ", want: true},
		{name: "heading fixed two lines above an item that cannot interrupt", input: "#Title\nmore\n> 3. [x] task\n", offset: "#", insert: " ", want: true},
		{name: "punctuation removed directly before an ordered item from 3", input: "# Title:\n3. [ ] a\n", offset: "# Title", delete: 1, want: true},
		{name: "blank line after a blank line before an ordered item from 3", input: "# T\n\n3. [ ] a\n", offset: "# T\n\n", insert: "\n", want: true},
		{name: "line at the top of the lines above an item that cannot follow text", input: "a\n\n#Title\n> 3. [x] task\n", offset: "a\n\n", insert: "x\n", want: true},
		{name: "blank line above the lines above an item that cannot follow text", input: "a\n\n#Title\n> 3. [x] task\n", offset: "a\n", delete: 1, want: true},
		{name: "line ending removed above the blank line before an item from 3", input: "Intro\n\n3. [ ] a\n", offset: "Intro", delete: 1, want: true},
		{name: "line ending removed above a line of spaces before an item from 3", input: "Intro\n  \n3. [ ] a\n", offset: "Intro", delete: 1, want: true},
		{name: "text joined to the blank line before an item from 3", input: "Intro\n\n3. [ ] a\n", offset: "Intro\n", insert: "x", want: true},
		{name: "line ending removed above the blank line before a quoted item from 3", input: "Intro\n\n> 3. [x] task\n", offset: "Intro", delete: 1, want: true},
		{name: "text joined to the blank line before a quoted item from 3", input: "Intro\n\n> 3. [x] task\n", offset: "Intro\n", insert: "x", want: true},
		{name: "indented quote marker before an item that cannot follow text", input: "Intro\n    >\n> 3. [x] task\n", offset: "Intro\n", delete: 4, want: true},
		{name: "heading above a list from 3 with three items", input: "# T:\n3. [ ] a\n4. [ ] b\n5. [ ] c\n", offset: "# T", delete: 1, want: true},
		{name: "item opening a nested blockquote", input: "> Intro\n> > - [ ] a\n", offset: "> Intro\n", insert: ">\n", want: true},
		{name: "heading fixed before an item that cannot follow text, with CRLF", input: "#Title\r\n> 3. [x] task\r\n", offset: "#", insert: " ", want: true},
		{name: "lone carriage returns, where goldmark reads no list", input: "Intro\r- [ ] a\r", offset: "Intro\r", insert: "\r", want: true},
		{name: "indentation of the line above an item that cannot follow text", input: "#Title\n> 3. [x] task\n", offset: "", insert: " ", want: true},
		{name: "ordered list from 3 in a blockquote", input: "Intro\n> 3. [x] task\n", offset: "Intro\n", insert: ">\n", want: true},
		{name: "blockquote starting on the item's line", input: "Intro\n> - [x] task\n", offset: "Intro\n", insert: ">\n", want: true},
		{name: "item numbered 3 after text", input: "Intro\n3. [x] task\n", offset: "Intro\n", insert: "\n", want: true},
		{name: "line of text before an item", input: "Intro\n- [ ] a\n", offset: "Intro\n", insert: "Text\n", want: true},
		{name: "two blank lines before an item", input: "Intro\n- [ ] a\n", offset: "Intro\n", insert: "\n\n", want: true},
		{name: "blank line removed before an item", input: "Intro\n\n- [ ] a\n", offset: "Intro\n", delete: 1, want: true},
		{name: "line before an item removed after a blank line", input: "x\n\n[foo]: /url\n- [ ] a\n", offset: "x\n\n", delete: 12, want: true},
		{name: "line before an item made blank", input: "Intro\nmore\n- [ ] a\n", offset: "Intro\n", delete: 4, want: true},
		{name: "blank line between items", input: "- [ ] a\n- [ ] b\n", offset: "- [ ] a\n", insert: "\n", want: true},
		{name: "line after the last item", input: "- [ ] a\n", offset: "- [ ] a\n", insert: "x\n", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := strings.Index(tt.input, tt.offset)
			if start < 0 {
				t.Fatalf("%q not in %q", tt.offset, tt.input)
			}
			at := start + len(tt.offset)
			edit := fix.TextEdit{StartOffset: at, EndOffset: at + tt.delete, NewText: tt.insert}
			assert.Equal(t, tt.want, guardFor(t, tt.input).withholds([]fix.TextEdit{edit}))
		})
	}
}

// TestCheckboxGuardPlainBoundaryFixes checks fixes of several edits before a
// plain candidate: the guard reads what all of them leave directly before
// it, not what each leaves alone.
func TestCheckboxGuardPlainBoundaryFixes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		edits []fix.TextEdit
		want  bool // Whether the guard withholds the fix.
	}{
		{
			name: "both blank lines removed by two edits", input: "Intro\n\n\n3. [ ] a\n",
			edits: []fix.TextEdit{{StartOffset: 6, EndOffset: 7}, {StartOffset: 7, EndOffset: 8}}, want: true,
		},
		{
			name: "text joined to one blank line while the other goes", input: "# T\n\n\n3. [ ] a\n",
			edits: []fix.TextEdit{{StartOffset: 4, EndOffset: 4, NewText: "x"}, {StartOffset: 5, EndOffset: 6}}, want: true,
		},
		{
			// The first edit leaves an empty piece at the start of the
			// second blank line, inside the window once it widens.
			name: "a blank line made text and the two after it removed", input: "Intro\n\n\n\n3. [ ] a\n",
			edits: []fix.TextEdit{{StartOffset: 6, EndOffset: 7, NewText: "x\n"}, {StartOffset: 7, EndOffset: 9}}, want: true,
		},
		{
			name: "two of three blank lines removed", input: "Intro\n\n\n\n3. [ ] a\n",
			edits: []fix.TextEdit{{StartOffset: 6, EndOffset: 7}, {StartOffset: 7, EndOffset: 8}}, want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, guardFor(t, tt.input).withholds(tt.edits))
		})
	}
}

// TestHoldsCandidate checks the byte pattern of a candidate line: "[", one
// character and "]", or "[" at the end of the line.
func TestHoldsCandidate(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "[[]", want: true},
		{line: "[ab] [x]", want: true},
		{line: "[\xff]", want: true},
		{line: "a [", want: true},
		{line: "[é]", want: true},
		{line: "[é", want: false},
		{line: "[]", want: false},
		{line: "[x", want: false},
		{line: "[ab]", want: false},
		{line: "", want: false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, holdsCandidate([]byte(tt.line)), "%q", tt.line)
	}
}
