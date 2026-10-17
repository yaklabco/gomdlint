package goldmark

import (
	"bytes"
	"sort"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// mapper converts a goldmark AST into an mdast.Node tree and gives every
// node its full and content source spans (see the mdast package
// documentation for the contract).
type mapper struct {
	// source is the buffer goldmark parsed. Goldmark segments index it.
	source []byte

	// base is the offset of source[0] in content.
	base int

	// content and lines describe the original file.
	content []byte
	lines   []mdast.LineInfo

	// rec holds the positions recorded during parsing. It may be nil, in
	// which case spans are derived from goldmark segments alone.
	rec *spanRecorder
}

// newMapper creates a mapper for content parsed without span recording.
func newMapper(content []byte) *mapper {
	return &mapper{source: content, content: content, lines: mdast.BuildLines(content)}
}

// newSourceMapper creates a mapper for a document goldmark parsed from
// source, where source[0] is content[base].
func newSourceMapper(content, source []byte, base int, lines []mdast.LineInfo, rec *spanRecorder) *mapper {
	return &mapper{source: source, base: base, content: content, lines: lines, rec: rec}
}

// mapDocument converts a goldmark document node to an mdast.Node tree.
func (m *mapper) mapDocument(gmDoc ast.Node) *mdast.Node {
	doc := mdast.NewDocument()
	doc.SourceSpan = &mdast.SourceRange{StartOffset: 0, EndOffset: len(m.content)}
	m.mapChildren(gmDoc, doc)
	fillMissingSpans(doc)
	return doc
}

// mapChildren maps all children of a goldmark node to mdast nodes.
func (m *mapper) mapChildren(gmParent ast.Node, parent *mdast.Node) {
	for child := gmParent.FirstChild(); child != nil; child = child.NextSibling() {
		if textNode, ok := child.(*ast.Text); ok {
			m.appendText(parent, textNode)
			continue
		}
		if mdNode := m.mapNode(child); mdNode != nil {
			mdast.AppendChild(parent, mdNode)
		}
	}
	m.moveResidualDelimiters(parent)
}

// moveResidualDelimiters puts the leftover characters of a closing
// delimiter run where they are. When emphasis uses part of a run, goldmark
// keeps the start of the run's segment. That is right for an opener, whose
// leftover characters come first, but a closer's leftover characters come
// after the ones that closed the emphasis: in "*foo**" the literal star is
// the last one. Such a Text node overlaps the emphasis before it; it moves
// to start where that emphasis ends.
func (m *mapper) moveResidualDelimiters(parent *mdast.Node) {
	for child := parent.FirstChild; child != nil; child = child.Next {
		prev := child.Prev
		if child.Kind != mdast.NodeText || child.SourceSpan == nil || prev == nil || prev.SourceSpan == nil {
			continue
		}
		span := *child.SourceSpan
		if span.StartOffset >= prev.SourceSpan.EndOffset || !m.isDelimiterRun(span) {
			continue
		}
		moved := mdast.SourceRange{StartOffset: prev.SourceSpan.EndOffset, EndOffset: prev.SourceSpan.EndOffset + span.Len()}
		if moved.EndOffset > len(m.content) ||
			string(m.content[moved.StartOffset:moved.EndOffset]) != string(m.content[span.StartOffset:span.EndOffset]) {
			continue
		}
		if next := child.Next; next != nil && next.SourceSpan != nil && moved.EndOffset > next.SourceSpan.StartOffset {
			continue
		}
		setSpans(child, moved, moved)
	}
}

// isDelimiterRun reports whether span holds one emphasis or strikethrough
// delimiter character repeated.
func (m *mapper) isDelimiterRun(span mdast.SourceRange) bool {
	if span.IsEmpty() || span.EndOffset > len(m.content) {
		return false
	}
	first := m.content[span.StartOffset]
	if first != '*' && first != '_' && first != '~' {
		return false
	}
	for idx := span.StartOffset; idx < span.EndOffset; idx++ {
		if m.content[idx] != first {
			return false
		}
	}
	return true
}

// mapNode converts a single goldmark node to an mdast.Node with its spans.
func (m *mapper) mapNode(gmNode ast.Node) *mdast.Node {
	var node *mdast.Node

	switch gmn := gmNode.(type) {
	// Block-level nodes.
	case *ast.Document:
		node = mdast.NewNode(mdast.NodeDocument)
		m.mapChildren(gmNode, node)

	case *ast.Heading:
		node = m.mapHeading(gmn)

	case *ast.Paragraph, *ast.TextBlock:
		// A tight list item holds its text in a TextBlock: a paragraph
		// rendered without <p>. A block with no lines has no source.
		if gmNode.Lines().Len() == 0 && !gmNode.HasChildren() {
			return nil
		}
		node = mdast.NewNode(mdast.NodeParagraph)
		m.mapChildren(gmNode, node)
		m.setParagraphSpans(node, gmNode)

	case *ast.LinkReferenceDefinition:
		// A definition renders nothing, and the tree records no node for
		// it: the refs package finds definitions on the lines that no leaf
		// block covers. goldmark gives each definition a node of its own,
		// placed before the paragraph it started, if any text is left.
		return nil

	case *ast.List:
		node = m.mapList(gmn)

	case *ast.ListItem:
		node = mdast.NewNode(mdast.NodeListItem)
		node.Block = mdast.NewBlockAttrs().WithListItem(&mdast.ListItemAttrs{ContentIndent: gmn.Offset})
		m.mapChildren(gmNode, node)
		m.setListItemSpans(node, gmNode)

	case *ast.Blockquote:
		node = mdast.NewNode(mdast.NodeBlockquote)
		m.mapChildren(gmNode, node)
		m.setContainerSpans(node, gmNode, true)

	case *ast.FencedCodeBlock:
		node = m.mapFencedCodeBlock(gmn)

	case *ast.CodeBlock:
		node = m.mapIndentedCodeBlock(gmn)

	case *ast.ThematicBreak:
		node = mdast.NewNode(mdast.NodeThematicBreak)
		m.setLeafBlockSpans(node, gmNode)

	case *ast.HTMLBlock:
		node = mdast.NewNode(mdast.NodeHTMLBlock)
		m.setLeafBlockSpans(node, gmNode)

	// Inline-level nodes.
	case *ast.Emphasis:
		node = m.mapEmphasis(gmn)

	case *ast.CodeSpan:
		node = m.mapCodeSpan(gmn)

	case *ast.Link:
		node = m.mapLink(gmn)

	case *ast.Image:
		node = m.mapImage(gmn)

	case *ast.AutoLink:
		node = m.mapAutoLink(gmn)

	case *ast.RawHTML:
		node = mdast.NewNode(mdast.NodeHTMLInline)
		m.setRawHTMLSpans(node, gmn)

	case *ast.String:
		node = m.mapString(gmn)

	// GFM extension nodes.
	case *east.Strikethrough:
		node = m.mapStrikethrough(gmn)

	case *east.TaskCheckBox:
		node = m.mapTaskCheckBox(gmn)

	case *east.Table:
		node = m.mapTable(gmn)

	default:
		// Fallback for node types from extensions gomdlint does not enable.
		node = mdast.NewNode(mdast.NodeRaw)
		m.mapChildren(gmNode, node)
		if gmNode.Type() == ast.TypeBlock && gmNode.Lines().Len() > 0 {
			m.setParagraphSpans(node, gmNode)
		} else {
			m.setChildrenSpans(node)
		}
	}

	return node
}

// mapHeading converts a goldmark Heading to an mdast node.
func (m *mapper) mapHeading(heading *ast.Heading) *mdast.Node {
	node := mdast.NewNode(mdast.NodeHeading)
	node.Block = mdast.NewBlockAttrs().WithHeadingLevel(heading.Level)
	m.mapChildren(heading, node)

	lines := heading.Lines()
	extent := m.block(heading)
	if lines.Len() == 0 {
		if extent == nil {
			return node
		}
		full := m.extentRange(extent)
		setSpans(node, full, emptyAt(full.EndOffset))
		return node
	}

	first, last := lines.At(0), lines.At(lines.Len()-1)
	content := mdast.SourceRange{StartOffset: m.abs(first.Start), EndOffset: m.abs(last.Stop)}
	content.EndOffset = m.trimLineEnding(content.StartOffset, content.EndOffset)
	if extent == nil {
		setSpans(node, content, content)
		return node
	}
	full := m.extentRange(extent)
	if full.StartOffset < len(m.content) && m.content[full.StartOffset] == '#' {
		// The content keeps the spaces before a closing sequence, which
		// goldmark leaves out of the text segment.
		content.EndOffset = m.closingSequenceStart(content.EndOffset)
	}
	// A setext heading opens on its underline; its text lines come first.
	full.StartOffset = min(full.StartOffset, content.StartOffset)
	setSpans(node, full, clampTo(content, full))
	return node
}

// closingSequenceStart returns where the closing sequence of an ATX heading
// whose text ends at end starts, when only spaces and tabs come between
// them, and end otherwise. A closing sequence is a run of '#' that only
// spaces and tabs follow on its line.
func (m *mapper) closingSequenceStart(end int) int {
	isBlank := func(ch byte) bool { return ch == ' ' || ch == '\t' }
	start := end
	for start < len(m.content) && isBlank(m.content[start]) {
		start++
	}
	if start == end || start == len(m.content) || m.content[start] != '#' {
		return end
	}
	rest := start
	for rest < len(m.content) && m.content[rest] == '#' {
		rest++
	}
	for rest < len(m.content) && isBlank(m.content[rest]) {
		rest++
	}
	if rest < len(m.content) && m.content[rest] != '\n' && m.content[rest] != '\r' {
		return end
	}
	return start
}

// mapList converts a goldmark List to an mdast node.
func (m *mapper) mapList(list *ast.List) *mdast.Node {
	node := mdast.NewNode(mdast.NodeList)

	listAttrs := &mdast.ListAttrs{
		Ordered:     list.IsOrdered(),
		StartNumber: list.Start,
		Tight:       ListTight(list, m.source),
	}

	// goldmark's Marker is the bullet character of a bullet list and the
	// delimiter ('.' or ')') of an ordered list.
	if list.IsOrdered() {
		listAttrs.Delimiter = string(list.Marker)
	} else {
		listAttrs.BulletMarker = string(list.Marker)
	}

	node.Block = mdast.NewBlockAttrs().WithList(listAttrs)
	m.mapChildren(list, node)
	m.setContainerSpans(node, list, false)
	return node
}

// ListTight reports whether a list that goldmark parsed from source is tight
// as CommonMark reads it. Use it instead of list.IsTight, which goldmark
// gets wrong when an item starts with link reference definitions.
//
// goldmark reads a list as loose when an item but the first, or a block of
// an item but its first, follows a blank line, by the blocks' blank-line
// flags (listParser.Close). It gives each link reference definition that
// starts a paragraph a node of its own before the paragraph, and the first
// of them takes the paragraph's flag. The rest of the paragraph then counts
// as a later block of the item, although no blank line separates it from the
// definitions, and keeps the flag that its first line got when the item
// followed a blank line, so goldmark reads the list as loose. Definitions
// render nothing, so they and the rest of their paragraph count as one block.
//
// A list is tight exactly when none of its items makes it loose, as
// ListItemLoose reports for each.
func ListTight(list *ast.List, source []byte) bool {
	if list.IsTight {
		return true
	}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if ListItemLoose(item, item == list.FirstChild(), source) {
			return false
		}
	}
	return true
}

