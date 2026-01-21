package lint

import (
	"bytes"

	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

// Node query helpers.

// Headings returns all heading nodes in the document.
func Headings(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeHeading)
}

// Lists returns all list nodes in the document.
func Lists(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeList)
}

// CodeBlocks returns all code block nodes in the document.
func CodeBlocks(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeCodeBlock)
}

// Links returns all link nodes in the document.
func Links(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeLink)
}

// Images returns all image nodes in the document.
func Images(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeImage)
}

// Paragraphs returns all paragraph nodes in the document.
func Paragraphs(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeParagraph)
}

// Node accessor helpers.

// HeadingLevel returns the heading level for a heading node, or 0 if not a heading.
func HeadingLevel(n *mdast.Node) int {
	if n == nil || n.Kind != mdast.NodeHeading || n.Block == nil {
		return 0
	}
	return n.Block.HeadingLevel
}

// IsOrderedList returns true if the node is an ordered list.
func IsOrderedList(n *mdast.Node) bool {
	if n == nil || n.Kind != mdast.NodeList || n.Block == nil || n.Block.List == nil {
		return false
	}
	return n.Block.List.Ordered
}

// IsTightList returns true if the node is a tight list.
func IsTightList(n *mdast.Node) bool {
	if n == nil || n.Kind != mdast.NodeList || n.Block == nil || n.Block.List == nil {
		return false
	}
	return n.Block.List.Tight
}

// CodeBlockInfo returns the info string for a code block, or empty string.
func CodeBlockInfo(n *mdast.Node) string {
	if n == nil || n.Kind != mdast.NodeCodeBlock || n.Block == nil || n.Block.CodeBlock == nil {
		return ""
	}
	return n.Block.CodeBlock.Info
}

// LinkDestination returns the destination URL for a link or image.
func LinkDestination(n *mdast.Node) string {
	if n == nil || n.Inline == nil || n.Inline.Link == nil {
		return ""
	}
	return n.Inline.Link.Destination
}

// Line-based helpers.

// LineContent returns the content of the specified 1-based line number.
// Returns nil if the line number is out of range.
func LineContent(file *mdast.FileSnapshot, lineNum int) []byte {
	if file == nil || lineNum < 1 || lineNum > len(file.Lines) {
		return nil
	}
	line := file.Lines[lineNum-1]
	return file.Content[line.StartOffset:line.NewlineStart]
}

// LineLength returns the length of the specified 1-based line (excluding newline).
// Returns 0 if the line number is out of range.
func LineLength(file *mdast.FileSnapshot, lineNum int) int {
	if file == nil || lineNum < 1 || lineNum > len(file.Lines) {
		return 0
	}
	line := file.Lines[lineNum-1]
	return line.NewlineStart - line.StartOffset
}

// HasTrailingWhitespace returns true if the line has trailing whitespace.
func HasTrailingWhitespace(file *mdast.FileSnapshot, lineNum int) bool {
	content := LineContent(file, lineNum)
	if len(content) == 0 {
		return false
	}
	last := content[len(content)-1]
	return last == ' ' || last == '\t'
}

// TrailingWhitespaceRange returns the range of trailing whitespace on a line.
// Returns (-1, -1) if no trailing whitespace or line is out of range.
func TrailingWhitespaceRange(file *mdast.FileSnapshot, lineNum int) (int, int) {
	if file == nil || lineNum < 1 || lineNum > len(file.Lines) {
		return -1, -1
	}
	line := file.Lines[lineNum-1]
	content := file.Content[line.StartOffset:line.NewlineStart]
	if len(content) == 0 {
		return -1, -1
	}

	endOffset := line.NewlineStart
	startOffset := endOffset
	for idx := len(content) - 1; idx >= 0; idx-- {
		if content[idx] != ' ' && content[idx] != '\t' {
			break
		}
		startOffset = line.StartOffset + idx
	}

	if startOffset == endOffset {
		return -1, -1
	}
	return startOffset, endOffset
}

// IsBlankLine returns true if the line contains only whitespace.
func IsBlankLine(file *mdast.FileSnapshot, lineNum int) bool {
	content := LineContent(file, lineNum)
	return len(bytes.TrimSpace(content)) == 0
}

// List helpers.

// ListItems returns the direct children of a list node that are list items.
func ListItems(list *mdast.Node) []*mdast.Node {
	if list == nil || list.Kind != mdast.NodeList {
		return nil
	}

	var items []*mdast.Node
	for child := list.FirstChild; child != nil; child = child.Next {
		if child.Kind == mdast.NodeListItem {
			items = append(items, child)
		}
	}

	return items
}

// ListBulletMarker returns the bullet marker for a list, or empty string if not available.
func ListBulletMarker(list *mdast.Node) string {
	if list == nil || list.Kind != mdast.NodeList || list.Block == nil || list.Block.List == nil {
		return ""
	}
	return list.Block.List.BulletMarker
}

