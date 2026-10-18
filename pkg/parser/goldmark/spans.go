package goldmark

import (
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// blockExtent records where a block opened and the last source line it owns.
// Offsets are relative to the source goldmark parsed.
type blockExtent struct {
	// start is the offset of the block's first byte: its marker, or its first
	// non-blank byte when it has no marker. Indented code starts at its
	// indentation.
	start int

	// lastLine is an offset on the last line the block owns.
	lastLine int

	// closed records that the block consumed a closing line, such as the
	// closing fence of fenced code.
	closed bool

	// contentStart is the offset where a list item's content starts on its
	// first line, after the marker and the spaces the item claims. A tab
	// the item claims only in part belongs to the content. It is -1 for
	// other blocks and for an item whose first line holds only its marker.
	contentStart int
}

// inlineExtent records the source range an inline parser consumed.
type inlineExtent struct {
	start, end int

	// contentStart and contentEnd delimit the label of links and images.
	contentStart, contentEnd int
}

// tableRows records the paragraph lines a GFM table was built from: the
// header row, the delimiter row and every body row, in source order.
type tableRows struct {
	lines []text.Segment
}

// spanRecorder collects source positions while goldmark parses one document.
// goldmark parsers are shared between concurrent parses, so all state lives
// here and the recorder lives in the parser.Context of a single parse.
type spanRecorder struct {
	blocks  map[ast.Node]*blockExtent
	inlines map[ast.Node]inlineExtent
	tables  map[ast.Node]tableRows

	// labels is a stack of open link label start offsets. It mirrors the
	// link parser's own label list so a closing bracket finds its opener in
	// constant time.
	labels []int
}

func newSpanRecorder() *spanRecorder {
	return &spanRecorder{
		blocks:  make(map[ast.Node]*blockExtent),
		inlines: make(map[ast.Node]inlineExtent),
		tables:  make(map[ast.Node]tableRows),
	}
}

// spanContextKey stores the per-parse spanRecorder in a parser.Context.
// goldmark's NewContextKey increments a package counter that NewContext
// reads without synchronization, so the key is allocated once, while the
// package initializes and before any parse can start.
var spanContextKey = parser.NewContextKey() //nolint:gochecknoglobals // goldmark context keys must exist before any parse starts.

// spanKey locates the per-parse spanRecorder in a parser.Context. Parse sets
// it and the wrapped parsers read it.
type spanKey struct {
	key parser.ContextKey
}

// recorder returns the spanRecorder of the current parse, or nil when the
// parse was started without one.
func (k spanKey) recorder(pc parser.Context) *spanRecorder {
	if pc == nil {
		return nil
	}
	rec, ok := pc.Get(k.key).(*spanRecorder)
	if !ok {
		return nil
	}
	return rec
}

// openBlock records the opening line of a block. segment is the reader's
// view of the line before the block parser consumed anything.
func (r *spanRecorder) openBlock(node ast.Node, source []byte, segment text.Segment) {
	start := segment.Start
	if _, indented := node.(*ast.CodeBlock); !indented {
		start = firstNonBlank(source, segment.Start, segment.Stop)
	}
	r.blocks[node] = &blockExtent{start: start, lastLine: segment.Start, contentStart: -1}
}

// openContent records where a list item's content starts on its first
// line. after is the reader's position once the item has consumed its
// marker and the spaces after it. goldmark measures those spaces from where
// the item starts rather than from the start of the line, so the item's
// content indentation does not locate its content on a line with tabs.
// When the item claims only part of a tab, the reader is past the tab and
// its padding holds the columns left to the content.
func (r *spanRecorder) openContent(node ast.Node, source []byte, after text.Segment) {
	extent, ok := r.blocks[node]
	if !ok {
		return
	}
	start := after.Start
	if after.Padding > 0 && start > 0 && source[start-1] == '\t' {
		start--
	}
	extent.contentStart = start
}

// ownLine records that a block owns the line containing offset.
func (r *spanRecorder) ownLine(node ast.Node, offset int) {
	if extent, ok := r.blocks[node]; ok && offset > extent.lastLine {
		extent.lastLine = offset
	}
}

// firstNonBlank returns the offset of the first byte in [from, to) that is
// not a space or tab, or from when there is none.
func firstNonBlank(source []byte, from, to int) int {
	to = min(to, len(source))
	for idx := from; idx < to; idx++ {
		if source[idx] != ' ' && source[idx] != '\t' {
			return idx
		}
	}
	return from
}

// sourceSpans is a parser option that records source positions. It wraps
// every block parser, inline parser and paragraph transformer configured at
// the time it is applied, so it must be added after all extensions.
type sourceSpans struct {
	spanKey
}

func (s sourceSpans) SetParserOption(cfg *parser.Config) {
	for idx := range cfg.BlockParsers {
		if bp, ok := cfg.BlockParsers[idx].Value.(parser.BlockParser); ok {
			cfg.BlockParsers[idx].Value = &spanBlockParser{BlockParser: bp, spanKey: s.spanKey}
		}
	}
	linkParser := parser.NewLinkParser()
	for idx := range cfg.InlineParsers {
		inline, ok := cfg.InlineParsers[idx].Value.(parser.InlineParser)
		if !ok {
			continue
		}
		if inline == linkParser {
			cfg.InlineParsers[idx].Value = &spanLinkParser{InlineParser: inline, spanKey: s.spanKey}
			continue
		}
		cfg.InlineParsers[idx].Value = &spanInlineParser{InlineParser: inline, spanKey: s.spanKey}
	}
	for idx := range cfg.ParagraphTransformers {
		if pt, ok := cfg.ParagraphTransformers[idx].Value.(parser.ParagraphTransformer); ok {
			cfg.ParagraphTransformers[idx].Value = &spanParagraphTransformer{ParagraphTransformer: pt, spanKey: s.spanKey}
		}
	}
}

// forwardOption passes a parser option to a wrapped parser that accepts it.
func forwardOption(inner any, name parser.OptionName, value any) {
	if setter, ok := inner.(parser.SetOptioner); ok {
		setter.SetOption(name, value)
	}
}

// spanBlockParser records where a block opens and which lines it owns.
type spanBlockParser struct {
	parser.BlockParser
	spanKey
}

func (p *spanBlockParser) SetOption(name parser.OptionName, value any) {
	forwardOption(p.BlockParser, name, value)
}

func (p *spanBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	_, segment := reader.PeekLine()
	node, state := p.BlockParser.Open(parent, reader, pc)
	if node != nil {
		if rec := p.recorder(pc); rec != nil {
			rec.openBlock(node, reader.Source(), segment)
			if _, item := node.(*ast.ListItem); item && state&parser.HasChildren != 0 {
				_, after := reader.Position()
				rec.openContent(node, reader.Source(), after)
			}
		}
	}
	return node, state
}

func (p *spanBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, segment := reader.PeekLine()
	state := p.BlockParser.Continue(node, reader, pc)
	rec := p.recorder(pc)
	if rec == nil || line == nil {
		return state
	}
	if state&parser.Continue != 0 {
		// A block owns the lines it continues on, except blank ones: blank
		// lines between list items or after a block belong to no block.
		// Fenced code owns blank lines because they are part of its content.
		_, fenced := node.(*ast.FencedCodeBlock)
		if fenced || !util.IsBlank(line) {
			rec.ownLine(node, segment.Start)
		}
		return state
	}
	// A parser may consume a closing line and then close, like the closing
	// fence of fenced code or the last line of an HTML block.
	_, after := reader.Position()
	if after.Start > segment.Start && !util.IsBlank(reader.Source()[segment.Start:min(after.Start, len(reader.Source()))]) {
		rec.ownLine(node, segment.Start)
		if extent, ok := rec.blocks[node]; ok {
			extent.closed = true
		}
	}
	return state
}

// spanInlineParser records the source consumed by inline parsers whose nodes
// carry no complete source segment of their own.
type spanInlineParser struct {
	parser.InlineParser
	spanKey
}

func (p *spanInlineParser) SetOption(name parser.OptionName, value any) {
	forwardOption(p.InlineParser, name, value)
}

func (p *spanInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	_, before := block.Position()
	node := p.InlineParser.Parse(parent, block, pc)
	if node == nil {
		return nil
	}
	switch node.(type) {
	case *ast.CodeSpan, *ast.AutoLink, *ast.RawHTML, *east.TaskCheckBox:
		if rec := p.recorder(pc); rec != nil {
			_, after := block.Position()
			rec.inlines[node] = inlineExtent{start: before.Start, end: after.Start}
		}
	}
	return node
}

func (p *spanInlineParser) CloseBlock(parent ast.Node, block text.Reader, pc parser.Context) {
	if closer, ok := p.InlineParser.(parser.CloseBlocker); ok {
		closer.CloseBlock(parent, block, pc)
	}
}

// spanLinkParser records the full source of links and images. goldmark's
// link parser opens a label at "[" or "![" and builds the link at the
// matching "]", so the recorder keeps a stack of open labels.
type spanLinkParser struct {
	parser.InlineParser
	spanKey
}

func (p *spanLinkParser) SetOption(name parser.OptionName, value any) {
	forwardOption(p.InlineParser, name, value)
}

func (p *spanLinkParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	rec := p.recorder(pc)
	line, segment := block.PeekLine()
	if rec == nil || len(line) == 0 {
		return p.InlineParser.Parse(parent, block, pc)
	}
	if line[0] != ']' {
		node := p.InlineParser.Parse(parent, block, pc)
		if node != nil {
			rec.labels = append(rec.labels, segment.Start)
		}
		return node
	}
	// Every closing bracket consumes the most recent open label, whether or
	// not it forms a link.
	opener := -1
	if count := len(rec.labels); count > 0 {
		opener = rec.labels[count-1]
		rec.labels = rec.labels[:count-1]
	}
	node := p.InlineParser.Parse(parent, block, pc)
	if node == nil || opener < 0 {
		return node
	}
	_, after := block.Position()
	contentStart := opener + 1
	if _, image := node.(*ast.Image); image {
		contentStart++
	}
	rec.inlines[node] = inlineExtent{
		start:        opener,
		end:          after.Start,
		contentStart: contentStart,
		contentEnd:   segment.Start,
	}
	return node
}

func (p *spanLinkParser) CloseBlock(parent ast.Node, block text.Reader, pc parser.Context) {
	if rec := p.recorder(pc); rec != nil {
		rec.labels = rec.labels[:0]
	}
	if closer, ok := p.InlineParser.(parser.CloseBlocker); ok {
		closer.CloseBlock(parent, block, pc)
	}
}

// spanParagraphTransformer records the source lines of tables built from
// paragraphs, since goldmark's table rows carry no line segments, and gives
// such a table the blank-line flag of the paragraph it replaces, which
// goldmark leaves out.
type spanParagraphTransformer struct {
	parser.ParagraphTransformer
	spanKey
}

func (t *spanParagraphTransformer) SetOption(name parser.OptionName, value any) {
	forwardOption(t.ParagraphTransformer, name, value)
}

func (t *spanParagraphTransformer) Transform(node *ast.Paragraph, reader text.Reader, pc parser.Context) {
	rec := t.recorder(pc)
	if rec == nil {
		t.ParagraphTransformer.Transform(node, reader, pc)
		return
	}
	before := node.Lines()
	lines := make([]text.Segment, before.Len())
	for idx := range lines {
		lines[idx] = before.At(idx)
	}
	parent, prev := node.Parent(), node.PreviousSibling()
	blank := node.HasBlankPreviousLines()

	t.ParagraphTransformer.Transform(node, reader, pc)

	var next ast.Node
	remaining := 0
	switch {
	case node.Parent() != nil:
		next = node.NextSibling()
		remaining = node.Lines().Len()
	case prev != nil:
		next = prev.NextSibling()
	case parent != nil:
		next = parent.FirstChild()
	}
	table, ok := next.(*east.Table)
	if !ok || remaining >= len(lines) {
		return
	}
	// GFM builds the table without the paragraph's blank-line flag, so a
	// list whose item gains a blank line before a table would still read as
	// tight. A table that takes the paragraph's first line follows what the
	// paragraph followed, unless link reference definitions came off the
	// paragraph's start: the first of them took the flag, and the table
	// follows the last of them directly. goldmark moves the paragraph's
	// position to the first line it keeps, so the paragraph's first line is
	// the one it opened on, as recorded then.
	if opened, found := rec.blocks[node]; found && remaining == 0 && len(lines) > 0 &&
		sameLine(reader.Source(), opened.start, lines[0].Start) {
		table.SetBlankPreviousLines(blank)
	}
	if _, seen := rec.tables[table]; seen {
		return
	}
	rec.tables[table] = tableRows{lines: lines[remaining:]}
}

// sameLine reports whether two offsets of source lie on one line.
func sameLine(source []byte, first, second int) bool {
	if first < 0 || second < 0 || first > len(source) || second > len(source) {
		return false
	}
	if first > second {
		first, second = second, first
	}
	for _, ch := range source[first:second] {
		if ch == '\n' || ch == '\r' {
			return false
		}
	}
	return true
}
