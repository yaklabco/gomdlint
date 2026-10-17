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
				{mdast.NodeHeading, 0, "## Title ##  ", "Title "},
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

func TestLineBreaksFollowTheirText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  mdast.NodeKind
		brk   string
	}{
		{"soft break", "multi\nline\n", mdast.NodeSoftBreak, "\n"},
		{"soft break after trailing space", "multi \nline\n", mdast.NodeSoftBreak, " \n"},
		{"hard break with spaces", "multi  \nline\n", mdast.NodeHardBreak, "  \n"},
		{"hard break with backslash", "multi\\\nline\n", mdast.NodeHardBreak, "\\\n"},
		{"soft break with CRLF", "multi\r\nline\r\n", mdast.NodeSoftBreak, "\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseForSpans(t, FlavorCommonMark, tt.input)
			para := nthNode(file.Root, mdast.NodeParagraph, 0)
			require.NotNil(t, para)
			children := para.Children()
			require.Len(t, children, 3)
			require.Equal(t, mdast.NodeText, children[0].Kind)
			require.Equal(t, "multi", string(children[0].Inline.Text))
			require.Equal(t, "multi", string(children[0].Text()))
			require.Equal(t, tt.kind, children[1].Kind)
			require.Equal(t, tt.brk, string(children[1].Text()))
			require.Empty(t, children[1].Inline.Text, "a break holds no text")
			require.Equal(t, mdast.NodeText, children[2].Kind)
			require.Equal(t, "line", string(children[2].Inline.Text))
		})
	}
}

func TestLineBreakAfterInlineNode(t *testing.T) {
	file := parseForSpans(t, FlavorCommonMark, "*a*\nb\n")
	para := nthNode(file.Root, mdast.NodeParagraph, 0)
	require.NotNil(t, para)
	kinds := make([]mdast.NodeKind, 0, 3)
	for _, child := range para.Children() {
		kinds = append(kinds, child.Kind)
	}
	require.Equal(t, []mdast.NodeKind{mdast.NodeEmphasis, mdast.NodeSoftBreak, mdast.NodeText}, kinds)
	require.Equal(t, "\n", string(para.Children()[1].Text()))
}

func TestResidualDelimitersFollowTheirEmphasis(t *testing.T) {
	// goldmark leaves the unused characters of a closing delimiter run
	// pointing at the start of the run; they belong after the closer.
	tests := []struct {
		input    string
		emphasis string
		residual string
		start    int
	}{
		{"*foo**\n", "*foo*", "*", 5},
		{"**foo***\n", "**foo**", "*", 7},
		{"*foo****\n", "*foo*", "***", 5},
		{"__foo___\n", "__foo__", "_", 7},
		{"_foo____\n", "_foo_", "___", 5},
		{"a *b** c\n", "*b*", "*", 5},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			file := parseForSpans(t, FlavorCommonMark, tt.input)
			para := nthNode(file.Root, mdast.NodeParagraph, 0)
			require.NotNil(t, para)
			var emphasis, residual *mdast.Node
			for child := para.FirstChild; child != nil; child = child.Next {
				if child.Kind == mdast.NodeEmphasis || child.Kind == mdast.NodeStrong {
					emphasis = child
					residual = child.Next
					break
				}
			}
			require.NotNil(t, emphasis)
			require.Equal(t, tt.emphasis, string(emphasis.Text()))
			require.NotNil(t, residual)
			require.Equal(t, mdast.NodeText, residual.Kind)
			require.Equal(t, tt.residual, string(residual.Text()))
			require.Equal(t, tt.start, residual.SourceRange().StartOffset)
			require.LessOrEqual(t, emphasis.SourceRange().EndOffset, residual.SourceRange().StartOffset)
		})
	}
}

