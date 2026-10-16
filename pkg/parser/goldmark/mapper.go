package goldmark

import (
	"sort"

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

	case *ast.Paragraph:
		node = mdast.NewNode(mdast.NodeParagraph)
		m.mapChildren(gmNode, node)
		m.setParagraphSpans(node, gmNode)

	case *ast.List:
		node = m.mapList(gmn)

	case *ast.ListItem:
		node = mdast.NewNode(mdast.NodeListItem)
		m.mapChildren(gmNode, node)
		m.setContainerSpans(node, gmNode, true)

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
		// Fallback for unknown node types, including tight-list text blocks.
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
	content.EndOffset = m.trimRight(content.StartOffset, content.EndOffset)
	if extent == nil {
		setSpans(node, content, content)
		return node
	}
	full := m.extentRange(extent)
	// A setext heading opens on its underline; its text lines come first.
	full.StartOffset = min(full.StartOffset, content.StartOffset)
	setSpans(node, full, clampTo(content, full))
	return node
}

// mapList converts a goldmark List to an mdast node.
func (m *mapper) mapList(list *ast.List) *mdast.Node {
	node := mdast.NewNode(mdast.NodeList)

	listAttrs := &mdast.ListAttrs{
		Ordered:     list.IsOrdered(),
		StartNumber: list.Start,
		Tight:       list.IsTight,
	}

	// Determine bullet marker from source if available.
	if !list.IsOrdered() {
		listAttrs.BulletMarker = string(list.Marker)
	} else {
		// For ordered lists, determine delimiter.
		// goldmark doesn't expose this directly, so we infer from marker.
		listAttrs.Delimiter = "."
	}

	node.Block = mdast.NewBlockAttrs().WithList(listAttrs)
	m.mapChildren(list, node)
	m.setContainerSpans(node, list, false)
	return node
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

// appendText appends a goldmark Text node, followed by a break node when
// the text ends a line with a soft or hard line break.
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

	if breakKind == mdast.NodeText {
		node := mdast.NewNode(mdast.NodeText)
		node.Inline = mdast.NewInlineAttrs().WithText(textNode.Value(m.source))
		setSpans(node, span, span)
		mdast.AppendChild(parent, node)
		return
	}

	node := mdast.NewNode(breakKind)
	node.Inline = mdast.NewInlineAttrs().WithText(textNode.Value(m.source))
	full := mdast.SourceRange{StartOffset: span.StartOffset, EndOffset: m.lineEndingEnd(span.EndOffset)}
	setSpans(node, full, full)
	mdast.AppendChild(parent, node)
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
// Note: goldmark normalizes all reference-style links during parsing, so we cannot
// directly detect the original syntax. Reference style detection is handled by the
// refs package which analyzes source content directly.
func (m *mapper) mapLink(link *ast.Link) *mdast.Node {
	node := mdast.NewNode(mdast.NodeLink)

	linkAttrs := &mdast.LinkAttrs{
		Destination:    string(link.Destination),
		Title:          string(link.Title),
		ReferenceStyle: mdast.RefStyleInline, // Default; refs package will detect actual style
	}

	node.Inline = mdast.NewInlineAttrs().WithLink(linkAttrs)
	m.mapChildren(link, node)
	m.setLinkSpans(node, link)
	return node
}

// mapImage converts a goldmark Image to an mdast node.
// Note: goldmark normalizes all reference-style images during parsing, so we cannot
// directly detect the original syntax. Reference style detection is handled by the
// refs package which analyzes source content directly.
func (m *mapper) mapImage(img *ast.Image) *mdast.Node {
	node := mdast.NewNode(mdast.NodeImage)

	linkAttrs := &mdast.LinkAttrs{
		Destination:    string(img.Destination),
		Title:          string(img.Title),
		ReferenceStyle: mdast.RefStyleInline, // Default; refs package will detect actual style
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
	node := mdast.NewNode(mdast.NodeEmphasis)
	node.Ext = map[string]any{"strikethrough": true}
	m.mapChildren(s, node)
	m.setStrikethroughSpans(node)
	return node
}

// mapTaskCheckBox converts a GFM TaskCheckBox to an mdast node.
func (m *mapper) mapTaskCheckBox(cb *east.TaskCheckBox) *mdast.Node {
	node := mdast.NewNode(mdast.NodeText)
	node.Ext = map[string]any{
		"taskCheckbox": true,
		"checked":      cb.IsChecked,
	}
	if extent, ok := m.inline(cb); ok {
		full := m.extentSpan(extent)
		full.EndOffset = m.trimRight(full.StartOffset, full.EndOffset)
		setSpans(node, full, full)
	}
	return node
}

// mapTable converts a GFM Table to an mdast node.
func (m *mapper) mapTable(table *east.Table) *mdast.Node {
	node := mdast.NewNode(mdast.NodeRaw)
	node.Ext = map[string]any{
		"table":      true,
		"alignments": table.Alignments,
	}

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
			mdast.AppendChild(node, m.mapTableRow(row, map[string]any{"tableHeader": true}, rowSpan))
		case *east.TableRow:
			mdast.AppendChild(node, m.mapTableRow(row, map[string]any{"tableRow": true}, rowSpan))
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
func (m *mapper) mapTableRow(row ast.Node, ext map[string]any, span *mdast.SourceRange) *mdast.Node {
	node := mdast.NewNode(mdast.NodeRaw)
	node.Ext = ext
	for child := row.FirstChild(); child != nil; child = child.NextSibling() {
		if cell, ok := child.(*east.TableCell); ok {
			mdast.AppendChild(node, m.mapTableCell(cell))
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

// mapTableCell converts a GFM TableCell to an mdast node.
func (m *mapper) mapTableCell(tc *east.TableCell) *mdast.Node {
	node := mdast.NewNode(mdast.NodeRaw)
	node.Ext = map[string]any{
		"tableCell": true,
		"alignment": tc.Alignment,
	}
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
	end = min(end, len(m.content))
	for end > start {
		switch m.content[end-1] {
		case ' ', '\t', '\r', '\n':
			end--
			continue
		}
		break
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

// setLinkSpans gives links and images their recorded spans.
func (m *mapper) setLinkSpans(node *mdast.Node, gmNode ast.Node) {
	if extent, ok := m.inline(gmNode); ok {
		full := m.extentSpan(extent)
		content := mdast.SourceRange{StartOffset: m.abs(extent.contentStart), EndOffset: m.abs(max(extent.contentStart, extent.contentEnd))}
		setSpans(node, full, clampTo(content, full))
		return
	}
	m.setChildrenSpans(node)
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
