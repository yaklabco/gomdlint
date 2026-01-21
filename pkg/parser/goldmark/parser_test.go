package goldmark

import (
	"context"
	"testing"
	"time"

	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

func TestParser_New(t *testing.T) {
	tests := []struct {
		name       string
		flavor     string
		wantFlavor string
	}{
		{"commonmark", FlavorCommonMark, FlavorCommonMark},
		{"gfm", FlavorGFM, FlavorGFM},
		{"invalid defaults to commonmark", "invalid", FlavorCommonMark},
		{"empty defaults to commonmark", "", FlavorCommonMark},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.flavor)

			if p.Flavor() != tt.wantFlavor {
				t.Errorf("Flavor() = %q, want %q", p.Flavor(), tt.wantFlavor)
			}
		})
	}
}

func TestParser_Parse_Basic(t *testing.T) {
	parser := New(FlavorCommonMark)
	ctx := context.Background()

	content := []byte("# Hello\n\nWorld")
	snapshot, err := parser.Parse(ctx, "test.md", content)

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}

	// Check path.
	if snapshot.Path != "test.md" {
		t.Errorf("Path = %q, want %q", snapshot.Path, "test.md")
	}

	// Check content is copied.
	if string(snapshot.Content) != string(content) {
		t.Errorf("Content mismatch")
	}

	// Verify content is a copy, not the same slice.
	if &snapshot.Content[0] == &content[0] {
		t.Error("Content should be a copy, not the same slice")
	}

	// Check lines.
	if len(snapshot.Lines) == 0 {
		t.Error("expected Lines to be populated")
	}

	// Check tokens.
	if len(snapshot.Tokens) == 0 {
		t.Error("expected Tokens to be populated")
	}

	if !mdast.ValidateTokens(snapshot.Tokens, len(snapshot.Content)) {
		t.Error("tokens are not valid")
	}

	// Check root.
	if snapshot.Root == nil {
		t.Fatal("expected Root to be non-nil")
	}

	if snapshot.Root.Kind != mdast.NodeDocument {
		t.Errorf("Root.Kind = %v, want NodeDocument", snapshot.Root.Kind)
	}

	// Check file back-references.
	err = mdast.Walk(snapshot.Root, func(n *mdast.Node) error {
		if n.File != snapshot {
			t.Errorf("node %v has incorrect File reference", n.Kind)
		}
		return nil
	})
	if err != nil {
		t.Errorf("Walk error: %v", err)
	}
}

func TestParser_Parse_Empty(t *testing.T) {
	parser := New(FlavorCommonMark)
	ctx := context.Background()

	snapshot, err := parser.Parse(ctx, "empty.md", []byte{})

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if snapshot == nil {
		t.Fatal("expected non-nil snapshot for empty content")
	}

	if snapshot.Root == nil {
		t.Fatal("expected Root to be non-nil for empty content")
	}

	if snapshot.Root.Kind != mdast.NodeDocument {
		t.Errorf("Root.Kind = %v, want NodeDocument", snapshot.Root.Kind)
	}
}

