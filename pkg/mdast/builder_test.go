package mdast_test

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

func TestNewNode(t *testing.T) {
	t.Parallel()

	node := mdast.NewNode(mdast.NodeParagraph)

	if node.Kind != mdast.NodeParagraph {
		t.Errorf("expected Paragraph, got %s", node.Kind)
	}

	if node.FirstToken != -1 || node.LastToken != -1 {
		t.Error("expected token indices to be -1")
	}

	if node.Parent != nil || node.FirstChild != nil || node.LastChild != nil {
		t.Error("expected nil parent and children")
	}
}

func TestNewDocument(t *testing.T) {
	t.Parallel()

	doc := mdast.NewDocument()

	if doc.Kind != mdast.NodeDocument {
		t.Errorf("expected Document, got %s", doc.Kind)
	}
}

func TestAppendChild(t *testing.T) {
	t.Parallel()

	parent := mdast.NewNode(mdast.NodeDocument)
	child1 := mdast.NewNode(mdast.NodeParagraph)
	child2 := mdast.NewNode(mdast.NodeHeading)

	mdast.AppendChild(parent, child1)

	if parent.FirstChild != child1 || parent.LastChild != child1 {
		t.Error("first child not set correctly")
	}

	if child1.Parent != parent {
		t.Error("child1 parent not set")
	}

	mdast.AppendChild(parent, child2)

	if parent.FirstChild != child1 {
		t.Error("first child should still be child1")
	}

	if parent.LastChild != child2 {
		t.Error("last child should be child2")
	}

	if child1.Next != child2 || child2.Prev != child1 {
		t.Error("sibling links not set correctly")
	}
}

func TestPrependChild(t *testing.T) {
	t.Parallel()

	parent := mdast.NewNode(mdast.NodeDocument)
	child1 := mdast.NewNode(mdast.NodeParagraph)
	child2 := mdast.NewNode(mdast.NodeHeading)

	mdast.AppendChild(parent, child1)
	mdast.PrependChild(parent, child2)

	if parent.FirstChild != child2 {
		t.Error("first child should be child2")
	}

	if parent.LastChild != child1 {
		t.Error("last child should be child1")
	}

	if child2.Next != child1 || child1.Prev != child2 {
		t.Error("sibling links not set correctly")
	}
}

func TestInsertBefore(t *testing.T) {
	t.Parallel()

	parent := mdast.NewNode(mdast.NodeDocument)
	child1 := mdast.NewNode(mdast.NodeParagraph)
	child2 := mdast.NewNode(mdast.NodeHeading)
	newNode := mdast.NewNode(mdast.NodeCodeBlock)

	mdast.AppendChild(parent, child1)
	mdast.AppendChild(parent, child2)

	mdast.InsertBefore(child2, newNode)

	if parent.FirstChild != child1 {
		t.Error("first child should still be child1")
	}

	if child1.Next != newNode {
		t.Error("child1.Next should be newNode")
	}

	if newNode.Prev != child1 || newNode.Next != child2 {
		t.Error("newNode sibling links incorrect")
	}

	if child2.Prev != newNode {
		t.Error("child2.Prev should be newNode")
	}
}

func TestInsertAfter(t *testing.T) {
	t.Parallel()

	parent := mdast.NewNode(mdast.NodeDocument)
	child1 := mdast.NewNode(mdast.NodeParagraph)
	child2 := mdast.NewNode(mdast.NodeHeading)
	newNode := mdast.NewNode(mdast.NodeCodeBlock)

	mdast.AppendChild(parent, child1)
	mdast.AppendChild(parent, child2)

	mdast.InsertAfter(child1, newNode)

	if child1.Next != newNode {
		t.Error("child1.Next should be newNode")
	}

	if newNode.Prev != child1 || newNode.Next != child2 {
		t.Error("newNode sibling links incorrect")
	}

	if child2.Prev != newNode {
		t.Error("child2.Prev should be newNode")
	}
}

