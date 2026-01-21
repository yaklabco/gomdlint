package mdast

// BlockAttrs holds attributes for block-level nodes.
type BlockAttrs struct {
	// HeadingLevel is the heading level (1-6) for NodeHeading.
	HeadingLevel int

	// List holds list-specific attributes for NodeList.
	List *ListAttrs

	// CodeBlock holds code block attributes for NodeCodeBlock.
	CodeBlock *CodeBlockAttrs
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
	// FenceChar is the fence character ('`' or '~').
	FenceChar byte

	// FenceLength is the number of fence characters.
	FenceLength int

	// Info is the info string (language identifier, etc.).
	Info string

	// Indented is true for indented code blocks (vs fenced).
	Indented bool
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

	// ReferenceLabel is the label for reference-style links.
	// Empty for inline links and autolinks.
	ReferenceLabel string

	// ReferenceStyle indicates the syntax style used.
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
