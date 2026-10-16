package goldmark

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// spanExpectation describes the full and content span of the nth node of a
// kind, counted in document (pre-order) order.
type spanExpectation struct {
	kind    mdast.NodeKind
	index   int
	full    string
	content string
}

// nthNode returns the nth node of the given kind in pre-order, or nil.
func nthNode(root *mdast.Node, kind mdast.NodeKind, index int) *mdast.Node {
	nodes := mdast.FindByKind(root, kind)
	if index >= len(nodes) {
		return nil
	}
	return nodes[index]
}

func parseForSpans(t *testing.T, flavor, input string) *mdast.FileSnapshot {
	t.Helper()
	file, err := New(flavor).Parse(context.Background(), "spans.md", []byte(input))
	require.NoError(t, err)
	return file
}

func assertSpans(t *testing.T, file *mdast.FileSnapshot, expectations []spanExpectation) {
	t.Helper()
	for _, want := range expectations {
		node := nthNode(file.Root, want.kind, want.index)
		require.NotNil(t, node, "no %s #%d", want.kind, want.index)
		require.Equal(t, want.full, string(node.Text()), "full span of %s #%d", want.kind, want.index)
		require.Equal(t, want.content, string(node.ContentText()), "content span of %s #%d", want.kind, want.index)
	}
}

