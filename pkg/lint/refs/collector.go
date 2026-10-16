package refs

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
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
	coll.collectDefinitionsFromSource()
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
	style, label := c.detectLinkStyle(node, isImage)
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

// referenceLabel returns the label a collapsed or shortcut reference is
// matched by: its link text as written in the source, such as "**foo**" for
// "[**foo**][]", falling back to the rendered text.
func referenceLabel(node *mdast.Node) string {
	if label := node.Inline.Link.ReferenceLabel; label != "" {
		return label
	}
	return extractLinkText(node)
}

// detectLinkStyle examines the source to determine the link's syntax style.
func (c *collector) detectLinkStyle(node *mdast.Node, isImage bool) (ReferenceStyle, string) {
	// First check if the mapper already detected the style
	if node.Inline != nil && node.Inline.Link != nil {
		mdastStyle := node.Inline.Link.ReferenceStyle
		switch mdastStyle {
		case mdast.RefStyleAutolink:
			return StyleAutolink, ""
		case mdast.RefStyleFull:
			return StyleFull, node.Inline.Link.ReferenceLabel
		case mdast.RefStyleCollapsed:
			return StyleCollapsed, referenceLabel(node)
		case mdast.RefStyleShortcut:
			return StyleShortcut, referenceLabel(node)
		}
	}

	if c.ctx.File == nil {
		return StyleInline, ""
	}
	return linkStyleFromSource(c.ctx.File.Content, node, isImage)
}

// linkStyleFromSource classifies a link or image by its own source text: the
// bracketed link text followed by "(" (inline), "[]" (collapsed), "[label]"
// (full), or nothing (shortcut). For collapsed and shortcut references the
// label is the link text.
func linkStyleFromSource(content []byte, node *mdast.Node, isImage bool) (ReferenceStyle, string) {
	span := node.SourceRange()
	if span.StartOffset < 0 || span.EndOffset > len(content) || span.IsEmpty() {
		return StyleInline, ""
	}
	src := content[:span.EndOffset]
	open := span.StartOffset
	if isImage && src[open] == '!' {
		open++
	}
	closing, ok := mdscan.LinkTextEnd(src, open)
	if !ok {
		return StyleInline, ""
	}
	text := string(src[open+1 : closing])
	after := closing + 1
	switch {
	case after >= len(src):
		return StyleShortcut, text
	case src[after] == '(':
		return StyleInline, ""
	case src[after] != '[':
		return StyleShortcut, text
	case after+1 < len(src) && src[after+1] == ']':
		return StyleCollapsed, text
	}
	if labelEnd, found := mdscan.LinkLabelEnd(src, after); found {
		return StyleFull, string(src[after+1 : labelEnd])
	}
	return StyleShortcut, text
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

// Reference definition pattern: [label]: destination "optional title"
// Matches at start of line (with up to 3 spaces indent).
var refDefPattern = regexp.MustCompile(
	`^\s{0,3}\[([^\]]+)\]:\s*(\S+)(?:\s+"([^"]*)"|\s+'([^']*)'|\s+\(([^)]*)\))?\s*$`,
)

// buildCodeBlockLines returns a set of line numbers that are inside code blocks.
// These lines should be skipped when scanning for reference definitions.
func (c *collector) buildCodeBlockLines() map[int]struct{} {
	lines := make(map[int]struct{})
	if c.root == nil {
		return lines
	}

	//nolint:errcheck // Walk visitor never returns error in this usage
	mdast.Walk(c.root, func(node *mdast.Node) error {
		if node.Kind == mdast.NodeCodeBlock {
			pos := node.SourcePosition()
			if pos.IsValid() {
				for line := pos.StartLine; line <= pos.EndLine; line++ {
					lines[line] = struct{}{}
				}
			}
		}
		return nil
	})

	return lines
}

// collectDefinitionsFromSource parses reference definitions from the source.
func (c *collector) collectDefinitionsFromSource() {
	if c.ctx.File == nil || len(c.ctx.File.Content) == 0 {
		return
	}

	// Build set of lines inside code blocks - these cannot contain reference definitions
	codeBlockLines := c.buildCodeBlockLines()

	for lineNum, lineInfo := range c.ctx.File.Lines {
		// Skip lines inside code blocks (lineNum is 0-indexed, positions are 1-indexed)
		if _, inCodeBlock := codeBlockLines[lineNum+1]; inCodeBlock {
			continue
		}

		line := c.ctx.File.Content[lineInfo.StartOffset:lineInfo.NewlineStart]
		if lineNum == 0 {
			// The parser skips a byte order mark, so a definition can follow
			// one on the first line.
			line = bytes.TrimPrefix(line, []byte(mdast.ByteOrderMark))
		}
		matches := refDefPattern.FindSubmatch(line)
		if matches == nil {
			continue
		}

		label := string(matches[1])
		normalized := NormalizeLabel(label)

		// Extract title from whichever group matched
		title := coalesce(string(matches[3]), string(matches[4]), string(matches[5]))

		def := &ReferenceDefinition{
			Label:           label,
			NormalizedLabel: normalized,
			Destination:     string(matches[2]),
			Title:           title,
			LineNumber:      lineNum + 1,
			Position: mdast.SourcePosition{
				StartLine:   lineNum + 1,
				EndLine:     lineNum + 1,
				StartColumn: 1,
			},
		}

		// Check for duplicates
		if _, exists := c.ctx.Definitions[normalized]; exists {
			def.IsDuplicate = true
		} else {
			c.ctx.Definitions[normalized] = def
		}

		c.ctx.AllDefinitions = append(c.ctx.AllDefinitions, def)
	}
}

// coalesce returns the first non-empty string.
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
