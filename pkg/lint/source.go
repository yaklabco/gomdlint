package lint

import (
	"bytes"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Source structure helpers built on the node span contract documented in
// package mdast.

// CodeContentLines returns the 1-based numbers of the lines that hold code:
// every line of an indented code block, and the lines between the fences of
// a fenced code block. The fence lines themselves are not included; use the
// full span of a code block (or RuleContext.IsLineInCodeBlock) to cover them.
func CodeContentLines(blocks []*mdast.Node) map[int]bool {
	lines := make(map[int]bool)
	for _, block := range blocks {
		pos := block.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		first, last := pos.StartLine, pos.EndLine
		if IsFencedCodeBlock(block) {
			first++
			if block.Block.CodeBlock.Closed {
				last--
			}
		}
		for line := first; line <= last; line++ {
			lines[line] = true
		}
	}
	return lines
}

// Container helpers.
//
// A node inside blockquotes or list items sits behind a container prefix on
// each of its lines: the '>' markers of the blockquotes and the indentation
// (or, on an item's first line, the marker) of the list items. Rules that
// count or insert blank lines next to such a node must look past that
// prefix, and must repeat it in what they insert, or a fix splits the
// container.

// tabWidth is the tab stop CommonMark uses to measure indentation.
const tabWidth = 4

// maxMarkerIndent is the indentation a blockquote marker may have.
const maxMarkerIndent = 3

// utf8BOM is the UTF-8 byte order mark, which is not part of a line's text.
const utf8BOM = "\xEF\xBB\xBF"

// prefixPart is the part of a line that one container claims.
type prefixPart struct {
	kind mdast.NodeKind

	// indent is the indentation, in columns, before a blockquote's '>'.
	indent int

	// space records that the optional space after a blockquote's '>' was
	// consumed. For a tab it is one column of the tab.
	space bool

	// width is the number of columns a list item claims.
	width int
}

// lineCursor walks the columns at the start of a line. A tab spans the
// columns up to the next tab stop and can be consumed one column at a time,
// as CommonMark does for the space after '>'.
type lineCursor struct {
	content []byte

	// pos is the offset of the next byte not yet fully consumed.
	pos int

	// col is the column the cursor is at.
	col int

	// tabRest is the number of columns of the tab at pos that are not yet
	// consumed, after some of them were.
	tabRest int
}

// peek returns the byte at the cursor, 0 at the end of the line. The rest of
// a partly consumed tab reads as a space.
func (c *lineCursor) peek() byte {
	if c.tabRest > 0 {
		return ' '
	}
	if c.pos >= len(c.content) {
		return 0
	}
	return c.content[c.pos]
}

// atEnd reports whether every column of the line is consumed.
func (c *lineCursor) atEnd() bool {
	return c.tabRest == 0 && c.pos >= len(c.content)
}

// advance consumes one column.
func (c *lineCursor) advance() {
	switch {
	case c.tabRest > 0:
		c.tabRest--
		c.col++
		if c.tabRest == 0 {
			c.pos++
		}
	case c.pos >= len(c.content):
	case c.content[c.pos] == '\t':
		width := tabWidth - c.col%tabWidth
		c.col++
		if width == 1 {
			c.pos++
		} else {
			c.tabRest = width - 1
		}
	default:
		c.pos++
		c.col++
	}
}

// restIsBlank reports whether the line holds only whitespace from the cursor.
func (c *lineCursor) restIsBlank() bool {
	return len(bytes.TrimSpace(c.content[c.pos:])) == 0
}

// isBlank reports whether ch is a space or a tab.
func isBlank(ch byte) bool {
	return ch == ' ' || ch == '\t'
}

// containersOf returns the blockquotes and list items enclosing node,
// outermost first. The node itself is not included.
func containersOf(node *mdast.Node) []*mdast.Node {
	var containers []*mdast.Node
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeBlockquote || parent.Kind == mdast.NodeListItem {
			containers = append(containers, parent)
		}
	}
	for left, right := 0, len(containers)-1; left < right; left, right = left+1, right-1 {
		containers[left], containers[right] = containers[right], containers[left]
	}
	return containers
}

// listItemIndent returns the content indentation of a list item.
func listItemIndent(item *mdast.Node) int {
	if item.Block != nil && item.Block.ListItem != nil {
		return item.Block.ListItem.ContentIndent
	}
	return 0
}