func TestBlockSpans(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []spanExpectation
	}{
		{
			name:  "atx heading",
			input: "# Title\n",
			expect: []spanExpectation{
				{mdast.NodeHeading, 0, "# Title", "Title"},
			},
		},
		{
			name:  "closed atx heading with trailing spaces",
			input: "## Title ##  \n",
			expect: []spanExpectation{
				{mdast.NodeHeading, 0, "## Title ##  ", "Title"},
			},
		},
		{
			name:  "indented atx heading",
			input: "   # Title\n",
			expect: []spanExpectation{
				{mdast.NodeHeading, 0, "# Title", "Title"},
			},
		},
		{
			name:  "empty atx heading",
			input: "#\n\nText\n",
			expect: []spanExpectation{
				{mdast.NodeHeading, 0, "#", ""},
			},
		},
		{
			name:  "setext heading",
			input: "Setext\n======\n\nText\n",
			expect: []spanExpectation{
				{mdast.NodeHeading, 0, "Setext\n======", "Setext"},
			},
		},
		{
			name:  "multi-line setext heading",
			input: "First\nsecond line\n-----\n",
			expect: []spanExpectation{
				{mdast.NodeHeading, 0, "First\nsecond line\n-----", "First\nsecond line"},
			},
		},
		{
			name:  "fenced code block",
			input: "```js\ncode\n```\n",
			expect: []spanExpectation{
				{mdast.NodeCodeBlock, 0, "```js\ncode\n```", "code"},
			},
		},
		{
			name:  "tilde fence in blockquote",
			input: "> ~~~text\n> x\n> ~~~\n",
			expect: []spanExpectation{
				{mdast.NodeBlockquote, 0, "> ~~~text\n> x\n> ~~~", "~~~text\n> x\n> ~~~"},
				{mdast.NodeCodeBlock, 0, "~~~text\n> x\n> ~~~", "x"},
			},
		},
		{
			name:  "empty fenced code block",
			input: "````\n````\n",
			expect: []spanExpectation{
				{mdast.NodeCodeBlock, 0, "````\n````", ""},
			},
		},
		{
			name:  "unclosed fenced code block",
			input: "```\ncode\n",
			expect: []spanExpectation{
				{mdast.NodeCodeBlock, 0, "```\ncode", "code"},
			},
		},
		{
			name:  "fenced code block in list item",
			input: "- item\n\n  ```sh\n  ls\n  ```\n",
			expect: []spanExpectation{
				{mdast.NodeCodeBlock, 0, "```sh\n  ls\n  ```", "ls"},
				{mdast.NodeListItem, 0, "- item\n\n  ```sh\n  ls\n  ```", "item\n\n  ```sh\n  ls\n  ```"},
			},
		},
		{
			name:  "indented code block",
			input: "Para\n\n    code1\n    code2\n\nAfter\n",
			expect: []spanExpectation{
				{mdast.NodeCodeBlock, 0, "    code1\n    code2", "code1\n    code2"},
			},
		},
		{
			name:  "thematic breaks",
			input: "Para\n\n***\n\n  - - -\n",
			expect: []spanExpectation{
				{mdast.NodeThematicBreak, 0, "***", "***"},
				{mdast.NodeThematicBreak, 1, "- - -", "- - -"},
			},
		},
		{
			name:  "html block ending at blank line",
			input: "<div>\nx\n</div>\n\nPara\n",
			expect: []spanExpectation{
				{mdast.NodeHTMLBlock, 0, "<div>\nx\n</div>", "<div>\nx\n</div>"},
				{mdast.NodeParagraph, 0, "Para", "Para"},
			},
		},
		{
			name:  "html comment block with closing line",
			input: "<!-- a\nb -->\nPara\n",
			expect: []spanExpectation{
				{mdast.NodeHTMLBlock, 0, "<!-- a\nb -->", "<!-- a\nb -->"},
				{mdast.NodeParagraph, 0, "Para", "Para"},
			},
		},
		{
			name:  "blockquote with blank marker line",
			input: "> a\n>\n> b\n",
			expect: []spanExpectation{
				{mdast.NodeBlockquote, 0, "> a\n>\n> b", "a\n>\n> b"},
				{mdast.NodeParagraph, 0, "a", "a"},
				{mdast.NodeParagraph, 1, "b", "b"},
			},
		},
		{
			name:  "lazy paragraph continuation",
			input: "> a\nb\n",
			expect: []spanExpectation{
				{mdast.NodeBlockquote, 0, "> a\nb", "a\nb"},
				{mdast.NodeParagraph, 0, "a\nb", "a\nb"},
			},
		},
		{
			name:  "tight bullet list",
			input: "- a\n- b\n",
			expect: []spanExpectation{
				{mdast.NodeList, 0, "- a\n- b", "- a\n- b"},
				{mdast.NodeListItem, 0, "- a", "a"},
				{mdast.NodeListItem, 1, "- b", "b"},
			},
		},
		{
			name:  "empty list item",
			input: "-\n- b\n",
			expect: []spanExpectation{
				{mdast.NodeListItem, 0, "-", ""},
				{mdast.NodeListItem, 1, "- b", "b"},
			},
		},
		{
			name:  "ordered list",
			input: "1. one\n2. two\n",
			expect: []spanExpectation{
				{mdast.NodeList, 0, "1. one\n2. two", "1. one\n2. two"},
				{mdast.NodeListItem, 1, "2. two", "two"},
			},
		},
		{
			name:  "loose list excludes trailing blank lines",
			input: "- a\n\n- b\n\nPara\n",
			expect: []spanExpectation{
				{mdast.NodeList, 0, "- a\n\n- b", "- a\n\n- b"},
				{mdast.NodeListItem, 0, "- a", "a"},
				{mdast.NodeParagraph, 2, "Para", "Para"},
			},
		},
		{
			name:  "nested list",
			input: "- a\n  - b\n",
			expect: []spanExpectation{
				{mdast.NodeListItem, 0, "- a\n  - b", "a\n  - b"},
				{mdast.NodeList, 1, "- b", "- b"},
			},
		},
		{
			name:  "heading in list item",
			input: "- # H\n",
			expect: []spanExpectation{
				{mdast.NodeHeading, 0, "# H", "H"},
				{mdast.NodeListItem, 0, "- # H", "# H"},
			},
		},
		{
			name:  "paragraph with trailing spaces",
			input: "text  \n",
			expect: []spanExpectation{
				{mdast.NodeParagraph, 0, "text  ", "text"},
			},
		},
		{
			name:  "document",
			input: "# A\n\nB\n",
			expect: []spanExpectation{
				{mdast.NodeDocument, 0, "# A\n\nB\n", "# A\n\nB\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSpans(t, parseForSpans(t, FlavorCommonMark, tt.input), tt.expect)
		})
	}
}