// ListItemLoose reports whether an item of a list that goldmark parsed from
// source makes the list loose as CommonMark reads it (see ListTight): it
// follows a blank line and is not the list's first item, or a block of it
// after its first block follows a blank line. It reads the blank-line flags
// of the item and of the blocks after its first block, not those of its
// first block, which follow from the line before the item.
func ListItemLoose(item ast.Node, first bool, source []byte) bool {
	if !first && item.HasBlankPreviousLines() {
		return true
	}
	end := firstBlockEnd(item.FirstChild(), source)
	if end == nil {
		return false
	}
	for block := end.NextSibling(); block != nil; block = block.NextSibling() {
		if block.HasBlankPreviousLines() {
			return true
		}
	}
	return false
}

// firstBlockEnd returns the last node of an item's first block, which starts
// at node. That is node itself unless it is a link reference definition.
// Then the block is the paragraph the definition started: it runs over the
// definitions on the lines that follow and ends with the node that starts
// on the line after the last of them, which holds the rest of the paragraph
// (text or a setext heading) or a block that ended it. Definitions after a
// blank line started another paragraph and are a block of their own.
func firstBlockEnd(node ast.Node, source []byte) ast.Node {
	if _, ok := node.(*ast.LinkReferenceDefinition); !ok {
		return node
	}
	for next := node.NextSibling(); next != nil && startsOnNextLine(node, next, source); next = node.NextSibling() {
		node = next
		if _, ok := next.(*ast.LinkReferenceDefinition); !ok {
			break
		}
	}
	return node
}

