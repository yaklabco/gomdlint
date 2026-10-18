package lint

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Plain task list items.
//
// Task lists such as "- [ ] todo" are common, and the list rules fix them.
// The guard lets through the fixes of one form of candidate whose
// checkbox both renderers read before and after them, a plain candidate:
//
//   - the brackets hold a space, "x" or "X", and the "]" is followed on its
//     line by spaces or tabs and then text;
//   - the "[" is on its item's marker line, after a single list marker and
//     one tab or one to four spaces;
//   - goldmark reads the line as the first line of a list item whose first
//     content is the paragraph that starts at the "[";
//   - no footnote definition starts on or above the line.
//
// On a plain candidate's marker line a fix may change the list marker for
// another of its kind, a bullet for a bullet or a number and delimiter for
// a number and delimiter (MD004, MD029); the space between the marker and
// the "[" to one to four spaces (MD030, MD010); the blockquote markers'
// spaces and the indentation before the marker (MD005, MD007, MD010,
// MD027); and the whitespace after the line's text (MD009). It must leave
// the bytes from the "[" through the first character of text after the
// "]", and the rest of the text, as they are, and add or remove no line
// ending. On the line after, a fix may change only the trailing whitespace,
// or make the changes above when that line is a plain candidate itself.
//
// Why both renderers still read the checkbox. micromark reads one where a
// list item's first content is a paragraph starting with "[", a space or
// "x" or "X", "]", whitespace and more text. cmark-gfm reads one where the
// marker line holds, after the marker and spacing, "[ ]", "[x]" or "[X]"
// and a space or tab (its 0.29.0.gfm.0 build also wants nothing but spaces
// before the marker, which the allowed changes keep or keep failing, since
// the blockquote markers stay). After an allowed change the marker is still
// a list marker, followed by one tab, which is one to four columns wherever
// it falls, or by one to four spaces, which CommonMark and both renderers
// take whole as the marker's spacing, so the item's content still starts at
// the "["; the indentation can only move the item within the list
// structure, which the fix checks compare; the brackets, their content, the
// whitespace and text after them are the same bytes; and with no line
// ending added or removed, the paragraph still starts on the marker line.
// The next line stays a paragraph line that can turn the paragraph into a
// heading or a table no more than before: only its trailing whitespace
// changes, which no setext underline or table delimiter row depends on, or
// it stays a plain candidate line, which holds "[" and text and so is
// neither.

// lineKind tells how the guard treats a line of a candidate's region.
type lineKind uint8

const (
	// lineFree holds no candidate.
	lineFree lineKind = iota
	// linePlain is a plain candidate's marker line.
	linePlain
	// linePlainNext is the line after a plain candidate's marker line.
	linePlainNext
	// lineGuarded is in the region of a candidate that is not plain.
	lineGuarded
)

// plainLine is the parts of a plain candidate's marker line.
type plainLine struct {
	// prefix holds the blockquote markers and indentation before the
	// marker, with a byte order mark on the first line.
	prefix  []byte
	ordered bool
	// spacing holds the spaces or tab between the marker and the "[".
	spacing []byte
	// box runs from the "[" through the first character of text after the
	// "]", which starts boxAt in the line; text runs from there through the
	// line's last non-whitespace character.
	box, text []byte
	boxAt     int
}

// readPlainLine reads the parts of a line, without its line ending, that
// holds a single list marker followed by spacing and "[ ]", "[x]" or
// "[X]", spaces or tabs and text. first tells whether the line starts the
// content.
func readPlainLine(line []byte, first bool) (plainLine, bool) {
	pos := 0
	if first && bytes.HasPrefix(line, []byte(mdast.ByteOrderMark)) {
		pos = len(mdast.ByteOrderMark)
	}
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t' || line[pos] == '>') {
		pos++
	}
	var parts plainLine
	parts.prefix = line[:pos]
	markerEnd, ordered, ok := listMarkerEnd(line, pos)
	if !ok {
		return plainLine{}, false
	}
	parts.ordered = ordered
	pos = markerEnd
	start := pos
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}
	parts.spacing = line[start:pos]
	if len(parts.spacing) == 0 || pos+3 > len(line) || line[pos] != '[' || line[pos+2] != ']' ||
		(line[pos+1] != ' ' && line[pos+1] != 'x' && line[pos+1] != 'X') {
		return plainLine{}, false
	}
	parts.boxAt = pos
	pos += 3
	spaceStart := pos
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}
	if pos == spaceStart || pos == len(line) {
		return plainLine{}, false
	}
	_, size := utf8.DecodeRune(line[pos:])
	pos += size
	parts.box = line[parts.boxAt:pos]
	parts.text = bytes.TrimRight(line[pos:], " \t")
	return parts, true
}

