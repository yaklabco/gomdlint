package mdast_test

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

func TestNode_IsBlock(t *testing.T) {
	t.Parallel()

	blockKinds := []mdast.NodeKind{
		mdast.NodeDocument,
		mdast.NodeParagraph,
		mdast.NodeHeading,
		mdast.NodeList,
		mdast.NodeListItem,
		mdast.NodeBlockquote,
		mdast.NodeCodeBlock,
		mdast.NodeThematicBreak,
		mdast.NodeHTMLBlock,
	}

	for _, kind := range blockKinds {
		node := &mdast.Node{Kind: kind}
		if !node.IsBlock() {
			t.Errorf("expected %s to be block", kind)
		}
	}

	inlineKinds := []mdast.NodeKind{
		mdast.NodeText,
		mdast.NodeEmphasis,
		mdast.NodeStrong,
		mdast.NodeCodeSpan,
		mdast.NodeLink,
	}

	for _, kind := range inlineKinds {
		node := &mdast.Node{Kind: kind}
		if node.IsBlock() {
			t.Errorf("expected %s to not be block", kind)
		}
	}
}

func TestNode_IsInline(t *testing.T) {
	t.Parallel()

	inlineKinds := []mdast.NodeKind{
		mdast.NodeText,
		mdast.NodeEmphasis,
		mdast.NodeStrong,
		mdast.NodeCodeSpan,
		mdast.NodeLink,
		mdast.NodeImage,
		mdast.NodeSoftBreak,
		mdast.NodeHardBreak,
		mdast.NodeHTMLInline,
	}

	for _, kind := range inlineKinds {
		node := &mdast.Node{Kind: kind}
		if !node.IsInline() {
			t.Errorf("expected %s to be inline", kind)
		}
	}

	blockKinds := []mdast.NodeKind{
		mdast.NodeDocument,
		mdast.NodeParagraph,
		mdast.NodeHeading,
	}

	for _, kind := range blockKinds {
		node := &mdast.Node{Kind: kind}
		if node.IsInline() {
			t.Errorf("expected %s to not be inline", kind)
		}
	}
}

func TestNode_HasChildren(t *testing.T) {
	t.Parallel()

	parent := mdast.NewNode(mdast.NodeDocument)
	child := mdast.NewNode(mdast.NodeParagraph)

	if parent.HasChildren() {
		t.Error("expected empty node to have no children")
	}

	mdast.AppendChild(parent, child)

	if !parent.HasChildren() {
		t.Error("expected node with child to have children")
	}
}

func TestNode_ChildCount(t *testing.T) {
	t.Parallel()

	parent := mdast.NewNode(mdast.NodeDocument)

	if parent.ChildCount() != 0 {
		t.Errorf("expected 0 children, got %d", parent.ChildCount())
	}

	mdast.AppendChild(parent, mdast.NewNode(mdast.NodeParagraph))
	if parent.ChildCount() != 1 {
		t.Errorf("expected 1 child, got %d", parent.ChildCount())
	}

	mdast.AppendChild(parent, mdast.NewNode(mdast.NodeParagraph))
	mdast.AppendChild(parent, mdast.NewNode(mdast.NodeParagraph))
	if parent.ChildCount() != 3 {
		t.Errorf("expected 3 children, got %d", parent.ChildCount())
	}
}

func TestNode_Children(t *testing.T) {
	t.Parallel()

	parent := mdast.NewNode(mdast.NodeDocument)
	child1 := mdast.NewNode(mdast.NodeParagraph)
	child2 := mdast.NewNode(mdast.NodeHeading)
	child3 := mdast.NewNode(mdast.NodeCodeBlock)

	mdast.AppendChild(parent, child1)
	mdast.AppendChild(parent, child2)
	mdast.AppendChild(parent, child3)

	children := parent.Children()

	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}

	if children[0] != child1 || children[1] != child2 || children[2] != child3 {
		t.Error("children not in expected order")
	}
}

