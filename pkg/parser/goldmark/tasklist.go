package goldmark

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// GFM is goldmark's GFM extension (autolink literals, tables,
// strikethrough and task list items) with task list checkboxes read where
// GFM reads them. The GFM flavor parses with it, and so do the fix checks,
// so that both read the same checkboxes.
//
// goldmark's own task list extension reads "[ ]", "[x]" or "[X]" at the
// start of a list item's first block whatever the block is and whatever
// follows the brackets. GFM reads a checkbox only at the start of a
// paragraph, and only when a space or tab and more of the paragraph, or a
// line ending inside it, follow the brackets (as
// micromark-extension-gfm-task-list-item reads one). Anywhere else the
// brackets are text or the start of a link: "- ## [ ](/url)" holds a link
// whose text is a space, and "- [x](https://example.com)" a link to the
// address.
//
//nolint:gochecknoglobals // A stateless extension, like goldmark's extension.GFM.
var GFM goldmark.Extender = gfmExtension{}

type gfmExtension struct{}

// Extend adds goldmark's GFM extensions, with GFM's task list checkboxes in
// place of goldmark's.
func (gfmExtension) Extend(markdown goldmark.Markdown) {
	extension.Linkify.Extend(markdown)
	extension.Table.Extend(markdown)
	extension.Strikethrough.Extend(markdown)
	markdown.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(taskCheckBoxParser{}, 0),
	))
	markdown.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(extension.NewTaskCheckBoxHTMLRenderer(), 500),
	))
}

// taskCheckBoxParser reads a task list checkbox where GFM reads one. It
// leaves the reading to goldmark's own checkbox parser, which also wants
// the brackets to start the first child of a list item, and only hands it
// the brackets GFM reads.
type taskCheckBoxParser struct{}

// Trigger returns the byte that can start a checkbox.
func (taskCheckBoxParser) Trigger() []byte {
	return []byte{'['}
}

// Parse returns a checkbox when the brackets at the reader start a
// paragraph and are followed as GFM requires (see taskMarker), and nil
// otherwise. goldmark makes each paragraph of a tight list a text block.
//
//nolint:ireturn // goldmark's parser.InlineParser interface fixes this signature.
func (taskCheckBoxParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	if kind := parent.Kind(); kind != ast.KindParagraph && kind != ast.KindTextBlock {
		return nil
	}
	line, _ := block.PeekLine()
	if !taskMarker(line) {
		return nil
	}
	return goldmarkTaskCheckBoxParser.Parse(parent, block, pc)
}

// CloseBlock does nothing; a checkbox keeps no state.
func (taskCheckBoxParser) CloseBlock(ast.Node, parser.Context) {}

//nolint:gochecknoglobals // goldmark's checkbox parser is stateless.
var goldmarkTaskCheckBoxParser = extension.NewTaskCheckBoxParser()

// taskMarker reports whether line, the rest of a paragraph's first line,
// starts with a checkbox as micromark-extension-gfm-task-list-item reads
// one: "[", a space, a tab, "x" or "X", and "]", followed by a line ending,
// or by spaces and tabs and then anything but the end of the paragraph.
// goldmark trims the white space that ends a paragraph from its last line,
// so a line that ends without a line ending ends the paragraph. A line
// ending cannot come inside the brackets: goldmark reads a paragraph a line
// at a time.
func taskMarker(line []byte) bool {
	const markerLength = len("[ ]")
	if len(line) < markerLength || line[0] != '[' || line[2] != ']' {
		return false
	}
	switch line[1] {
	case ' ', '\t', 'x', 'X':
	default:
		return false
	}
	rest := line[markerLength:]
	switch {
	case len(rest) == 0:
		return false
	case rest[0] == '\n' || rest[0] == '\r':
		return true
	case rest[0] == ' ' || rest[0] == '\t':
		return len(bytes.TrimLeft(rest, " \t")) > 0
	default:
		return false
	}
}
