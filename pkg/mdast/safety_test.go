package mdast_test

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// newTree returns a document with three children, and its middle child.
func newTree() (*mdast.Node, *mdast.Node) {
	doc := mdast.NewDocument()
	second := mdast.NewNode(mdast.NodeHeading)
	mdast.AppendChild(doc, mdast.NewNode(mdast.NodeParagraph))
	mdast.AppendChild(doc, second)
	mdast.AppendChild(doc, mdast.NewNode(mdast.NodeThematicBreak))
	return doc, second
}

func childKinds(parent *mdast.Node) []mdast.NodeKind {
	var kinds []mdast.NodeKind
	for child := parent.FirstChild; child != nil; child = child.Next {
		kinds = append(kinds, child.Kind)
	}
	return kinds
}

func equalKinds(a, b []mdast.NodeKind) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

func TestWalkSurvivesRemovingTheVisitedNode(t *testing.T) {
	t.Parallel()

	doc, second := newTree()
	var visited []mdast.NodeKind
	err := mdast.Walk(doc, func(node *mdast.Node) error {
		visited = append(visited, node.Kind)
		if node == second {
			mdast.RemoveChild(doc, node)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []mdast.NodeKind{mdast.NodeDocument, mdast.NodeParagraph, mdast.NodeHeading, mdast.NodeThematicBreak}
	if !equalKinds(visited, want) {
		t.Errorf("visited %v, want %v", visited, want)
	}
}

func TestWalkWithContextSurvivesRemovingTheVisitedNode(t *testing.T) {
	t.Parallel()

	doc, second := newTree()
	var left []mdast.NodeKind
	err := mdast.WalkWithContext(doc, func(node *mdast.Node) error {
		if node == second {
			mdast.RemoveChild(doc, node)
		}
		return nil
	}, func(node *mdast.Node) error {
		left = append(left, node.Kind)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []mdast.NodeKind{mdast.NodeParagraph, mdast.NodeHeading, mdast.NodeThematicBreak, mdast.NodeDocument}
	if !equalKinds(left, want) {
		t.Errorf("left %v, want %v", left, want)
	}
}

func TestReplaceChildWithItselfKeepsSiblings(t *testing.T) {
	t.Parallel()

	doc, second := newTree()
	mdast.ReplaceChild(doc, second, second)
	want := []mdast.NodeKind{mdast.NodeParagraph, mdast.NodeHeading, mdast.NodeThematicBreak}
	if got := childKinds(doc); !equalKinds(got, want) {
		t.Errorf("children %v, want %v", got, want)
	}
}

func TestTreeEditsRefuseCycles(t *testing.T) {
	t.Parallel()

	edits := map[string]func(doc, child, grandchild *mdast.Node){
		"append ancestor":        func(_, child, grandchild *mdast.Node) { mdast.AppendChild(grandchild, child) },
		"append self":            func(_, child, _ *mdast.Node) { mdast.AppendChild(child, child) },
		"prepend ancestor":       func(doc, _, grandchild *mdast.Node) { mdast.PrependChild(grandchild, doc) },
		"insert ancestor before": func(_, child, grandchild *mdast.Node) { mdast.InsertBefore(grandchild, child) },
		"insert ancestor after":  func(_, child, grandchild *mdast.Node) { mdast.InsertAfter(grandchild, child) },
		"replace with an ancestor": func(_, child, grandchild *mdast.Node) {
			mdast.ReplaceChild(child, grandchild, child)
		},
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			doc := mdast.NewDocument()
			child := mdast.NewNode(mdast.NodeBlockquote)
			grandchild := mdast.NewNode(mdast.NodeParagraph)
			mdast.AppendChild(doc, child)
			mdast.AppendChild(child, grandchild)

			edit(doc, child, grandchild)

			if child.Parent != doc || grandchild.Parent != child || doc.Parent != nil {
				t.Fatalf("tree changed: doc.Parent=%v child.Parent=%v grandchild.Parent=%v", doc.Parent, child.Parent, grandchild.Parent)
			}
			count := 0
			err := mdast.Walk(doc, func(*mdast.Node) error {
				count++
				if count > 10 {
					t.Fatal("walk does not terminate")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if count != 3 {
				t.Errorf("walked %d nodes, want 3", count)
			}
		})
	}
}