func TestRemoveChild(t *testing.T) {
	t.Parallel()

	parent := mdast.NewNode(mdast.NodeDocument)
	child1 := mdast.NewNode(mdast.NodeParagraph)
	child2 := mdast.NewNode(mdast.NodeHeading)
	child3 := mdast.NewNode(mdast.NodeCodeBlock)

	mdast.AppendChild(parent, child1)
	mdast.AppendChild(parent, child2)
	mdast.AppendChild(parent, child3)

	// Remove middle child.
	mdast.RemoveChild(parent, child2)

	if child1.Next != child3 || child3.Prev != child1 {
		t.Error("sibling links not updated after removal")
	}

	if child2.Parent != nil || child2.Prev != nil || child2.Next != nil {
		t.Error("removed child should have nil links")
	}

	// Remove first child.
	mdast.RemoveChild(parent, child1)

	if parent.FirstChild != child3 {
		t.Error("first child should now be child3")
	}

	// Remove last child.
	mdast.RemoveChild(parent, child3)

	if parent.FirstChild != nil || parent.LastChild != nil {
		t.Error("parent should have no children")
	}
}

func TestReplaceChild(t *testing.T) {
	t.Parallel()

	parent := mdast.NewNode(mdast.NodeDocument)
	child1 := mdast.NewNode(mdast.NodeParagraph)
	child2 := mdast.NewNode(mdast.NodeHeading)
	child3 := mdast.NewNode(mdast.NodeCodeBlock)
	newChild := mdast.NewNode(mdast.NodeBlockquote)

	mdast.AppendChild(parent, child1)
	mdast.AppendChild(parent, child2)
	mdast.AppendChild(parent, child3)

	mdast.ReplaceChild(parent, child2, newChild)

	if child1.Next != newChild {
		t.Error("child1.Next should be newChild")
	}

	if newChild.Prev != child1 || newChild.Next != child3 {
		t.Error("newChild sibling links incorrect")
	}

	if child3.Prev != newChild {
		t.Error("child3.Prev should be newChild")
	}

	if child2.Parent != nil {
		t.Error("old child should have nil parent")
	}
}

func TestSetTokenRange(t *testing.T) {
	t.Parallel()

	node := mdast.NewNode(mdast.NodeParagraph)

	mdast.SetTokenRange(node, 5, 10)

	if node.FirstToken != 5 || node.LastToken != 10 {
		t.Errorf("expected tokens (5, 10), got (%d, %d)", node.FirstToken, node.LastToken)
	}
}

func TestSetFile(t *testing.T) {
	t.Parallel()

	doc := mdast.NewDocument()
	child1 := mdast.NewNode(mdast.NodeParagraph)
	child2 := mdast.NewNode(mdast.NodeText)

	mdast.AppendChild(doc, child1)
	mdast.AppendChild(child1, child2)

	snapshot := &mdast.FileSnapshot{Path: "test.md"}

	mdast.SetFile(doc, snapshot)

	if doc.File != snapshot {
		t.Error("doc.File not set")
	}

	if child1.File != snapshot {
		t.Error("child1.File not set")
	}

	if child2.File != snapshot {
		t.Error("child2.File not set")
	}
}

func TestAppendChild_MovesFromPreviousParent(t *testing.T) {
	t.Parallel()

	parent1 := mdast.NewNode(mdast.NodeDocument)
	parent2 := mdast.NewNode(mdast.NodeDocument)
	child := mdast.NewNode(mdast.NodeParagraph)

	mdast.AppendChild(parent1, child)

	if parent1.FirstChild != child {
		t.Error("child should be in parent1")
	}

	// Move to parent2.
	mdast.AppendChild(parent2, child)

	if parent1.FirstChild != nil {
		t.Error("parent1 should have no children after move")
	}

	if parent2.FirstChild != child {
		t.Error("child should be in parent2")
	}

	if child.Parent != parent2 {
		t.Error("child.Parent should be parent2")
	}
}
