package goldmark

import (
	"bytes"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// GFMExtensions returns the goldmark extensions of the gfm flavor:
// goldmark's linkify, table and strikethrough extensions and a task list
// extension that reads checkboxes as GFM does. Every goldmark instance
// that reads GFM, such as the fix checks', builds from these, so that all
// of them read the same checkboxes as the parser.
func GFMExtensions() []goldmark.Extender {
	return []goldmark.Extender{
		extension.Linkify,
		extension.Table,
		extension.Strikethrough,
		gfmTaskList{},
	}
}

// Priorities of goldmark's task list extension: its checkbox parser runs
// before the link parser, and its renderer after goldmark's own renderers.
const (
	taskCheckBoxParserPriority   = 0
	taskCheckBoxRendererPriority = 500
)

// gfmTaskList is a task list extension that reads a checkbox where
// micromark-extension-gfm-task-list-item does, which goldmark's own task
// list extension does not: goldmark also reads one at the start of a
// heading, before other text as in "[x]foo", with a form feed or a tab of
// any width inside, and never over a line ending or after link reference
// definitions.
type gfmTaskList struct{}

// Extend registers the checkbox parser and goldmark's checkbox renderer
// with the priorities goldmark's task list extension gives them.
func (gfmTaskList) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(gfmCheckBoxParser{}, taskCheckBoxParserPriority),
	))
	md.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(extension.NewTaskCheckBoxHTMLRenderer(), taskCheckBoxRendererPriority),
	))
}

// gfmCheckBoxParser reads a task checkbox at the start of a list item's
// first content. A checkbox is "[", then a space, an "x", an "X", a tab one
// column wide or a line ending, then "]", then a space, a tab or a line
// ending and more of the paragraph. A line ending inside the brackets
// counts only when the paragraph's next line starts with the "]".
type gfmCheckBoxParser struct{}

// Trigger returns the byte that can start a checkbox.
func (gfmCheckBoxParser) Trigger() []byte {
	return []byte{'['}
}

// tabWidth is the number of columns between tab stops, as in CommonMark.
const tabWidth = 4

// Parse reads a checkbox at the start of a list item's first content, or
// returns nil so that the other inline parsers read the brackets.
func (gfmCheckBoxParser) Parse(parent ast.Node, block text.Reader, _ parser.Context) ast.Node {
	if parent.HasChildren() || !firstContentOfListItem(parent, block.Source()) {
		return nil
	}
	checked, length, ok := readCheckBox(parent, block)
	if !ok {
		return nil
	}
	block.Advance(length)
	return east.NewTaskCheckBox(checked)
}

// firstContentOfListItem reports whether a block is the paragraph that is a
// list item's first content: its first child, or the rest of the paragraph
// whose link reference definitions goldmark made the item's first children.
func firstContentOfListItem(node ast.Node, source []byte) bool {
	switch node.(type) {
	case *ast.Paragraph, *ast.TextBlock:
	default:
		return false
	}
	if _, ok := node.Parent().(*ast.ListItem); !ok {
		return false
	}
	previous := node.PreviousSibling()
	if previous == nil {
		return true
	}
	for sibling := previous; sibling != nil; sibling = sibling.PreviousSibling() {
		if _, ok := sibling.(*ast.LinkReferenceDefinition); !ok {
			return false
		}
	}
	// The definitions came off the start of this paragraph when nothing but
	// a line ending lies between the last of them and the paragraph.
	defLines, lines := previous.Lines(), node.Lines()
	if defLines.Len() == 0 || lines.Len() == 0 {
		return false
	}
	end, start := defLines.At(defLines.Len()-1).Stop, lines.At(0).Start
	return end <= start && bytes.Count(source[end:start], []byte{'\n'}) == 1
}

// readCheckBox reads a checkbox at the reader's position, which holds "[",
// and returns whether it is checked and the number of bytes to advance over
// it and the whitespace after it on its last line.
func readCheckBox(parent ast.Node, block text.Reader) (bool, int, bool) {
	line, segment := block.PeekLine()
	if len(line) < 2 || line[0] != '[' || segment.Padding != 0 {
		return false, 0, false
	}
	source := block.Source()
	switch inside := line[1]; inside {
	case 'x', 'X', ' ':
		return closeCheckBox(line, 2, inside != ' ', isLastLine(parent, block))
	case '\t':
		if tabColumns(source, segment.Start+1) != 1 {
			return false, 0, false
		}
		return closeCheckBox(line, 2, false, isLastLine(parent, block))
	case '\n', '\r':
		return closeCheckBoxOnNextLine(parent, block, line)
	default:
		return false, 0, false
	}
}

// closeCheckBox reads the "]" at line[offset] and the space after it, and
// returns the bytes to advance from the start of line (see spaceBeforeText).
func closeCheckBox(line []byte, offset int, checked, last bool) (bool, int, bool) {
	if offset >= len(line) || line[offset] != ']' || !followedByContent(line[offset+1:], last) {
		return false, 0, false
	}
	return checked, offset + 1 + spaceBeforeText(line[offset+1:]), true
}

