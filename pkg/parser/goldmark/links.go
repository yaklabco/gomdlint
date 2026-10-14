package goldmark

import (
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

const linkSourceAttribute = "gomdlint-link-source"
const linkLabelAttribute = "gomdlint-link-label"

// positionedLinks replaces the stock link parser while retaining its grammar.
type positionedLinks struct{}

func (positionedLinks) SetParserOption(cfg *parser.Config) {
	for idx := range cfg.InlineParsers {
		if cfg.InlineParsers[idx].Value == parser.NewLinkParser() {
			cfg.InlineParsers[idx].Value = &positionedLinkParser{InlineParser: parser.NewLinkParser()}
		}
	}
}

type positionedLinkParser struct {
	parser.InlineParser
}

func (p *positionedLinkParser) Parse(parent ast.Node, reader text.Reader, ctx parser.Context) ast.Node {
	line, segment := reader.PeekLine()
	start := segment.Start
	closing := len(line) > 0 && line[0] == ']'
	if closing {
		// The last outstanding label belongs to this closing bracket.
		for child := parent.LastChild(); child != nil; child = child.PreviousSibling() {
			if value, ok := child.AttributeString(linkLabelAttribute); ok {
				if rng, valid := value.(mdast.SourceRange); valid {
					start = rng.StartOffset
					break
				}
			}
		}
	}
	node := p.InlineParser.Parse(parent, reader, ctx)
	if node != nil {
		_, end := reader.Position()
		attribute := linkSourceAttribute
		if !closing {
			attribute = linkLabelAttribute
		}
		node.SetAttributeString(attribute, mdast.SourceRange{StartOffset: start, EndOffset: end.Start})
	}
	return node
}

func (p *positionedLinkParser) CloseBlock(parent ast.Node, reader text.Reader, ctx parser.Context) {
	if closer, ok := p.InlineParser.(parser.CloseBlocker); ok {
		closer.CloseBlock(parent, reader, ctx)
	}
}
