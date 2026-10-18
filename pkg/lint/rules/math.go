package rules

import (
	"bytes"
	"cmp"
	"slices"
	"sort"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// mathRegions holds the math that markdownlint 0.41 reads in a document,
// which parses math with micromark-extension-math while goldmark reads
// none: the lines of math blocks and the source ranges of inline math.
type mathRegions struct {
	lines  map[int]bool
	ranges []mdast.SourceRange // Sorted and apart.
}

// findMath finds the math in a document the way micromark-extension-math
// reads it. A math block opens with a line of two or more dollar signs and
// text without a dollar sign, after its container prefix and at most three
// columns of space, where a block may start (see mathOpening), and runs to
// a line of at least as many dollar signs, or to the end of its container;
// it may hold blank lines. Inline math runs from a run of dollar signs to
// the next run of the same length in the same block, like a code span, and
// a dollar sign that a backslash escapes, or that is code, raw HTML or a
// link destination, opens none. Code and HTML blocks hold no math.
func findMath(ctx *lint.RuleContext) mathRegions {
	math := mathRegions{lines: make(map[int]bool)}
	if ctx.File == nil || ctx.Root == nil || !bytes.Contains(ctx.File.Content, []byte("$")) {
		return math
	}
	math.findBlocks(ctx)
	math.findInline(ctx)
	return math
}

// findBlocks records the lines of math blocks.
func (m *mathRegions) findBlocks(ctx *lint.RuleContext) {
	file := ctx.File
	htmlLines := htmlBlockLines(ctx)
	tableLines := nodeLines(ctx.Tables())
	var scan *blockScan // Built at the first line that needs it.
	var item *virtualItem
	for line := file.FrontMatterLines + 1; line <= len(file.Lines); line++ {
		if ctx.IsLineInCodeBlock(line) || htmlLines[line] {
			item = nil
			continue
		}
		hasFence := bytes.Contains(lint.RawLineContent(file, line), []byte("$$"))
		// micromark starts a list where goldmark reads paragraph or table
		// text right after a math block, which leaves nothing open, and in
		// a table, which a list ends.
		listMayStart := m.lines[line-1] || tableLines[line]
		if !hasFence && !listMayStart && item == nil {
			continue
		}
		if scan == nil {
			scan = newBlockScan(ctx, m)
		}
		ended := false
		if item != nil && !item.holds(file, line, scan.continuations) {
			// A line that ends the item may start the next one.
			item, ended = nil, true
		}
		if item == nil && (listMayStart || ended) {
			item = startItem(file, scan.containers, line)
		}
		if !hasFence {
			continue
		}
		// A container that goldmark reads from this line on, inside the
		// item, holds the fence; otherwise the item does.
		container, offset, markerLine := scan.containers.at(line), 0, false
		if item != nil && (container == nil || container.SourcePosition().StartLine != line) {
			container, offset, markerLine = item.container, item.col, item.line == line
		}
		fence, ok := mathOpening(file, container, line, offset, markerLine)
		if !ok {
			continue
		}
		closing := mathBlockEnd(file, container, line, fence, offset)
		for covered := line; covered <= closing; covered++ {
			m.lines[covered] = true
		}
		if item != nil {
			// Math leaves nothing open for a lazy line to continue.
			item.open = false
		}
		line = closing
	}
}

// blockScan holds what findBlocks reads about the blocks of a document:
// the containers micromark reads, and the lines that continue a paragraph
// or table that goldmark reads from an earlier line.
type blockScan struct {
	containers    *micromarkContainers
	continuations map[int]bool
}

// newBlockScan builds the scan of ctx's file, reading math from math.
func newBlockScan(ctx *lint.RuleContext, math *mathRegions) *blockScan {
	scan := &blockScan{containers: newMicromarkContainers(ctx, math), continuations: make(map[int]bool)}
	for _, nodes := range [][]*mdast.Node{ctx.Paragraphs(), ctx.Tables()} {
		for _, node := range nodes {
			if pos := node.SourcePosition(); pos.IsValid() {
				for line := pos.StartLine + 1; line <= pos.EndLine; line++ {
					scan.continuations[line] = true
				}
			}
		}
	}
	return scan
}

// mathOpening returns the length of the fence that opens a math block on
// line, inside container, which micromark reads around it, or outside
// every container when container is nil, with the block's lines offset
// columns past the container's content. The fence follows the container's
// prefix, offset columns and at most three more columns of space, as
// micromark-extension-math reads it. With more space the line is paragraph
// text or indented code. A fence may interrupt a paragraph, as in
// micromark. On markerLine, the first line of an item that micromark
// starts where goldmark reads text, the fence follows the item's marker.
func mathOpening(file *mdast.FileSnapshot, container *mdast.Node, line, offset int, markerLine bool) (int, bool) {
	indent, start, ok := lint.IndentAfterContainers(file, container, line)
	if !ok {
		return 0, false
	}
	raw := lint.RawLineContent(file, line)
	if markerLine {
		_, after, isItem := listItemContent(raw, start, indent)
		if !isItem {
			return 0, false
		}
		return mathFenceOpening(raw[after:])
	}
	if indent < offset || indent-offset > maxFenceIndent {
		return 0, false
	}
	return mathFenceOpening(raw[start:])
}

// virtualItem is a list item that micromark starts on line, where goldmark
// reads paragraph or table text: in container, with its content col
// columns past the container's content. open tells whether its last line
// holds paragraph text, which a lazy line may continue.
type virtualItem struct {
	container *mdast.Node
	col, line int
	open      bool
}

// startItem returns the list item that micromark starts on line, where
// goldmark reads text, or nil when the line holds no list item marker
// where a block may start, or goldmark reads the item too.
func startItem(file *mdast.FileSnapshot, containers *micromarkContainers, line int) *virtualItem {
	container := containers.at(line)
	if container != nil && container.Kind == mdast.NodeListItem && container.SourcePosition().StartLine == line {
		return nil
	}
	indent, start, ok := lint.IndentAfterContainers(file, container, line)
	if !ok || indent > maxFenceIndent {
		return nil
	}
	col, _, isItem := listItemContent(lint.RawLineContent(file, line), start, indent)
	if !isItem {
		return nil
	}
	return &virtualItem{container: container, col: col, line: line, open: true}
}

// holds reports whether the item goes on to line: the line continues the
// item's container and is blank or indented to the item's content, or it
// continues lazily the paragraph of the item's last line, as a line that
// goldmark reads as the continuation of a paragraph or table
// (continuations) and that opens no math and no list item. holds also
// records whether the item's paragraph stays open.
func (v *virtualItem) holds(file *mdast.FileSnapshot, line int, continuations map[int]bool) bool {
	if line == v.line {
		return true
	}
	raw := lint.RawLineContent(file, line)
	container := v.container
	if container != nil && !lint.ContinuesContainers(file, container, line) {
		// A lazy line, judged from the start of the line.
		container = nil
	}
	indent, start, ok := lint.IndentAfterContainers(file, container, line)
	if !ok {
		return false
	}
	if container == v.container {
		switch {
		case len(bytes.TrimSpace(raw[start:])) == 0:
			v.open = false
			return true
		case indent >= v.col:
			v.open = true
			return true
		}
	}
	if !v.open || !continuations[line] {
		return false
	}
	if indent > maxFenceIndent {
		return true
	}
	// A fence or a list item marker starts a block, not a lazy line.
	_, opens := mathFenceOpening(raw[start:])
	_, _, isItem := listItemContent(raw, start, indent)
	return !opens && !isItem
}

// nodeLines returns the 1-based lines that nodes span.
func nodeLines(nodes []*mdast.Node) map[int]bool {
	lines := make(map[int]bool)
	for _, node := range nodes {
		if pos := node.SourcePosition(); pos.IsValid() {
			for line := pos.StartLine; line <= pos.EndLine; line++ {
				lines[line] = true
			}
		}
	}
	return lines
}

// listItemContent reads a list item marker at raw[start:], indent columns
// past its container's content, and returns the column past that content
// where the item's content starts, and the offset in raw of its first
// byte: after the marker and one to four columns of space, or one column
// after the marker when more space follows. Tabs stop at every fourth
// column of the line.
func listItemContent(raw []byte, start, indent int) (int, int, bool) {
	text := raw[start:]
	marker := 0
	switch {
	case len(text) > 0 && (text[0] == '-' || text[0] == '+' || text[0] == '*'):
		marker = 1
	default:
		for marker < len(text) && marker < maxOrderedDigits && text[marker] >= '0' && text[marker] <= '9' {
			marker++
		}
		if marker == 0 || marker >= len(text) || (text[marker] != '.' && text[marker] != ')') {
			return 0, 0, false
		}
		marker++
	}
	markerCol := lineColumn(raw, start)
	base := markerCol - indent
	afterMarker := markerCol + marker
	col, pos := afterMarker, start+marker
	for pos < len(raw) && (raw[pos] == ' ' || raw[pos] == '\t') {
		if raw[pos] == '\t' {
			col += tabStop - col%tabStop
		} else {
			col++
		}
		pos++
	}
	if pos == start+marker || pos == len(raw) {
		return 0, 0, false
	}
	if col-afterMarker > tabStop {
		return afterMarker + 1 - base, start + marker + 1, true
	}
	return col - base, pos, true
}

// lineColumn returns the column where raw[offset] starts in its line, with
// tabs that stop at every fourth column and a byte order mark that takes
// none.
func lineColumn(raw []byte, offset int) int {
	col, pos := 0, 0
	if bytes.HasPrefix(raw, []byte(mdast.ByteOrderMark)) {
		pos = len(mdast.ByteOrderMark)
	}
	for ; pos < offset && pos < len(raw); pos++ {
		if raw[pos] == '\t' {
			col += tabStop - col%tabStop
		} else {
			col++
		}
	}
	return col
}

// maxOrderedDigits is the most digits an ordered list marker has.
const maxOrderedDigits = 9

// micromarkContainers tells which blockquote or list item micromark reads
// around a line: goldmark's innermost container, without the containers
// the line does not continue explicitly, and without a list item that
// micromark does not start (refuses).
type micromarkContainers struct {
	file    *mdast.FileSnapshot
	holders []*mdast.Node
	math    *mathRegions // The math found above the line asked about.
	refused map[*mdast.Node]bool
}

// newMicromarkContainers builds the containers of ctx's file, reading math
// from math.
func newMicromarkContainers(ctx *lint.RuleContext, math *mathRegions) *micromarkContainers {
	return &micromarkContainers{
		file:    ctx.File,
		holders: innermostContainers(ctx),
		math:    math,
		refused: make(map[*mdast.Node]bool),
	}
}

// at returns the innermost container micromark reads around line, or nil
// when there is none.
func (c *micromarkContainers) at(line int) *mdast.Node {
	container := c.holders[line]
	for container != nil && (c.inRefusedItem(container) || !lint.ContinuesContainers(c.file, container, line)) {
		container = enclosingContainer(container)
	}
	return container
}

// inRefusedItem reports whether node, a blockquote or list item, is or lies
// in a list item that micromark does not start.
func (c *micromarkContainers) inRefusedItem(node *mdast.Node) bool {
	for ; node != nil; node = enclosingContainer(node) {
		if node.Kind == mdast.NodeListItem && c.isRefused(node) {
			return true
		}
	}
	return false
}

// isRefused reports, once for each item, whether micromark does not start
// list item (see refuses).
func (c *micromarkContainers) isRefused(item *mdast.Node) bool {
	refused, known := c.refused[item]
	if !known {
		refused = c.refuses(item)
		c.refused[item] = refused
	}
	return refused
}

// refuses reports whether micromark does not start list item, which
// goldmark reads. micromark lets a list start after an open paragraph,
// indented code block or link reference definition only with a bullet or
// the marker "1." or "1)", and it reads what came before the outermost
// container that starts on the item's line, so a blockquote that starts on
// the line opens nothing new ("Intro\n> 3. $$" is text). Indented code
// stays open across blank lines. After any other block micromark starts
// the list, and it starts an item after another of its list. After an item
// it does not start, whose text continues the paragraph, it starts one
// only as it would after paragraph text, or after indented code when that
// item ends in indented code.
func (c *micromarkContainers) refuses(item *mdast.Node) bool {
	list := item.Parent
	if list == nil || startsListAfterText(c.file.Content, item) {
		return false
	}
	before := item.SourcePosition().StartLine - 1
	if before <= c.file.FrontMatterLines || c.math.lines[before] {
		return false
	}
	if list.FirstChild != item {
		prev := item.Prev
		return prev != nil && c.isRefused(prev) &&
			(c.holdsTextIn(enclosingContainer(item), before) || endsInIndentedCode(prev))
	}
	outer := list
	for outer.Parent != nil && outer.Parent.Kind != mdast.NodeDocument &&
		outer.Parent.SourcePosition().StartLine == before+1 {
		outer = outer.Parent
	}
	prev := outer.Prev
	switch {
	case prev != nil && prev.Kind == mdast.NodeCodeBlock && !lint.IsFencedCodeBlock(prev):
		return true
	case prev != nil && prev.SourcePosition().EndLine == before:
		return prev.Kind == mdast.NodeParagraph
	default:
		// Text on the line before that no block holds is a link reference
		// definition, which micromark reads as open content.
		return c.holdsTextIn(enclosingContainer(outer), before)
	}
}

// endsInIndentedCode reports whether the last block of list item is an
// indented code block, which micromark keeps open across blank lines.
func endsInIndentedCode(item *mdast.Node) bool {
	last := item.LastChild
	return last != nil && last.Kind == mdast.NodeCodeBlock && !lint.IsFencedCodeBlock(last)
}

// holdsTextIn reports whether line holds text past the prefix of
// container, or does not continue container explicitly.
func (c *micromarkContainers) holdsTextIn(container *mdast.Node, line int) bool {
	_, at, ok := lint.IndentAfterContainers(c.file, container, line)
	return !ok || holdsText(lint.RawLineContent(c.file, line)[at:])
}

// startsListAfterText reports whether micromark lets list item, the first
// of its list, start the list after paragraph text: it is a bullet item or
// one marked "1." or "1)", the single digit 1. The item's span starts at
// its marker.
func startsListAfterText(content []byte, item *mdast.Node) bool {
	at := item.SourceRange().StartOffset
	if at < 0 || at >= len(content) {
		return false
	}
	switch content[at] {
	case '-', '+', '*':
		return true
	case '1':
		return at+1 < len(content) && (content[at+1] == '.' || content[at+1] == ')')
	default:
		return false
	}
}

// holdsText reports whether a line, without its line ending, holds anything
// but spaces, tabs and blockquote markers. A marker follows at most three
// columns of space, or four after another marker, whose one optional space
// they include.
func holdsText(text []byte) bool {
	col, pos, limit := 0, 0, maxContainerIndent
	for {
		start := col
		for pos < len(text) && (text[pos] == ' ' || text[pos] == '\t') {
			if text[pos] == '\t' {
				col += tabStop - col%tabStop
			} else {
				col++
			}
			pos++
		}
		if pos >= len(text) || text[pos] != '>' || col-start > limit {
			break
		}
		pos++
		col++
		limit = maxContainerIndent + 1
	}
	return len(bytes.Trim(text[pos:], " \t")) > 0
}

// mathBlockEnd returns the last line of a math block whose fence, fence
// dollar signs long, opens on line opening inside container, or outside
// every container when container is nil, with its lines offset columns
// past the container's content: the line of its closing fence, or the line
// before the first line that does not continue the container explicitly,
// or that holds text less than offset columns in, since
// micromark-extension-math allows no lazy line, or the last line of the
// document.
//
// The closing fence follows the container's own prefix, offset columns and
// at most three more columns of space, as the opening fence does. No
// container starts inside math, so a fence after other blockquote or list
// markers, or after four columns of space, is math.
func mathBlockEnd(file *mdast.FileSnapshot, container *mdast.Node, opening, fence, offset int) int {
	for next := opening + 1; next <= len(file.Lines); next++ {
		if container != nil && !lint.ContinuesContainers(file, container, next) {
			return next - 1
		}
		indent, start, ok := lint.IndentAfterContainers(file, container, next)
		rest := lint.RawLineContent(file, next)[start:]
		if !ok || (indent < offset && len(bytes.TrimSpace(rest)) > 0) {
			return next - 1
		}
		if indent >= offset && indent-offset <= maxFenceIndent && mathFenceCloses(rest, fence) {
			return next
		}
	}
	return len(file.Lines)
}

// Columns of space: a tab stop, and the most before a fence.
const (
	tabStop        = 4
	maxFenceIndent = 3
)

// enclosingContainer returns the blockquote or list item around node, or
// nil when there is none.
func enclosingContainer(node *mdast.Node) *mdast.Node {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeBlockquote || parent.Kind == mdast.NodeListItem {
			return parent
		}
	}
	return nil
}