func TestParser_Parse_ContextCancelled(t *testing.T) {
	parser := New(FlavorCommonMark)

	// Create already-cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := parser.Parse(ctx, "test.md", []byte("# Hello"))

	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestParser_Parse_ContextTimeout(t *testing.T) {
	parser := New(FlavorCommonMark)

	// Create context with very short timeout (already expired).
	ctx, cancel := context.WithTimeout(context.Background(), -1*time.Second)
	defer cancel()

	_, err := parser.Parse(ctx, "test.md", []byte("# Hello"))

	if err == nil {
		t.Error("expected error for timed out context")
	}
}

func TestParser_Parse_CommonMark(t *testing.T) {
	parser := New(FlavorCommonMark)
	ctx := context.Background()

	content := []byte(`# Heading

Paragraph with *emphasis* and **strong**.

- Item 1
- Item 2

> Blockquote

` + "```go" + `
func main() {}
` + "```" + `

[Link](url)
`)

	snapshot, err := parser.Parse(ctx, "test.md", content)

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Verify structure.
	headings := mdast.FindByKind(snapshot.Root, mdast.NodeHeading)
	if len(headings) != 1 {
		t.Errorf("expected 1 heading, got %d", len(headings))
	}

	paragraphs := mdast.FindByKind(snapshot.Root, mdast.NodeParagraph)
	if len(paragraphs) < 1 {
		t.Errorf("expected at least 1 paragraph, got %d", len(paragraphs))
	}

	lists := mdast.FindByKind(snapshot.Root, mdast.NodeList)
	if len(lists) != 1 {
		t.Errorf("expected 1 list, got %d", len(lists))
	}

	blockquotes := mdast.FindByKind(snapshot.Root, mdast.NodeBlockquote)
	if len(blockquotes) != 1 {
		t.Errorf("expected 1 blockquote, got %d", len(blockquotes))
	}

	codeBlocks := mdast.FindByKind(snapshot.Root, mdast.NodeCodeBlock)
	if len(codeBlocks) != 1 {
		t.Errorf("expected 1 code block, got %d", len(codeBlocks))
	}

	links := mdast.FindByKind(snapshot.Root, mdast.NodeLink)
	if len(links) != 1 {
		t.Errorf("expected 1 link, got %d", len(links))
	}
}

func TestParser_Parse_GFM(t *testing.T) {
	parser := New(FlavorGFM)
	ctx := context.Background()

	content := []byte(`# GFM Features

- [x] Task 1
- [ ] Task 2

| Header 1 | Header 2 |
|----------|----------|
| Cell 1   | Cell 2   |

~~strikethrough~~

https://example.com
`)

	snapshot, err := parser.Parse(ctx, "test.md", content)

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// GFM should parse without error.
	if snapshot.Root == nil {
		t.Fatal("expected Root to be non-nil")
	}

	// Verify tokens are valid.
	if !mdast.ValidateTokens(snapshot.Tokens, len(snapshot.Content)) {
		t.Error("tokens are not valid")
	}
}

func TestParser_Parse_PositionMapping(t *testing.T) {
	parser := New(FlavorCommonMark)
	ctx := context.Background()

	content := []byte("# Heading\n\nParagraph")
	snapshot, err := parser.Parse(ctx, "test.md", content)

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	headings := mdast.FindByKind(snapshot.Root, mdast.NodeHeading)
	if len(headings) != 1 {
		t.Fatal("expected 1 heading")
	}

	h := headings[0]
	pos := h.SourcePosition()

	// Heading should be on line 1.
	if pos.StartLine != 1 {
		t.Errorf("heading StartLine = %d, want 1", pos.StartLine)
	}

	paragraphs := mdast.FindByKind(snapshot.Root, mdast.NodeParagraph)
	if len(paragraphs) != 1 {
		t.Fatal("expected 1 paragraph")
	}

	para := paragraphs[0]
	paraPos := para.SourcePosition()

	// Paragraph should be on line 3 (after blank line).
	if paraPos.StartLine != 3 {
		t.Errorf("paragraph StartLine = %d, want 3", paraPos.StartLine)
	}
}

func TestParser_Parse_TokenRanges(t *testing.T) {
	parser := New(FlavorCommonMark)
	ctx := context.Background()

	content := []byte("# Hello")
	snapshot, err := parser.Parse(ctx, "test.md", content)

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Document should have valid token range.
	if snapshot.Root.FirstToken < 0 || snapshot.Root.LastToken < 0 {
		t.Error("document should have valid token range")
	}

	// Document should span all tokens.
	if snapshot.Root.FirstToken != 0 {
		t.Errorf("document FirstToken = %d, want 0", snapshot.Root.FirstToken)
	}

	if snapshot.Root.LastToken != len(snapshot.Tokens)-1 {
		t.Errorf("document LastToken = %d, want %d", snapshot.Root.LastToken, len(snapshot.Tokens)-1)
	}
}

func TestParser_Parse_MultipleFiles(t *testing.T) {
	parser := New(FlavorCommonMark)
	ctx := context.Background()

	files := []struct {
		path    string
		content string
	}{
		{"file1.md", "# File 1"},
		{"file2.md", "# File 2\n\nContent"},
		{"file3.md", "- List\n- Items"},
	}

	for _, file := range files {
		t.Run(file.path, func(t *testing.T) {
			snapshot, err := parser.Parse(ctx, file.path, []byte(file.content))

			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if snapshot.Path != file.path {
				t.Errorf("Path = %q, want %q", snapshot.Path, file.path)
			}

			if !mdast.ValidateTokens(snapshot.Tokens, len(snapshot.Content)) {
				t.Error("tokens are not valid")
			}
		})
	}
}

func TestParser_Parse_NestedLists(t *testing.T) {
	parser := New(FlavorCommonMark)
	ctx := context.Background()

	content := []byte(`- Item 1
  - Nested 1
  - Nested 2
- Item 2
`)

	snapshot, err := parser.Parse(ctx, "test.md", content)

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	lists := mdast.FindByKind(snapshot.Root, mdast.NodeList)
	if len(lists) < 2 {
		t.Errorf("expected at least 2 lists (outer and nested), got %d", len(lists))
	}

	items := mdast.FindByKind(snapshot.Root, mdast.NodeListItem)
	if len(items) != 4 {
		t.Errorf("expected 4 list items, got %d", len(items))
	}
}

func TestParser_Parse_ComplexDocument(t *testing.T) {
	parser := New(FlavorCommonMark)
	ctx := context.Background()

	content := []byte(`# Main Title

This is the introduction with *emphasis*, **strong**, and ` + "`code`" + `.

## Section 1

Paragraph with a [link](https://example.com "Title").

### Subsection 1.1

> A blockquote with
> multiple lines.

## Section 2

1. First item
2. Second item
   - Nested bullet
3. Third item

` + "```python" + `
def hello():
    print("Hello, World!")
` + "```" + `

---

![Image](image.png)

Final paragraph.
`)

	snapshot, err := parser.Parse(ctx, "complex.md", content)

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Validate overall structure.
	if !mdast.ValidateTokens(snapshot.Tokens, len(snapshot.Content)) {
		t.Error("tokens are not valid")
	}

	// Check heading count.
	headings := mdast.FindByKind(snapshot.Root, mdast.NodeHeading)
	if len(headings) != 4 {
		t.Errorf("expected 4 headings, got %d", len(headings))
	}

	// Check various elements exist.
	emphasis := mdast.FindByKind(snapshot.Root, mdast.NodeEmphasis)
	if len(emphasis) == 0 {
		t.Error("expected at least one emphasis")
	}

	strong := mdast.FindByKind(snapshot.Root, mdast.NodeStrong)
	if len(strong) == 0 {
		t.Error("expected at least one strong")
	}

	codeSpans := mdast.FindByKind(snapshot.Root, mdast.NodeCodeSpan)
	if len(codeSpans) == 0 {
		t.Error("expected at least one code span")
	}

	codeBlocks := mdast.FindByKind(snapshot.Root, mdast.NodeCodeBlock)
	if len(codeBlocks) != 1 {
		t.Errorf("expected 1 code block, got %d", len(codeBlocks))
	}

	blockquotes := mdast.FindByKind(snapshot.Root, mdast.NodeBlockquote)
	if len(blockquotes) != 1 {
		t.Errorf("expected 1 blockquote, got %d", len(blockquotes))
	}

	thematicBreaks := mdast.FindByKind(snapshot.Root, mdast.NodeThematicBreak)
	if len(thematicBreaks) != 1 {
		t.Errorf("expected 1 thematic break, got %d", len(thematicBreaks))
	}

	images := mdast.FindByKind(snapshot.Root, mdast.NodeImage)
	if len(images) != 1 {
		t.Errorf("expected 1 image, got %d", len(images))
	}
}

func TestParser_ImplementsInterface(_ *testing.T) {
	// Compile-time check that Parser implements lint.Parser.
	var _ lint.Parser = (*Parser)(nil)
}

func TestParser_Parse_Deterministic(t *testing.T) {
	parser := New(FlavorCommonMark)
	ctx := context.Background()

	content := []byte("# Hello\n\n*World*")

	// Parse the same content multiple times.
	snapshots := make([]*FileSnapshot, 0, 3)
	for range 3 {
		snapshot, err := parser.Parse(ctx, "test.md", content)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		snapshots = append(snapshots, snapshot)
	}

	// All snapshots should have the same token count.
	tokenCount := len(snapshots[0].Tokens)
	for i, s := range snapshots {
		if len(s.Tokens) != tokenCount {
			t.Errorf("snapshot[%d] token count = %d, want %d", i, len(s.Tokens), tokenCount)
		}
	}

	// All snapshots should have the same node structure.
	nodeCount := countNodes(snapshots[0].Root)
	for i, s := range snapshots {
		if countNodes(s.Root) != nodeCount {
			t.Errorf("snapshot[%d] node count = %d, want %d", i, countNodes(s.Root), nodeCount)
		}
	}
}

// countNodes counts all nodes in the tree.
func countNodes(root *mdast.Node) int {
	count := 0
	err := mdast.Walk(root, func(_ *mdast.Node) error {
		count++
		return nil
	})
	if err != nil {
		return 0
	}
	return count
}
