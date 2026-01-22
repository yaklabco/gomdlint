package lint

import "github.com/jamesainslie/gomdlint/pkg/mdast"

// NodeCache provides pre-computed collections of AST nodes by type.
//
// # Purpose
//
// NodeCache dramatically improves lint performance by walking the AST once
// and caching nodes by type, rather than walking it repeatedly for each rule
// that needs a specific node type.
//
// Without caching, if 10 rules each call Headings(), the AST is walked 10 times.
// With caching, the AST is walked once, and all 10 rules share the result.
//
// # Performance Impact
//
// Profiling showed that mdast.Walk and mdast.FindAll accounted for ~24% of CPU
// time. With 40+ rules, many needing the same node types, this optimization
// reduces AST traversal from O(rules × nodes) to O(nodes).
//
// # Usage
//
// NodeCache is used internally by RuleContext. Rules access cached nodes via
// RuleContext methods:
//
//	func (r *MyRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
//	    // Fast: uses cached nodes, no AST walk
//	    headings := ctx.Headings()
//
//	    // Slow: walks AST (deprecated pattern)
//	    headings := lint.Headings(ctx.Root)
//	}
//
// # IMPORTANT: Do Not Mutate Returned Slices
//
// The slices returned by NodeCache methods are shared across all rules.
// Mutating them (sorting, appending, filtering in place) will corrupt
// the cache and cause incorrect behavior in other rules.
//
// WRONG:
//
//	headings := ctx.Headings()
//	sort.Slice(headings, ...) // CORRUPTS CACHE - affects other rules!
//
// RIGHT:
//
//	headings := ctx.Headings()
//	sorted := make([]*mdast.Node, len(headings))
//	copy(sorted, headings)
//	sort.Slice(sorted, ...) // Safe - working with a copy
//
// If you need to mutate a node slice, always copy it first.
//
// # Thread Safety
//
// NodeCache is NOT thread-safe. It is designed for single-threaded use within
// a RuleContext, where rules execute sequentially for a single file.
// File-level parallelism (multiple files linted concurrently) is safe because
// each file gets its own RuleContext and NodeCache.
//
// # Lazy Initialization
//
// The cache is built lazily on first access to any node type. This means:
//   - Files where no rules need node collections pay zero cache cost
//   - The first rule to request any node type pays the full cache build cost
//   - Subsequent rules get instant access
//
// # Supported Node Types
//
// The cache indexes the following node types:
//   - Headings (NodeHeading)
//   - Lists (NodeList)
//   - ListItems (NodeListItem)
//   - CodeBlocks (NodeCodeBlock)
//   - CodeSpans (NodeCodeSpan)
//   - Links (NodeLink)
//   - Images (NodeImage)
//   - Paragraphs (NodeParagraph)
//   - Blockquotes (NodeBlockquote)
//   - Tables (NodeTable)
//   - ThematicBreaks (NodeThematicBreak)
//   - HTMLBlocks (NodeHTMLBlock)
//   - HTMLInlines (NodeHTMLInline)
//   - Emphasis (NodeEmphasis)
//   - Strong (NodeStrong)
//
// Other node types can be accessed via FindByKind() which still walks the AST.
type NodeCache struct {
	// Core block elements
	headings       []*mdast.Node
	lists          []*mdast.Node
	listItems      []*mdast.Node
	codeBlocks     []*mdast.Node
	paragraphs     []*mdast.Node
	blockquotes    []*mdast.Node
	tables         []*mdast.Node
	thematicBreaks []*mdast.Node
	htmlBlocks     []*mdast.Node

	// Inline elements
	codeSpans   []*mdast.Node
	links       []*mdast.Node
	images      []*mdast.Node
	htmlInlines []*mdast.Node
	emphasis    []*mdast.Node
	strong      []*mdast.Node

	// Build state
	built bool
}

// newNodeCache creates an empty NodeCache.
// Call build() to populate it from an AST root.
// Initial capacity constants for pre-allocation based on typical document structure.
const (
	initCapHeadings      = 16
	initCapLists         = 8
	initCapListItems     = 32
	initCapCodeBlocks    = 8
	initCapParagraphs    = 32
	initCapBlockquotes   = 4
	initCapTables        = 4
	initCapThematicBreak = 4
	initCapHTMLBlocks    = 4
	initCapCodeSpans     = 16
	initCapLinks         = 16
	initCapImages        = 8
	initCapHTMLInlines   = 4
	initCapEmphasis      = 8
	initCapStrong        = 8
)

func newNodeCache() *NodeCache {
	return &NodeCache{}
}