// mathFenceOpening returns the length of the dollar sign run that opens a
// math block on a line, after its container prefix: two or more dollar
// signs, then text that holds no dollar sign.
func mathFenceOpening(text []byte) (int, bool) {
	run := dollarRun(text, 0)
	if run < 2 || bytes.IndexByte(text[run:], '$') >= 0 {
		return 0, false
	}
	return run, true
}

// mathFenceCloses reports whether a line, after its container prefix, closes
// a math block opened by fence dollar signs: at least as many dollar signs,
// then only spaces and tabs.
func mathFenceCloses(text []byte, fence int) bool {
	run := dollarRun(text, 0)
	return run >= fence && len(bytes.Trim(text[run:], " \t")) == 0
}

// dollarRun returns how many dollar signs follow one another in text from
// offset on.
func dollarRun(text []byte, offset int) int {
	count := 0
	for offset+count < len(text) && text[offset+count] == '$' {
		count++
	}
	return count
}

// innermostContainers returns, for each line, the innermost blockquote or
// list item whose span holds it, or nil. The containers are written from
// the longest to the shortest, and from the outermost to the innermost
// among equally long ones, so an inner one overwrites the outer ones, in
// time that grows with the lines the containers hold.
func innermostContainers(ctx *lint.RuleContext) []*mdast.Node {
	type heldLines struct {
		node               *mdast.Node
		first, last, depth int
	}
	var spans []heldLines
	for _, containers := range [][]*mdast.Node{ctx.Blockquotes(), ctx.ListItems()} {
		for _, container := range containers {
			if pos := container.SourcePosition(); pos.IsValid() {
				depth := 0
				for parent := container.Parent; parent != nil; parent = parent.Parent {
					depth++
				}
				spans = append(spans, heldLines{container, pos.StartLine, pos.EndLine, depth})
			}
		}
	}
	slices.SortFunc(spans, func(a, b heldLines) int {
		return cmp.Or(cmp.Compare(b.last-b.first, a.last-a.first), cmp.Compare(a.depth, b.depth))
	})
	holders := make([]*mdast.Node, len(ctx.File.Lines)+1)
	for _, span := range spans {
		for line := max(span.first, 1); line <= span.last && line < len(holders); line++ {
			holders[line] = span.node
		}
	}
	return holders
}

