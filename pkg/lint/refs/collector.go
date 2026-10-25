package refs

import (
	"bytes"
	"regexp"

	"github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// LinePrefix returns the length of the container prefix at the start of a
// 1-based line, for content inside node: the blockquote markers and list
// item indentation that belong to the containers, not to the content.
type LinePrefix func(node *mdast.Node, line int) int

// Collect walks the AST and source to build a reference Context. A label
// read from the source keeps whole continuation lines; CollectWithPrefix
// leaves out their container prefixes.
func Collect(root *mdast.Node, file *mdast.FileSnapshot) *Context {
	return CollectWithPrefix(root, file, nil)
}

// CollectWithPrefix builds a reference Context like Collect, using prefix
// (which may be nil) to leave the container prefix of each continuation
// line out of labels read from the source.
func CollectWithPrefix(root *mdast.Node, file *mdast.FileSnapshot, prefix LinePrefix) *Context {
	return CollectWithOptions(root, file, Options{Prefix: prefix})
}

// Options configure CollectWithOptions.
type Options struct {
	// Prefix, which may be nil, gives the container prefix of each
	// continuation line, which labels read from the source leave out.
	Prefix LinePrefix

	// HTMLBlockTags, which may be nil, gives the tags of an HTML block as
	// ranges of the file's content, each of which may name an anchor (see
	// HTMLAnchor). markdownlint finds them by parsing the block's lines
	// again as Markdown, so that a tag in a code span is code. Without it,
	// every HTML construct in the block's source counts.
	HTMLBlockTags func(block *mdast.Node) []mdast.SourceRange
}

// CollectWithOptions builds a reference Context like Collect, leaving the
// container prefixes that options.Prefix gives out of labels read from the
// source, and reading the tags of HTML blocks that options.HTMLBlockTags
// gives for anchors.
func CollectWithOptions(root *mdast.Node, file *mdast.FileSnapshot, options Options) *Context {
	if root == nil || file == nil {
		return NewContext(file)
	}

	coll := &collector{
		ctx:           NewContext(file),
		root:          root,
		prefix:        options.Prefix,
		htmlBlockTags: options.HTMLBlockTags,
	}
	coll.collect(root)
	coll.collectUnresolvedReferences()
	coll.collectDefinitions()
	coll.resolveReferences()

	return coll.ctx
}

// collector builds a Context by walking the AST and source.
type collector struct {
	ctx           *Context
	root          *mdast.Node
	prefix        LinePrefix
	htmlBlockTags func(block *mdast.Node) []mdast.SourceRange
	// chain is the last unresolved reference of the container scanned, and
	// closers counts its closing brackets (see closingBrackets).
	chain   referenceChain
	closers []int
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

// collectHeadingAnchor generates an anchor from a heading, and adds the
// custom anchors written in it.
func (c *collector) collectHeadingAnchor(node *mdast.Node) {
	text := extractHeadingText(node)
	if text == "" {
		return
	}

	pos := node.SourcePosition()
	c.ctx.Anchors.AddFromHeading(text, pos)
	c.collectCustomAnchors(node, pos)
}

// customAnchorPattern matches a custom heading anchor, "{#name}", as
// markdownlint reads it: words of lowercase ASCII letters and digits joined
// by single hyphens or underscores.
var customAnchorPattern = regexp.MustCompile(`\{#([a-z\d]+(?:[-_][a-z\d]+)*)\}`)

// collectCustomAnchors adds an anchor for each "{#name}" in a heading's text
// as written, as markdownlint does. The slug of the heading's text, which
// holds the name too, stays an anchor, and slugs of later headings are
// numbered as before.
func (c *collector) collectCustomAnchors(node *mdast.Node, pos mdast.SourcePosition) {
	content := node.ContentRange()
	file := c.ctx.File
	if file == nil || content.StartOffset < 0 || content.EndOffset <= content.StartOffset ||
		content.EndOffset > len(file.Content) {
		return
	}
	source := file.Content[content.StartOffset:content.EndOffset]
	if !bytes.Contains(source, []byte("{#")) {
		return
	}
	for _, match := range customAnchorPattern.FindAllSubmatch(source, -1) {
		id := string(match[1])
		if c.ctx.Anchors.Has(id) {
			continue
		}
		c.ctx.Anchors.Add(&Anchor{ID: id, Source: AnchorFromCustomID, Position: pos, Text: string(match[0])})
	}
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

// collectHTMLAnchors adds the anchors that the tags of an HTML node name
// (see HTMLAnchor): inline HTML is one tag, read over all of its lines, and
// an HTML block holds the tags that blockTags finds on any of its lines.
func (c *collector) collectHTMLAnchors(node *mdast.Node) {
	if c.ctx.File == nil {
		return
	}
	if node.Kind == mdast.NodeHTMLInline {
		c.addHTMLAnchor(node.SourceRange())
		return
	}
	for _, tag := range c.blockTags(node) {
		c.addHTMLAnchor(tag)
	}
}

// blockTags returns the ranges of the tags in an HTML block: those that
// the HTMLBlockTags option gives, or without it every HTML construct in the
// block's source.
func (c *collector) blockTags(block *mdast.Node) []mdast.SourceRange {
	if c.htmlBlockTags != nil {
		return c.htmlBlockTags(block)
	}
	span := block.SourceRange()
	constructs := mdscan.ScanHTML(c.ctx.File.Content, span.StartOffset, span.EndOffset)
	tags := make([]mdast.SourceRange, len(constructs))
	for idx, construct := range constructs {
		tags[idx] = mdast.SourceRange{StartOffset: construct.Start, EndOffset: construct.End}
	}
	return tags
}

// addHTMLAnchor adds the anchor that the tag at rng names, if any.
func (c *collector) addHTMLAnchor(rng mdast.SourceRange) {
	file := c.ctx.File
	if rng.StartOffset < 0 || rng.EndOffset > len(file.Content) || rng.EndOffset <= rng.StartOffset {
		return
	}
	id, source, ok := HTMLAnchor(file.Content[rng.StartOffset:rng.EndOffset])
	if !ok {
		return
	}
	startLine, startColumn := file.LineAt(rng.StartOffset)
	endLine, endColumn := file.LineAt(rng.EndOffset)
	c.ctx.Anchors.Add(&Anchor{
		ID:     id,
		Source: source,
		Position: mdast.SourcePosition{
			StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn,
		},
	})
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