// ListStartNumber returns the start number for an ordered list, or 0 if not ordered.
func ListStartNumber(list *mdast.Node) int {
	if list == nil || list.Kind != mdast.NodeList || list.Block == nil || list.Block.List == nil {
		return 0
	}
	if !list.Block.List.Ordered {
		return 0
	}
	return list.Block.List.StartNumber
}

// ListDelimiter returns the delimiter for an ordered list ("." or ")"), or empty string.
func ListDelimiter(list *mdast.Node) string {
	if list == nil || list.Kind != mdast.NodeList || list.Block == nil || list.Block.List == nil {
		return ""
	}
	return list.Block.List.Delimiter
}

// Code block helpers.

// IsLineInCodeBlock returns true if the given line number falls within any code block.
func IsLineInCodeBlock(file *mdast.FileSnapshot, root *mdast.Node, lineNum int) bool {
	if file == nil || root == nil || lineNum < 1 {
		return false
	}

	codeBlocks := CodeBlocks(root)
	for _, cb := range codeBlocks {
		pos := cb.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		if lineNum >= pos.StartLine && lineNum <= pos.EndLine {
			return true
		}
	}

	return false
}

// LineContainsURL returns true if the line contains a URL (http:// or https://).
func LineContainsURL(file *mdast.FileSnapshot, lineNum int) bool {
	content := LineContent(file, lineNum)
	return bytes.Contains(content, []byte("http://")) || bytes.Contains(content, []byte("https://"))
}

// Link and image helpers.

// LinkTitle returns the title for a link or image.
func LinkTitle(n *mdast.Node) string {
	if n == nil || n.Inline == nil || n.Inline.Link == nil {
		return ""
	}
	return n.Inline.Link.Title
}

// LinkText returns the text content of a link node's children.
func LinkText(n *mdast.Node) string {
	if n == nil || (n.Kind != mdast.NodeLink && n.Kind != mdast.NodeImage) {
		return ""
	}
	return extractTextContent(n)
}

// ImageAlt returns the alt text for an image node.
// For images, this is the text content of the image's children.
func ImageAlt(n *mdast.Node) string {
	if n == nil || n.Kind != mdast.NodeImage {
		return ""
	}
	return extractTextContent(n)
}

// extractTextContent extracts all text content from a node's descendants.
func extractTextContent(n *mdast.Node) string {
	if n == nil {
		return ""
	}
	var buf bytes.Buffer
	_ = mdast.Walk(n, func(node *mdast.Node) error { //nolint:errcheck // Walk visitor never returns error
		if node.Kind == mdast.NodeText && node.Inline != nil {
			buf.Write(node.Inline.Text)
		}
		return nil
	})
	return buf.String()
}

// IsEmptyLink returns true if the link has an empty destination.
func IsEmptyLink(n *mdast.Node) bool {
	if n == nil || n.Kind != mdast.NodeLink {
		return false
	}
	return LinkDestination(n) == ""
}

// IsEmptyLinkText returns true if the link has no text content.
func IsEmptyLinkText(n *mdast.Node) bool {
	if n == nil || n.Kind != mdast.NodeLink {
		return false
	}
	text := LinkText(n)
	return len(bytes.TrimSpace([]byte(text))) == 0
}

// Code block helpers.

// IsFencedCodeBlock returns true if the code block is fenced (not indented).
func IsFencedCodeBlock(n *mdast.Node) bool {
	if n == nil || n.Kind != mdast.NodeCodeBlock || n.Block == nil || n.Block.CodeBlock == nil {
		return false
	}
	return !n.Block.CodeBlock.Indented
}

// IsIndentedCodeBlock returns true if the code block is indented (not fenced).
func IsIndentedCodeBlock(n *mdast.Node) bool {
	if n == nil || n.Kind != mdast.NodeCodeBlock || n.Block == nil || n.Block.CodeBlock == nil {
		return false
	}
	return n.Block.CodeBlock.Indented
}

// CodeFenceChar returns the fence character ('`' or '~') for a fenced code block.
func CodeFenceChar(n *mdast.Node) byte {
	if n == nil || n.Kind != mdast.NodeCodeBlock || n.Block == nil || n.Block.CodeBlock == nil {
		return 0
	}
	return n.Block.CodeBlock.FenceChar
}

// CodeFenceLength returns the number of fence characters for a fenced code block.
func CodeFenceLength(n *mdast.Node) int {
	if n == nil || n.Kind != mdast.NodeCodeBlock || n.Block == nil || n.Block.CodeBlock == nil {
		return 0
	}
	return n.Block.CodeBlock.FenceLength
}

// HTML helpers.

// HTMLBlocks returns all HTML block nodes in the document.
func HTMLBlocks(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeHTMLBlock)
}

