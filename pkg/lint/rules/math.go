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
// text without a dollar sign, after its container prefix, and runs to a
// line of at least as many dollar signs, or to the end of its container;
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
	var holders []*mdast.Node // Built at the first math block.
	for line := file.FrontMatterLines + 1; line <= len(file.Lines); line++ {
		if ctx.IsLineInCodeBlock(line) || htmlLines[line] {
			continue
		}
		fence, ok := mathFenceOpening(withoutContainerPrefix(lint.LineText(file, line)))
		if !ok {
			continue
		}
		if holders == nil {
			holders = innermostContainers(ctx)
		}
		// goldmark takes a lazy line into the containers of the paragraph it
		// continues. A fence opens no paragraph, so micromark reads it in the
		// containers the line continues explicitly.
		container := holders[line]
		for container != nil && !lint.ContinuesContainers(file, container, line) {
			container = enclosingContainer(container)
		}
		closing := mathBlockEnd(file, container, line, fence)
		for covered := line; covered <= closing; covered++ {
			m.lines[covered] = true
		}
		line = closing
	}
}

// mathBlockEnd returns the last line of a math block whose fence, fence
// dollar signs long, opens on line opening inside container, or outside
// every container when container is nil: the line of its closing fence, or
// the line before the first line that does not continue the container
// explicitly, since micromark-extension-math allows no lazy line, or the
// last line of the document.
func mathBlockEnd(file *mdast.FileSnapshot, container *mdast.Node, opening, fence int) int {
	for next := opening + 1; next <= len(file.Lines); next++ {
		if container != nil && !lint.ContinuesContainers(file, container, next) {
			return next - 1
		}
		if mathFenceCloses(withoutContainerPrefix(lint.LineText(file, next)), fence) {
			return next
		}
	}
	return len(file.Lines)
}

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
	if l.found == nil {
		found := findMath(l.ctx)
		l.found = &found
	}
	return l.found.overlaps(start, end)
}

// overlaps reports whether content[start:end] lies partly in inline math.
func (m mathRegions) overlaps(start, end int) bool {
	idx := sort.Search(len(m.ranges), func(idx int) bool { return m.ranges[idx].EndOffset > start })
	return idx < len(m.ranges) && m.ranges[idx].StartOffset < end
}