// startsOnNextLine reports whether the first line of next is the line after
// the last line of prev. Blocks without lines never are.
func startsOnNextLine(prev, next ast.Node, source []byte) bool {
	prevLines, nextLines := prev.Lines(), next.Lines()
	if prevLines.Len() == 0 || nextLines.Len() == 0 {
		return false
	}
	end, start := prevLines.At(prevLines.Len()-1).Stop, nextLines.At(0).Start
	if end > start || start > len(source) {
		return false
	}
	return bytes.Count(source[end:start], []byte{'\n'}) == 1
}

// mapFencedCodeBlock converts a goldmark FencedCodeBlock to an mdast node.
func (m *mapper) mapFencedCodeBlock(codeBlock *ast.FencedCodeBlock) *mdast.Node {
	node := mdast.NewNode(mdast.NodeCodeBlock)

	info := ""
	if codeBlock.Info != nil {
		info = string(codeBlock.Info.Value(m.source))
	}

	codeAttrs := &mdast.CodeBlockAttrs{
		FenceChar:   '`',
		FenceLength: 3,
		Info:        info,
		Indented:    false,
	}
	node.Block = mdast.NewBlockAttrs().WithCodeBlock(codeAttrs)

	extent := m.block(codeBlock)
	if extent == nil {
		if content, ok := m.codeContent(codeBlock); ok {
			setSpans(node, content, content)
		}
		return node
	}

	full := m.extentRange(extent)
	codeAttrs.Closed = extent.closed
	// The opening fence starts the block: read its character and length.
	if full.StartOffset < len(m.content) {
		if fenceChar := m.content[full.StartOffset]; fenceChar == '`' || fenceChar == '~' {
			codeAttrs.FenceChar = fenceChar
			codeAttrs.FenceLength = runLength(m.content, full.StartOffset, fenceChar)
		}
	}
	content, ok := m.codeContent(codeBlock)
	if !ok {
		content = emptyAt(m.lineEnd(full.StartOffset))
	}
	setSpans(node, full, clampTo(content, full))
	return node
}

// mapIndentedCodeBlock converts a goldmark indented CodeBlock to an mdast node.
func (m *mapper) mapIndentedCodeBlock(codeBlock *ast.CodeBlock) *mdast.Node {
	node := mdast.NewNode(mdast.NodeCodeBlock)

	codeAttrs := &mdast.CodeBlockAttrs{
		Indented: true,
	}

	node.Block = mdast.NewBlockAttrs().WithCodeBlock(codeAttrs)

	content, hasContent := m.codeContent(codeBlock)
	extent := m.block(codeBlock)
	switch {
	case extent != nil && hasContent:
		full := m.extentRange(extent)
		setSpans(node, full, clampTo(content, full))
	case hasContent:
		setSpans(node, content, content)
	}
	return node
}