// listMarkerEnd returns the end of a list item marker at line[pos], a
// bullet or one to nine digits with "." or ")", and whether it is ordered.
func listMarkerEnd(line []byte, pos int) (int, bool, bool) {
	if pos >= len(line) {
		return 0, false, false
	}
	switch line[pos] {
	case '-', '+', '*':
		return pos + 1, false, true
	}
	end := pos
	for end < len(line) && end-pos < maxOrderedMarkerDigits && line[end] >= '0' && line[end] <= '9' {
		end++
	}
	if end == pos || end >= len(line) || (line[end] != '.' && line[end] != ')') {
		return 0, false, false
	}
	return end + 1, true, true
}

// plainSpacing reports whether spacing, between a marker and its item's
// content, is one tab or one to four spaces, which both renderers take
// whole as the marker's spacing wherever the marker stands.
func plainSpacing(spacing []byte) bool {
	return string(spacing) == "\t" || plainSpaces(spacing)
}

// plainSpaces reports whether spacing is one to four spaces.
func plainSpaces(spacing []byte) bool {
	const maxSpaces = 4
	return len(spacing) >= 1 && len(spacing) <= maxSpaces && len(bytes.Trim(spacing, " ")) == 0
}

// allows reports whether after, the new text of a plain candidate's marker
// line, keeps its checkbox: the same blockquote markers before a marker of
// the same kind, the same spacing or one to four spaces, and the same box
// and text, with any trailing whitespace.
func (before plainLine) allows(after []byte, first bool) bool {
	parts, ok := readPlainLine(after, first)
	if !ok || parts.ordered != before.ordered {
		return false
	}
	if !bytes.Equal(withoutSpace(parts.prefix), withoutSpace(before.prefix)) {
		return false
	}
	if !bytes.Equal(parts.spacing, before.spacing) && !plainSpaces(parts.spacing) {
		return false
	}
	return bytes.Equal(parts.box, before.box) && bytes.Equal(parts.text, before.text)
}

// withoutSpace returns text without its spaces and tabs.
func withoutSpace(text []byte) []byte {
	return bytes.Map(func(char rune) rune {
		if char == ' ' || char == '\t' {
			return -1
		}
		return char
	}, text)
}

// plainItemStarts returns the offsets where goldmark reads a list item
// whose first content is a paragraph that starts on the item's first
// line, at the item's content.
func plainItemStarts(root *mdast.Node) map[int]bool {
	starts := make(map[int]bool)
	if root == nil {
		return starts
	}
	for _, item := range mdast.FindByKind(root, mdast.NodeListItem) {
		first := item.FirstChild
		if first == nil || first.Kind != mdast.NodeParagraph {
			continue
		}
		content := item.ContentRange()
		if first.SourceRange().StartOffset == content.StartOffset &&
			first.SourcePosition().StartLine == item.SourcePosition().StartLine {
			starts[content.StartOffset] = true
		}
	}
	return starts
}

// splitAtLineEndings splits edit into one edit for each line it changes,
// when its new text keeps the line endings of the text it replaces, the
// same ones in the same order, as when MD007 moves an item and the item
// nested in it in one edit. ok is false when the edit adds, removes or
// changes a line ending.
func splitAtLineEndings(content []byte, edit fix.TextEdit) ([]fix.TextEdit, bool) {
	oldText := content[edit.StartOffset:edit.EndOffset]
	newText := []byte(edit.NewText)
	oldLines, newLines := splitLines(oldText), splitLines(newText)
	if len(oldLines) != len(newLines) {
		return nil, false
	}
	pieces := make([]fix.TextEdit, 0, len(oldLines))
	pos := edit.StartOffset
	for idx := range oldLines {
		oldLine, newLine := oldLines[idx], newLines[idx]
		if !bytes.Equal(lineEnding(oldLine), lineEnding(newLine)) {
			return nil, false
		}
		oldBody, newBody := trimLineEnding(oldLine), trimLineEnding(newLine)
		pieces = append(pieces, fix.TextEdit{StartOffset: pos, EndOffset: pos + len(oldBody), NewText: string(newBody)})
		pos += len(oldLine)
	}
	return pieces, true
}