// scanPrefix splits the start of a line into the parts claimed by the
// containers of node and returns them with the offset where the prefix
// ends. It stops at the first container the line does not continue, such as
// a lazy continuation line without a '>' marker. Columns follow CommonMark:
// tabs advance to the next multiple of four, and a tab that is only partly
// claimed stays with the rest of the line. A byte order mark at the start of
// the first line is skipped and counts as part of the prefix.
func scanPrefix(content []byte, node *mdast.Node, lineNum int) ([]prefixPart, int) {
	cursor := &lineCursor{content: content}
	if lineNum == 1 && bytes.HasPrefix(content, []byte(utf8BOM)) {
		cursor.pos = len(utf8BOM)
	}
	var parts []prefixPart
	for _, container := range containersOf(node) {
		switch container.Kind {
		case mdast.NodeBlockquote:
			saved := *cursor
			indent := 0
			for indent < maxMarkerIndent && isBlank(cursor.peek()) {
				cursor.advance()
				indent++
			}
			if cursor.peek() != '>' {
				*cursor = saved
				return parts, cursor.pos
			}
			cursor.advance()
			part := prefixPart{kind: mdast.NodeBlockquote, indent: indent}
			if isBlank(cursor.peek()) {
				cursor.advance()
				part.space = true
			}
			parts = append(parts, part)
		case mdast.NodeListItem:
			width := listItemIndent(container)
			if lineNum == container.SourcePosition().StartLine {
				// The item's first line: the marker and the spaces after it.
				for consumed := 0; consumed < width && !cursor.atEnd(); consumed++ {
					cursor.advance()
				}
				parts = append(parts, prefixPart{kind: mdast.NodeListItem, width: width})
				continue
			}
			saved := *cursor
			consumed := 0
			for consumed < width && isBlank(cursor.peek()) {
				cursor.advance()
				consumed++
			}
			if consumed < width && !cursor.restIsBlank() {
				// Not indented enough to continue the item.
				*cursor = saved
				return parts, cursor.pos
			}
			parts = append(parts, prefixPart{kind: mdast.NodeListItem, width: width})
		}
	}
	return parts, cursor.pos
}

// ContainerPrefix returns the bytes at the start of a 1-based line that
// belong to the blockquotes and list items enclosing node: for each
// blockquote, up to three spaces, '>' and one optional space; for each list
// item, its marker and the spaces after it on the item's first line, or its
// content indentation on later lines. It stops at the first container the
// line does not continue, so a lazy continuation line or a line outside the
// containers gets a shorter prefix. Columns are measured with tab stops of
// four; a tab that the containers claim only in part is left out. A byte
// order mark at the start of the first line is included, so the prefix
// length is an offset into the line.
func ContainerPrefix(file *mdast.FileSnapshot, node *mdast.Node, lineNum int) []byte {
	content := RawLineContent(file, lineNum)
	if content == nil || node == nil {
		return nil
	}
	_, end := scanPrefix(content, node, lineNum)
	return content[:end]
}

// IsBlankLineInContainer reports whether a 1-based line is blank once the
// container prefix of node is removed. Inside a blockquote a line holding
// only '>' is blank; outside one it is not. Metadata lines are blank, as in
// IsBlankLine.
func IsBlankLineInContainer(file *mdast.FileSnapshot, node *mdast.Node, lineNum int) bool {
	content := LineContent(file, lineNum)
	if node == nil {
		return len(bytes.TrimSpace(content)) == 0
	}
	_, end := scanPrefix(content, node, lineNum)
	return len(bytes.TrimSpace(content[end:])) == 0
}

// CountBlankLinesBeforeInContainer counts the consecutive lines before a
// 1-based line that are blank within the containers of node.
func CountBlankLinesBeforeInContainer(file *mdast.FileSnapshot, node *mdast.Node, lineNum int) int {
	if file == nil {
		return 0
	}
	count := 0
	for line := lineNum - 1; line >= 1 && IsBlankLineInContainer(file, node, line); line-- {
		count++
	}
	return count
}

// CountBlankLinesAfterInContainer counts the consecutive lines after a
// 1-based line that are blank within the containers of node. The empty
// line after a final newline does not count.
func CountBlankLinesAfterInContainer(file *mdast.FileSnapshot, node *mdast.Node, lineNum int) int {
	if file == nil {
		return 0
	}
	count := 0
	for line := lineNum + 1; line <= lastContentLine(file) && IsBlankLineInContainer(file, node, line); line++ {
		count++
	}
	return count
}

// lastContentLine returns the number of the last line, not counting the
// empty line that follows a final newline.
func lastContentLine(file *mdast.FileSnapshot) int {
	count := len(file.Lines)
	if count > 0 && file.Lines[count-1].StartOffset == len(file.Content) {
		count--
	}
	return count
}

// ContinuationPrefix returns the prefix a new line needs to continue inside
// the containers of node, as they stand on the node's first line: each
// blockquote's indentation, '>' and optional space, and each list item's
// content indentation, all written with spaces. Tabs and a byte order mark
// are not reproduced.
func ContinuationPrefix(file *mdast.FileSnapshot, node *mdast.Node) string {
	if file == nil || node == nil {
		return ""
	}
	lineNum := node.SourcePosition().StartLine
	parts, _ := scanPrefix(RawLineContent(file, lineNum), node, lineNum)
	var prefix strings.Builder
	for _, part := range parts {
		switch part.kind {
		case mdast.NodeBlockquote:
			prefix.WriteString(strings.Repeat(" ", part.indent))
			prefix.WriteByte('>')
			if part.space {
				prefix.WriteByte(' ')
			}
		default:
			prefix.WriteString(strings.Repeat(" ", part.width))
		}
	}
	return prefix.String()
}

// BlankLineText returns the text that inserts one blank line inside the
// containers of node: the continuation prefix without trailing whitespace,
// followed by a newline. Outside blockquotes it is "\n"; inside one it is
// ">\n", which keeps the blockquote whole where a bare newline would end it.
func BlankLineText(file *mdast.FileSnapshot, node *mdast.Node) string {
	return strings.TrimRight(ContinuationPrefix(file, node), " \t") + "\n"
}