// codeContent returns the range of a code block's content lines, excluding
// the line ending of the last one.
func (m *mapper) codeContent(codeBlock ast.Node) (mdast.SourceRange, bool) {
	lines := codeBlock.Lines()
	if lines.Len() == 0 {
		return mdast.SourceRange{}, false
	}
	first, last := lines.At(0), lines.At(lines.Len()-1)
	start := m.abs(first.Start)
	return mdast.SourceRange{StartOffset: start, EndOffset: max(start, m.lineEnd(m.abs(last.Start)))}, true
}

// appendText appends a goldmark Text node. Goldmark marks the text that
// ends a line with the line break that follows it; the mapper appends the
// text and then a separate break node, so the text stays a Text node.
func (m *mapper) appendText(parent *mdast.Node, textNode *ast.Text) {
	segment := textNode.Segment
	span := mdast.SourceRange{StartOffset: m.abs(segment.Start), EndOffset: m.abs(segment.Stop)}

	breakKind := mdast.NodeText
	switch {
	case textNode.HardLineBreak():
		breakKind = mdast.NodeHardBreak
	case textNode.SoftLineBreak():
		breakKind = mdast.NodeSoftBreak
	}

	if breakKind == mdast.NodeText || !span.IsEmpty() {
		node := mdast.NewNode(mdast.NodeText)
		node.Inline = mdast.NewInlineAttrs().WithText(textNode.Value(m.source))
		setSpans(node, span, span)
		mdast.AppendChild(parent, node)
	}
	if breakKind == mdast.NodeText {
		return
	}

	// The break runs from the end of the text, over any trailing spaces or
	// backslash, through the line ending.
	brk := mdast.NewNode(breakKind)
	brk.Inline = mdast.NewInlineAttrs()
	full := mdast.SourceRange{StartOffset: span.EndOffset, EndOffset: m.lineEndingEnd(span.EndOffset)}
	setSpans(brk, full, full)
	mdast.AppendChild(parent, brk)
}

// mapEmphasis converts a goldmark Emphasis node to an mdast node.
func (m *mapper) mapEmphasis(emphasis *ast.Emphasis) *mdast.Node {
	var node *mdast.Node

	if emphasis.Level == 2 {
		node = mdast.NewNode(mdast.NodeStrong)
		node.Inline = mdast.NewInlineAttrs().WithEmphasisLevel(2)
	} else {
		node = mdast.NewNode(mdast.NodeEmphasis)
		node.Inline = mdast.NewInlineAttrs().WithEmphasisLevel(1)
	}

	m.mapChildren(emphasis, node)
	m.setDelimitedSpans(node, emphasis.Level, "*_")
	return node
}

// mapCodeSpan converts a goldmark CodeSpan to an mdast node.
func (m *mapper) mapCodeSpan(codeSpan *ast.CodeSpan) *mdast.Node {
	node := mdast.NewNode(mdast.NodeCodeSpan)

	// Extract the code content.
	var value []byte
	for child := codeSpan.FirstChild(); child != nil; child = child.NextSibling() {
		if textNode, ok := child.(*ast.Text); ok {
			value = append(value, textNode.Value(m.source)...)
		}
	}

	node.Inline = mdast.NewInlineAttrs().WithText(value)

	if extent, ok := m.inline(codeSpan); ok {
		full := m.extentSpan(extent)
		ticks := min(runLength(m.content, full.StartOffset, '`'), full.Len()/2)
		setSpans(node, full, mdast.SourceRange{StartOffset: full.StartOffset + ticks, EndOffset: full.EndOffset - ticks})
		return node
	}
	if span, ok := m.childSegments(codeSpan); ok {
		setSpans(node, span, span)
	}
	return node
}

// mapLink converts a goldmark Link to an mdast node.
// goldmark resolves reference links during parsing, so the reference style
// and label are read back from the link's recorded source span.
func (m *mapper) mapLink(link *ast.Link) *mdast.Node {
	node := mdast.NewNode(mdast.NodeLink)

	linkAttrs := &mdast.LinkAttrs{
		Destination:    string(link.Destination),
		Title:          string(link.Title),
		ReferenceStyle: mdast.RefStyleInline, // Replaced from the source span when recorded.
	}

	node.Inline = mdast.NewInlineAttrs().WithLink(linkAttrs)
	m.mapChildren(link, node)
	m.setLinkSpans(node, link)
	return node
}

// mapImage converts a goldmark Image to an mdast node, reading its reference
// style the same way as mapLink.
func (m *mapper) mapImage(img *ast.Image) *mdast.Node {
	node := mdast.NewNode(mdast.NodeImage)

	linkAttrs := &mdast.LinkAttrs{
		Destination:    string(img.Destination),
		Title:          string(img.Title),
		ReferenceStyle: mdast.RefStyleInline, // Replaced from the source span when recorded.
	}

	node.Inline = mdast.NewInlineAttrs().WithLink(linkAttrs)
	m.mapChildren(img, node)
	m.setLinkSpans(node, img)
	return node
}