func TestNodeKind_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind     mdast.NodeKind
		expected string
	}{
		{mdast.NodeDocument, "Document"},
		{mdast.NodeParagraph, "Paragraph"},
		{mdast.NodeHeading, "Heading"},
		{mdast.NodeList, "List"},
		{mdast.NodeText, "Text"},
		{mdast.NodeEmphasis, "Emphasis"},
		{mdast.NodeRaw, "Raw"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			if tt.kind.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.kind.String())
			}
		})
	}
}

func TestNode_SourceRange(t *testing.T) {
	t.Parallel()

	content := []byte("# Heading\n\nParagraph text.")
	snapshot := &mdast.FileSnapshot{
		Path:    "test.md",
		Content: content,
		Lines:   mdast.BuildLines(content),
		Tokens: []mdast.Token{
			{Kind: mdast.TokHeadingMarker, StartOffset: 0, EndOffset: 2},
			{Kind: mdast.TokText, StartOffset: 2, EndOffset: 9},
			{Kind: mdast.TokNewline, StartOffset: 9, EndOffset: 10},
			{Kind: mdast.TokNewline, StartOffset: 10, EndOffset: 11},
			{Kind: mdast.TokText, StartOffset: 11, EndOffset: 26},
		},
	}

	// Node with tokens 0-2 (heading line).
	node := &mdast.Node{
		Kind:       mdast.NodeHeading,
		FirstToken: 0,
		LastToken:  2,
		File:       snapshot,
	}

	sourceRange := node.SourceRange()

	if sourceRange.StartOffset != 0 {
		t.Errorf("expected StartOffset 0, got %d", sourceRange.StartOffset)
	}

	if sourceRange.EndOffset != 10 {
		t.Errorf("expected EndOffset 10, got %d", sourceRange.EndOffset)
	}
}

func TestNode_SourceRangeNoFile(t *testing.T) {
	t.Parallel()

	node := &mdast.Node{
		Kind:       mdast.NodeParagraph,
		FirstToken: 0,
		LastToken:  1,
		File:       nil,
	}

	sourceRange := node.SourceRange()

	if sourceRange.StartOffset != 0 || sourceRange.EndOffset != 0 {
		t.Error("expected empty source range for node without file")
	}
}

func TestNode_SourcePosition(t *testing.T) {
	t.Parallel()

	content := []byte("line1\nline2")
	snapshot := &mdast.FileSnapshot{
		Path:    "test.md",
		Content: content,
		Lines:   mdast.BuildLines(content),
		Tokens: []mdast.Token{
			{Kind: mdast.TokText, StartOffset: 0, EndOffset: 5},
			{Kind: mdast.TokNewline, StartOffset: 5, EndOffset: 6},
			{Kind: mdast.TokText, StartOffset: 6, EndOffset: 11},
		},
	}

	// Node spanning line 2.
	node := &mdast.Node{
		Kind:       mdast.NodeText,
		FirstToken: 2,
		LastToken:  2,
		File:       snapshot,
	}

	sourcePos := node.SourcePosition()

	if sourcePos.StartLine != 2 || sourcePos.StartColumn != 1 {
		t.Errorf("expected start (2, 1), got (%d, %d)", sourcePos.StartLine, sourcePos.StartColumn)
	}

	if sourcePos.EndLine != 2 || sourcePos.EndColumn != 6 {
		t.Errorf("expected end (2, 6), got (%d, %d)", sourcePos.EndLine, sourcePos.EndColumn)
	}
}

func TestNode_Text(t *testing.T) {
	t.Parallel()

	content := []byte("hello world")
	snapshot := &mdast.FileSnapshot{
		Path:    "test.md",
		Content: content,
		Lines:   mdast.BuildLines(content),
		Tokens: []mdast.Token{
			{Kind: mdast.TokText, StartOffset: 0, EndOffset: 5},
			{Kind: mdast.TokWhitespace, StartOffset: 5, EndOffset: 6},
			{Kind: mdast.TokText, StartOffset: 6, EndOffset: 11},
		},
	}

	node := &mdast.Node{
		Kind:       mdast.NodeText,
		FirstToken: 0,
		LastToken:  0,
		File:       snapshot,
	}

	text := node.Text()
	if string(text) != "hello" {
		t.Errorf("expected 'hello', got %q", text)
	}
}
