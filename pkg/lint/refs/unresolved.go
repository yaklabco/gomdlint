package refs

import (
	"strings"

	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// maxLabelLength is the CommonMark limit on characters inside a link label.
const maxLabelLength = 999

// inlineChar is one character of an inline container's source, or an opaque
// placeholder for an inline node that cannot be part of a label: an
// autolink, or a resolved link or image. A literal character comes from a
// code span or raw HTML: it is part of a label's text as written, but it is
// never a bracket or an escape.
type inlineChar struct {
	ch      byte
	offset  int // source offset of ch; -1 when unknown
	opaque  bool
	literal bool
	node    *mdast.Node // node holding ch
}

// collectUnresolvedReferences finds reference syntax the parser left as text
// because the label matched no definition: full references [text][label],
// collapsed references [label][], and shortcut references [label], for links
// and images. The parser creates no node for them, so they are found by
// matching brackets in the text of each inline container, the way a
// CommonMark parser does before it looks labels up.
func (c *collector) collectUnresolvedReferences() {
	_ = mdast.Walk(c.root, func(node *mdast.Node) error { //nolint:errcheck // visitor never returns error
		if isInlineContainer(node) {
			c.scanUnresolved(flattenInline(node))
		}
		return nil
	})
}

// isInlineContainer reports whether a node's children form one run of inline
// content: a block such as a paragraph, heading, or table cell, or the text
// of a link or image. An autolink's text is its address, which holds no
// references.
func isInlineContainer(node *mdast.Node) bool {
	if node.Kind == mdast.NodeLink || node.Kind == mdast.NodeImage {
		return node.Inline == nil || node.Inline.Link == nil ||
			node.Inline.Link.ReferenceStyle != mdast.RefStyleAutolink
	}
	if node.IsInline() {
		return false
	}
	for child := node.FirstChild; child != nil; child = child.Next {
		if child.IsInline() {
			return true
		}
	}
	return false
}

// flattenInline turns a container's inline children into characters, so
// labels keep the source they were written with. Emphasis, strong, and
// strikethrough are transparent to bracket matching, which comes before
// emphasis, but their delimiters stay in the text; code spans and raw HTML
// are kept as written, as literal characters.
func flattenInline(container *mdast.Node) []inlineChar {
	var chars []inlineChar
	source := func(node *mdast.Node, from, to int, literal bool) {
		if node.File == nil || from < 0 || to > len(node.File.Content) {
			return
		}
		for offset := from; offset < to; offset++ {
			chars = append(chars, inlineChar{ch: node.File.Content[offset], offset: offset, literal: literal, node: node})
		}
	}
	var add func(parent *mdast.Node)
	add = func(parent *mdast.Node) {
		for child := parent.FirstChild; child != nil; child = child.Next {
			switch child.Kind {
			case mdast.NodeText, mdast.NodeSoftBreak, mdast.NodeHardBreak:
				if child.Inline == nil {
					continue // A task list checkbox has no text.
				}
				base := -1
				if span := child.SourceRange(); span.EndOffset > span.StartOffset {
					base = span.StartOffset
				}
				for idx := range len(child.Inline.Text) {
					offset := -1
					if base >= 0 {
						offset = base + idx
					}
					chars = append(chars, inlineChar{ch: child.Inline.Text[idx], offset: offset, node: child})
				}
				if child.Kind != mdast.NodeText {
					chars = append(chars, inlineChar{ch: '\n', offset: -1, node: child})
				}
			case mdast.NodeEmphasis, mdast.NodeStrong, mdast.NodeStrikethrough:
				full, inner := child.SourceRange(), child.ContentRange()
				source(child, full.StartOffset, inner.StartOffset, false)
				add(child)
				source(child, inner.EndOffset, full.EndOffset, false)
			case mdast.NodeCodeSpan, mdast.NodeHTMLInline:
				chars = append(chars, literalChars(child)...)
			default:
				chars = append(chars, inlineChar{offset: -1, opaque: true, node: child})
			}
		}
	}
	add(container)
	return chars
}

// literalChars returns a code span or raw HTML as literal characters: its
// source when it sits on one line, and otherwise, for a code span, its
// content between backtick runs, since the source of later lines holds
// their container prefix.
func literalChars(node *mdast.Node) []inlineChar {
	span := node.SourceRange()
	text := []byte(nil)
	switch pos := node.SourcePosition(); {
	case node.File != nil && span.EndOffset <= len(node.File.Content) && (pos.StartLine == pos.EndLine || node.Kind != mdast.NodeCodeSpan):
		text = node.File.Content[span.StartOffset:span.EndOffset]
	case node.Inline != nil:
		fence := strings.Repeat("`", max(1, node.ContentRange().StartOffset-span.StartOffset))
		text = []byte(fence + string(node.Inline.Text) + fence)
	}
	chars := make([]inlineChar, 0, len(text))
	for _, ch := range text {
		chars = append(chars, inlineChar{ch: ch, offset: -1, literal: true, node: node})
	}
	return chars
}

// bracketOpener is an unmatched '[' in the character stream.
type bracketOpener struct {
	index int
	image bool
}

// scanUnresolved matches brackets in one container's characters and records
// each reference whose syntax is complete.
func (c *collector) scanUnresolved(chars []inlineChar) {
	var openers []bracketOpener
	for idx := 0; idx < len(chars); idx++ {
		char := chars[idx]
		if char.opaque || char.literal {
			continue
		}
		switch char.ch {
		case '\\':
			if idx+1 < len(chars) && !chars[idx+1].opaque && mdscan.IsASCIIPunct(chars[idx+1].ch) {
				idx++
			}
		case '[':
			image := idx > 0 && isChar(chars, idx-1, '!') && !isEscapedChar(chars, idx-1)
			openers = append(openers, bracketOpener{index: idx, image: image})
		case ']':
			if len(openers) == 0 {
				continue
			}
			opener := openers[len(openers)-1]
			openers = openers[:len(openers)-1]
			idx = c.recordReference(chars, opener, idx)
		}
	}
}

// recordReference classifies the bracket pair closing at chars[closing] by
// what follows it, records the reference, and returns the index the scan
// continues from. A "(" after the pair began an inline link the parser
// rejected, since a valid one would be a link node, so the pair falls back
// to a reference as in CommonMark.
func (c *collector) recordReference(chars []inlineChar, opener bracketOpener, closing int) int {
	next := closing + 1
	switch {
	case isChar(chars, next, '[') && isChar(chars, next+1, ']'):
		label, ok := linkTextLabel(chars, opener.index, closing)
		if ok {
			c.addUnresolved(chars, opener, closing, next+1, StyleCollapsed, label)
		}
		return next + 1
	case isChar(chars, next, '['):
		if labelEnd, ok := labelEndIndex(chars, next); ok {
			label := charsText(chars, next+1, labelEnd)
			c.addUnresolved(chars, opener, closing, labelEnd, StyleFull, label)
			return labelEnd
		}
	}
	if label, ok := linkTextLabel(chars, opener.index, closing); ok {
		c.addUnresolved(chars, opener, closing, closing, StyleShortcut, label)
	}
	return closing
}

// addUnresolved records an unresolved reference whose link text closes at
// chars[closing] and whose syntax ends at chars[end].
func (c *collector) addUnresolved(
	chars []inlineChar, opener bracketOpener, closing, end int, style ReferenceStyle, label string,
) {
	start := opener.index
	if opener.image {
		start--
	}
	usage := &ReferenceUsage{
		Style:           style,
		IsImage:         opener.image,
		Text:            charsText(chars, opener.index+1, closing),
		Label:           label,
		NormalizedLabel: NormalizeLabel(label),
		Node:            chars[opener.index].node,
		Unresolved:      true,
	}
	if c.ctx.File != nil && chars[start].offset >= 0 && chars[end].offset >= 0 {
		startLine, startColumn := c.ctx.File.LineAt(chars[start].offset)
		endLine, endColumn := c.ctx.File.LineAt(chars[end].offset + 1)
		usage.Position = mdast.SourcePosition{
			StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn,
		}
	} else if usage.Node != nil {
		usage.Position = usage.Node.SourcePosition()
	}
	c.ctx.Usages = append(c.ctx.Usages, usage)
}

// linkTextLabel returns the link text between two brackets as a label, as
// written. It is not a label when it holds an unescaped bracket (a bracket
// inside a code span counts, since a label is raw text) or a link, is
// blank, or is longer than CommonMark allows.
func linkTextLabel(chars []inlineChar, open, closing int) (string, bool) {
	for idx := open + 1; idx < closing; idx++ {
		if chars[idx].opaque {
			return "", false
		}
		if (chars[idx].ch == '[' || chars[idx].ch == ']') && (chars[idx].literal || !isEscapedChar(chars, idx)) {
			return "", false
		}
	}
	label := charsText(chars, open+1, closing)
	if len(label) > maxLabelLength || strings.TrimSpace(label) == "" {
		return "", false
	}
	return label, true
}

// labelEndIndex finds the ']' that closes the link label opened at
// chars[open]. Labels contain no unescaped brackets or inline nodes.
func labelEndIndex(chars []inlineChar, open int) (int, bool) {
	for idx := open + 1; idx < len(chars) && idx-open-1 <= maxLabelLength; idx++ {
		if chars[idx].opaque || (chars[idx].literal && (chars[idx].ch == '[' || chars[idx].ch == ']')) {
			return 0, false
		}
		if chars[idx].literal {
			continue
		}
		switch chars[idx].ch {
		case '\\':
			if idx+1 < len(chars) && !chars[idx+1].opaque && mdscan.IsASCIIPunct(chars[idx+1].ch) {
				idx++
			}
		case '[':
			return 0, false
		case ']':
			if strings.TrimSpace(charsText(chars, open+1, idx)) == "" {
				return 0, false
			}
			return idx, true
		}
	}
	return 0, false
}

// charsText concatenates the characters in chars[from:to], skipping opaque nodes.
func charsText(chars []inlineChar, from, to int) string {
	var buf strings.Builder
	for idx := from; idx < to; idx++ {
		if !chars[idx].opaque {
			_ = buf.WriteByte(chars[idx].ch) // strings.Builder.WriteByte never fails
		}
	}
	return buf.String()
}

func isChar(chars []inlineChar, idx int, ch byte) bool {
	return idx >= 0 && idx < len(chars) && !chars[idx].opaque && !chars[idx].literal && chars[idx].ch == ch
}

// isEscapedChar reports whether chars[idx] follows an odd number of backslashes.
func isEscapedChar(chars []inlineChar, idx int) bool {
	backslashes := 0
	for prev := idx - 1; prev >= 0 && isChar(chars, prev, '\\'); prev-- {
		backslashes++
	}
	return backslashes%2 == 1
}