// mapAutoLink converts a goldmark AutoLink to an mdast node.
func (m *mapper) mapAutoLink(al *ast.AutoLink) *mdast.Node {
	node := mdast.NewNode(mdast.NodeLink)

	url := string(al.URL(m.source))
	linkAttrs := &mdast.LinkAttrs{
		Destination:    url,
		ReferenceStyle: mdast.RefStyleAutolink,
	}

	node.Inline = mdast.NewInlineAttrs().WithLink(linkAttrs)

	// Create a text child with the URL.
	label := al.Label(m.source)
	textNode := mdast.NewNode(mdast.NodeText)
	textNode.Inline = mdast.NewInlineAttrs().WithText(label)
	mdast.AppendChild(node, textNode)

	if extent, ok := m.inline(al); ok {
		full := m.extentSpan(extent)
		content := mdast.SourceRange{StartOffset: max(full.StartOffset, full.EndOffset-len(label)), EndOffset: full.EndOffset}
		if full.Len() >= 2 && m.content[full.StartOffset] == '<' && m.content[full.EndOffset-1] == '>' {
			// An autolink in angle brackets: <https://example.com>.
			content = mdast.SourceRange{StartOffset: full.StartOffset + 1, EndOffset: full.EndOffset - 1}
		} else {
			// An extended (GFM) autolink. The parser may have consumed the
			// character that triggered it; the link is the label alone.
			full = content
		}
		setSpans(node, full, content)
		setSpans(textNode, content, content)
	}

	return node
}

// mapString converts a goldmark String node to an mdast text node.
func (m *mapper) mapString(s *ast.String) *mdast.Node {
	node := mdast.NewNode(mdast.NodeText)
	node.Inline = mdast.NewInlineAttrs().WithText(s.Value)
	return node
}

// mapStrikethrough converts a GFM Strikethrough to an mdast node.
func (m *mapper) mapStrikethrough(s *east.Strikethrough) *mdast.Node {
	node := mdast.NewNode(mdast.NodeStrikethrough)
	m.mapChildren(s, node)
	m.setStrikethroughSpans(node)
	return node
}

// mapTaskCheckBox converts a GFM TaskCheckBox to an mdast node.
func (m *mapper) mapTaskCheckBox(cb *east.TaskCheckBox) *mdast.Node {
	node := mdast.NewNode(mdast.NodeTaskCheckbox)
	node.Inline = mdast.NewInlineAttrs()
	node.Inline.Checked = cb.IsChecked
	// The parser also consumes the spaces after the closing bracket.
	if extent, ok := m.inline(cb); ok {
		full := m.extentSpan(extent)
		full.EndOffset = m.trimRight(full.StartOffset, full.EndOffset)
		setSpans(node, full, full)
	}
	return node
}

// mapTable converts a GFM Table to an mdast node.
func (m *mapper) mapTable(table *east.Table) *mdast.Node {
	node := mdast.NewNode(mdast.NodeTable)
	alignments := make([]mdast.Alignment, len(table.Alignments))
	for idx, alignment := range table.Alignments {
		alignments[idx] = mapAlignment(alignment)
	}
	node.Block = mdast.NewBlockAttrs().WithTable(&mdast.TableAttrs{Alignments: alignments})

	var rows []mdast.SourceRange
	if recorded, ok := m.tableRows(table); ok {
		rows = recorded
	}
	rowIndex := 0
	for child := table.FirstChild(); child != nil; child = child.NextSibling() {
		var rowSpan *mdast.SourceRange
		if rowIndex < len(rows) {
			rowSpan = &rows[rowIndex]
		}
		rowIndex++
		switch row := child.(type) {
		case *east.TableHeader:
			mdast.AppendChild(node, m.mapTableRow(row, true, rowSpan))
		case *east.TableRow:
			mdast.AppendChild(node, m.mapTableRow(row, false, rowSpan))
		default:
			if mdNode := m.mapNode(child); mdNode != nil {
				mdast.AppendChild(node, mdNode)
			}
		}
	}

	if len(rows) > 0 {
		full := mdast.SourceRange{StartOffset: rows[0].StartOffset, EndOffset: rows[len(rows)-1].EndOffset}
		if delimiter, ok := m.tableDelimiter(table); ok {
			full.EndOffset = max(full.EndOffset, delimiter.EndOffset)
		}
		setSpans(node, full, full)
		return node
	}
	m.setChildrenSpans(node)
	return node
}

// tableRows returns the source range of each header and body row of a
// table, in child order, from the lines recorded when it was built.
func (m *mapper) tableRows(table *east.Table) ([]mdast.SourceRange, bool) {
	if m.rec == nil {
		return nil, false
	}
	recorded, ok := m.rec.tables[table]
	if !ok || len(recorded.lines) < 2 {
		return nil, false
	}
	rows := make([]mdast.SourceRange, 0, len(recorded.lines)-1)
	for idx, segment := range recorded.lines {
		if idx == 1 {
			continue // The delimiter row has no node.
		}
		rows = append(rows, m.trimmedLine(segment))
	}
	return rows, true
}

// tableDelimiter returns the source range of a table's delimiter row.
func (m *mapper) tableDelimiter(table *east.Table) (mdast.SourceRange, bool) {
	if m.rec == nil {
		return mdast.SourceRange{}, false
	}
	recorded, ok := m.rec.tables[table]
	if !ok || len(recorded.lines) < 2 {
		return mdast.SourceRange{}, false
	}
	return m.trimmedLine(recorded.lines[1]), true
}

