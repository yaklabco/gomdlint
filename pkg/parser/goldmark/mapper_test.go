package goldmark

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func TestMapper_Document(t *testing.T) {
	content := []byte("Hello, world!")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	if doc == nil {
		t.Fatal("expected non-nil document")
	}

	if doc.Kind != mdast.NodeDocument {
		t.Errorf("expected NodeDocument, got %v", doc.Kind)
	}
}

func TestMapper_Heading(t *testing.T) {
	tests := []struct {
		name    string
		content string
		level   int
	}{
		{"h1", "# Heading 1", 1},
		{"h2", "## Heading 2", 2},
		{"h3", "### Heading 3", 3},
		{"h4", "#### Heading 4", 4},
		{"h5", "##### Heading 5", 5},
		{"h6", "###### Heading 6", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)
			mapper := newMapper(content)

			md := goldmark.New()
			reader := text.NewReader(content)
			gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

			doc := mapper.mapDocument(gmDoc)

			// Find heading node.
			headings := mdast.FindByKind(doc, mdast.NodeHeading)
			if len(headings) != 1 {
				t.Fatalf("expected 1 heading, got %d", len(headings))
			}

			heading := headings[0]
			if heading.Block == nil {
				t.Fatal("expected Block attrs")
			}

			if heading.Block.HeadingLevel != tt.level {
				t.Errorf("heading level = %d, want %d", heading.Block.HeadingLevel, tt.level)
			}
		})
	}
}

func TestMapper_Paragraph(t *testing.T) {
	content := []byte("This is a paragraph.")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	paragraphs := mdast.FindByKind(doc, mdast.NodeParagraph)
	if len(paragraphs) != 1 {
		t.Fatalf("expected 1 paragraph, got %d", len(paragraphs))
	}
}

func TestMapper_List(t *testing.T) {
	tests := []struct {
		name    string
		content string
		ordered bool
	}{
		{"unordered dash", "- item 1\n- item 2", false},
		{"unordered asterisk", "* item 1\n* item 2", false},
		{"unordered plus", "+ item 1\n+ item 2", false},
		{"ordered", "1. item 1\n2. item 2", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)
			mapper := newMapper(content)

			md := goldmark.New()
			reader := text.NewReader(content)
			gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

			doc := mapper.mapDocument(gmDoc)

			lists := mdast.FindByKind(doc, mdast.NodeList)
			if len(lists) != 1 {
				t.Fatalf("expected 1 list, got %d", len(lists))
			}

			list := lists[0]
			if list.Block == nil || list.Block.List == nil {
				t.Fatal("expected List attrs")
			}

			if list.Block.List.Ordered != tt.ordered {
				t.Errorf("ordered = %v, want %v", list.Block.List.Ordered, tt.ordered)
			}

			// Check list items.
			items := mdast.FindByKind(list, mdast.NodeListItem)
			if len(items) != 2 {
				t.Errorf("expected 2 list items, got %d", len(items))
			}
		})
	}
}

func TestMapper_Blockquote(t *testing.T) {
	content := []byte("> This is a quote")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	quotes := mdast.FindByKind(doc, mdast.NodeBlockquote)
	if len(quotes) != 1 {
		t.Fatalf("expected 1 blockquote, got %d", len(quotes))
	}
}

func TestMapper_FencedCodeBlock(t *testing.T) {
	content := []byte("```go\nfunc main() {}\n```")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	codeBlocks := mdast.FindByKind(doc, mdast.NodeCodeBlock)
	if len(codeBlocks) != 1 {
		t.Fatalf("expected 1 code block, got %d", len(codeBlocks))
	}

	cb := codeBlocks[0]
	if cb.Block == nil || cb.Block.CodeBlock == nil {
		t.Fatal("expected CodeBlock attrs")
	}

	if cb.Block.CodeBlock.Info != "go" {
		t.Errorf("info = %q, want %q", cb.Block.CodeBlock.Info, "go")
	}

	if cb.Block.CodeBlock.Indented {
		t.Error("expected Indented = false for fenced code block")
	}
}

func TestMapper_IndentedCodeBlock(t *testing.T) {
	content := []byte("    code line 1\n    code line 2")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	codeBlocks := mdast.FindByKind(doc, mdast.NodeCodeBlock)
	if len(codeBlocks) != 1 {
		t.Fatalf("expected 1 code block, got %d", len(codeBlocks))
	}

	cb := codeBlocks[0]
	if cb.Block == nil || cb.Block.CodeBlock == nil {
		t.Fatal("expected CodeBlock attrs")
	}

	if !cb.Block.CodeBlock.Indented {
		t.Error("expected Indented = true for indented code block")
	}
}

func TestMapper_ThematicBreak(t *testing.T) {
	content := []byte("---")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	breaks := mdast.FindByKind(doc, mdast.NodeThematicBreak)
	if len(breaks) != 1 {
		t.Fatalf("expected 1 thematic break, got %d", len(breaks))
	}
}

func TestMapper_Text(t *testing.T) {
	content := []byte("Hello, world!")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	texts := mdast.FindByKind(doc, mdast.NodeText)
	if len(texts) == 0 {
		t.Fatal("expected at least one text node")
	}

	// Check that text content is captured (may be split into multiple nodes).
	var allText []byte
	for _, txt := range texts {
		if txt.Inline != nil {
			allText = append(allText, txt.Inline.Text...)
		}
	}

	if string(allText) != "Hello, world!" {
		t.Errorf("combined text = %q, want %q", allText, "Hello, world!")
	}
}

