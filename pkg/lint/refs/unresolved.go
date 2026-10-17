package refs

import (
	"strings"
	"unicode/utf8"

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
			c.scanUnresolved(node, flattenInline(node))
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
func (c *collector) scanUnresolved(container *mdast.Node, chars []inlineChar) {
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
			idx = c.recordReference(container, chars, opener, idx)
		}
	}
}

// recordReference classifies the bracket pair closing at chars[closing] by
// what follows it, records the reference, and returns the index the scan
// continues from. A "(" after the pair began an inline link the parser
// rejected, since a valid one would be a link node, so the pair falls back
// to a reference as in CommonMark.
func (c *collector) recordReference(container *mdast.Node, chars []inlineChar, opener bracketOpener, closing int) int {
	next := closing + 1
	switch {
	case isChar(chars, next, '[') && isChar(chars, next+1, ']'):
		label, ok := c.linkTextLabel(container, chars, opener.index, closing)
		if ok {
			c.addUnresolved(chars, opener, closing, next+1, StyleCollapsed, label)
		}
		return next + 1
	case isChar(chars, next, '['):
		if labelEnd, ok := labelEndIndex(chars, next); ok {
			if label := c.labelSource(container, chars, next, labelEnd); validLabel(label) {
				c.addUnresolved(chars, opener, closing, labelEnd, StyleFull, label)
				return labelEnd
			}
		}
	}
	if label, ok := c.linkTextLabel(container, chars, opener.index, closing); ok {
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
// written. It is not a label when it holds a link or its source is not a
// valid label (see validLabel). An unescaped bracket inside, which counts
// inside a code span too since a label is raw text, rules it out before its
// source is read, so nested brackets stay cheap.
func (c *collector) linkTextLabel(container *mdast.Node, chars []inlineChar, open, closing int) (string, bool) {
	for idx := open + 1; idx < closing; idx++ {
		if chars[idx].opaque {
			return "", false
		}
		if (chars[idx].ch == '[' || chars[idx].ch == ']') && !isEscapedRaw(chars, idx) {
			return "", false
		}
	}
	label := c.labelSource(container, chars, open, closing)
	if !validLabel(label) {
		return "", false
	}
	return label, true
}

// labelSource returns the label between the brackets at chars[open] and
// chars[closing] as it is written in the source (see sourceText). It falls
// back to the characters when a bracket has no offset.
func (c *collector) labelSource(container *mdast.Node, chars []inlineChar, open, closing int) string {
	start, end := chars[open].offset+1, chars[closing].offset
	if c.ctx.File == nil || chars[open].offset < 0 || end < start || end > len(c.ctx.File.Content) {
		return charsText(chars, open+1, closing)
	}
	return c.sourceText(container, start, end)
}

// sourceText returns the source from start to end the way CommonMark reads a
// label: code span padding, backslashes, entities, markup delimiters, and
// indentation stay, the container prefix of each continuation line is left
// out, and lines are joined by "\n". inside is a node within the containers
// whose prefixes are left out.
func (c *collector) sourceText(inside *mdast.Node, start, end int) string {
	file := c.ctx.File
	startLine, _ := file.LineAt(start)
	endLine, _ := file.LineAt(end)
	var text strings.Builder
	for line := startLine; line <= endLine && line <= len(file.Lines); line++ {
		info := file.Lines[line-1]
		from, to := max(start, info.StartOffset), min(end, info.NewlineStart)
		if line > startLine {
			_ = text.WriteByte('\n') // strings.Builder.WriteByte never fails
			if c.prefix != nil {
				from = max(from, info.StartOffset+c.prefix(inside, line))
			}
		}
		if from < to {
			text.Write(file.Content[from:to])
		}
	}
	return text.String()
}

// validLabel reports whether raw, the source between a label's brackets as
// sourceText returns it, is a valid link label: it has a character besides
// spaces, tabs, and line endings, every bracket in it is escaped, and it has
// at most maxLabelLength characters (not bytes). As in markdownlint, line
// endings do not count toward the limit and indentation does.
func validLabel(raw string) bool {
	blank := true
	length := 0
	for idx := 0; idx < len(raw); idx++ {
		switch ch := raw[idx]; ch {
		case '\n':
			continue
		case '\\':
			blank = false
			if idx+1 < len(raw) && mdscan.IsASCIIPunct(raw[idx+1]) {
				idx++
				length++
			}
		case '[', ']':
			return false
		case ' ', '\t':
		default:
			blank = false
		}
		if length += runeStarts(raw[idx]); length > maxLabelLength {
			return false
		}
	}
	return !blank
}

// labelEndIndex finds the ']' that closes the link label opened at
// chars[open]. A label is raw text: a backslash escapes the next punctuation
// character inside a code span too, and an unescaped '[' or a link or image
// inside makes it invalid. validLabel checks the rest on the label's source.
func labelEndIndex(chars []inlineChar, open int) (int, bool) {
	for idx := open + 1; idx < len(chars); idx++ {
		char := chars[idx]
		if char.opaque {
			return 0, false
		}
		switch char.ch {
		case '\\':
			if idx+1 < len(chars) && !chars[idx+1].opaque && mdscan.IsASCIIPunct(chars[idx+1].ch) {
				idx++
			}
		case '[':
			return 0, false
		case ']':
			// An unescaped bracket inside a code span would end the label
			// inside it, which no parser builds a reference from.
			if char.literal {
				return 0, false
			}
			return idx, true
		}
	}
	return 0, false
}

// runeStarts returns 1 when ch starts a UTF-8 character and 0 when it
// continues one, so summing it over bytes counts characters.
func runeStarts(ch byte) int {
	if utf8.RuneStart(ch) {
		return 1
	}
	return 0
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

// isEscapedRaw reports whether chars[idx] follows an odd number of
// backslashes in the raw text, counting those inside code spans.
func isEscapedRaw(chars []inlineChar, idx int) bool {
	backslashes := 0
	for prev := idx - 1; prev >= 0 && !chars[prev].opaque && chars[prev].ch == '\\'; prev-- {
		backslashes++
	}
	return backslashes%2 == 1
}

// isEscapedChar reports whether chars[idx] follows an odd number of backslashes.
func isEscapedChar(chars []inlineChar, idx int) bool {
	backslashes := 0
	for prev := idx - 1; prev >= 0 && isChar(chars, prev, '\\'); prev-- {
		backslashes++
	}
	return backslashes%2 == 1
}
