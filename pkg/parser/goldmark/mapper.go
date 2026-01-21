package goldmark

import (
	"github.com/jamesainslie/gomdlint/pkg/mdast"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

// mapper converts a goldmark AST into an mdast.Node tree.
type mapper struct {
	content []byte
}

// newMapper creates a new mapper for the given content.
func newMapper(content []byte) *mapper {
	return &mapper{content: content}
}

// mapDocument converts a goldmark document node to an mdast.Node tree.
func (m *mapper) mapDocument(gmDoc ast.Node) *mdast.Node {
	doc := mdast.NewDocument()
	m.mapChildren(gmDoc, doc)
	return doc
}

// mapChildren recursively maps all children of a goldmark node to mdast nodes.
func (m *mapper) mapChildren(gmParent ast.Node, parent *mdast.Node) {
	for child := gmParent.FirstChild(); child != nil; child = child.NextSibling() {
		if mdNode := m.mapNode(child); mdNode != nil {
			mdast.AppendChild(parent, mdNode)
		}
	}
}

// mapNode converts a single goldmark node to an mdast.Node.
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

	case *ast.List:
		node = m.mapList(gmn)

	case *ast.ListItem:
		node = mdast.NewNode(mdast.NodeListItem)
		m.mapChildren(gmNode, node)

	case *ast.Blockquote:
		node = mdast.NewNode(mdast.NodeBlockquote)
		m.mapChildren(gmNode, node)

	case *ast.FencedCodeBlock:
		node = m.mapFencedCodeBlock(gmn)

	case *ast.CodeBlock:
		node = m.mapIndentedCodeBlock(gmn)

	case *ast.ThematicBreak:
		node = mdast.NewNode(mdast.NodeThematicBreak)

	case *ast.HTMLBlock:
		node = mdast.NewNode(mdast.NodeHTMLBlock)

	// Inline-level nodes.
	case *ast.Text:
		node = m.mapText(gmn)

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

	case *ast.String:
		node = m.mapString(gmn)

	// GFM extension nodes.
	case *east.Strikethrough:
		node = m.mapStrikethrough(gmn)

	case *east.TaskCheckBox:
		node = m.mapTaskCheckBox(gmn)

	case *east.Table:
		node = m.mapTable(gmn)

	case *east.TableHeader:
		node = m.mapTableHeader(gmn)

	case *east.TableRow:
		node = m.mapTableRow(gmn)

	case *east.TableCell:
		node = m.mapTableCell(gmn)

	default:
		// Fallback for unknown node types.
		node = mdast.NewNode(mdast.NodeRaw)
		m.mapChildren(gmNode, node)
	}

	return node
}

// mapHeading converts a goldmark Heading to an mdast node.
func (m *mapper) mapHeading(h *ast.Heading) *mdast.Node {
	node := mdast.NewNode(mdast.NodeHeading)
	node.Block = mdast.NewBlockAttrs().WithHeadingLevel(h.Level)
	m.mapChildren(h, node)
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
	return node
}

// mapFencedCodeBlock converts a goldmark FencedCodeBlock to an mdast node.
func (m *mapper) mapFencedCodeBlock(codeBlock *ast.FencedCodeBlock) *mdast.Node {
	node := mdast.NewNode(mdast.NodeCodeBlock)

	info := ""
	if codeBlock.Info != nil {
		info = string(codeBlock.Info.Value(m.content))
	}

	// Detect fence character and length from source content.
	fenceChar, fenceLength := m.detectFenceStyle(codeBlock)

	codeAttrs := &mdast.CodeBlockAttrs{
		FenceChar:   fenceChar,
		FenceLength: fenceLength,
		Info:        info,
		Indented:    false,
	}

	node.Block = mdast.NewBlockAttrs().WithCodeBlock(codeAttrs)
	return node
}

// detectFenceStyle extracts the fence character and length from a fenced code block.
func (m *mapper) detectFenceStyle(codeBlock *ast.FencedCodeBlock) (byte, int) {
	return m.detectFenceFromPosition(codeBlock)
}

// detectFenceFromPosition detects the fence style by examining the raw content
// at the block's position.
func (m *mapper) detectFenceFromPosition(codeBlock *ast.FencedCodeBlock) (byte, int) {
	// Find the start of the block by looking at its position.
	// We need to find the opening fence line.
	lines := codeBlock.Lines()
	if lines.Len() == 0 {
		// No content lines, return defaults.
		return '`', 3
	}

	// Content starts at first line - search before it.
	searchStart := lines.At(0).Start

	// Search backwards for the fence line.
	lineStart := searchStart
	for lineStart > 0 && m.content[lineStart-1] != '\n' {
		lineStart--
	}

	// The fence should be on the line before.
	if lineStart > 0 {
		// Go to previous line.
		prevLineEnd := lineStart - 1
		prevLineStart := prevLineEnd
		for prevLineStart > 0 && m.content[prevLineStart-1] != '\n' {
			prevLineStart--
		}

		// Check the previous line for fence characters.
		return m.extractFenceFromLine(prevLineStart, prevLineEnd)
	}

	return '`', 3
}

// extractFenceFromLine extracts fence character and length from a line.
func (m *mapper) extractFenceFromLine(start, end int) (byte, int) {
	if start >= end || start >= len(m.content) {
		return '`', 3
	}

	// Skip leading whitespace.
	pos := start
	for pos < end && pos < len(m.content) && (m.content[pos] == ' ' || m.content[pos] == '\t') {
		pos++
	}

	if pos >= end || pos >= len(m.content) {
		return '`', 3
	}

	// Detect fence character.
	fenceChar := m.content[pos]
	if fenceChar != '`' && fenceChar != '~' {
		return '`', 3
	}

	// Count fence length.
	fenceLength := 0
	for pos < end && pos < len(m.content) && m.content[pos] == fenceChar {
		fenceLength++
		pos++
	}

	if fenceLength < 3 {
		fenceLength = 3
	}

	return fenceChar, fenceLength
}

