package mdast

//go:generate stringer -type=NodeKind -trimprefix=Node

// NodeKind classifies the type of an AST node.
type NodeKind uint16

// Node kinds for block-level and inline-level Markdown elements.
const (
	NodeDocument NodeKind = iota

	// Block-level nodes.
	NodeParagraph
	NodeHeading
	NodeList
	NodeListItem
	NodeBlockquote
	NodeCodeBlock
	NodeThematicBreak
	NodeHTMLBlock

	// Inline-level nodes.
	NodeText
	NodeEmphasis
	NodeStrong
	NodeCodeSpan
	NodeLink
	NodeImage
	NodeSoftBreak
	NodeHardBreak
	NodeHTMLInline

	// Fallback for unrecognized content.
	NodeRaw
)

// Node represents a single node in the Markdown AST.
// Nodes form a tree structure with parent/child/sibling relationships.
type Node struct {
	// Kind identifies what type of node this is.
	Kind NodeKind

	// Tree structure pointers.
	Parent     *Node
	FirstChild *Node
	LastChild  *Node
	Prev       *Node
	Next       *Node

	// Token span (indices into FileSnapshot.Tokens).
	// FirstToken <= LastToken for non-empty nodes.
	// Both are -1 for synthetic/degenerate nodes.
	FirstToken int
	LastToken  int

	// File is a back-reference to the containing FileSnapshot.
	File *FileSnapshot

	// Block holds attributes for block-level nodes.
	Block *BlockAttrs

	// Inline holds attributes for inline-level nodes.
	Inline *InlineAttrs

	// Ext holds extension-specific attributes (e.g., GFM).
	Ext map[string]any
}

// IsBlock returns true if this is a block-level node.
func (n *Node) IsBlock() bool {
	switch n.Kind {
	case NodeDocument, NodeParagraph, NodeHeading, NodeList, NodeListItem,
		NodeBlockquote, NodeCodeBlock, NodeThematicBreak, NodeHTMLBlock:
		return true
	default:
		return false
	}
}

// IsInline returns true if this is an inline-level node.
func (n *Node) IsInline() bool {
	switch n.Kind {
	case NodeText, NodeEmphasis, NodeStrong, NodeCodeSpan, NodeLink,
		NodeImage, NodeSoftBreak, NodeHardBreak, NodeHTMLInline:
		return true
	default:
		return false
	}
}

// HasChildren returns true if this node has any children.
func (n *Node) HasChildren() bool {
	return n.FirstChild != nil
}

// ChildCount returns the number of direct children.
func (n *Node) ChildCount() int {
	count := 0
	for child := n.FirstChild; child != nil; child = child.Next {
		count++
	}
	return count
}

// Children returns a slice of all direct children.
func (n *Node) Children() []*Node {
	var children []*Node
	for child := n.FirstChild; child != nil; child = child.Next {
		children = append(children, child)
	}
	return children
}