// findInline records the inline math of every paragraph, heading and table
// cell outside math blocks.
func (m *mathRegions) findInline(ctx *lint.RuleContext) {
	skips := mathSkipRanges(ctx)
	blocks := make([]*mdast.Node, 0, len(ctx.Paragraphs())+len(ctx.Headings()))
	blocks = append(blocks, ctx.Paragraphs()...)
	blocks = append(blocks, ctx.Headings()...)
	for _, table := range ctx.Tables() {
		blocks = append(blocks, mdast.FindByKind(table, mdast.NodeTableCell)...)
	}
	for _, block := range blocks {
		span := block.SourceRange()
		if span.EndOffset <= span.StartOffset || span.EndOffset > len(ctx.File.Content) ||
			m.lines[block.SourcePosition().StartLine] {
			continue
		}
		m.ranges = append(m.ranges, inlineMath(ctx.File.Content, span, skips)...)
	}
	slices.SortFunc(m.ranges, func(a, b mdast.SourceRange) int { return cmp.Compare(a.StartOffset, b.StartOffset) })
}

// mathSkipRanges returns the sorted source ranges where no inline math can
// open: code spans, raw HTML, autolinks and the destinations of links and
// images.
func mathSkipRanges(ctx *lint.RuleContext) []mdast.SourceRange {
	var skips []mdast.SourceRange
	for _, nodes := range [][]*mdast.Node{ctx.CodeSpans(), ctx.HTMLInlines()} {
		for _, node := range nodes {
			skips = append(skips, node.SourceRange())
		}
	}
	for _, nodes := range [][]*mdast.Node{ctx.Links(), ctx.Images()} {
		for _, node := range nodes {
			full := node.SourceRange()
			if node.Inline != nil && node.Inline.Link != nil && node.Inline.Link.ReferenceStyle == mdast.RefStyleAutolink {
				skips = append(skips, full)
				continue
			}
			if label := node.ContentRange(); label.StartOffset > full.StartOffset && label.EndOffset < full.EndOffset {
				skips = append(skips, mdast.SourceRange{StartOffset: label.EndOffset, EndOffset: full.EndOffset})
			}
		}
	}
	slices.SortFunc(skips, func(a, b mdast.SourceRange) int { return cmp.Compare(a.StartOffset, b.StartOffset) })
	return skips
}