// splitLines splits text after each line ending, keeping it; text that
// ends with a line ending has an empty last part.
func splitLines(text []byte) [][]byte {
	var parts [][]byte
	start := 0
	for idx := range text {
		switch {
		case text[idx] == '\n':
			parts = append(parts, text[start:idx+1])
			start = idx + 1
		case text[idx] == '\r' && (idx+1 == len(text) || text[idx+1] != '\n'):
			parts = append(parts, text[start:idx+1])
			start = idx + 1
		}
	}
	return append(parts, text[start:])
}

// lineEnding returns the line ending that ends line, if any.
func lineEnding(line []byte) []byte {
	return line[len(trimLineEnding(line)):]
}

// carvedOut returns the line that holds edit when the edit stays inside
// the text of a plain candidate's marker line or the line after it, before
// the line ending, and adds no line ending.
func (g *checkboxGuard) carvedOut(edit fix.TextEdit) (int, bool) {
	if strings.ContainsAny(edit.NewText, "\r\n") {
		return 0, false
	}
	line := g.lineOf(edit.StartOffset)
	if edit.EndOffset > g.lines[line]+len(g.line(line)) {
		return 0, false
	}
	switch g.kinds[line] {
	case linePlain, linePlainNext:
		return line, true
	default:
		return 0, false
	}
}

// allowsOnLine reports whether edits, sorted and all on one line of a
// plain candidate's region, keep its checkbox.
func (g *checkboxGuard) allowsOnLine(line int, edits []fix.TextEdit) bool {
	start := g.lines[line]
	before := g.line(line)
	var after []byte
	pos := 0
	for _, edit := range edits {
		after = append(after, before[pos:edit.StartOffset-start]...)
		after = append(after, edit.NewText...)
		pos = edit.EndOffset - start
	}
	after = append(after, before[pos:]...)
	if g.kinds[line] == linePlain {
		return g.plain[line].allows(after, line == 0)
	}
	return bytes.Equal(bytes.TrimRight(after, " \t"), bytes.TrimRight(before, " \t"))
}

// classifyLines marks the lines of each candidate's region: a plain
// candidate's marker line and the line after it, and every line of the
// region of a candidate that is not plain, which wins.
func (g *checkboxGuard) classifyLines(candidates []checkboxCandidate, shapes []lineShape, root *mdast.Node) {
	g.kinds = make([]lineKind, len(g.lines))
	g.plain = make(map[int]plainLine)
	footnote := len(g.lines)
	for idx, shape := range shapes {
		if shape.footnote {
			footnote = idx
			break
		}
	}
	starts := plainItemStarts(root)
	var guarded []checkboxCandidate
	for _, candidate := range candidates {
		parts, ok := g.plainCandidate(candidate, footnote, starts)
		if !ok {
			guarded = append(guarded, candidate)
			continue
		}
		g.plain[candidate.line] = parts
		g.kinds[candidate.line] = linePlain
		if next := candidate.line + 1; next < len(g.lines) && g.kinds[next] == lineFree {
			g.kinds[next] = linePlainNext
		}
	}
	for _, candidate := range guarded {
		for line := candidate.marker; line <= candidate.last; line++ {
			g.kinds[line] = lineGuarded
		}
	}
	for line, kind := range g.kinds {
		if kind != linePlain {
			delete(g.plain, line)
		}
	}
}

// plainCandidate reports whether a candidate is plain, and returns the
// parts of its line. footnote is the first line that starts a footnote
// definition, and starts holds where goldmark reads plain list items.
func (g *checkboxGuard) plainCandidate(candidate checkboxCandidate, footnote int, starts map[int]bool) (plainLine, bool) {
	if candidate.marker != candidate.line || candidate.line >= footnote {
		return plainLine{}, false
	}
	line := g.line(candidate.line)
	parts, ok := readPlainLine(line, candidate.line == 0)
	if !ok || !plainSpacing(parts.spacing) {
		return plainLine{}, false
	}
	if !starts[g.lines[candidate.line]+parts.boxAt] {
		return plainLine{}, false
	}
	return parts, true
}