func TestLinkReferenceStyles(t *testing.T) {
	input := "[inline](https://x.y) [full][Ref] [ref][] [ref] [multi\nline][] ![img][ref] <https://a.b>\n\n" +
		"[ref]: https://x.y\n[multi line]: https://x.y\n"
	file := parseForSpans(t, FlavorCommonMark, input)
	var links []*mdast.Node
	//nolint:errcheck // The visitor never returns an error.
	mdast.Walk(file.Root, func(node *mdast.Node) error {
		if node.Kind == mdast.NodeLink || node.Kind == mdast.NodeImage {
			links = append(links, node)
		}
		return nil
	})
	want := []struct {
		style mdast.ReferenceStyle
		label string
	}{
		{mdast.RefStyleInline, ""},
		{mdast.RefStyleFull, "Ref"},
		{mdast.RefStyleCollapsed, "ref"},
		{mdast.RefStyleShortcut, "ref"},
		{mdast.RefStyleCollapsed, "multi\nline"},
		{mdast.RefStyleFull, "ref"},
		{mdast.RefStyleAutolink, ""},
	}
	require.Len(t, links, len(want))
	for idx, link := range links {
		require.Equal(t, want[idx].style, link.Inline.Link.ReferenceStyle, "link %d %q", idx, link.Text())
		require.Equal(t, want[idx].label, link.Inline.Link.ReferenceLabel, "link %d %q", idx, link.Text())
	}
}

func TestTightListItemsHoldParagraphs(t *testing.T) {
	file := parseForSpans(t, FlavorCommonMark, "- a\n- b\n")
	assertSpans(t, file, []spanExpectation{
		{mdast.NodeParagraph, 0, "a", "a"},
		{mdast.NodeParagraph, 1, "b", "b"},
	})
	require.Empty(t, mdast.FindByKind(file.Root, mdast.NodeRaw))

	var blocks []mdast.NodeKind
	//nolint:errcheck // The visitor never returns an error.
	mdast.WalkBlocks(file.Root, func(node *mdast.Node) error {
		blocks = append(blocks, node.Kind)
		return nil
	})
	require.Equal(t, []mdast.NodeKind{
		mdast.NodeDocument, mdast.NodeList,
		mdast.NodeListItem, mdast.NodeParagraph,
		mdast.NodeListItem, mdast.NodeParagraph,
	}, blocks)
}

func TestReferenceDefinitionsLeaveNoEmptyNodes(t *testing.T) {
	file := parseForSpans(t, FlavorCommonMark, "- [a]: https://example.com\n- b\n\n[x]: https://x.y\n\nText\n")
	require.Empty(t, mdast.FindByKind(file.Root, mdast.NodeRaw))
	item := nthNode(file.Root, mdast.NodeListItem, 0)
	require.Nil(t, item.FirstChild, "a list item holding only a definition has no children")
	require.Equal(t, "- [a]: https://example.com", string(item.Text()))
	kinds := make([]mdast.NodeKind, 0, 2)
	for child := file.Root.FirstChild; child != nil; child = child.Next {
		kinds = append(kinds, child.Kind)
	}
	require.Equal(t, []mdast.NodeKind{mdast.NodeList, mdast.NodeParagraph}, kinds)
}

func TestTableNodes(t *testing.T) {
	input := "Lead\n\n| a | b | c |\n| :- | :-: | -: |\n| 1 | 2 |\n\n> | q |\n> |---|\n> | r |\n"
	file := parseForSpans(t, FlavorGFM, input)
	table := "| a | b | c |\n| :- | :-: | -: |\n| 1 | 2 |"
	quoted := "| q |\n> |---|\n> | r |"
	assertSpans(t, file, []spanExpectation{
		{mdast.NodeTable, 0, table, table},
		{mdast.NodeTableRow, 0, "| a | b | c |", "| a | b | c |"},
		{mdast.NodeTableRow, 1, "| 1 | 2 |", "| 1 | 2 |"},
		{mdast.NodeTableCell, 0, "a", "a"},
		{mdast.NodeTableCell, 1, "b", "b"},
		{mdast.NodeTableCell, 3, "1", "1"},
		{mdast.NodeTableCell, 5, "", ""},
		{mdast.NodeTable, 1, quoted, quoted},
	})

	first := nthNode(file.Root, mdast.NodeTable, 0)
	require.True(t, first.IsBlock())
	require.NotNil(t, first.Block.Table)
	require.Equal(t, []mdast.Alignment{mdast.AlignLeft, mdast.AlignCenter, mdast.AlignRight}, first.Block.Table.Alignments)

	header := nthNode(file.Root, mdast.NodeTableRow, 0)
	require.True(t, header.Block.Table.Header)
	require.False(t, nthNode(file.Root, mdast.NodeTableRow, 1).Block.Table.Header)
	require.Equal(t, mdast.AlignRight, nthNode(file.Root, mdast.NodeTableCell, 2).Block.Table.Alignment)
	require.True(t, nthNode(file.Root, mdast.NodeTableCell, 0).Block.Table.Header)
	require.Empty(t, mdast.FindByKind(file.Root, mdast.NodeRaw))
}