// inlineMath returns the ranges of inline math in content[span]: from a run
// of dollar signs that is not escaped and lies outside skips to the next run
// of the same length. Inside math nothing is escaped, as in micromark.
func inlineMath(content []byte, span mdast.SourceRange, skips []mdast.SourceRange) []mdast.SourceRange {
	var ranges []mdast.SourceRange
	skip := sort.Search(len(skips), func(idx int) bool { return skips[idx].EndOffset > span.StartOffset })
	for pos := span.StartOffset; pos < span.EndOffset; {
		for skip < len(skips) && skips[skip].EndOffset <= pos {
			skip++
		}
		if skip < len(skips) && skips[skip].StartOffset <= pos {
			pos = skips[skip].EndOffset
			continue
		}
		switch content[pos] {
		case '\\':
			if pos+1 < span.EndOffset && mdscan.IsASCIIPunct(content[pos+1]) {
				pos += 2
				continue
			}
		case '$':
			run := dollarRun(content[:span.EndOffset], pos)
			if end, ok := closingDollarRun(content[:span.EndOffset], pos+run, run); ok {
				ranges = append(ranges, mdast.SourceRange{StartOffset: pos, EndOffset: end})
				pos = end
			} else {
				pos += run
			}
			continue
		}
		pos++
	}
	return ranges
}

