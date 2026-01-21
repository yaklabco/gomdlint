package mdast_test

import (
	"errors"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

func buildTestTree() *mdast.Node {
	// Build a simple tree:
	// Document
	//   Heading
	//     Text
	//   Paragraph
	//     Text
	//     Emphasis
	//       Text

	doc := mdast.NewNode(mdast.NodeDocument)

	heading := mdast.NewNode(mdast.NodeHeading)
	headingText := mdast.NewNode(mdast.NodeText)
	mdast.AppendChild(heading, headingText)
	mdast.AppendChild(doc, heading)

	para := mdast.NewNode(mdast.NodeParagraph)
	paraText := mdast.NewNode(mdast.NodeText)
	mdast.AppendChild(para, paraText)

	emphasis := mdast.NewNode(mdast.NodeEmphasis)
	emphText := mdast.NewNode(mdast.NodeText)
	mdast.AppendChild(emphasis, emphText)
	mdast.AppendChild(para, emphasis)

	mdast.AppendChild(doc, para)

	return doc
}

func TestWalk(t *testing.T) {
	t.Parallel()

	doc := buildTestTree()

	var visited []mdast.NodeKind
	err := mdast.Walk(doc, func(n *mdast.Node) error {
		visited = append(visited, n.Kind)
		return nil
	})

	if err != nil {
		t.Fatalf("Walk returned error: %v", err)
	}

	expected := []mdast.NodeKind{
		mdast.NodeDocument,
		mdast.NodeHeading,
		mdast.NodeText,
		mdast.NodeParagraph,
		mdast.NodeText,
		mdast.NodeEmphasis,
		mdast.NodeText,
	}

	if len(visited) != len(expected) {
		t.Fatalf("expected %d nodes, got %d", len(expected), len(visited))
	}

	for i, kind := range expected {
		if visited[i] != kind {
			t.Errorf("node %d: expected %s, got %s", i, kind, visited[i])
		}
	}
}

func TestWalk_NilRoot(t *testing.T) {
	t.Parallel()

	err := mdast.Walk(nil, func(_ *mdast.Node) error {
		t.Error("callback should not be called for nil root")
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error for nil root, got %v", err)
	}
}

func TestWalk_EmptyDocument(t *testing.T) {
	t.Parallel()

	doc := mdast.NewNode(mdast.NodeDocument)

	count := 0
	err := mdast.Walk(doc, func(_ *mdast.Node) error {
		count++
		return nil
	})

	if err != nil {
		t.Fatalf("Walk returned error: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 node (document), got %d", count)
	}
}

func TestWalk_EarlyTermination(t *testing.T) {
	t.Parallel()

	doc := buildTestTree()

	expectedErr := errors.New("stop here")
	count := 0

	err := mdast.Walk(doc, func(n *mdast.Node) error {
		count++
		if n.Kind == mdast.NodeParagraph {
			return expectedErr
		}
		return nil
	})

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Should have visited: Document, Heading, Text, Paragraph (then stopped).
	if count != 4 {
		t.Errorf("expected 4 nodes before stopping, got %d", count)
	}
}

func TestWalkWithContext(t *testing.T) {
	t.Parallel()

	doc := buildTestTree()

	var enterOrder []mdast.NodeKind
	var leaveOrder []mdast.NodeKind

	err := mdast.WalkWithContext(doc,
		func(n *mdast.Node) error {
			enterOrder = append(enterOrder, n.Kind)
			return nil
		},
		func(n *mdast.Node) error {
			leaveOrder = append(leaveOrder, n.Kind)
			return nil
		},
	)

	if err != nil {
		t.Fatalf("WalkWithContext returned error: %v", err)
	}

	// Enter order should be pre-order.
	expectedEnter := []mdast.NodeKind{
		mdast.NodeDocument,
		mdast.NodeHeading,
		mdast.NodeText,
		mdast.NodeParagraph,
		mdast.NodeText,
		mdast.NodeEmphasis,
		mdast.NodeText,
	}

	// Leave order should be post-order.
	expectedLeave := []mdast.NodeKind{
		mdast.NodeText,
		mdast.NodeHeading,
		mdast.NodeText,
		mdast.NodeText,
		mdast.NodeEmphasis,
		mdast.NodeParagraph,
		mdast.NodeDocument,
	}

	if len(enterOrder) != len(expectedEnter) {
		t.Fatalf("enter: expected %d, got %d", len(expectedEnter), len(enterOrder))
	}

	for i, kind := range expectedEnter {
		if enterOrder[i] != kind {
			t.Errorf("enter %d: expected %s, got %s", i, kind, enterOrder[i])
		}
	}

	if len(leaveOrder) != len(expectedLeave) {
		t.Fatalf("leave: expected %d, got %d", len(expectedLeave), len(leaveOrder))
	}

	for i, kind := range expectedLeave {
		if leaveOrder[i] != kind {
			t.Errorf("leave %d: expected %s, got %s", i, kind, leaveOrder[i])
		}
	}
}

func TestWalkWithContext_NilCallbacks(t *testing.T) {
	t.Parallel()

	doc := buildTestTree()

	// Should not panic with nil callbacks.
	err := mdast.WalkWithContext(doc, nil, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestWalkBlocks(t *testing.T) {
	t.Parallel()

	doc := buildTestTree()

	var visited []mdast.NodeKind
	err := mdast.WalkBlocks(doc, func(n *mdast.Node) error {
		visited = append(visited, n.Kind)
		return nil
	})

	if err != nil {
		t.Fatalf("WalkBlocks returned error: %v", err)
	}

	expected := []mdast.NodeKind{
		mdast.NodeDocument,
		mdast.NodeHeading,
		mdast.NodeParagraph,
	}

	if len(visited) != len(expected) {
		t.Fatalf("expected %d blocks, got %d", len(expected), len(visited))
	}

	for i, kind := range expected {
		if visited[i] != kind {
			t.Errorf("block %d: expected %s, got %s", i, kind, visited[i])
		}
	}
}

func TestWalkInlines(t *testing.T) {
	t.Parallel()

	doc := buildTestTree()

	var visited []mdast.NodeKind
	err := mdast.WalkInlines(doc, func(n *mdast.Node) error {
		visited = append(visited, n.Kind)
		return nil
	})

	if err != nil {
		t.Fatalf("WalkInlines returned error: %v", err)
	}

	expected := []mdast.NodeKind{
		mdast.NodeText,
		mdast.NodeText,
		mdast.NodeEmphasis,
		mdast.NodeText,
	}

	if len(visited) != len(expected) {
		t.Fatalf("expected %d inlines, got %d", len(expected), len(visited))
	}

	for i, kind := range expected {
		if visited[i] != kind {
			t.Errorf("inline %d: expected %s, got %s", i, kind, visited[i])
		}
	}
}

func TestFindAll(t *testing.T) {
	t.Parallel()

	doc := buildTestTree()

	textNodes := mdast.FindAll(doc, func(n *mdast.Node) bool {
		return n.Kind == mdast.NodeText
	})

	if len(textNodes) != 3 {
		t.Errorf("expected 3 text nodes, got %d", len(textNodes))
	}
}

func TestFindFirst(t *testing.T) {
	t.Parallel()

	doc := buildTestTree()

	para := mdast.FindFirst(doc, func(n *mdast.Node) bool {
		return n.Kind == mdast.NodeParagraph
	})

	if para == nil {
		t.Fatal("expected to find paragraph")
	}

	if para.Kind != mdast.NodeParagraph {
		t.Errorf("expected Paragraph, got %s", para.Kind)
	}

	// Should not find non-existent node.
	notFound := mdast.FindFirst(doc, func(n *mdast.Node) bool {
		return n.Kind == mdast.NodeCodeBlock
	})

	if notFound != nil {
		t.Error("expected nil for non-existent node")
	}
}

func TestFindByKind(t *testing.T) {
	t.Parallel()

	doc := buildTestTree()

	headings := mdast.FindByKind(doc, mdast.NodeHeading)
	if len(headings) != 1 {
		t.Errorf("expected 1 heading, got %d", len(headings))
	}

	paragraphs := mdast.FindByKind(doc, mdast.NodeParagraph)
	if len(paragraphs) != 1 {
		t.Errorf("expected 1 paragraph, got %d", len(paragraphs))
	}

	codeBlocks := mdast.FindByKind(doc, mdast.NodeCodeBlock)
	if len(codeBlocks) != 0 {
		t.Errorf("expected 0 code blocks, got %d", len(codeBlocks))
	}
}