// mapTableRow converts a GFM table header or row to an mdast node.
func (m *mapper) mapTableRow(row ast.Node, header bool, span *mdast.SourceRange) *mdast.Node {
	node := mdast.NewNode(mdast.NodeTableRow)
	node.Block = mdast.NewBlockAttrs().WithTable(&mdast.TableAttrs{Header: header})
	for child := row.FirstChild(); child != nil; child = child.NextSibling() {
		if cell, ok := child.(*east.TableCell); ok {
			mdast.AppendChild(node, m.mapTableCell(cell, header))
		}
	}
	if span != nil {
		setSpans(node, *span, *span)
		// Cells padded in by goldmark for missing columns have no source:
		// place them at the end of the row.
		for cell := node.FirstChild; cell != nil; cell = cell.Next {
			if cell.SourceSpan == nil {
				setSpans(cell, emptyAt(span.EndOffset), emptyAt(span.EndOffset))
			}
		}
		return node
	}
	m.setChildrenSpans(node)
	return node
}

// mapAlignment converts a goldmark table alignment.
func mapAlignment(alignment east.Alignment) mdast.Alignment {
	switch alignment {
	case east.AlignLeft:
		return mdast.AlignLeft
	case east.AlignCenter:
		return mdast.AlignCenter
	case east.AlignRight:
		return mdast.AlignRight
	case east.AlignNone:
		return mdast.AlignNone
	default:
		return mdast.AlignNone
	}
}

// mapTableCell converts a GFM TableCell to an mdast node.
func (m *mapper) mapTableCell(tc *east.TableCell, header bool) *mdast.Node {
	node := mdast.NewNode(mdast.NodeTableCell)
	node.Block = mdast.NewBlockAttrs().WithTable(&mdast.TableAttrs{Header: header, Alignment: mapAlignment(tc.Alignment)})
	m.mapChildren(tc, node)
	if lines := tc.Lines(); lines.Len() > 0 {
		segment := lines.At(0)
		span := mdast.SourceRange{StartOffset: m.abs(segment.Start), EndOffset: m.abs(segment.Stop)}
		setSpans(node, span, span)
	}
	return node
}

// Span helpers.

// abs converts an offset in the parsed source to an offset in the content.
func (m *mapper) abs(offset int) int {
	return offset + m.base
}

// block returns the recorded extent of a block, or nil.
func (m *mapper) block(node ast.Node) *blockExtent {
	if m.rec == nil {
		return nil
	}
	return m.rec.blocks[node]
}

// inline returns the recorded extent of an inline node.
func (m *mapper) inline(node ast.Node) (inlineExtent, bool) {
	if m.rec == nil {
		return inlineExtent{}, false
	}
	extent, ok := m.rec.inlines[node]
	return extent, ok
}

// extentRange converts a block extent into a full span: from the block's
// start to the end of the last line it owns, excluding the line ending.
func (m *mapper) extentRange(extent *blockExtent) mdast.SourceRange {
	start := m.abs(extent.start)
	return mdast.SourceRange{StartOffset: start, EndOffset: max(start, m.lineEnd(m.abs(extent.lastLine)))}
}

// extentSpan converts a recorded inline extent into a span.
func (m *mapper) extentSpan(extent inlineExtent) mdast.SourceRange {
	return mdast.SourceRange{StartOffset: m.abs(extent.start), EndOffset: m.abs(max(extent.start, extent.end))}
}

// lineIndex returns the index of the line containing offset.
func (m *mapper) lineIndex(offset int) int {
	idx := sort.Search(len(m.lines), func(i int) bool {
		return m.lines[i].EndOffset > offset
	})
	return min(idx, len(m.lines)-1)
}

// lineEnd returns the offset where the line containing offset ends,
// excluding its line ending.
func (m *mapper) lineEnd(offset int) int {
	if len(m.lines) == 0 {
		return offset
	}
	return m.lines[m.lineIndex(offset)].NewlineStart
}

// lineEndingEnd returns the offset just after the line ending of the line
// containing offset.
func (m *mapper) lineEndingEnd(offset int) int {
	if len(m.lines) == 0 {
		return offset
	}
	return m.lines[m.lineIndex(offset)].EndOffset
}

// trimRight moves end back over trailing whitespace, never before start.
func (m *mapper) trimRight(start, end int) int {
	return m.trimRightSet(start, end, " \t\r\n")
}

// trimLineEnding moves end back over a trailing line ending, never before
// start.
func (m *mapper) trimLineEnding(start, end int) int {
	return m.trimRightSet(start, end, "\r\n")
}

func (m *mapper) trimRightSet(start, end int, set string) int {
	end = min(end, len(m.content))
	for end > start && strings.IndexByte(set, m.content[end-1]) >= 0 {
		end--
	}
	return end
}

// trimmedLine returns the range of a line segment without surrounding
// whitespace or its line ending.
func (m *mapper) trimmedLine(segment text.Segment) mdast.SourceRange {
	start := m.abs(segment.Start)
	end := min(m.abs(segment.Stop), m.lineEnd(start))
	for start < end && (m.content[start] == ' ' || m.content[start] == '\t') {
		start++
	}
	return mdast.SourceRange{StartOffset: start, EndOffset: m.trimRight(start, end)}
}

// setParagraphSpans gives paragraph-like blocks their spans from their line
// segments. Goldmark trims each paragraph line, so the first segment starts
// at the first non-blank byte; the full span runs to the end of the last line.
func (m *mapper) setParagraphSpans(node *mdast.Node, gmNode ast.Node) {
	lines := gmNode.Lines()
	if lines.Len() == 0 {
		m.setChildrenSpans(node)
		return
	}
	first, last := lines.At(0), lines.At(lines.Len()-1)
	start := m.abs(first.Start)
	full := mdast.SourceRange{StartOffset: start, EndOffset: max(start, m.lineEnd(m.abs(last.Start)))}
	content := mdast.SourceRange{StartOffset: start, EndOffset: m.trimRight(start, m.abs(last.Stop))}
	setSpans(node, full, clampTo(content, full))
}