// closingDollarRun returns the end of the first run of exactly size dollar
// signs in content from offset on.
func closingDollarRun(content []byte, offset, size int) (int, bool) {
	for pos := offset; pos < len(content); {
		if content[pos] != '$' {
			pos++
			continue
		}
		run := dollarRun(content, pos)
		if run == size {
			return pos + run, true
		}
		pos += run
	}
	return 0, false
}

// lazyMath finds the math of a document the first time a rule asks about
// it, so that a rule that seldom needs it pays only then.
type lazyMath struct {
	ctx   *lint.RuleContext
	found *mathRegions
}

// overlaps reports whether content[start:end] lies partly in inline math.
func (l *lazyMath) overlaps(start, end int) bool {
	return l.regions().overlaps(start, end)
}

// holds reports whether math holds content[start:end] on a 1-based line:
// the line belongs to a math block, or the range lies partly in inline
// math.
func (l *lazyMath) holds(line, start, end int) bool {
	math := l.regions()
	return math.lines[line] || math.overlaps(start, end)
}

// inMath reports whether math holds a node, which markdownlint then reads
// as math: the node starts on a line of a math block or lies partly in
// inline math.
func inMath(math *lazyMath, node *mdast.Node) bool {
	span := node.SourceRange()
	return math.holds(node.SourcePosition().StartLine, span.StartOffset, max(span.EndOffset, span.StartOffset+1))
}

// regions finds the math on first use.
func (l *lazyMath) regions() *mathRegions {
	if l.found == nil {
		found := findMath(l.ctx)
		l.found = &found
	}
	return l.found
}

// overlaps reports whether content[start:end] lies partly in inline math.
func (m mathRegions) overlaps(start, end int) bool {
	idx := sort.Search(len(m.ranges), func(idx int) bool { return m.ranges[idx].EndOffset > start })
	return idx < len(m.ranges) && m.ranges[idx].StartOffset < end
}
