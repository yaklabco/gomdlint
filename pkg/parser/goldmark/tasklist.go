package goldmark

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// GFMExtensions returns the goldmark extensions of the gfm flavor:
// goldmark's linkify, table and strikethrough extensions, and its task list
// extension changed to read no checkbox in a heading (see gfmTaskList).
// Every goldmark instance that reads GFM, such as the fix checks', builds
// from these, so that all of them read the same checkboxes as the parser.
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

// gfmTaskList is goldmark's task list extension, except that it reads no
// checkbox in a heading. goldmark reads one at the start of any block that
// is a list item's first child, which hides the link in "- ## [ ](/url)."
// from the rules, where micromark and GitHub read a link. Everywhere else
// it reads the checkboxes goldmark reads.
type gfmTaskList struct{}

// Extend registers the checkbox parser and goldmark's checkbox renderer
// with the priorities goldmark's task list extension gives them.
func (gfmTaskList) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(paragraphCheckBoxParser{InlineParser: extension.NewTaskCheckBoxParser()}, taskCheckBoxParserPriority),
	))
	md.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(extension.NewTaskCheckBoxHTMLRenderer(), taskCheckBoxRendererPriority),
	))
}

// paragraphCheckBoxParser is goldmark's checkbox parser that reads a
// checkbox only in a paragraph: goldmark's parser decides where and what,
// at the start of a list item's first child. goldmark's parser has nothing
// to do when a block closes, so the wrapper keeps no CloseBlock.
type paragraphCheckBoxParser struct {
	parser.InlineParser
}

// Parse reads a checkbox as goldmark's parser does when parent is a
// paragraph, and otherwise returns nil so that the other inline parsers
// read the brackets.
func (p paragraphCheckBoxParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	switch parent.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		return p.InlineParser.Parse(parent, block, pc)
	default:
		return nil
	}
}
