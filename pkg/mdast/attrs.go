package mdast

// BlockAttrs holds attributes for block-level nodes.
type BlockAttrs struct {
	// HeadingLevel is the heading level (1-6) for NodeHeading.
	HeadingLevel int

	// List holds list-specific attributes for NodeList.
	List *ListAttrs

	// ListItem holds list item attributes for NodeListItem.
	ListItem *ListItemAttrs

	// CodeBlock holds code block attributes for NodeCodeBlock.
	CodeBlock *CodeBlockAttrs

	// Table holds GFM table attributes for NodeTable, NodeTableRow and
	// NodeTableCell.
	Table *TableAttrs
}

// Alignment is the alignment of a GFM table column.
type Alignment uint8

// Column alignments, set by the colons of a table's delimiter row.
const (
	// AlignNone has no colon: "---".
	AlignNone Alignment = iota
	// AlignLeft has a leading colon: ":--".
	AlignLeft
	// AlignCenter has colons on both sides: ":-:".
	AlignCenter
	// AlignRight has a trailing colon: "--:".
	AlignRight
)

// String returns the name of the alignment.
func (a Alignment) String() string {
	switch a {
	case AlignLeft:
		return "left"
	case AlignCenter:
		return "center"
	case AlignRight:
		return "right"
	default:
		return "none"
	}
}

// TableAttrs holds attributes for GFM table nodes. A NodeTable sets
// Alignments; a header NodeTableRow and its cells set Header; a
// NodeTableCell sets Alignment.
type TableAttrs struct {
	// Alignments holds the alignment of each column, from the delimiter row.
	Alignments []Alignment

	// Header is true for the header row and its cells.
	Header bool

	// Alignment is the alignment of a cell's column.
	Alignment Alignment
}

// ListItemAttrs holds attributes for list item nodes.
type ListItemAttrs struct {
	// ContentIndent is the width, in columns, from the start of the
	// enclosing container's content to the start of the item's content: the
	// marker's own indentation, the marker and the spaces after it. Lines
	// that continue the item are indented by at least this much.
	ContentIndent int
}

// ListAttrs holds attributes for list nodes.
type ListAttrs struct {
	// Ordered is true for ordered lists (1., 2., etc.).
	Ordered bool

	// BulletMarker is the bullet character used ("-", "+", "*").
	BulletMarker string

	// StartNumber is the starting number for ordered lists.
	StartNumber int

	// Delimiter is the delimiter for ordered lists ("." or ")").
	Delimiter string

	// Tight is true if this is a tight list (no blank lines between items).
	Tight bool
}

// CodeBlockAttrs holds attributes for code block nodes.
type CodeBlockAttrs struct {
	// FenceChar is the fence character ('`' or '~') of a fenced code block,
	// read from its opening fence.
	FenceChar byte

	// FenceLength is the number of characters in the opening fence.
	FenceLength int

	// Info is the info string (language identifier, etc.).
	Info string

	// Indented is true for indented code blocks (vs fenced).
	Indented bool

	// Closed is true when a fenced code block ends with a closing fence. The
	// closing fence is then the last line of the block's full span. An
	// unclosed block runs to the end of its container.
	Closed bool
}

// InlineAttrs holds attributes for inline-level nodes.
type InlineAttrs struct {
	// Text holds the text content for NodeText and NodeCodeSpan.
	Text []byte

	// Link holds link attributes for NodeLink and NodeImage.
	Link *LinkAttrs

	// EmphasisLevel indicates emphasis strength (1 for emphasis, 2 for strong).
	EmphasisLevel int
}

// ReferenceStyle indicates the syntax style of a link or image reference.
type ReferenceStyle uint8

const (
	// RefStyleInline represents inline links: [text](url) or ![alt](url).
	RefStyleInline ReferenceStyle = iota

	// RefStyleFull represents full reference links: [text][label] or ![alt][label].
	RefStyleFull

	// RefStyleCollapsed represents collapsed reference links: [label][] or ![label][].
	RefStyleCollapsed

	// RefStyleShortcut represents shortcut reference links: [label] or ![label].
	RefStyleShortcut

	// RefStyleAutolink represents autolinks: <https://example.com>.
	RefStyleAutolink
)

// String returns a human-readable name for the reference style.
func (s ReferenceStyle) String() string {
	switch s {
	case RefStyleInline:
		return "inline"
	case RefStyleFull:
		return "full"
	case RefStyleCollapsed:
		return "collapsed"
	case RefStyleShortcut:
		return "shortcut"
	case RefStyleAutolink:
		return "autolink"
	default:
		return "unknown"
	}
}

// LinkAttrs holds attributes for link and image nodes.
type LinkAttrs struct {
	// Destination is the link URL.
	Destination string

	// Title is the optional link title.
	Title string

	// ReferenceLabel is the label of a reference-style link as written in
	// the source, before normalization: the second bracket pair of a full
	// reference, or the link text of a collapsed or shortcut one.
	// Empty for inline links and autolinks.
	ReferenceLabel string

	// ReferenceStyle indicates the syntax style used, as read from the source.
	ReferenceStyle ReferenceStyle
}

// NewBlockAttrs creates a new BlockAttrs with default values.
func NewBlockAttrs() *BlockAttrs {
	return &BlockAttrs{}
}

// NewInlineAttrs creates a new InlineAttrs with default values.
func NewInlineAttrs() *InlineAttrs {
	return &InlineAttrs{}
}

// WithHeadingLevel sets the heading level and returns the BlockAttrs for chaining.
func (a *BlockAttrs) WithHeadingLevel(level int) *BlockAttrs {
	a.HeadingLevel = level
	return a
}

// WithList sets list attributes and returns the BlockAttrs for chaining.
func (a *BlockAttrs) WithList(attrs *ListAttrs) *BlockAttrs {
	a.List = attrs
	return a
}

// WithListItem sets list item attributes and returns the BlockAttrs for chaining.
func (a *BlockAttrs) WithListItem(attrs *ListItemAttrs) *BlockAttrs {
	a.ListItem = attrs
	return a
}

// WithTable sets table attributes and returns the BlockAttrs for chaining.
func (a *BlockAttrs) WithTable(attrs *TableAttrs) *BlockAttrs {
	a.Table = attrs
	return a
}

// WithCodeBlock sets code block attributes and returns the BlockAttrs for chaining.
func (a *BlockAttrs) WithCodeBlock(attrs *CodeBlockAttrs) *BlockAttrs {
	a.CodeBlock = attrs
	return a
}

// WithText sets the text content and returns the InlineAttrs for chaining.
func (a *InlineAttrs) WithText(text []byte) *InlineAttrs {
	a.Text = text
	return a
}

// WithLink sets link attributes and returns the InlineAttrs for chaining.
func (a *InlineAttrs) WithLink(attrs *LinkAttrs) *InlineAttrs {
	a.Link = attrs
	return a
}

// WithEmphasisLevel sets the emphasis level and returns the InlineAttrs for chaining.
func (a *InlineAttrs) WithEmphasisLevel(level int) *InlineAttrs {
	a.EmphasisLevel = level
	return a
}