// mapIndentedCodeBlock converts a goldmark indented CodeBlock to an mdast node.
func (m *mapper) mapIndentedCodeBlock(_ *ast.CodeBlock) *mdast.Node {
	node := mdast.NewNode(mdast.NodeCodeBlock)

	codeAttrs := &mdast.CodeBlockAttrs{
		Indented: true,
	}

	node.Block = mdast.NewBlockAttrs().WithCodeBlock(codeAttrs)
	return node
}

// mapText converts a goldmark Text node to an mdast node.
func (m *mapper) mapText(textNode *ast.Text) *mdast.Node {
	// Check for soft/hard breaks.
	if textNode.SoftLineBreak() {
		return mdast.NewNode(mdast.NodeSoftBreak)
	}
	if textNode.HardLineBreak() {
		return mdast.NewNode(mdast.NodeHardBreak)
	}

	node := mdast.NewNode(mdast.NodeText)
	node.Inline = mdast.NewInlineAttrs().WithText(textNode.Value(m.content))
	return node
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
	return node
}

// mapCodeSpan converts a goldmark CodeSpan to an mdast node.
func (m *mapper) mapCodeSpan(codeSpan *ast.CodeSpan) *mdast.Node {
	node := mdast.NewNode(mdast.NodeCodeSpan)

	// Extract the code content.
	var text []byte
	for child := codeSpan.FirstChild(); child != nil; child = child.NextSibling() {
		if textNode, ok := child.(*ast.Text); ok {
			text = append(text, textNode.Value(m.content)...)
		}
	}

	node.Inline = mdast.NewInlineAttrs().WithText(text)
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
	return node
}

// mapAutoLink converts a goldmark AutoLink to an mdast node.
func (m *mapper) mapAutoLink(al *ast.AutoLink) *mdast.Node {
	node := mdast.NewNode(mdast.NodeLink)

	url := string(al.URL(m.content))
	linkAttrs := &mdast.LinkAttrs{
		Destination:    url,
		ReferenceStyle: mdast.RefStyleAutolink,
	}

	node.Inline = mdast.NewInlineAttrs().WithLink(linkAttrs)

	// Create a text child with the URL.
	textNode := mdast.NewNode(mdast.NodeText)
	textNode.Inline = mdast.NewInlineAttrs().WithText(al.Label(m.content))
	mdast.AppendChild(node, textNode)

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
	return node
}

// mapTaskCheckBox converts a GFM TaskCheckBox to an mdast node.
func (m *mapper) mapTaskCheckBox(cb *east.TaskCheckBox) *mdast.Node {
	node := mdast.NewNode(mdast.NodeText)
	node.Ext = map[string]any{
		"taskCheckbox": true,
		"checked":      cb.IsChecked,
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
	m.mapChildren(table, node)
	return node
}

// mapTableHeader converts a GFM TableHeader to an mdast node.
func (m *mapper) mapTableHeader(th *east.TableHeader) *mdast.Node {
	node := mdast.NewNode(mdast.NodeRaw)
	node.Ext = map[string]any{"tableHeader": true}
	m.mapChildren(th, node)
	return node
}

// mapTableRow converts a GFM TableRow to an mdast node.
func (m *mapper) mapTableRow(tr *east.TableRow) *mdast.Node {
	node := mdast.NewNode(mdast.NodeRaw)
	node.Ext = map[string]any{"tableRow": true}
	m.mapChildren(tr, node)
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
	return node
}

// getNodeByteRange extracts the byte range for a goldmark node.
func getNodeByteRange(gmNode ast.Node, content []byte) (int, int) {
	// Inline nodes don't have Lines() and will panic if called.
	if gmNode.Type() == ast.TypeInline {
		return getInlineNodeByteRange(gmNode, content)
	}

	lines := gmNode.Lines()
	if lines.Len() == 0 {
		return -1, -1
	}

	// Get the first and last line segments.
	first := lines.At(0)
	last := lines.At(lines.Len() - 1)

	return first.Start, last.Stop
}

// getInlineNodeByteRange extracts byte range for inline nodes.
func getInlineNodeByteRange(gmNode ast.Node, _ []byte) (int, int) {
	// For inline nodes, we need to traverse text segments.
	start := -1
	end := -1

	// Handle RawHTML nodes specially - they have Segments.
	if rawHTML, ok := gmNode.(*ast.RawHTML); ok {
		segs := rawHTML.Segments
		for i := range segs.Len() {
			seg := segs.At(i)
			if start == -1 || seg.Start < start {
				start = seg.Start
			}
			if seg.Stop > end {
				end = seg.Stop
			}
		}
		return start, end
	}

	// Try to get range from text children.
	for child := gmNode.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			seg := t.Segment
			if start == -1 || seg.Start < start {
				start = seg.Start
			}
			if seg.Stop > end {
				end = seg.Stop
			}
		}
	}

	// Also check the node's own text segment if it's a Text node.
	if t, ok := gmNode.(*ast.Text); ok {
		seg := t.Segment
		if start == -1 || seg.Start < start {
			start = seg.Start
		}
		if seg.Stop > end {
			end = seg.Stop
		}
	}

	return start, end
}
