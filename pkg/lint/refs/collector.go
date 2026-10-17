package refs

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Collect walks the AST and source to build a reference Context.
func Collect(root *mdast.Node, file *mdast.FileSnapshot) *Context {
	if root == nil || file == nil {
		return NewContext(file)
	}

	coll := &collector{
		ctx:  NewContext(file),
		root: root,
	}
	coll.collect(root)
	coll.collectUnresolvedReferences()
	coll.collectDefinitions()
	coll.resolveReferences()

	return coll.ctx
}

// collector builds a Context by walking the AST and source.
type collector struct {
	ctx  *Context
	root *mdast.Node
}

// collect walks the AST to collect anchors and usages.
func (c *collector) collect(root *mdast.Node) {
	_ = mdast.Walk(root, c.visit) //nolint:errcheck // visitor never returns error
}

// visit processes a single node during AST traversal.
func (c *collector) visit(node *mdast.Node) error {
	switch node.Kind {
	case mdast.NodeHeading:
		c.collectHeadingAnchor(node)
	case mdast.NodeLink:
		c.collectLinkUsage(node, false)
	case mdast.NodeImage:
		c.collectLinkUsage(node, true)
	case mdast.NodeHTMLBlock, mdast.NodeHTMLInline:
		c.collectHTMLAnchors(node)
	}
	return nil
}

// collectHeadingAnchor generates an anchor from a heading.
func (c *collector) collectHeadingAnchor(node *mdast.Node) {
	text := extractHeadingText(node)
	if text == "" {
		return
	}

	pos := node.SourcePosition()
	c.ctx.Anchors.AddFromHeading(text, pos)
}

// HeadingAnchors returns the anchor of every heading under root, in document
// order, as GitHub generates them and as MD051 checks fragments against:
// from the same heading text, with github-slugger's duplicate suffixes.
func HeadingAnchors(root *mdast.Node) []string {
	anchors := NewAnchorMap()
	var ids []string
	_ = mdast.Walk(root, func(node *mdast.Node) error { //nolint:errcheck // visitor never returns error
		if node.Kind == mdast.NodeHeading {
			if text := extractHeadingText(node); text != "" {
				ids = append(ids, anchors.GenerateAnchor(text))
			}
		}
		return nil
	})
	return ids
}

// extractHeadingText returns the text GitHub derives a heading's anchor from:
// the rendered text content. Text has its backslash escapes and entity
// references resolved, code spans contribute their content, and images and
// raw HTML contribute nothing. Line breaks contribute nothing either; the
// slug would drop the line ending anyway.
func extractHeadingText(node *mdast.Node) string {
	if node == nil || node.Kind != mdast.NodeHeading {
		return ""
	}

	var buf bytes.Buffer
	var collect func(parent *mdast.Node)
	collect = func(parent *mdast.Node) {
		for child := parent.FirstChild; child != nil; child = child.Next {
			switch child.Kind {
			case mdast.NodeImage, mdast.NodeHTMLInline:
				continue
			case mdast.NodeText, mdast.NodeSoftBreak, mdast.NodeHardBreak:
				// Break nodes may carry the text that precedes the line ending.
				if child.Inline != nil {
					buf.Write(renderedText(child.Inline.Text))
				}
			case mdast.NodeCodeSpan:
				if child.Inline != nil {
					buf.Write(child.Inline.Text)
				}
			default:
				collect(child)
			}
		}
	}
	collect(node)
	return buf.String()
}

// renderedText resolves backslash escapes and entity references in raw text
// the way a renderer does, and drops line endings.
func renderedText(raw []byte) []byte {
	text := bytes.TrimRight(raw, "\r\n")
	text = util.UnescapePunctuations(text)
	text = util.ResolveNumericReferences(text)
	return util.ResolveEntityNames(text)
}

// collectLinkUsage records a link or image usage.
func (c *collector) collectLinkUsage(node *mdast.Node, isImage bool) {
	if node.Inline == nil || node.Inline.Link == nil {
		return
	}

	link := node.Inline.Link
	dest := link.Destination

	usage := &ReferenceUsage{
		IsImage:     isImage,
		Text:        extractLinkText(node),
		Destination: dest,
		Fragment:    ExtractFragment(dest),
		Position:    node.SourcePosition(),
		Node:        node,
	}

	// Detect reference style by examining source
	style, label := detectLinkStyle(node)
	usage.Style = style
	usage.Label = label
	usage.NormalizedLabel = NormalizeLabel(label)

	c.ctx.Usages = append(c.ctx.Usages, usage)
}

