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

// prefixPart is the part of a line that one container claims.
type prefixPart struct {
	start, end int
	// marker is true when the part holds a list item marker, which a
	// continuation line replaces with spaces.
	marker bool
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

// prefixParts splits the start of a line into the parts claimed by the
// containers of node. It stops at the first container the line does not
// continue, such as a lazy continuation line without a '>' marker.
func prefixParts(content []byte, node *mdast.Node, lineNum int) []prefixPart {
	var parts []prefixPart
	pos, col := 0, 0
	for _, container := range containersOf(node) {
		switch container.Kind {
		case mdast.NodeBlockquote:
			end, endCol := skipSpaces(content, pos, col, col+maxMarkerIndent)
			if end >= len(content) || content[end] != '>' {
				return parts
			}
			end++
			endCol++
			if end < len(content) && (content[end] == ' ' || content[end] == '\t') {
				end, endCol = end+1, advanceColumn(content[end], endCol)
			}
			parts = append(parts, prefixPart{start: pos, end: end})
			pos, col = end, endCol
		case mdast.NodeListItem:
			target := col + listItemIndent(container)
			if lineNum == container.SourcePosition().StartLine {
				// The item's first line: the marker and the spaces after it.
				end, endCol := pos, col
				for end < len(content) && endCol < target {
					end, endCol = end+1, advanceColumn(content[end], endCol)
				}
				parts = append(parts, prefixPart{start: pos, end: end, marker: true})
				pos, col = end, endCol
				continue
			}
			end, endCol := skipSpaces(content, pos, col, target)
			if endCol < target && len(bytes.TrimSpace(content[end:])) > 0 {
				// Not indented enough to continue the item.
				return parts
			}
			parts = append(parts, prefixPart{start: pos, end: end})
			pos, col = end, endCol
		}
	}
	return parts
}

// skipSpaces consumes spaces and tabs from pos until column limit.
func skipSpaces(content []byte, pos, col, limit int) (int, int) {
	for pos < len(content) && col < limit && (content[pos] == ' ' || content[pos] == '\t') {
		pos, col = pos+1, advanceColumn(content[pos], col)
	}
	return pos, col
}

// advanceColumn returns the column after ch, expanding tabs.
func advanceColumn(ch byte, col int) int {
	if ch == '\t' {
		return col + tabWidth - col%tabWidth
	}
	return col + 1
}

// prefixLength returns the length of the container prefix in parts.
func prefixLength(parts []prefixPart) int {
	if len(parts) == 0 {
		return 0
	}
	return parts[len(parts)-1].end
}

// ContainerPrefix returns the bytes at the start of a 1-based line that
// belong to the blockquotes and list items enclosing node: for each
// blockquote, up to three spaces, '>' and one optional space; for each list
// item, its marker and the spaces after it on the item's first line, or its
// content indentation on later lines. It stops at the first container the
// line does not continue, so a lazy continuation line or a line outside the
// containers gets a shorter prefix.
func ContainerPrefix(file *mdast.FileSnapshot, node *mdast.Node, lineNum int) []byte {
	content := RawLineContent(file, lineNum)
	if content == nil || node == nil {
		return nil
	}
	return content[:prefixLength(prefixParts(content, node, lineNum))]
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
	rest := content[prefixLength(prefixParts(content, node, lineNum)):]
	return len(bytes.TrimSpace(rest)) == 0
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
// the containers of node: the blockquote markers and list indentation of
// the node's first line, with list item markers replaced by spaces.
func ContinuationPrefix(file *mdast.FileSnapshot, node *mdast.Node) string {
	if file == nil || node == nil {
		return ""
	}
	lineNum := node.SourcePosition().StartLine
	content := RawLineContent(file, lineNum)
	var prefix strings.Builder
	for _, part := range prefixParts(content, node, lineNum) {
		if !part.marker {
			prefix.Write(content[part.start:part.end])
			continue
		}
		col := 0
		for _, ch := range content[:part.start] {
			col = advanceColumn(ch, col)
		}
		endCol := col
		for _, ch := range content[part.start:part.end] {
			endCol = advanceColumn(ch, endCol)
		}
		prefix.WriteString(strings.Repeat(" ", endCol-col))
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
