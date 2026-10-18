package rules

import (
	"bytes"
	"cmp"
	"slices"
	"sort"
	"strings"
	"unicode"

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

// mathMode tells findBlocks how far it trusts goldmark's reading of a
// document's blocks.
type mathMode struct {
	// guarded stops reading math blocks at the first fence where micromark
	// may read the blocks differently from goldmark (see blockScan).
	guarded bool
	// htmlFlow tells whether the parse can hold HTML blocks. The lines of an
	// HTML block, which the HTML rules parse again as Markdown, hold none.
	htmlFlow bool
}

// findMath finds the math in a document the way micromark-extension-math
// reads it, as far as gomdlint reads math: the math blocks that findBlocks
// reads and the inline math that findInline reads. Inline math runs from a
// run of dollar signs to the next run of the same length in the same
// block, like a code span, and a dollar sign that a backslash escapes, or
// that is code, raw HTML or a link destination, opens none.
func findMath(ctx *lint.RuleContext) mathRegions {
	return findMathWith(ctx, mathMode{guarded: true, htmlFlow: true})
}

// findMathInHTMLBlock finds the math in the lines of an HTML block that the
// HTML rules parse again as Markdown, without HTML blocks.
func findMathInHTMLBlock(ctx *lint.RuleContext) mathRegions {
	return findMathWith(ctx, mathMode{guarded: true})
}

// findMathWith finds the math in a document as mode tells.
func findMathWith(ctx *lint.RuleContext, mode mathMode) mathRegions {
	math := mathRegions{lines: make(map[int]bool)}
	if ctx.File == nil || ctx.Root == nil || !bytes.Contains(ctx.File.Content, []byte("$")) {
		return math
	}
	math.findBlocks(ctx, mode)
	math.findInline(ctx, mode)
	return math
}

// possibleMathLines returns the lines of every math block that a fence at
// column 0, or one indented outside goldmark's containers, opens, read
// without findMath's guards. MD052 reads no footnote definition there. Math
// only hides definitions, so reading more of it can only make MD052 report
// more, as main does.
func possibleMathLines(ctx *lint.RuleContext) map[int]bool {
	math := mathRegions{lines: make(map[int]bool)}
	if ctx.File != nil && ctx.Root != nil && bytes.Contains(ctx.File.Content, []byte("$$")) {
		math.findBlocks(ctx, mathMode{htmlFlow: true})
	}
	return math.lines
}

// findBlocks records the lines of the math blocks that gomdlint reads.
//
// A math block opens on a line that starts with "$$" at column 0, after a
// byte order mark on the first line, outside goldmark's code and HTML
// blocks and front matter, and runs to its closing fence or to the end of
// the document (topLevelMathEnd). micromark reads such a fence as
// top-level math while it reads the lines before it as goldmark does: a
// math fence interrupts a paragraph, and a line at column 0 continues no
// blockquote, list item or footnote, since math allows no lazy line.
// gomdlint does not read math inside containers, or behind a fence that is
// indented, so the rules report there as they did before they skipped
// math.
//
// Guarded, findBlocks reads no more math blocks from the first fence where
// micromark may read the blocks differently (blockScan). Skipping that
// fence alone would not do: micromark may read it as math, and then read
// the fence that closes it as one that opens math. Unguarded, for MD052's
// footnote definitions, it reads every such block, and also the top-level
// math behind a fence indented one to three spaces (indentedTopLevelFence).
func (m *mathRegions) findBlocks(ctx *lint.RuleContext, mode mathMode) {
	file := ctx.File
	if mode.guarded && !linesAsMicromark(file) {
		return
	}
	scan := newBlockScan(ctx, mode)
	for line := file.FrontMatterLines + 1; line <= len(file.Lines); line++ {
		if scan.inBlock(line) {
			scan.passBlockLine(line)
			continue
		}
		text := lint.LineText(file, line)
		fence, ok := mathFenceOpening(text)
		if !ok && !mode.guarded {
			fence, ok = scan.indentedTopLevelFence(line, text)
		}
		if !ok {
			scan.passLine(line, text)
			continue
		}
		if scan.doubts(line) {
			return
		}
		closing := topLevelMathEnd(file, line, fence)
		for covered := line; covered <= closing; covered++ {
			m.lines[covered] = true
		}
		if scan.startsBlockIn(line+1, closing) {
			return
		}
		scan.doubt = false
		line = closing
	}
}

// topLevelMathEnd returns the last line of a top-level math block whose
// fence, fence dollar signs long, opens on line opening: the first later
// line that holds, after at most three spaces, at least as many dollar
// signs and then only spaces and tabs, as micromark-extension-math closes
// the block, or the last line of the document. Every line between is math,
// whatever goldmark reads there.
func topLevelMathEnd(file *mdast.FileSnapshot, opening, fence int) int {
	for next := opening + 1; next <= len(file.Lines); next++ {
		text := lint.RawLineContent(file, next)
		indent := len(text) - len(bytes.TrimLeft(text, " "))
		if indent <= maxFenceIndent && mathFenceCloses(text[indent:], fence) {
			return next
		}
	}
	return len(file.Lines)
}

// linesAsMicromark reports whether markdownlint reads file's lines and
// front matter as gomdlint does. It does not when a carriage return ends a
// line alone, which gomdlint reads as text and micromark as a line ending;
// when a second byte order mark follows the first, which micromark skips
// after markdownlint skips the first; when "---" and whitespace other than
// spaces and tabs open front matter that markdownlint reads and gomdlint
// does not; and when "---" front matter closes at "...", where gomdlint
// closes it and markdownlint does not. Front matter lines read raw, since
// LineText reads them as empty.
func linesAsMicromark(file *mdast.FileSnapshot) bool {
	for idx, char := range file.Content {
		if char == '\r' && (idx+1 == len(file.Content) || file.Content[idx+1] != '\n') {
			return false
		}
	}
	first := bytes.TrimPrefix(lint.RawLineContent(file, 1), []byte(mdast.ByteOrderMark))
	if bytes.HasPrefix(first, []byte(mdast.ByteOrderMark)) {
		return false
	}
	dashes := bytes.HasPrefix(first, []byte("---"))
	if file.FrontMatterLines == 0 {
		rest := first[min(len(first), len("---")):]
		return !dashes || len(bytes.TrimFunc(rest, unicode.IsSpace)) > 0 || len(bytes.Trim(rest, " \t")) == 0
	}
	closing := lint.RawLineContent(file, file.FrontMatterLines)
	return !dashes || bytes.Equal(bytes.TrimRight(closing, " \t"), []byte("---"))
}

// blockScan tells, for a guarded findBlocks, where micromark may read a
// document's blocks differently from goldmark before a math fence, so that
// the fence is not top-level math to micromark or its block ends elsewhere.
// Each doubt is a block over the fence that micromark may read and goldmark
// does not:
//
//   - a line outside goldmark's code and HTML blocks could start an HTML
//     block (htmlBlockStart) that goldmark does not read there, as after
//     math or in a table;
//   - goldmark ends a code or HTML block without its end condition, where a
//     container ends that micromark may not read, so that micromark may go
//     on with the block, or starts one in an ordered list item that
//     micromark may not read (mayBeRefused);
//   - a fence that micromark may read as top-level math is indented, so
//     findBlocks does not read it, and its closing fence at column 0 would
//     read as one that opens math (passLine);
//   - a fence comes in a ":::" container directive, which micromark reads
//     and goldmark does not (passDirective);
//   - goldmark starts a code or HTML block inside math (startsBlockIn): the
//     block may run past the math, where micromark reads lines afresh.
//
// An HTML block that a blank line ends makes the fences up to the next
// blank line doubtful. Every other doubt makes every later fence doubtful.
type blockScan struct {
	ctx       *lint.RuleContext
	mode      mathMode
	htmlLines map[int]bool
	// starts holds the lines where goldmark's code and HTML blocks start,
	// and tails the last lines of its HTML blocks that a blank line should
	// have ended.
	starts, tails map[int]bool
	// stopFrom is the first line where no math may open.
	stopFrom int
	// doubt tells whether a doubt arose since the last blank line or math
	// block.
	doubt bool
	// directives holds the colons of the directives that may be open.
	directives []int
	// holders holds goldmark's innermost container on each line, built
	// when an indented fence first needs it.
	holders []*mdast.Node
}

// newBlockScan builds the scan of ctx's document, finding the code and HTML
// blocks that goldmark ends without their end conditions.
func newBlockScan(ctx *lint.RuleContext, mode mathMode) *blockScan {
	file := ctx.File
	last := len(file.Lines)
	scan := &blockScan{
		ctx:       ctx,
		mode:      mode,
		htmlLines: htmlBlockLines(ctx),
		starts:    make(map[int]bool),
		tails:     make(map[int]bool),
		stopFrom:  last + 1,
	}
	for _, block := range ctx.CodeBlocks() {
		pos := block.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		scan.starts[pos.StartLine] = true
		if inRefusableItem(file, block) {
			scan.stopFrom = min(scan.stopFrom, pos.StartLine)
		}
		if lint.IsFencedCodeBlock(block) && !block.Block.CodeBlock.Closed && pos.EndLine < last {
			scan.stopFrom = min(scan.stopFrom, pos.EndLine+1)
		}
	}
	for _, block := range ctx.HTMLBlocks() {
		rng := block.SourceRange()
		if rng.StartOffset < 0 || rng.EndOffset <= rng.StartOffset || rng.EndOffset > len(file.Content) {
			continue
		}
		first, _ := file.LineAt(rng.StartOffset)
		end, _ := file.LineAt(rng.EndOffset - 1)
		if first <= 0 || end < first {
			continue
		}
		scan.starts[first] = true
		if inRefusableItem(file, block) {
			scan.stopFrom = min(scan.stopFrom, first)
		}
		opening := file.Content[rng.StartOffset:max(file.Lines[first-1].NewlineStart, rng.StartOffset)]
		if lt := bytes.IndexByte(opening, '<'); lt >= 0 {
			opening = opening[lt:]
		}
		kind, markers := htmlBlockStart(opening)
		switch {
		case kind == htmlNone:
			// goldmark reads an HTML block that micromark may not.
			scan.stopFrom = min(scan.stopFrom, first)
		case end >= last:
		case kind == htmlHoldsBlankLines && !containsAnyFold(lint.RawLineContent(file, end), markers):
			scan.stopFrom = min(scan.stopFrom, end+1)
		case kind == htmlEndsAtBlankLine && len(bytes.Trim(lint.RawLineContent(file, end+1), " \t>")) > 0:
			scan.tails[end] = true
		}
	}
	return scan
}

// inBlock reports whether line is in one of goldmark's code or HTML blocks.
func (s *blockScan) inBlock(line int) bool {
	return s.ctx.IsLineInCodeBlock(line) || s.htmlLines[line]
}

// passBlockLine follows a line of goldmark's code and HTML blocks.
func (s *blockScan) passBlockLine(line int) {
	if s.tails[line] {
		s.doubt = true
	}
}

// passLine follows a line outside goldmark's code and HTML blocks that opens
// no math, text without its line ending.
func (s *blockScan) passLine(line int, text []byte) {
	if !s.mode.guarded {
		return
	}
	if len(bytes.Trim(text, " \t")) == 0 {
		s.doubt = false
		return
	}
	s.passDirective(text)
	indent := 0
	for indent < len(text) && indent < maxFenceIndent && text[indent] == ' ' {
		indent++
	}
	if _, opens := mathFenceOpening(text[indent:]); opens && indent > 0 && !s.contained(line) {
		s.stopFrom = min(s.stopFrom, line)
	}
	if !s.mode.htmlFlow {
		return
	}
	switch kind, _ := htmlBlockStart(text[indent:]); kind {
	case htmlHoldsBlankLines:
		s.stopFrom = min(s.stopFrom, line)
	case htmlEndsAtBlankLine:
		s.doubt = true
	}
}

// passDirective follows ":::" container directives, which micromark reads
// and goldmark does not. After at most three spaces, three or more colons
// and a letter open one, as micromark reads a directive's name; at least as
// many colons as the innermost open one has, and then only spaces and
// tabs, close it. Colons followed by a space, as in Pandoc's "::: note",
// are text to micromark.
func (s *blockScan) passDirective(text []byte) {
	indent := 0
	for indent < len(text) && indent < maxFenceIndent && text[indent] == ' ' {
		indent++
	}
	colons := 0
	for indent+colons < len(text) && text[indent+colons] == ':' {
		colons++
	}
	if colons < minDirectiveColons {
		return
	}
	rest := text[indent+colons:]
	top := len(s.directives) - 1
	switch {
	case len(bytes.Trim(rest, " \t")) == 0:
		if top >= 0 && colons >= s.directives[top] {
			s.directives = s.directives[:top]
		}
	case isASCIILetter(rest[0]):
		s.directives = append(s.directives, colons)
	}
}

// contained reports whether line explicitly continues goldmark's
// blockquotes and list items around it, all of which micromark reads too,
// so that a math fence on it opens math inside them, which a line at
// column 0 ends. A line outside containers, or a lazy line, is not.
func (s *blockScan) contained(line int) bool {
	if s.holders == nil {
		s.holders = innermostContainers(s.ctx)
	}
	container := s.holders[line]
	if container == nil || !lint.ContinuesContainers(s.ctx.File, container, line) {
		return false
	}
	if container.Kind == mdast.NodeListItem && mayBeRefused(s.ctx.File, container) {
		return false
	}
	return !inRefusableItem(s.ctx.File, container)
}

// inRefusableItem reports whether node lies in a list item that micromark
// may not read (mayBeRefused).
func inRefusableItem(file *mdast.FileSnapshot, node *mdast.Node) bool {
	for item := enclosingContainer(node); item != nil; item = enclosingContainer(item) {
		if item.Kind == mdast.NodeListItem && mayBeRefused(file, item) {
			return true
		}
	}
	return false
}

// mayBeRefused reports whether micromark may not read list item, which
// goldmark reads: its list is ordered, its first marker is not "1." or
// "1)", the single digit 1, and the line before the list holds text, as
// after a paragraph or indented code, where micromark starts no such list.
// An item's span starts at its marker.
func mayBeRefused(file *mdast.FileSnapshot, item *mdast.Node) bool {
	list := item.Parent
	if list == nil || list.Block == nil || list.Block.List == nil || !list.Block.List.Ordered || list.FirstChild == nil {
		return false
	}
	at := list.FirstChild.SourceRange().StartOffset
	if at >= 0 && at+1 < len(file.Content) && file.Content[at] == '1' &&
		(file.Content[at+1] == '.' || file.Content[at+1] == ')') {
		return false
	}
	first := list.SourcePosition().StartLine
	return first > file.FrontMatterLines+1 && len(bytes.Trim(lint.RawLineContent(file, first-1), " \t>")) > 0
}

// indentedTopLevelFence returns the length of a math fence that text, a
// line outside goldmark's containers or lazy in them, holds after one to
// three spaces, where micromark reads top-level math. Only an unguarded
// findBlocks, for MD052's footnote definitions, reads such math, where more
// math only reports more footnote calls.
func (s *blockScan) indentedTopLevelFence(line int, text []byte) (int, bool) {
	indent := 0
	for indent < len(text) && indent < maxFenceIndent && text[indent] == ' ' {
		indent++
	}
	if indent == 0 {
		return 0, false
	}
	fence, ok := mathFenceOpening(text[indent:])
	if !ok || s.contained(line) {
		return 0, false
	}
	return fence, true
}

// minDirectiveColons is the fewest colons that open a container directive.
const minDirectiveColons = 3

// doubts reports whether a guarded findBlocks may not read a fence on line
// as top-level math.
func (s *blockScan) doubts(line int) bool {
	return s.mode.guarded && (s.doubt || len(s.directives) > 0 || line >= s.stopFrom)
}

// startsBlockIn reports whether, for a guarded findBlocks, goldmark starts a
// code or HTML block on one of the lines from first through last.
func (s *blockScan) startsBlockIn(first, last int) bool {
	if !s.mode.guarded {
		return false
	}
	for line := first; line <= last; line++ {
		if s.starts[line] {
			return true
		}
	}
	return false
}

// htmlKind is the kind of HTML block that a line may start.
type htmlKind uint8

const (
	htmlNone htmlKind = iota
	// htmlEndsAtBlankLine ends at a blank line: CommonMark's kinds 6 and 7.
	htmlEndsAtBlankLine
	// htmlHoldsBlankLines ends at a line that holds one of its end markers,
	// and may hold blank lines: kinds 1 to 5.
	htmlHoldsBlankLines
)

// htmlBlockStart returns the kind of HTML block that micromark may start
// with text, a line after its indentation, as CommonMark defines the kinds,
// and the end markers of a block of kinds 1 to 5, any of which ends it.
// micromark starts a block of kind 7 only where no paragraph is open, which
// findBlocks does not know, so it counts one wherever one could start.
func htmlBlockStart(text []byte) (htmlKind, []string) {
	if len(text) < 2 || text[0] != '<' {
		return htmlNone, nil
	}
	lower := strings.ToLower(string(text))
	switch {
	case strings.HasPrefix(lower, "<!--"):
		return htmlHoldsBlankLines, []string{"-->"}
	case strings.HasPrefix(lower, "<?"):
		return htmlHoldsBlankLines, []string{"?>"}
	case strings.HasPrefix(lower, "<![cdata["):
		return htmlHoldsBlankLines, []string{"]]>"}
	case lower[1] == '!' && len(lower) > 2 && isASCIILetter(lower[2]):
		return htmlHoldsBlankLines, []string{">"}
	}
	for _, name := range []string{"script", "pre", "style", "textarea"} {
		if tagNameEnds(lower[1:], name, false) {
			return htmlHoldsBlankLines, []string{"</script>", "</pre>", "</style>", "</textarea>"}
		}
	}
	name := strings.TrimPrefix(lower[1:], "/")
	size := 0
	for size < len(name) && (isASCIILetter(name[size]) || (size > 0 && name[size] >= '0' && name[size] <= '9')) {
		size++
	}
	if size > 0 && isHTMLBlockTagName(name[:size]) && tagNameEnds(name, name[:size], true) {
		return htmlEndsAtBlankLine, nil
	}
	construct, ok := mdscan.MatchHTML(text, 0)
	if ok && construct.Name != "" && !strings.HasPrefix(construct.Name, "?") &&
		len(bytes.Trim(text[construct.End:], " \t")) == 0 {
		return htmlEndsAtBlankLine, nil
	}
	return htmlNone, nil
}

// tagNameEnds reports whether text starts with the tag name name, followed
// by a space, a tab, ">", or nothing, or by "/>" when selfClosing allows it.
func tagNameEnds(text, name string, selfClosing bool) bool {
	if !strings.HasPrefix(text, name) {
		return false
	}
	rest := text[len(name):]
	return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '>' ||
		(selfClosing && strings.HasPrefix(rest, "/>"))
}

// isHTMLBlockTagName reports whether a lowercase tag name starts an HTML
// block of CommonMark's kind 6, as micromark lists the names.
func isHTMLBlockTagName(name string) bool {
	switch name {
	case "address", "article", "aside", "base", "basefont", "blockquote", "body", "caption", "center",
		"col", "colgroup", "dd", "details", "dialog", "dir", "div", "dl", "dt", "fieldset",
		"figcaption", "figure", "footer", "form", "frame", "frameset", "h1", "h2", "h3", "h4", "h5",
		"h6", "head", "header", "hr", "html", "iframe", "legend", "li", "link", "main", "menu",
		"menuitem", "nav", "noframes", "ol", "optgroup", "option", "p", "param", "search", "section",
		"summary", "table", "tbody", "td", "tfoot", "th", "thead", "title", "tr", "track", "ul":
		return true
	}
	return false
}

// containsAnyFold reports whether text holds one of markers, ignoring case.
func containsAnyFold(text []byte, markers []string) bool {
	lower := strings.ToLower(string(text))
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// Columns of space: a tab stop, and the most before a fence.
const (
	tabStop        = 4
	maxFenceIndent = 3
)

// mathFenceOpening returns the length of the dollar sign run that opens a
// math block at the start of text: two or more dollar signs, then text
// that holds no dollar sign.
func mathFenceOpening(text []byte) (int, bool) {
	run := dollarRun(text, 0)
	if run < 2 || bytes.IndexByte(text[run:], '$') >= 0 {
		return 0, false
	}
	return run, true
}

// mathFenceCloses reports whether text, a line after its indentation,
// closes a math block opened by fence dollar signs: at least as many dollar
// signs, then only spaces and tabs.
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

// findInline records the inline math of every paragraph, heading and table
// cell that starts outside math blocks, read up to the first line where
// micromark may end it (inlineMathEnd). Where the parse can hold HTML
// blocks, it reads none in a block whose first line could start one, nor
// in a table with such a line, which may end the table in micromark and
// hold the rows after it.
func (m *mathRegions) findInline(ctx *lint.RuleContext, mode mathMode) {
	skips := mathSkipRanges(ctx)
	blocks := make([]*mdast.Node, 0, len(ctx.Paragraphs())+len(ctx.Headings()))
	blocks = append(blocks, ctx.Paragraphs()...)
	blocks = append(blocks, ctx.Headings()...)
	for _, table := range ctx.Tables() {
		blocks = append(blocks, mdast.FindByKind(table, mdast.NodeTableCell)...)
	}
	var htmlTables map[*mdast.Node]bool
	if mode.htmlFlow {
		htmlTables = tablesWithHTMLLines(ctx)
	}
	for _, block := range blocks {
		span := block.SourceRange()
		if span.EndOffset <= span.StartOffset || span.EndOffset > len(ctx.File.Content) ||
			m.lines[block.SourcePosition().StartLine] || htmlTables[enclosingTable(block)] {
			continue
		}
		// A block whose first line could start an HTML block may be one to
		// micromark, which reads no math in it.
		if kind, _ := htmlBlockStart(lineAt(ctx.File.Content, span.StartOffset)); mode.htmlFlow && kind != htmlNone {
			continue
		}
		span.EndOffset = inlineMathEnd(ctx.File, block, span, mode.htmlFlow)
		m.ranges = append(m.ranges, inlineMath(ctx.File.Content, span, skips)...)
	}
	slices.SortFunc(m.ranges, func(a, b mdast.SourceRange) int { return cmp.Compare(a.StartOffset, b.StartOffset) })
}

// inlineMathEnd returns where findInline stops reading block, whose span is
// span: at the first line after its first where micromark may end the
// paragraph, so that dollar signs before that line never pair with dollar
// signs after it, as in "Costs $5", math, "for $6". After spaces, tabs and
// blockquote markers, such a line holds a math fence, a directive, a
// footnote definition, or what could start an HTML block (breaksParagraph).
// When the block's own first line holds a fence, the block is read through
// the next line with a fence, whose dollar signs close the first line's.
// Reading less of a block finds no inline math that reading all of it
// would not cover.
func inlineMathEnd(file *mdast.FileSnapshot, block *mdast.Node, span mdast.SourceRange, htmlFlow bool) int {
	_, opensWithFence := mathFenceOpening(lineAt(file.Content, span.StartOffset))
	pos := block.SourcePosition()
	for line := pos.StartLine + 1; line <= pos.EndLine && line <= len(file.Lines); line++ {
		text := bytes.TrimLeft(lint.RawLineContent(file, line), " \t>")
		_, fence := mathFenceOpening(text)
		if !fence && !breaksParagraph(text, htmlFlow) {
			continue
		}
		end := file.Lines[line-1].StartOffset
		if opensWithFence && fence {
			end = file.Lines[line-1].NewlineStart
		}
		return min(max(end, span.StartOffset), span.EndOffset)
	}
	return span.EndOffset
}

// breaksParagraph reports whether micromark may end a paragraph before
// text, a line after its indentation and blockquote markers, where goldmark
// reads it on: a directive, two or more colons and a letter; a footnote
// definition; or, when the parse can hold HTML blocks (htmlFlow), what could
// start one.
func breaksParagraph(text []byte, htmlFlow bool) bool {
	colons := 0
	for colons < len(text) && text[colons] == ':' {
		colons++
	}
	if colons >= 2 && colons < len(text) && isASCIILetter(text[colons]) {
		return true
	}
	if _, ok := footnoteDefinitionLabel(text); ok {
		return true
	}
	kind, _ := htmlBlockStart(text)
	return htmlFlow && kind != htmlNone
}

// tablesWithHTMLLines returns the tables that hold a line that could start
// an HTML block, after at most three spaces.
func tablesWithHTMLLines(ctx *lint.RuleContext) map[*mdast.Node]bool {
	tables := make(map[*mdast.Node]bool)
	for _, table := range ctx.Tables() {
		pos := table.SourcePosition()
		for line := pos.StartLine; pos.IsValid() && line <= pos.EndLine && line <= len(ctx.File.Lines); line++ {
			text := lint.RawLineContent(ctx.File, line)
			indent := 0
			for indent < len(text) && indent < maxFenceIndent && text[indent] == ' ' {
				indent++
			}
			if kind, _ := htmlBlockStart(text[indent:]); kind != htmlNone {
				tables[table] = true
				break
			}
		}
	}
	return tables
}

// enclosingTable returns the table that holds node, or nil.
func enclosingTable(node *mdast.Node) *mdast.Node {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeTable {
			return parent
		}
	}
	return nil
}

// lineAt returns content from offset to the end of its line.
func lineAt(content []byte, offset int) []byte {
	rest := content[offset:]
	if end := bytes.IndexAny(rest, "\r\n"); end >= 0 {
		return rest[:end]
	}
	return rest
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

// inBlock reports whether a 1-based line belongs to a math block.
func (l *lazyMath) inBlock(line int) bool {
	return l.regions().lines[line]
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
