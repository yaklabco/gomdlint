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
	var ends []int // Built at the first math block.
	for line := file.FrontMatterLines + 1; line <= len(file.Lines); line++ {
		if ctx.IsLineInCodeBlock(line) || htmlLines[line] {
			continue
		}
		fence, ok := mathFenceOpening(withoutContainerPrefix(lint.LineText(file, line)))
		if !ok {
			continue
		}
		if ends == nil {
			ends = containerLastLines(ctx)
		}
		last := len(file.Lines)
		if ends[line] > 0 {
			last = ends[line]
		}
		closing := last
		for next := line + 1; next <= last; next++ {
			if mathFenceCloses(withoutContainerPrefix(lint.LineText(file, next)), fence) {
				closing = next
				break
			}
		}
		for covered := line; covered <= closing; covered++ {
			m.lines[covered] = true
		}
		line = closing
	}
}

// mathFenceOpening returns the length of the dollar sign run that opens a
// math block on a line, after its container prefix: two or more dollar
// signs, then text that holds no dollar sign.
func mathFenceOpening(text []byte) (int, bool) {
	run := runLength(text, 0, '$')
	if run < 2 || bytes.IndexByte(text[run:], '$') >= 0 {
		return 0, false
	}
	return run, true
}

// mathFenceCloses reports whether a line, after its container prefix, closes
// a math block opened by fence dollar signs: at least as many dollar signs,
// then only spaces and tabs.
func mathFenceCloses(text []byte, fence int) bool {
	run := runLength(text, 0, '$')
	return run >= fence && len(bytes.Trim(text[run:], " \t")) == 0
}

// runLength returns how many times ch repeats in text from offset on.
func runLength(text []byte, offset int, ch byte) int {
	count := 0
	for offset+count < len(text) && text[offset+count] == ch {
		count++
	}
	return count
}

// containerLastLines returns, for each line, the last line of the innermost
// blockquote or list item that holds it, and 0 for a line that none holds.
// A math block cannot outlast its container. The containers are written
// from the longest to the shortest, so an inner one overwrites the outer
// ones, in time that grows with the lines the containers hold.
func containerLastLines(ctx *lint.RuleContext) []int {
	type lineSpan struct{ first, last int }
	var spans []lineSpan
	for _, containers := range [][]*mdast.Node{ctx.Blockquotes(), ctx.ListItems()} {
		for _, container := range containers {
			if pos := container.SourcePosition(); pos.IsValid() {
				spans = append(spans, lineSpan{pos.StartLine, pos.EndLine})
			}
		}
	}
	slices.SortStableFunc(spans, func(a, b lineSpan) int { return cmp.Compare(b.last-b.first, a.last-a.first) })
	ends := make([]int, len(ctx.File.Lines)+1)
	for _, span := range spans {
		for line := max(span.first, 1); line <= span.last && line < len(ends); line++ {
			ends[line] = span.last
		}
	}
	return ends
}

// findInline records the inline math of every paragraph, heading and table
// cell outside math blocks.
func (m *mathRegions) findInline(ctx *lint.RuleContext) {
	skips := mathSkipRanges(ctx)
	var blocks []*mdast.Node
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
			run := runLength(content[:span.EndOffset], pos, '$')
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
		run := runLength(content, pos, '$')
		if run == size {
			return pos + run, true
		}
		pos += run
	}
	return 0, false
}

// overlaps reports whether content[start:end] lies partly in inline math.
func (m mathRegions) overlaps(start, end int) bool {
	idx := sort.Search(len(m.ranges), func(idx int) bool { return m.ranges[idx].EndOffset > start })
	return idx < len(m.ranges) && m.ranges[idx].StartOffset < end
}