func TestMapper_Emphasis(t *testing.T) {
	content := []byte("*emphasis*")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	emphasis := mdast.FindByKind(doc, mdast.NodeEmphasis)
	if len(emphasis) != 1 {
		t.Fatalf("expected 1 emphasis node, got %d", len(emphasis))
	}

	em := emphasis[0]
	if em.Inline == nil {
		t.Fatal("expected Inline attrs")
	}

	if em.Inline.EmphasisLevel != 1 {
		t.Errorf("emphasis level = %d, want 1", em.Inline.EmphasisLevel)
	}
}

func TestMapper_Strong(t *testing.T) {
	content := []byte("**strong**")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	strong := mdast.FindByKind(doc, mdast.NodeStrong)
	if len(strong) != 1 {
		t.Fatalf("expected 1 strong node, got %d", len(strong))
	}

	strongNode := strong[0]
	if strongNode.Inline == nil {
		t.Fatal("expected Inline attrs")
	}

	if strongNode.Inline.EmphasisLevel != 2 {
		t.Errorf("emphasis level = %d, want 2", strongNode.Inline.EmphasisLevel)
	}
}

func TestMapper_CodeSpan(t *testing.T) {
	content := []byte("Use `code` here")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	codeSpans := mdast.FindByKind(doc, mdast.NodeCodeSpan)
	if len(codeSpans) != 1 {
		t.Fatalf("expected 1 code span, got %d", len(codeSpans))
	}

	cs := codeSpans[0]
	if cs.Inline == nil {
		t.Fatal("expected Inline attrs")
	}

	if string(cs.Inline.Text) != "code" {
		t.Errorf("code span text = %q, want %q", cs.Inline.Text, "code")
	}
}

func TestMapper_Link(t *testing.T) {
	content := []byte("[text](https://example.com)")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	links := mdast.FindByKind(doc, mdast.NodeLink)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	link := links[0]
	if link.Inline == nil || link.Inline.Link == nil {
		t.Fatal("expected Link attrs")
	}

	if link.Inline.Link.Destination != "https://example.com" {
		t.Errorf("destination = %q, want %q", link.Inline.Link.Destination, "https://example.com")
	}
}

func TestMapper_LinkWithTitle(t *testing.T) {
	content := []byte(`[text](https://example.com "Title")`)
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	links := mdast.FindByKind(doc, mdast.NodeLink)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	link := links[0]
	if link.Inline == nil || link.Inline.Link == nil {
		t.Fatal("expected Link attrs")
	}

	if link.Inline.Link.Title != "Title" {
		t.Errorf("title = %q, want %q", link.Inline.Link.Title, "Title")
	}
}

func TestMapper_Image(t *testing.T) {
	content := []byte("![alt text](image.png)")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	images := mdast.FindByKind(doc, mdast.NodeImage)
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}

	img := images[0]
	if img.Inline == nil || img.Inline.Link == nil {
		t.Fatal("expected Link attrs for image")
	}

	if img.Inline.Link.Destination != "image.png" {
		t.Errorf("destination = %q, want %q", img.Inline.Link.Destination, "image.png")
	}
}

func TestMapper_NestedStructure(t *testing.T) {
	content := []byte(`# Heading

Paragraph with *emphasis* and **strong**.

- Item 1
- Item 2
  - Nested item
`)

	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	// Check document structure.
	if doc.Kind != mdast.NodeDocument {
		t.Errorf("expected NodeDocument, got %v", doc.Kind)
	}

	// Should have heading, paragraph, and list as direct children.
	headings := mdast.FindByKind(doc, mdast.NodeHeading)
	if len(headings) != 1 {
		t.Errorf("expected 1 heading, got %d", len(headings))
	}

	paragraphs := mdast.FindByKind(doc, mdast.NodeParagraph)
	if len(paragraphs) < 1 {
		t.Errorf("expected at least 1 paragraph, got %d", len(paragraphs))
	}

	lists := mdast.FindByKind(doc, mdast.NodeList)
	if len(lists) < 1 {
		t.Errorf("expected at least 1 list, got %d", len(lists))
	}

	// Check inline elements.
	emphasis := mdast.FindByKind(doc, mdast.NodeEmphasis)
	if len(emphasis) != 1 {
		t.Errorf("expected 1 emphasis, got %d", len(emphasis))
	}

	strong := mdast.FindByKind(doc, mdast.NodeStrong)
	if len(strong) != 1 {
		t.Errorf("expected 1 strong, got %d", len(strong))
	}
}

func TestMapper_ParentChildRelationships(t *testing.T) {
	content := []byte("# Heading\n\nParagraph")
	mapper := newMapper(content)

	md := goldmark.New()
	reader := text.NewReader(content)
	gmDoc := md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	doc := mapper.mapDocument(gmDoc)

	// Document should be parent of heading.
	headings := mdast.FindByKind(doc, mdast.NodeHeading)
	if len(headings) != 1 {
		t.Fatal("expected 1 heading")
	}

	heading := headings[0]
	if heading.Parent != doc {
		t.Error("heading parent should be document")
	}

	// Check sibling relationships.
	paragraphs := mdast.FindByKind(doc, mdast.NodeParagraph)
	if len(paragraphs) != 1 {
		t.Fatal("expected 1 paragraph")
	}

	para := paragraphs[0]
	if para.Parent != doc {
		t.Error("paragraph parent should be document")
	}

	// Heading and paragraph should be siblings.
	if heading.Next != para {
		t.Error("heading.Next should be paragraph")
	}

	if para.Prev != heading {
		t.Error("paragraph.Prev should be heading")
	}
}