func TestTaskCheckboxNodes(t *testing.T) {
	file := parseForSpans(t, FlavorGFM, "- [ ] todo\n- [x] done\n- [X] also\n")
	assertSpans(t, file, []spanExpectation{
		{mdast.NodeTaskCheckbox, 0, "[ ]", "[ ]"},
		{mdast.NodeTaskCheckbox, 1, "[x]", "[x]"},
		{mdast.NodeTaskCheckbox, 2, "[X]", "[X]"},
		{mdast.NodeText, 0, "todo", "todo"},
	})
	for idx, want := range []bool{false, true, true} {
		box := nthNode(file.Root, mdast.NodeTaskCheckbox, idx)
		require.True(t, box.IsInline())
		require.Nil(t, box.Ext)
		require.Equal(t, want, box.Inline.Checked)
		require.Equal(t, mdast.NodeParagraph, box.Parent.Kind)
	}
}

func TestStrikethroughNode(t *testing.T) {
	file := parseForSpans(t, FlavorGFM, "~~Struck~~ and *em* and ~one~\n")
	assertSpans(t, file, []spanExpectation{
		{mdast.NodeStrikethrough, 0, "~~Struck~~", "Struck"},
		{mdast.NodeStrikethrough, 1, "~one~", "one"},
		{mdast.NodeEmphasis, 0, "*em*", "em"},
	})
	require.Len(t, mdast.FindByKind(file.Root, mdast.NodeEmphasis), 1, "strikethrough is not emphasis")
	strike := nthNode(file.Root, mdast.NodeStrikethrough, 0)
	require.True(t, strike.IsInline())
	require.Nil(t, strike.Ext)
}