func TestInlineSpans(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []spanExpectation
	}{
		{
			name:  "emphasis and strong",
			input: "a *b* and **c** d\n",
			expect: []spanExpectation{
				{mdast.NodeEmphasis, 0, "*b*", "b"},
				{mdast.NodeStrong, 0, "**c**", "c"},
			},
		},
		{
			name:  "nested emphasis",
			input: "***a***\n",
			expect: []spanExpectation{
				{mdast.NodeEmphasis, 0, "***a***", "**a**"},
				{mdast.NodeStrong, 0, "**a**", "a"},
			},
		},
		{
			name:  "unbalanced emphasis delimiters",
			input: "**a*\n",
			expect: []spanExpectation{
				{mdast.NodeEmphasis, 0, "*a*", "a"},
			},
		},
		{
			name:  "code span",
			input: "x `` a`b `` y\n",
			expect: []spanExpectation{
				{mdast.NodeCodeSpan, 0, "`` a`b ``", " a`b "},
			},
		},
		{
			name:  "inline link and image",
			input: "See [text](https://x.y) and ![alt](i.png).\n",
			expect: []spanExpectation{
				{mdast.NodeLink, 0, "[text](https://x.y)", "text"},
				{mdast.NodeImage, 0, "![alt](i.png)", "alt"},
			},
		},
		{
			name:  "reference links",
			input: "[full][ref], [ref][] and [ref].\n\n[ref]: https://x.y\n",
			expect: []spanExpectation{
				{mdast.NodeLink, 0, "[full][ref]", "full"},
				{mdast.NodeLink, 1, "[ref][]", "ref"},
				{mdast.NodeLink, 2, "[ref]", "ref"},
			},
		},
		{
			name:  "empty link label",
			input: "[](https://x.y)\n",
			expect: []spanExpectation{
				{mdast.NodeLink, 0, "[](https://x.y)", ""},
			},
		},
		{
			name:  "angle bracket autolink",
			input: "Go to <https://x.y> now.\n",
			expect: []spanExpectation{
				{mdast.NodeLink, 0, "<https://x.y>", "https://x.y"},
			},
		},
		{
			name:  "inline html",
			input: "a <b>c</b> d\n",
			expect: []spanExpectation{
				{mdast.NodeHTMLInline, 0, "<b>", "<b>"},
				{mdast.NodeHTMLInline, 1, "</b>", "</b>"},
			},
		},
		{
			name:  "text inside heading",
			input: "## Title text\n",
			expect: []spanExpectation{
				{mdast.NodeText, 0, "Title text", "Title text"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSpans(t, parseForSpans(t, FlavorCommonMark, tt.input), tt.expect)
		})
	}
}

func TestExtendedAutolinkSpans(t *testing.T) {
	input := "Visit www.example.com, (https://example.org/path) or me@example.com.\n"
	assertSpans(t, parseForSpans(t, FlavorGFM, input), []spanExpectation{
		{mdast.NodeLink, 0, "www.example.com", "www.example.com"},
		{mdast.NodeLink, 1, "https://example.org/path", "https://example.org/path"},
		{mdast.NodeLink, 2, "me@example.com", "me@example.com"},
	})
}

func TestSpansSurviveContainers(t *testing.T) {
	input := "> - item one\n>   continued\n> - item two\n>\n>   para\n"
	file := parseForSpans(t, FlavorCommonMark, input)
	assertSpans(t, file, []spanExpectation{
		{mdast.NodeBlockquote, 0, "> - item one\n>   continued\n> - item two\n>\n>   para", "- item one\n>   continued\n> - item two\n>\n>   para"},
		{mdast.NodeList, 0, "- item one\n>   continued\n> - item two\n>\n>   para", "- item one\n>   continued\n> - item two\n>\n>   para"},
		{mdast.NodeListItem, 0, "- item one\n>   continued", "item one\n>   continued"},
		{mdast.NodeListItem, 1, "- item two\n>\n>   para", "item two\n>\n>   para"},
	})
}

func TestCodeBlockFenceAttributes(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		char   byte
		length int
		info   string
		closed bool
	}{
		{"backticks", "```go\nx\n```\n", '`', 3, "go", true},
		{"tildes in blockquote", "> ~~~text\n> x\n> ~~~\n", '~', 3, "text", true},
		{"empty tilde fence", "~~~~\n~~~~\n", '~', 4, "", true},
		{"empty backtick fence with info", "`````python\n`````\n", '`', 5, "python", true},
		{"indented fence in list", "- item\n\n    ~~~\n    x\n    ~~~\n", '~', 3, "", true},
		{"unclosed fence", "```sh\nls\n", '`', 3, "sh", false},
		{"shorter closing run does not close", "````\n```\n", '`', 4, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseForSpans(t, FlavorCommonMark, tt.input)
			blocks := mdast.FindByKind(file.Root, mdast.NodeCodeBlock)
			require.Len(t, blocks, 1)
			attrs := blocks[0].Block.CodeBlock
			require.NotNil(t, attrs)
			require.False(t, attrs.Indented)
			require.Equal(t, string(tt.char), string(attrs.FenceChar))
			require.Equal(t, tt.length, attrs.FenceLength)
			require.Equal(t, tt.info, attrs.Info)
			require.Equal(t, tt.closed, attrs.Closed)
		})
	}
}
