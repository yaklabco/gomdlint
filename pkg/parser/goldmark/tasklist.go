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

// gfmExtensions are goldmark's GFM extensions, with goldmark's task list
// replaced by gfmTaskList.
//
//nolint:gochecknoglobals // goldmark extensions are shared, stateless values.
var gfmExtensions = []goldmark.Extender{
	extension.Linkify,
	extension.Table,
	extension.Strikethrough,
	gfmTaskList{},
}

// gfmTaskList is goldmark's task list extension, reading a checkbox only
// where GFM does: at the start of the paragraph that is a list item's first
// block, followed by a space, a tab or a line ending and then more of the
// paragraph. goldmark's own parser also reads "[ ]", "[x]" or "[X]" at the
// start of a heading, and right before other text, as in "[x]foo" or
// "[ ](url)", where GFM reads text or a link.
type gfmTaskList struct{}

// Extend registers the checkbox parser and goldmark's checkbox renderer
// with the priorities goldmark's task list extension gives them.
func (gfmTaskList) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(paragraphCheckBoxParser{InlineParser: extension.NewTaskCheckBoxParser()}, 0),
	))
	md.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(extension.NewTaskCheckBoxHTMLRenderer(), 500),
	))
}

// paragraphCheckBoxParser hands goldmark's checkbox parser only the places
// where GFM reads a checkbox. goldmark's parser checks that the block is the
// first child of a list item and that nothing precedes the checkbox in it.
type paragraphCheckBoxParser struct {
	parser.InlineParser
}

// Parse reads a checkbox at the start of a paragraph, or returns nil so the
// other inline parsers read the brackets.
func (p paragraphCheckBoxParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	switch parent.(type) {
	case *ast.Paragraph, *ast.TextBlock:
	default:
		return nil
	}
	if line, _ := block.PeekLine(); !checkBoxFollowedByContent(line) {
		return nil
	}
	return p.InlineParser.Parse(parent, block, pc)
}

// checkBoxFollowedByContent reports whether line, which starts at "[",
// continues past the three characters of a checkbox the way GFM requires:
// with a space, a tab or a line ending, and then more of the paragraph.
// goldmark trims the whitespace at the end of a paragraph's last line, so
// whitespace after the checkbox is followed by text on the same line or by
// the paragraph's next line.
func checkBoxFollowedByContent(line []byte) bool {
	const checkBoxLength = len("[ ]")
	if len(line) <= checkBoxLength {
		return false
	}
	switch line[checkBoxLength] {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}