// build walks the AST once and categorizes all nodes by type.
// This is O(n) where n is the total number of nodes in the document.
// After build(), all accessor methods return in O(1) time.
func (nc *NodeCache) build(root *mdast.Node) {
	if nc.built || root == nil {
		return
	}

	// Pre-allocate with reasonable initial capacities to reduce allocations.
	nc.headings = make([]*mdast.Node, 0, initCapHeadings)
	nc.lists = make([]*mdast.Node, 0, initCapLists)
	nc.listItems = make([]*mdast.Node, 0, initCapListItems)
	nc.codeBlocks = make([]*mdast.Node, 0, initCapCodeBlocks)
	nc.paragraphs = make([]*mdast.Node, 0, initCapParagraphs)
	nc.blockquotes = make([]*mdast.Node, 0, initCapBlockquotes)
	nc.tables = make([]*mdast.Node, 0, initCapTables)
	nc.thematicBreaks = make([]*mdast.Node, 0, initCapThematicBreak)
	nc.htmlBlocks = make([]*mdast.Node, 0, initCapHTMLBlocks)
	nc.codeSpans = make([]*mdast.Node, 0, initCapCodeSpans)
	nc.links = make([]*mdast.Node, 0, initCapLinks)
	nc.images = make([]*mdast.Node, 0, initCapImages)
	nc.htmlInlines = make([]*mdast.Node, 0, initCapHTMLInlines)
	nc.emphasis = make([]*mdast.Node, 0, initCapEmphasis)
	nc.strong = make([]*mdast.Node, 0, initCapStrong)

	// Single walk to categorize all nodes.
	//nolint:errcheck // Walk visitor never returns error in this usage
	mdast.Walk(root, func(node *mdast.Node) error {
		switch node.Kind {
		case mdast.NodeHeading:
			nc.headings = append(nc.headings, node)
		case mdast.NodeList:
			nc.lists = append(nc.lists, node)
		case mdast.NodeListItem:
			nc.listItems = append(nc.listItems, node)
		case mdast.NodeCodeBlock:
			nc.codeBlocks = append(nc.codeBlocks, node)
		case mdast.NodeParagraph:
			nc.paragraphs = append(nc.paragraphs, node)
		case mdast.NodeBlockquote:
			nc.blockquotes = append(nc.blockquotes, node)
		case mdast.NodeThematicBreak:
			nc.thematicBreaks = append(nc.thematicBreaks, node)
		case mdast.NodeHTMLBlock:
			nc.htmlBlocks = append(nc.htmlBlocks, node)
		case mdast.NodeCodeSpan:
			nc.codeSpans = append(nc.codeSpans, node)
		case mdast.NodeLink:
			nc.links = append(nc.links, node)
		case mdast.NodeImage:
			nc.images = append(nc.images, node)
		case mdast.NodeHTMLInline:
			nc.htmlInlines = append(nc.htmlInlines, node)
		case mdast.NodeEmphasis:
			nc.emphasis = append(nc.emphasis, node)
		case mdast.NodeStrong:
			nc.strong = append(nc.strong, node)
		default:
			// Tables are GFM extensions stored in Ext map
			if node.Ext != nil {
				if _, ok := node.Ext["table"]; ok {
					nc.tables = append(nc.tables, node)
				}
			}
		}
		return nil
	})

	nc.built = true
}

// Headings returns all heading nodes. Do not mutate the returned slice.
func (nc *NodeCache) Headings() []*mdast.Node {
	return nc.headings
}

// Lists returns all list nodes. Do not mutate the returned slice.
func (nc *NodeCache) Lists() []*mdast.Node {
	return nc.lists
}

// ListItems returns all list item nodes. Do not mutate the returned slice.
func (nc *NodeCache) ListItems() []*mdast.Node {
	return nc.listItems
}

// CodeBlocks returns all code block nodes. Do not mutate the returned slice.
func (nc *NodeCache) CodeBlocks() []*mdast.Node {
	return nc.codeBlocks
}

// Paragraphs returns all paragraph nodes. Do not mutate the returned slice.
func (nc *NodeCache) Paragraphs() []*mdast.Node {
	return nc.paragraphs
}

// Blockquotes returns all blockquote nodes. Do not mutate the returned slice.
func (nc *NodeCache) Blockquotes() []*mdast.Node {
	return nc.blockquotes
}

// Tables returns all table nodes. Do not mutate the returned slice.
func (nc *NodeCache) Tables() []*mdast.Node {
	return nc.tables
}

// ThematicBreaks returns all thematic break nodes. Do not mutate the returned slice.
func (nc *NodeCache) ThematicBreaks() []*mdast.Node {
	return nc.thematicBreaks
}

// HTMLBlocks returns all HTML block nodes. Do not mutate the returned slice.
func (nc *NodeCache) HTMLBlocks() []*mdast.Node {
	return nc.htmlBlocks
}

// CodeSpans returns all code span nodes. Do not mutate the returned slice.
func (nc *NodeCache) CodeSpans() []*mdast.Node {
	return nc.codeSpans
}

// Links returns all link nodes. Do not mutate the returned slice.
func (nc *NodeCache) Links() []*mdast.Node {
	return nc.links
}

// Images returns all image nodes. Do not mutate the returned slice.
func (nc *NodeCache) Images() []*mdast.Node {
	return nc.images
}

// HTMLInlines returns all inline HTML nodes. Do not mutate the returned slice.
func (nc *NodeCache) HTMLInlines() []*mdast.Node {
	return nc.htmlInlines
}

// Emphasis returns all emphasis nodes. Do not mutate the returned slice.
func (nc *NodeCache) Emphasis() []*mdast.Node {
	return nc.emphasis
}

// Strong returns all strong nodes. Do not mutate the returned slice.
func (nc *NodeCache) Strong() []*mdast.Node {
	return nc.strong
}