// extractLinkText extracts plain text from a link/image node. A line break
// inside the text separates words, as whitespace in a label does.
func extractLinkText(node *mdast.Node) string {
	var buf bytes.Buffer
	_ = mdast.Walk(node, func(n *mdast.Node) error { //nolint:errcheck // visitor never returns error
		switch {
		case n.Kind == mdast.NodeText && n.Inline != nil:
			buf.Write(n.Inline.Text)
		case n.Kind == mdast.NodeSoftBreak || n.Kind == mdast.NodeHardBreak:
			buf.WriteByte(' ')
		}
		return nil
	})
	return buf.String()
}

// detectLinkStyle returns the reference syntax the parser recorded for a link
// or image and the label it was resolved with: the second bracket pair of a
// full reference, or the link text of a collapsed or shortcut one, as
// written and without the container prefix of a continuation line, such as
// "> " inside a blockquote.
func detectLinkStyle(node *mdast.Node) (ReferenceStyle, string) {
	if node.Inline == nil || node.Inline.Link == nil {
		return StyleInline, ""
	}
	link := node.Inline.Link
	switch link.ReferenceStyle {
	case mdast.RefStyleAutolink:
		return StyleAutolink, ""
	case mdast.RefStyleFull:
		return StyleFull, link.ReferenceLabel
	case mdast.RefStyleCollapsed:
		return StyleCollapsed, link.ReferenceLabel
	case mdast.RefStyleShortcut:
		return StyleShortcut, link.ReferenceLabel
	case mdast.RefStyleInline:
	}
	return StyleInline, ""
}

// collectHTMLAnchors extracts id and name attributes from HTML.
func (c *collector) collectHTMLAnchors(node *mdast.Node) {
	// Get HTML content from source position
	pos := node.SourcePosition()
	if !pos.IsValid() || c.ctx.File == nil {
		return
	}

	// Get the content span
	content := c.getNodeContent(node)
	if len(content) == 0 {
		return
	}

	// Extract id attributes: id="value" or id='value'
	c.extractHTMLAttribute(content, "id", AnchorFromHTMLID, pos)

	// Extract name attributes from anchors: name="value"
	c.extractHTMLAttribute(content, "name", AnchorFromHTMLName, pos)
}

// getNodeContent returns the source content for a node.
func (c *collector) getNodeContent(node *mdast.Node) []byte {
	pos := node.SourcePosition()
	if !pos.IsValid() || pos.StartLine < 1 || pos.StartLine > len(c.ctx.File.Lines) {
		return nil
	}

	// For simplicity, get the line content
	lineInfo := c.ctx.File.Lines[pos.StartLine-1]
	return c.ctx.File.Content[lineInfo.StartOffset:lineInfo.NewlineStart]
}

// htmlAttrPattern matches HTML attributes like id="value" or id='value'.
var htmlAttrPattern = regexp.MustCompile(`(?i)\b(id|name)\s*=\s*["']([^"']+)["']`)

// extractHTMLAttribute finds and adds anchors from HTML attributes.
func (c *collector) extractHTMLAttribute(content []byte, attr string, source AnchorSource, pos mdast.SourcePosition) {
	matches := htmlAttrPattern.FindAllSubmatch(content, -1)
	for _, match := range matches {
		if len(match) >= 3 && strings.EqualFold(string(match[1]), attr) {
			id := string(match[2])
			anchor := &Anchor{
				ID:       id,
				Source:   source,
				Position: pos,
			}
			c.ctx.Anchors.Add(anchor)
		}
	}
}

// resolveReferences links usages to their definitions and updates usage counts.
func (c *collector) resolveReferences() {
	for _, usage := range c.ctx.Usages {
		if usage.NormalizedLabel == "" {
			continue
		}

		def := c.ctx.Definitions[usage.NormalizedLabel]
		if def != nil {
			usage.ResolvedDefinition = def
			def.UsageCount++
		}
	}
}
