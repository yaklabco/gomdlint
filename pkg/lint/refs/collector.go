package refs

import (
	"bytes"
	"regexp"
	"strings"

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

// extractHeadingText extracts plain text from a heading node.
func extractHeadingText(node *mdast.Node) string {
	if node == nil || node.Kind != mdast.NodeHeading {
		return ""
	}

	var buf bytes.Buffer
	_ = mdast.Walk(node, func(n *mdast.Node) error { //nolint:errcheck // visitor never returns error
		if n.Kind == mdast.NodeText && n.Inline != nil {
			buf.Write(n.Inline.Text)
		}
		return nil
	})
	return buf.String()
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

// extractLinkText extracts plain text from a link/image node.
func extractLinkText(node *mdast.Node) string {
	var buf bytes.Buffer
	_ = mdast.Walk(node, func(n *mdast.Node) error { //nolint:errcheck // visitor never returns error
		if n.Kind == mdast.NodeText && n.Inline != nil {
			buf.Write(n.Inline.Text)
		}
		return nil
	})
	return buf.String()
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
			label := extractLinkText(node)
			return StyleCollapsed, label
		case mdast.RefStyleShortcut:
			label := extractLinkText(node)
			return StyleShortcut, label
		}
	}

	pos := node.SourcePosition()
	if !pos.IsValid() || c.ctx.File == nil {
		return StyleInline, ""
	}

	// Get the source line containing the link
	if pos.StartLine < 1 || pos.StartLine > len(c.ctx.File.Lines) {
		return StyleInline, ""
	}

	lineInfo := c.ctx.File.Lines[pos.StartLine-1]
	line := c.ctx.File.Content[lineInfo.StartOffset:lineInfo.NewlineStart]

	// Look for reference patterns in the line
	// This is a simplified heuristic based on source inspection
	text := extractLinkText(node)

	// Check for full reference: [text][label]
	if idx := findFullReference(line, text); idx >= 0 {
		label := extractFullReferenceLabel(line, idx, len(text))
		if label != "" {
			return StyleFull, label
		}
	}

	// Check for collapsed reference: [label][]
	if isCollapsedReference(line, text) {
		return StyleCollapsed, text
	}

	// Check for shortcut reference: [label] (no following brackets or parens)
	if isShortcutReference(line, text, isImage) {
		return StyleShortcut, text
	}

	// Default to inline
	return StyleInline, ""
}

// findFullReference looks for [text][label] pattern.
func findFullReference(line []byte, _ string) int {
	// Look for ][
	pattern := "]" + "["
	idx := bytes.Index(line, []byte(pattern))
	return idx
}

// extractFullReferenceLabel extracts the label from [text][label].
func extractFullReferenceLabel(line []byte, closeBracketIdx, _ int) string {
	// Find the opening [ of the label part
	start := closeBracketIdx + 2 // Skip ][
	if start >= len(line) {
		return ""
	}

	// Find closing ]
	end := bytes.IndexByte(line[start:], ']')
	if end < 0 {
		return ""
	}

	return string(line[start : start+end])
}

// isCollapsedReference checks for [label][] pattern.
func isCollapsedReference(line []byte, text string) bool {
	pattern := "[" + text + "][]"
	return bytes.Contains(line, []byte(pattern))
}

// isShortcutReference checks for [label] without following () or [].
func isShortcutReference(line []byte, text string, isImage bool) bool {
	// Build the pattern to look for
	var prefix string
	if isImage {
		prefix = "!["
	} else {
		prefix = "["
	}
	pattern := prefix + text + "]"
	patternBytes := []byte(pattern)

	idx := bytes.Index(line, patternBytes)
	if idx < 0 {
		return false
	}

	// Check what follows the closing bracket
	afterIdx := idx + len(patternBytes)
	if afterIdx >= len(line) {
		return true // Nothing follows - shortcut
	}

	nextChar := line[afterIdx]
	// If followed by ( or [, it's inline or full reference
	if nextChar == '(' || nextChar == '[' {
		return false
	}

	return true
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
func (c *collector) buildCodeBlockLines() map[int]bool {
	lines := make(map[int]bool)
	if c.root == nil {
		return lines
	}

	//nolint:errcheck // Walk visitor never returns error in this usage
	mdast.Walk(c.root, func(node *mdast.Node) error {
		if node.Kind == mdast.NodeCodeBlock {
			pos := node.SourcePosition()
			if pos.IsValid() {
				for line := pos.StartLine; line <= pos.EndLine; line++ {
					lines[line] = true
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
		if codeBlockLines[lineNum+1] {
			continue
		}

		line := c.ctx.File.Content[lineInfo.StartOffset:lineInfo.NewlineStart]
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