func TestQuotedMultilineReferenceLabels(t *testing.T) {
	// Labels that span lines inside a container do not include the
	// container prefix of their continuation lines.
	tests := []struct {
		input string
		style mdast.ReferenceStyle
		label string
	}{
		{"> [a\n> b][]\n\n[a b]: /u\n", mdast.RefStyleCollapsed, "a\nb"},
		{"> [a\n> b]\n\n[a b]: /u\n", mdast.RefStyleShortcut, "a\nb"},
		{"> [text][a\n> b]\n\n[a b]: /u\n", mdast.RefStyleFull, "a\nb"},
		{"- [x *y*\n  z][]\n\n[x *y* z]: /u\n", mdast.RefStyleCollapsed, "x *y*\nz"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			file := parseForSpans(t, FlavorCommonMark, tt.input)
			link := nthNode(file.Root, mdast.NodeLink, 0)
			require.NotNil(t, link)
			require.Equal(t, tt.style, link.Inline.Link.ReferenceStyle)
			require.Equal(t, tt.label, link.Inline.Link.ReferenceLabel)
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

func TestLeadingByteOrderMark(t *testing.T) {
	t.Run("heading", func(t *testing.T) {
		file := parseForSpans(t, FlavorGFM, utf8BOM+"# Title\n\nText\n")
		require.Equal(t, mdast.NodeHeading, file.Root.FirstChild.Kind)
		require.Equal(t, 3, file.Root.FirstChild.SourceRange().StartOffset, "offsets keep counting the BOM")
		assertSpans(t, file, []spanExpectation{
			{mdast.NodeHeading, 0, "# Title", "Title"},
			{mdast.NodeParagraph, 0, "Text", "Text"},
		})
	})
	t.Run("front matter", func(t *testing.T) {
		file := parseForSpans(t, FlavorGFM, utf8BOM+"---\ntitle: Example\n---\n# Document\n")
		require.Equal(t, 3, file.FrontMatterLines)
		require.Equal(t, mdast.NodeHeading, file.Root.FirstChild.Kind)
		require.Equal(t, 4, file.Root.FirstChild.SourcePosition().StartLine)
		require.Len(t, mdast.FindByKind(file.Root, mdast.NodeHeading), 1)
	})
	t.Run("indented heading", func(t *testing.T) {
		file := parseForSpans(t, FlavorGFM, utf8BOM+"   # Title\n")
		require.Equal(t, mdast.NodeHeading, file.Root.FirstChild.Kind)
	})
	t.Run("tab indented code", func(t *testing.T) {
		file := parseForSpans(t, FlavorGFM, utf8BOM+"\tcode\n")
		require.Equal(t, mdast.NodeCodeBlock, file.Root.FirstChild.Kind)
		require.Equal(t, "code", string(file.Root.FirstChild.ContentText()))
	})
	t.Run("only a BOM", func(t *testing.T) {
		file := parseForSpans(t, FlavorGFM, utf8BOM)
		require.Nil(t, file.Root.FirstChild)
	})
}

func TestOrderedListDelimiter(t *testing.T) {
	tests := []struct {
		input     string
		delimiter string
	}{
		{"1. one\n2. two\n", "."},
		{"1) one\n1) two\n3) three\n", ")"},
		{"> 7) quoted\n", ")"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			file := parseForSpans(t, FlavorCommonMark, tt.input)
			list := nthNode(file.Root, mdast.NodeList, 0)
			require.NotNil(t, list)
			require.True(t, list.Block.List.Ordered)
			require.Equal(t, tt.delimiter, list.Block.List.Delimiter)
			require.Empty(t, list.Block.List.BulletMarker)
		})
	}
}

func TestListItemContentIndent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		indent int
	}{
		{"bullet", "- a\n", 2},
		{"ordered with two spaces", "1.  b\n", 4},
		{"indented marker", "   - c\n", 5},
		{"content that is indented code", "-     code\n", 2},
		{"empty item", "-\n", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseForSpans(t, FlavorCommonMark, tt.input)
			item := nthNode(file.Root, mdast.NodeListItem, 0)
			require.NotNil(t, item)
			require.NotNil(t, item.Block)
			require.NotNil(t, item.Block.ListItem)
			require.Equal(t, tt.indent, item.Block.ListItem.ContentIndent)
		})
	}
}

// TestListItemContentStartsAfterItsMarker checks that a list item's content
// starts where goldmark found it on the item's first line. goldmark measures
// a tab after the marker from where the item starts, so ContentIndent alone
// does not locate the content when the item does not start on a tab stop.
func TestListItemContentStartsAfterItsMarker(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		index   int
		full    string
		content string
		indent  int
	}{
		{"tab after a bullet in a blockquote", "> -\t> # H\n", 0, "-\t> # H", "> # H", 4},
		{"tab after an ordered marker in a blockquote", "> 1.\t> # H\n", 0, "1.\t> # H", "> # H", 4},
		{"tab after a nested bullet", "- -\t# H\n", 1, "-\t# H", "# H", 4},
		{"definition only", "- [a]: https://example.com\n", 0, "- [a]: https://example.com", "[a]: https://example.com", 2},
		{"definition before a paragraph", "- [a]: /u\n  text\n", 0, "- [a]: /u\n  text", "[a]: /u\n  text", 2},
		{"first line holds only the marker", "-\n  text\n", 0, "-\n  text", "text", 2},
		{"indented code after spaces", "-     code\n", 0, "-     code", "    code", 2},
		{"tab claimed in part before indented code", "-\t\tcode\n", 0, "-\t\tcode", "\t\tcode", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseForSpans(t, FlavorCommonMark, tt.input)
			item := nthNode(file.Root, mdast.NodeListItem, tt.index)
			require.NotNil(t, item)
			require.Equal(t, tt.full, string(item.Text()))
			require.Equal(t, tt.content, string(item.ContentText()))
			require.Equal(t, tt.indent, item.Block.ListItem.ContentIndent)
		})
	}
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
