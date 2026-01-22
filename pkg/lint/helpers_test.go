package lint_test

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

func buildTestAST() *mdast.Node {
	// Build a document with various node types.
	doc := mdast.NewNode(mdast.NodeDocument)

	// Heading with level 2.
	heading := mdast.NewNode(mdast.NodeHeading)
	heading.Block = &mdast.BlockAttrs{HeadingLevel: 2}
	mdast.AppendChild(doc, heading)

	// Paragraph.
	para := mdast.NewNode(mdast.NodeParagraph)
	mdast.AppendChild(doc, para)

	// Ordered list.
	orderedList := mdast.NewNode(mdast.NodeList)
	orderedList.Block = &mdast.BlockAttrs{
		List: &mdast.ListAttrs{Ordered: true, Tight: true},
	}
	mdast.AppendChild(doc, orderedList)

	// Unordered list.
	unorderedList := mdast.NewNode(mdast.NodeList)
	unorderedList.Block = &mdast.BlockAttrs{
		List: &mdast.ListAttrs{Ordered: false, Tight: false},
	}
	mdast.AppendChild(doc, unorderedList)

	// Code block.
	codeBlock := mdast.NewNode(mdast.NodeCodeBlock)
	codeBlock.Block = &mdast.BlockAttrs{
		CodeBlock: &mdast.CodeBlockAttrs{Info: "go"},
	}
	mdast.AppendChild(doc, codeBlock)

	// Link.
	link := mdast.NewNode(mdast.NodeLink)
	link.Inline = &mdast.InlineAttrs{
		Link: &mdast.LinkAttrs{Destination: "https://example.com"},
	}
	mdast.AppendChild(para, link)

	// Image.
	image := mdast.NewNode(mdast.NodeImage)
	image.Inline = &mdast.InlineAttrs{
		Link: &mdast.LinkAttrs{Destination: "image.png"},
	}
	mdast.AppendChild(para, image)

	return doc
}

func TestHeadings(t *testing.T) {
	t.Parallel()

	doc := buildTestAST()

	headings := lint.Headings(doc)

	if len(headings) != 1 {
		t.Errorf("expected 1 heading, got %d", len(headings))
	}
}

func TestLists(t *testing.T) {
	t.Parallel()

	doc := buildTestAST()

	lists := lint.Lists(doc)

	if len(lists) != 2 {
		t.Errorf("expected 2 lists, got %d", len(lists))
	}
}

func TestCodeBlocks(t *testing.T) {
	t.Parallel()

	doc := buildTestAST()

	codeBlocks := lint.CodeBlocks(doc)

	if len(codeBlocks) != 1 {
		t.Errorf("expected 1 code block, got %d", len(codeBlocks))
	}
}

func TestLinks(t *testing.T) {
	t.Parallel()

	doc := buildTestAST()

	links := lint.Links(doc)

	if len(links) != 1 {
		t.Errorf("expected 1 link, got %d", len(links))
	}
}

func TestImages(t *testing.T) {
	t.Parallel()

	doc := buildTestAST()

	images := lint.Images(doc)

	if len(images) != 1 {
		t.Errorf("expected 1 image, got %d", len(images))
	}
}

func TestParagraphs(t *testing.T) {
	t.Parallel()

	doc := buildTestAST()

	paragraphs := lint.Paragraphs(doc)

	if len(paragraphs) != 1 {
		t.Errorf("expected 1 paragraph, got %d", len(paragraphs))
	}
}

func TestHeadingLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node *mdast.Node
		want int
	}{
		{
			name: "nil node",
			node: nil,
			want: 0,
		},
		{
			name: "non-heading node",
			node: mdast.NewNode(mdast.NodeParagraph),
			want: 0,
		},
		{
			name: "heading without block attrs",
			node: mdast.NewNode(mdast.NodeHeading),
			want: 0,
		},
		{
			name: "heading level 2",
			node: func() *mdast.Node {
				n := mdast.NewNode(mdast.NodeHeading)
				n.Block = &mdast.BlockAttrs{HeadingLevel: 2}
				return n
			}(),
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.HeadingLevel(tt.node)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsOrderedList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node *mdast.Node
		want bool
	}{
		{
			name: "nil node",
			node: nil,
			want: false,
		},
		{
			name: "non-list node",
			node: mdast.NewNode(mdast.NodeParagraph),
			want: false,
		},
		{
			name: "ordered list",
			node: func() *mdast.Node {
				n := mdast.NewNode(mdast.NodeList)
				n.Block = &mdast.BlockAttrs{List: &mdast.ListAttrs{Ordered: true}}
				return n
			}(),
			want: true,
		},
		{
			name: "unordered list",
			node: func() *mdast.Node {
				n := mdast.NewNode(mdast.NodeList)
				n.Block = &mdast.BlockAttrs{List: &mdast.ListAttrs{Ordered: false}}
				return n
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.IsOrderedList(tt.node)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTightList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node *mdast.Node
		want bool
	}{
		{
			name: "nil node",
			node: nil,
			want: false,
		},
		{
			name: "tight list",
			node: func() *mdast.Node {
				n := mdast.NewNode(mdast.NodeList)
				n.Block = &mdast.BlockAttrs{List: &mdast.ListAttrs{Tight: true}}
				return n
			}(),
			want: true,
		},
		{
			name: "loose list",
			node: func() *mdast.Node {
				n := mdast.NewNode(mdast.NodeList)
				n.Block = &mdast.BlockAttrs{List: &mdast.ListAttrs{Tight: false}}
				return n
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.IsTightList(tt.node)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodeBlockInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node *mdast.Node
		want string
	}{
		{
			name: "nil node",
			node: nil,
			want: "",
		},
		{
			name: "code block with info",
			node: func() *mdast.Node {
				n := mdast.NewNode(mdast.NodeCodeBlock)
				n.Block = &mdast.BlockAttrs{CodeBlock: &mdast.CodeBlockAttrs{Info: "python"}}
				return n
			}(),
			want: "python",
		},
		{
			name: "code block without info",
			node: func() *mdast.Node {
				n := mdast.NewNode(mdast.NodeCodeBlock)
				n.Block = &mdast.BlockAttrs{CodeBlock: &mdast.CodeBlockAttrs{}}
				return n
			}(),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.CodeBlockInfo(tt.node)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLinkDestination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node *mdast.Node
		want string
	}{
		{
			name: "nil node",
			node: nil,
			want: "",
		},
		{
			name: "link with destination",
			node: func() *mdast.Node {
				n := mdast.NewNode(mdast.NodeLink)
				n.Inline = &mdast.InlineAttrs{Link: &mdast.LinkAttrs{Destination: "https://example.com"}}
				return n
			}(),
			want: "https://example.com",
		},
		{
			name: "node without inline attrs",
			node: mdast.NewNode(mdast.NodeLink),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.LinkDestination(tt.node)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLineContent(t *testing.T) {
	t.Parallel()

	content := []byte("line1\nline2\nline3")
	file := &mdast.FileSnapshot{
		Content: content,
		Lines:   mdast.BuildLines(content),
	}

	tests := []struct {
		name    string
		lineNum int
		want    string
	}{
		{"line 1", 1, "line1"},
		{"line 2", 2, "line2"},
		{"line 3", 3, "line3"},
		{"line 0 (invalid)", 0, ""},
		{"line 4 (invalid)", 4, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.LineContent(file, tt.lineNum)
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestLineContent_NilFile(t *testing.T) {
	t.Parallel()

	got := lint.LineContent(nil, 1)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestLineLength(t *testing.T) {
	t.Parallel()

	content := []byte("short\nlonger line\n")
	file := &mdast.FileSnapshot{
		Content: content,
		Lines:   mdast.BuildLines(content),
	}

	tests := []struct {
		name    string
		lineNum int
		want    int
	}{
		{"line 1", 1, 5},
		{"line 2", 2, 11},
		{"invalid line", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.LineLength(file, tt.lineNum)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHasTrailingWhitespace(t *testing.T) {
	t.Parallel()

	content := []byte("no trailing\nwith space \nwith tab\t\n")
	file := &mdast.FileSnapshot{
		Content: content,
		Lines:   mdast.BuildLines(content),
	}

	tests := []struct {
		name    string
		lineNum int
		want    bool
	}{
		{"no trailing", 1, false},
		{"with space", 2, true},
		{"with tab", 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.HasTrailingWhitespace(file, tt.lineNum)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrailingWhitespaceRange(t *testing.T) {
	t.Parallel()

	content := []byte("no trailing\nwith space  \nwith tab\t\n")
	file := &mdast.FileSnapshot{
		Content: content,
		Lines:   mdast.BuildLines(content),
	}

	tests := []struct {
		name      string
		lineNum   int
		wantStart int
		wantEnd   int
	}{
		{"no trailing", 1, -1, -1},
		{"with space", 2, 22, 24},
		{"with tab", 3, 33, 34},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			start, end := lint.TrailingWhitespaceRange(file, tt.lineNum)
			if start != tt.wantStart || end != tt.wantEnd {
				t.Errorf("got [%d:%d], want [%d:%d]", start, end, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

func TestIsBlankLine(t *testing.T) {
	t.Parallel()

	content := []byte("content\n\n   \n\t\n")
	file := &mdast.FileSnapshot{
		Content: content,
		Lines:   mdast.BuildLines(content),
	}

	tests := []struct {
		name    string
		lineNum int
		want    bool
	}{
		{"content line", 1, false},
		{"empty line", 2, true},
		{"spaces only", 3, true},
		{"tab only", 4, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.IsBlankLine(file, tt.lineNum)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
