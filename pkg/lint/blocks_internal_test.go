package lint

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func blocksOf(t *testing.T, content string) *topLevelBlocks {
	t.Helper()

	snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(content))
	require.NoError(t, err)
	return newTopLevelBlocks(snapshot)
}

func TestTopLevelBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "blocks separated by blank lines",
			content: "# A\n\npara\n",
			want:    []string{"# A\n", "para\n"},
		},
		{
			name:    "adjacent blocks count as one",
			content: "# A\npara\n",
			want:    []string{"# A\npara\n"},
		},
		{
			// The full span of the code block runs from fence to fence.
			name:    "a code block with blank lines around its content",
			content: "~~~\n\n$ x\n\n~~~\n\nafter\n",
			want:    []string{"~~~\n\n$ x\n\n~~~\n", "after\n"},
		},
		{
			// The full span of the HTML block includes its closing line.
			name:    "an HTML block whose closing line follows a blank line",
			content: "<!--\n\nx\n\n-->\n\nafter\n",
			want:    []string{"<!--\n\nx\n\n-->\n", "after\n"},
		},
		{
			name:    "a loose list",
			content: "- a\n\n- b\n\npara\n",
			want:    []string{"- a\n\n- b\n", "para\n"},
		},
		{
			name:    "a list item that starts with a code block",
			content: "- ```\n\n  code\n\n  ```\n- b\n\nafter\n",
			want:    []string{"- ```\n\n  code\n\n  ```\n- b\n", "after\n"},
		},
		{
			name:    "a paragraph that continues a list item after a blank line",
			content: "- a\n\n  b\n\npara\n",
			want:    []string{"- a\n\n  b\n", "para\n"},
		},
		{
			// Two lists now, but narrowing "1.   " would make the bullet a
			// child of the ordered item.
			name:    "an indented list after a blank line",
			content: "1.   two\n\n  * three\n",
			want:    []string{"1.   two\n\n  * three\n"},
		},
		{
			// The item is empty and "foo" is a paragraph after the list.
			name:    "a list item that starts with a blank line",
			content: "-\n\n  foo\n",
			want:    []string{"-\n\n  foo\n"},
		},
		{
			name:    "indented code after a paragraph",
			content: "para\n\n    code\n\nafter\n",
			want:    []string{"para\n\n    code\n", "after\n"},
		},
		{
			name:    "indented code after a list",
			content: "- a\n\n    code\n\nafter\n",
			want:    []string{"- a\n\n    code\n", "after\n"},
		},
		{
			name:    "a line indented with a tab",
			content: "para\n\n\tcode\n\nafter\n",
			want:    []string{"para\n\n\tcode\n", "after\n"},
		},
		{
			name:    "an unindented block after a list",
			content: "- a\n\npara\n",
			want:    []string{"- a\n", "para\n"},
		},
		{
			// A line holding only ">" is not blank.
			name:    "a blockquote with a quoted blank line",
			content: "> a\n>\n> b\n\nafter\n",
			want:    []string{"> a\n>\n> b\n", "after\n"},
		},
		{
			name:    "blockquotes separated by a blank line",
			content: "> a\n\n> b\n",
			want:    []string{"> a\n", "> b\n"},
		},
		{
			name:    "an indented line after front matter",
			content: "---\na: b\n---\n\n    code\n",
			want:    []string{"---\na: b\n---\n", "    code\n"},
		},
		{
			name:    "a byte order mark before an indented first line",
			content: "\xEF\xBB\xBF  - a\n\npara\n",
			want:    []string{"\xEF\xBB\xBF  - a\n", "para\n"},
		},
		{
			name:    "front matter next to a heading",
			content: "---\ntitle: x\n---\n# H\n",
			want:    []string{"---\ntitle: x\n---\n", "# H\n"},
		},
		{
			name:    "a code block right after front matter",
			content: "---\na: b\n---\n```\n\nx\n```\n",
			want:    []string{"---\na: b\n---\n", "```\n\nx\n```\n"},
		},
		{
			name:    "an unclosed code block runs to the end of the file",
			content: "para\n\n```\ncode\n\nmore\n",
			want:    []string{"para\n", "```\ncode\n\nmore\n"},
		},
		{
			name:    "a byte order mark before a heading",
			content: "\xEF\xBB\xBF# A\n\npara\n",
			want:    []string{"\xEF\xBB\xBF# A\n", "para\n"},
		},
		{
			// The parser skips the mark, so the line is blank.
			name:    "a byte order mark on a line of its own",
			content: "\xEF\xBB\xBF\n# A\n\npara\n",
			want:    []string{"# A\n", "para\n"},
		},
		{
			name:    "blank lines at the start and the end",
			content: "\n\n# A\n\n\n",
			want:    []string{"# A\n"},
		},
		{
			name:    "no final line ending",
			content: "# A\n\npara",
			want:    []string{"# A\n", "para"},
		},
		{
			name:    "only blank lines",
			content: "\n \t\n",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			blocks := blocksOf(t, tt.content)
			var got []string
			for idx := range blocks.starts {
				got = append(got, tt.content[blocks.starts[idx]:blocks.ends[idx]])
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTopLevelBlocks_TouchedBy(t *testing.T) {
	t.Parallel()

	// Blocks: "# A\n" [0,4), "para one\n" [6,15), "para two\n" [16,25).
	const content = "# A\n\n\npara one\n\npara two\n"
	blocks := blocksOf(t, content)
	require.Equal(t, []int{0, 6, 16}, blocks.starts)
	require.Equal(t, []int{4, 15, 25}, blocks.ends)

	tests := []struct {
		name  string
		edits []fix.TextEdit
		want  []int
	}{
		{"an edit inside a block", []fix.TextEdit{{StartOffset: 7, EndOffset: 8, NewText: "x"}}, []int{1}},
		{"an insertion inside a block", []fix.TextEdit{{StartOffset: 10, EndOffset: 10, NewText: "x"}}, []int{1}},
		{"an edit that starts a block's first line", []fix.TextEdit{{StartOffset: 6, EndOffset: 7, NewText: "  p"}}, []int{0, 1}},
		{"an edit between two blocks", []fix.TextEdit{{StartOffset: 4, EndOffset: 5}}, []int{0, 1}},
		{"an insertion at the start of a block", []fix.TextEdit{{StartOffset: 6, EndOffset: 6, NewText: "x"}}, []int{0, 1}},
		{"an insertion at the end of a block", []fix.TextEdit{{StartOffset: 15, EndOffset: 15, NewText: "\n"}}, []int{1, 2}},
		{"an edit of the line ending that ends a block", []fix.TextEdit{{StartOffset: 14, EndOffset: 15}}, []int{1, 2}},
		{"an edit across two blocks", []fix.TextEdit{{StartOffset: 10, EndOffset: 20}}, []int{1, 2}},
		{"an insertion at the end of the file", []fix.TextEdit{{StartOffset: 25, EndOffset: 25, NewText: "\n"}}, []int{2}},
		{
			name: "a fix with edits in two blocks",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 1, NewText: "#"},
				{StartOffset: 17, EndOffset: 18, NewText: "A"},
			},
			want: []int{0, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, blocks.touchedBy(tt.edits))
		})
	}
}