// closeCheckBoxOnNextLine reads a checkbox whose space is the line ending
// that ends line: the paragraph's next line must start with "]". Nothing
// but the prefixes of the paragraph's containers may come before it on
// that line, since micromark reads any other space as the paragraph's own.
func closeCheckBoxOnNextLine(parent ast.Node, block text.Reader, line []byte) (bool, int, bool) {
	index, _ := block.Position()
	lines := parent.Lines()
	if index+1 >= lines.Len() {
		return false, 0, false
	}
	next := lines.At(index + 1)
	source := block.Source()
	if next.Start >= next.Stop || source[next.Start] != ']' ||
		!onlyContainerPrefix(source, bytes.LastIndexByte(source[:next.Start], '\n')+1, next.Start, parent) {
		return false, 0, false
	}
	rest := source[next.Start+1 : next.Stop]
	if !followedByContent(rest, index+1 == lines.Len()-1) {
		return false, 0, false
	}
	// Advancing over the line, line ending included, moves to the start of
	// the next line, where the "]" is.
	return false, len(line) + 1 + spaceBeforeText(rest), true
}

// followedByContent reports whether what follows a checkbox's "]" on its
// line is a space, a tab or a line ending, with more of the paragraph
// after it. goldmark trims the whitespace at the end of a paragraph's last
// line, so whitespace is followed by text or by the paragraph's next line.
func followedByContent(rest []byte, last bool) bool {
	if len(rest) == 0 {
		return false
	}
	switch rest[0] {
	case ' ', '\t':
		return !last || len(bytes.TrimLeft(rest, " \t")) > 0
	case '\n', '\r':
		return true
	default:
		return false
	}
}

// spaceBeforeText returns the length of the spaces and tabs after a
// checkbox's "]" that the checkbox takes: all of them when text follows on
// the line, so that the text starts after them, and none when only a line
// ending does, so that goldmark reads a hard line break from two or more of
// them as micromark does.
func spaceBeforeText(rest []byte) int {
	after := bytes.TrimLeft(rest, " \t")
	if len(after) == 0 || after[0] == '\n' || after[0] == '\r' {
		return 0
	}
	return len(rest) - len(after)
}

// isLastLine reports whether the reader is on the last line of a block.
func isLastLine(parent ast.Node, block text.Reader) bool {
	index, _ := block.Position()
	return index >= parent.Lines().Len()-1
}

// onlyContainerPrefix reports whether source[lineStart:end] is what the
// containers of block take from the start of a continuation line, as
// CommonMark matches them from the outermost on: a blockquote takes up to
// three spaces, ">" and one column of space, and a list item takes the
// columns up to its content. Matching stops at the first container whose
// prefix the line lacks, which makes the line lazy. The rest of the line
// belongs to the paragraph, so nothing may be left of the prefix, not even
// a part of a tab.
func onlyContainerPrefix(source []byte, lineStart, end int, block ast.Node) bool {
	var containers []ast.Node
	for node := block.Parent(); node != nil; node = node.Parent() {
		switch node.(type) {
		case *ast.Blockquote, *ast.ListItem:
			containers = append(containers, node)
		}
	}
	cursor := prefixCursor{source: source, pos: lineStart, end: end}
	for idx := len(containers) - 1; idx >= 0; idx-- {
		next := cursor
		var matched bool
		switch container := containers[idx].(type) {
		case *ast.Blockquote:
			matched = next.blockquoteMarker()
		case *ast.ListItem:
			matched = next.columns(container.Offset)
		}
		if !matched {
			break
		}
		cursor = next
	}
	return cursor.pos == end && cursor.virtual == 0
}

// prefixCursor walks a line's container prefix by columns, with tab stops
// every four columns from the start of the line. virtual holds the columns
// of a tab that a container took only in part.
type prefixCursor struct {
	source          []byte
	pos, end        int
	column, virtual int
}

// blockquoteMarker takes a blockquote's prefix: up to three columns of
// space, ">", and one column of a space or tab after it.
func (c *prefixCursor) blockquoteMarker() bool {
	if c.virtual > 0 {
		return false
	}
	for spaces := 0; c.pos < c.end && c.source[c.pos] == ' ' && spaces < 3; spaces++ {
		c.pos++
		c.column++
	}
	if c.pos >= c.end || c.source[c.pos] != '>' {
		return false
	}
	c.pos++
	c.column++
	if c.pos < c.end {
		switch c.source[c.pos] {
		case ' ':
			c.pos++
			c.column++
		case '\t':
			width := tabWidth - c.column%tabWidth
			c.pos++
			c.column += width
			c.virtual = width - 1
		}
	}
	return true
}

// columns takes count columns of space, using first what is left of a tab.
func (c *prefixCursor) columns(count int) bool {
	for count > 0 {
		switch {
		case c.virtual > 0:
			taken := min(c.virtual, count)
			c.virtual -= taken
			count -= taken
		case c.pos < c.end && c.source[c.pos] == ' ':
			c.pos++
			c.column++
			count--
		case c.pos < c.end && c.source[c.pos] == '\t':
			width := tabWidth - c.column%tabWidth
			c.pos++
			c.column += width
			taken := min(width, count)
			c.virtual = width - taken
			count -= taken
		default:
			return false
		}
	}
	return true
}

// tabColumns returns how many columns the tab at source[offset] spans: up
// to the next multiple of four, counting columns from the start of its
// line, as micromark does.
func tabColumns(source []byte, offset int) int {
	column := 0
	for pos := bytes.LastIndexByte(source[:offset], '\n') + 1; pos < offset; {
		char, size := utf8.DecodeRune(source[pos:])
		if char == '\t' {
			column += tabWidth - column%tabWidth
		} else {
			column++
		}
		pos += size
	}
	return tabWidth - column%tabWidth
}