// setContainerSpans gives lists, list items and blockquotes their spans:
// from the recorded opening marker to the end of the last line the block or
// any of its children owns. The content span covers the children.
func (m *mapper) setContainerSpans(node *mdast.Node, gmNode ast.Node, withContent bool) {
	children, hasChildren := childrenRange(node)
	extent := m.block(gmNode)
	if extent == nil {
		if hasChildren {
			setSpans(node, children, children)
		}
		return
	}
	full := m.extentRange(extent)
	if hasChildren {
		full.EndOffset = max(full.EndOffset, children.EndOffset)
	}
	if !withContent {
		setSpans(node, full, full)
		return
	}
	content := emptyAt(full.EndOffset)
	if hasChildren {
		content = children
	}
	setSpans(node, full, clampTo(content, full))
}

// setListItemSpans gives a list item its spans. The content starts where
// goldmark found it on the item's first line, after the marker and the
// spaces the item claims, so it also covers link reference definitions
// there, which have no node. When the first line holds only the marker, the
// content starts at the first child.
func (m *mapper) setListItemSpans(node *mdast.Node, gmNode ast.Node) {
	m.setContainerSpans(node, gmNode, true)
	extent := m.block(gmNode)
	if extent == nil || extent.contentStart < 0 || node.SourceSpan == nil {
		return
	}
	full := *node.SourceSpan
	content := mdast.SourceRange{StartOffset: m.abs(extent.contentStart), EndOffset: full.EndOffset}
	if children, ok := childrenRange(node); ok {
		content.EndOffset = children.EndOffset
	}
	setSpans(node, full, clampTo(content, full))
}

// setLeafBlockSpans gives thematic breaks and HTML blocks their spans.
func (m *mapper) setLeafBlockSpans(node *mdast.Node, gmNode ast.Node) {
	if extent := m.block(gmNode); extent != nil {
		full := m.extentRange(extent)
		setSpans(node, full, full)
		return
	}
	lines := gmNode.Lines()
	if lines.Len() == 0 {
		return
	}
	first, last := lines.At(0), lines.At(lines.Len()-1)
	span := mdast.SourceRange{StartOffset: m.abs(first.Start), EndOffset: m.trimRight(m.abs(first.Start), m.abs(last.Stop))}
	setSpans(node, span, span)
}

// setDelimitedSpans gives emphasis-like nodes their spans: the content runs
// from the first to the last child, and the full span adds width delimiter
// characters on each side.
func (m *mapper) setDelimitedSpans(node *mdast.Node, width int, delimiters string) {
	content, ok := childrenRange(node)
	if !ok {
		return
	}
	full := mdast.SourceRange{StartOffset: content.StartOffset - width, EndOffset: content.EndOffset + width}
	if !m.allIn(full.StartOffset, content.StartOffset, delimiters) || !m.allIn(content.EndOffset, full.EndOffset, delimiters) {
		full = content
	}
	setSpans(node, full, content)
}

// setStrikethroughSpans gives strikethrough its spans. Goldmark does not
// record how many tildes a strikethrough consumed, so count the run on
// each side, up to the two GFM allows.
func (m *mapper) setStrikethroughSpans(node *mdast.Node) {
	content, ok := childrenRange(node)
	if !ok {
		return
	}
	const maxTildes = 2
	before, after := 0, 0
	for before < maxTildes && content.StartOffset-before > 0 && m.content[content.StartOffset-before-1] == '~' {
		before++
	}
	for after < maxTildes && content.EndOffset+after < len(m.content) && m.content[content.EndOffset+after] == '~' {
		after++
	}
	m.setDelimitedSpans(node, min(before, after), "~")
}