func TestTopLevelBlocks_TouchedByInTheLeadOfAFirstLine(t *testing.T) {
	t.Parallel()

	// The block after the front matter [0,13) has an indented first line
	// but no block before it to join. Its lead is the indent and any list
	// marker.
	tests := []struct {
		name    string
		content string
		edits   []fix.TextEdit
		want    []int
	}{
		{"an edit of the indent", "---\na: b\n---\n  para\n", []fix.TextEdit{{StartOffset: 13, EndOffset: 14}}, []int{0, 1}},
		{
			name: "an insertion inside the indent", content: "---\na: b\n---\n  para\n",
			edits: []fix.TextEdit{{StartOffset: 14, EndOffset: 14, NewText: " "}}, want: []int{0, 1},
		},
		{
			name: "an insertion right after the indent", content: "---\na: b\n---\n  para\n",
			edits: []fix.TextEdit{{StartOffset: 15, EndOffset: 15, NewText: "- "}}, want: []int{0, 1},
		},
		{
			name: "an edit that starts right after the indent", content: "---\na: b\n---\n  para\n",
			edits: []fix.TextEdit{{StartOffset: 15, EndOffset: 16, NewText: " p"}}, want: []int{0, 1},
		},
		{
			name: "an edit later in the line", content: "---\na: b\n---\n  para\n",
			edits: []fix.TextEdit{{StartOffset: 16, EndOffset: 17, NewText: "A"}}, want: []int{1},
		},
		{
			name: "an edit of an ordered list delimiter", content: "---\na: b\n---\n  12) x\n",
			edits: []fix.TextEdit{{StartOffset: 17, EndOffset: 18, NewText: "."}}, want: []int{0, 1},
		},
		{
			name: "an edit of the space after a list marker", content: "---\na: b\n---\n  12) x\n",
			edits: []fix.TextEdit{{StartOffset: 18, EndOffset: 19, NewText: "  "}}, want: []int{0, 1},
		},
		{
			name: "an edit of a list item's text", content: "---\na: b\n---\n  12) x\n",
			edits: []fix.TextEdit{{StartOffset: 19, EndOffset: 20, NewText: "y"}}, want: []int{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			blocks := blocksOf(t, tt.content)
			require.Equal(t, []int{0, 13}, blocks.starts)
			assert.Equal(t, tt.want, blocks.touchedBy(tt.edits))
		})
	}
}

func TestLeadLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want int
	}{
		{"para", 0},
		{"  para", 2},
		{"\t- item", 2},
		{"- item", 1},
		{"   * item", 4},
		{"+", 1},
		{"1. item", 2},
		{"  123456789) item", 12},
		{"1234567890. not a marker", 0},
		{"12 apples", 0},
		{"#heading", 0},
		{"", 0},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, leadLength([]byte(tt.text)), "%q", tt.text)
	}
}

func TestTopLevelBlocks_TouchedByAtTheEdges(t *testing.T) {
	t.Parallel()

	leading := blocksOf(t, "\n# A\n\n")
	assert.Equal(t, []int{0}, leading.touchedBy([]fix.TextEdit{{StartOffset: 0, EndOffset: 1}}),
		"blank lines before the first block belong to it")
	assert.Equal(t, []int{0}, leading.touchedBy([]fix.TextEdit{{StartOffset: 5, EndOffset: 6}}),
		"blank lines after the last block belong to it")

	empty := blocksOf(t, "\n\n")
	assert.Equal(t, []int{0}, empty.touchedBy([]fix.TextEdit{{StartOffset: 0, EndOffset: 1}}),
		"without blocks, every edit touches one block")
}