// HTMLInlines returns all inline HTML nodes in the document.
func HTMLInlines(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeHTMLInline)
}

// ExtractHTMLTagName extracts the tag name from an HTML element.
// Returns empty string if no valid tag found.
func ExtractHTMLTagName(content []byte) string {
	content = bytes.TrimSpace(content)
	if len(content) < 2 || content[0] != '<' {
		return ""
	}

	// Skip '<' and optional '/'
	idx := 1
	if idx < len(content) && content[idx] == '/' {
		idx++
	}

	// Extract tag name (alphanumeric characters)
	start := idx
	for idx < len(content) {
		ch := content[idx]
		isAlphaNum := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-'
		if !isAlphaNum {
			break
		}
		idx++
	}

	if idx == start {
		return ""
	}

	return string(bytes.ToLower(content[start:idx]))
}

// Table helpers (GFM).

// Tables returns all table nodes in the document.
// Returns nil if the document has no tables or tables are not parsed (non-GFM).
func Tables(root *mdast.Node) []*mdast.Node {
	if root == nil {
		return nil
	}
	// Tables are stored in Ext map with key "table"
	var tables []*mdast.Node
	_ = mdast.Walk(root, func(n *mdast.Node) error { //nolint:errcheck // Walk visitor never returns error
		if n.Ext != nil {
			if _, ok := n.Ext["table"]; ok {
				tables = append(tables, n)
			}
		}
		return nil
	})
	return tables
}

// IsTableNode returns true if the node is a GFM table.
func IsTableNode(n *mdast.Node) bool {
	if n == nil || n.Ext == nil {
		return false
	}
	_, ok := n.Ext["table"]
	return ok
}

// IsLineInTable returns true if the given line number falls within any table.
func IsLineInTable(file *mdast.FileSnapshot, root *mdast.Node, lineNum int) bool {
	if file == nil || root == nil || lineNum < 1 {
		return false
	}

	tables := Tables(root)
	for _, table := range tables {
		pos := table.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		if lineNum >= pos.StartLine && lineNum <= pos.EndLine {
			return true
		}
	}

	return false
}

// Heading helpers.

// FirstHeading returns the first heading in the document, or nil if none.
func FirstHeading(root *mdast.Node) *mdast.Node {
	headings := Headings(root)
	if len(headings) == 0 {
		return nil
	}
	return headings[0]
}

// FirstBlock returns the first block-level node in the document (excluding Document itself).
func FirstBlock(root *mdast.Node) *mdast.Node {
	if root == nil {
		return nil
	}
	return root.FirstChild
}

// IsHeadingNode returns true if the node is a heading.
func IsHeadingNode(n *mdast.Node) bool {
	return n != nil && n.Kind == mdast.NodeHeading
}

// HeadingText returns the text content of a heading node.
func HeadingText(n *mdast.Node) string {
	if n == nil || n.Kind != mdast.NodeHeading {
		return ""
	}
	return extractTextContent(n)
}

// Blank line helpers.

// CountBlankLinesBefore counts consecutive blank lines before a given line.
func CountBlankLinesBefore(file *mdast.FileSnapshot, lineNum int) int {
	if file == nil || lineNum < 2 {
		return 0
	}
	count := 0
	for ln := lineNum - 1; ln >= 1; ln-- {
		if !IsBlankLine(file, ln) {
			break
		}
		count++
	}
	return count
}

// CountBlankLinesAfter counts consecutive blank lines after a given line.
func CountBlankLinesAfter(file *mdast.FileSnapshot, lineNum int) int {
	if file == nil || lineNum < 1 || lineNum >= len(file.Lines) {
		return 0
	}
	count := 0
	for ln := lineNum + 1; ln <= len(file.Lines); ln++ {
		if !IsBlankLine(file, ln) {
			break
		}
		count++
	}
	return count
}

// Inline code helpers.

// CodeSpans returns all inline code span nodes in the document.
func CodeSpans(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeCodeSpan)
}

// CodeSpanContent returns the text content of an inline code span.
func CodeSpanContent(node *mdast.Node) string {
	if node == nil || node.Kind != mdast.NodeCodeSpan {
		return ""
	}
	if node.Inline != nil && len(node.Inline.Text) > 0 {
		return string(node.Inline.Text)
	}
	return extractTextContent(node)
}

// Thematic break (horizontal rule) helpers.

// ThematicBreaks returns all thematic break (horizontal rule) nodes in the document.
func ThematicBreaks(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeThematicBreak)
}

// Emphasis helpers.

// EmphasisNodes returns all emphasis (italic) nodes in the document.
func EmphasisNodes(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeEmphasis)
}

// StrongNodes returns all strong (bold) nodes in the document.
func StrongNodes(root *mdast.Node) []*mdast.Node {
	return mdast.FindByKind(root, mdast.NodeStrong)
}