// allIn reports whether every byte of content[start:end] is in set.
func (m *mapper) allIn(start, end int, set string) bool {
	if start < 0 || end > len(m.content) || start > end {
		return false
	}
	for idx := start; idx < end; idx++ {
		found := false
		for pos := range len(set) {
			if m.content[idx] == set[pos] {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// setLinkSpans gives links and images their recorded spans, and records
// which reference syntax they use.
func (m *mapper) setLinkSpans(node *mdast.Node, gmNode ast.Node) {
	if extent, ok := m.inline(gmNode); ok {
		full := m.extentSpan(extent)
		content := clampTo(mdast.SourceRange{StartOffset: m.abs(extent.contentStart), EndOffset: m.abs(max(extent.contentStart, extent.contentEnd))}, full)
		setSpans(node, full, content)
		if node.Inline != nil && node.Inline.Link != nil {
			node.Inline.Link.ReferenceStyle, node.Inline.Link.ReferenceLabel = m.referenceStyle(gmNode, full, content)
		}
		return
	}
	m.setChildrenSpans(node)
}

// referenceStyle reads the syntax of a link or image from what follows the
// closing bracket of its text: "(" for an inline link, "[label]" for a full
// reference, "[]" for a collapsed one and nothing for a shortcut. Goldmark
// only builds reference links whose label is defined.
func (m *mapper) referenceStyle(gmNode ast.Node, full, content mdast.SourceRange) (mdast.ReferenceStyle, string) {
	closing := content.EndOffset
	if closing >= full.EndOffset || m.content[closing] != ']' {
		return mdast.RefStyleInline, ""
	}
	rest := m.content[closing+1 : full.EndOffset]
	switch {
	case len(rest) == 0:
		return mdast.RefStyleShortcut, m.blockText(gmNode, content)
	case string(rest) == "[]":
		return mdast.RefStyleCollapsed, m.blockText(gmNode, content)
	case rest[0] == '[' && rest[len(rest)-1] == ']':
		label := mdast.SourceRange{StartOffset: closing + 2, EndOffset: full.EndOffset - 1}
		return mdast.RefStyleFull, m.blockText(gmNode, label)
	default:
		return mdast.RefStyleInline, ""
	}
}

// blockText returns the text of span as the enclosing block's inline
// content holds it: its line segments, which leave out the container
// prefix of each continuation line (such as "> " in a blockquote) and the
// indentation goldmark trims. Markdown punctuation is kept.
func (m *mapper) blockText(gmNode ast.Node, span mdast.SourceRange) string {
	block := gmNode.Parent()
	for block != nil && block.Type() != ast.TypeBlock {
		block = block.Parent()
	}
	if block == nil || block.Lines().Len() == 0 {
		return string(m.content[span.StartOffset:span.EndOffset])
	}
	var label strings.Builder
	lines := block.Lines()
	for idx := range lines.Len() {
		segment := lines.At(idx)
		start := max(m.abs(segment.Start), span.StartOffset)
		end := min(m.abs(segment.Stop), span.EndOffset)
		if start < end {
			label.Write(m.content[start:end])
		}
	}
	return label.String()
}

// setRawHTMLSpans gives inline HTML its span from its segments.
func (m *mapper) setRawHTMLSpans(node *mdast.Node, raw *ast.RawHTML) {
	segments := raw.Segments
	if segments.Len() == 0 {
		return
	}
	start, end := segments.At(0).Start, segments.At(0).Stop
	for idx := 1; idx < segments.Len(); idx++ {
		start = min(start, segments.At(idx).Start)
		end = max(end, segments.At(idx).Stop)
	}
	span := mdast.SourceRange{StartOffset: m.abs(start), EndOffset: m.abs(end)}
	setSpans(node, span, span)
}

// setChildrenSpans gives a node the range covered by its children.
func (m *mapper) setChildrenSpans(node *mdast.Node) {
	if span, ok := childrenRange(node); ok {
		setSpans(node, span, span)
	}
}

// childSegments returns the range covered by the text segments of a
// goldmark node's children.
func (m *mapper) childSegments(gmNode ast.Node) (mdast.SourceRange, bool) {
	start, end := -1, -1
	for child := gmNode.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, ok := child.(*ast.Text)
		if !ok {
			continue
		}
		if start < 0 || textNode.Segment.Start < start {
			start = textNode.Segment.Start
		}
		end = max(end, textNode.Segment.Stop)
	}
	if start < 0 {
		return mdast.SourceRange{}, false
	}
	return mdast.SourceRange{StartOffset: m.abs(start), EndOffset: m.abs(end)}, true
}

// childrenRange returns the range from the first to the last child that has
// a source span.
func childrenRange(node *mdast.Node) (mdast.SourceRange, bool) {
	var first, last *mdast.Node
	for child := node.FirstChild; child != nil; child = child.Next {
		if child.SourceSpan == nil {
			continue
		}
		if first == nil {
			first = child
		}
		last = child
	}
	if first == nil {
		return mdast.SourceRange{}, false
	}
	return mdast.SourceRange{StartOffset: first.SourceSpan.StartOffset, EndOffset: last.SourceSpan.EndOffset}, true
}

// setSpans sets a node's full and content spans. The content span is only
// stored when it differs from the full span.
func setSpans(node *mdast.Node, full, content mdast.SourceRange) {
	node.SourceSpan = &full
	if content != full {
		node.ContentSpan = &content
	} else {
		node.ContentSpan = nil
	}
}

// clampTo limits a range to lie within outer.
func clampTo(r, outer mdast.SourceRange) mdast.SourceRange {
	start := min(max(r.StartOffset, outer.StartOffset), outer.EndOffset)
	end := min(max(r.EndOffset, start), outer.EndOffset)
	return mdast.SourceRange{StartOffset: start, EndOffset: end}
}

// emptyAt returns an empty range at offset.
func emptyAt(offset int) mdast.SourceRange {
	return mdast.SourceRange{StartOffset: offset, EndOffset: offset}
}

// runLength counts consecutive occurrences of char starting at offset.
func runLength(content []byte, offset int, char byte) int {
	count := 0
	for idx := offset; idx < len(content) && content[idx] == char; idx++ {
		count++
	}
	return count
}

// fillMissingSpans gives every node without a span an empty span where it
// sits: at the end of its previous sibling, or at the start of its parent's
// content. Only nodes with no source of their own lack a span.
func fillMissingSpans(root *mdast.Node) {
	for child := root.FirstChild; child != nil; child = child.Next {
		if child.SourceSpan == nil {
			offset := root.ContentRange().StartOffset
			if child.Prev != nil && child.Prev.SourceSpan != nil {
				offset = child.Prev.SourceSpan.EndOffset
			}
			setSpans(child, emptyAt(offset), emptyAt(offset))
		}
		fillMissingSpans(child)
	}
}
