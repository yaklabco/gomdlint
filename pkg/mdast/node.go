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

	// GFM extension nodes. They follow NodeRaw so existing values stay stable.

	// NodeStrikethrough is GFM strikethrough text (inline).
	NodeStrikethrough

	// NodeTable is a GFM table (block). Its children are NodeTableRow.
	NodeTable

	// NodeTableRow is a row of a GFM table, the header row included (block).
	// Its children are NodeTableCell.
	NodeTableRow

	// NodeTableCell is a cell of a GFM table row (block). Its children are
	// inline nodes.
	NodeTableCell
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
	// FirstToken <= LastToken whenever both are set; for parsed nodes they
	// are the tokens overlapping SourceSpan. Both are -1 for synthetic nodes.
	FirstToken int
	LastToken  int

	// SourceSpan is the node's full span: the exact source of the construct,
	// including its syntax markers. The parser sets it on every node; see
	// the package documentation for the contract per node kind. Nil (only
	// for hand-built nodes) derives the span from the token range.
	SourceSpan *SourceRange

	// ContentSpan is the part of the full span inside the node's markers,
	// such as the text of a heading or the code of a fenced code block.
	// Nil means the content span equals the full span.
	ContentSpan *SourceRange

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
		NodeBlockquote, NodeCodeBlock, NodeThematicBreak, NodeHTMLBlock,
		NodeTable, NodeTableRow, NodeTableCell:
		return true
	default:
		return false
	}
}

// IsInline returns true if this is an inline-level node.
func (n *Node) IsInline() bool {
	switch n.Kind {
	case NodeText, NodeEmphasis, NodeStrong, NodeCodeSpan, NodeLink,
		NodeImage, NodeSoftBreak, NodeHardBreak, NodeHTMLInline, NodeStrikethrough:
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
