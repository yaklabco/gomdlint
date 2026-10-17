package rules

import (
	"strconv"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// maxOrderedMarkerDigits is the CommonMark limit on the digits of an ordered
// list marker.
const maxOrderedMarkerDigits = 9

// listItemMarker describes the source of a list item marker.
type listItemMarker struct {
	// Line is the 1-based line holding the marker.
	Line int

	// LineStart is the byte offset where that line begins.
	LineStart int

	// TextStart is the byte offset where the text of that line begins: after
	// a byte order mark on the first line, and at LineStart otherwise.
	TextStart int

	// LineEnd is the byte offset where the line's content ends (its newline
	// offset, or the end of the file).
	LineEnd int

	// Start and End delimit the marker itself: the bullet character, or the
	// digits and delimiter of an ordered marker.
	Start, End int

	// ContentStart is the offset of the first non-whitespace byte after the
	// marker on its line, or LineEnd when the rest of the line is blank.
	ContentStart int

	// Ordered is true for ordered list markers.
	Ordered bool

	// Bullet is the bullet character of an unordered marker.
	Bullet byte

	// NumberText holds the digits of an ordered marker as written, and Number
	// their value.
	NumberText string
	Number     int

	// Delimiter is '.' or ')' for ordered markers.
	Delimiter byte
}

// Column returns the 0-based byte column of the marker within the text of
// its line, which leaves out a byte order mark.
func (m listItemMarker) Column() int {
	return m.Start - m.TextStart
}

// ByteColumn returns the 0-based byte column of the marker from the start of
// its line, the column diagnostics report.
func (m listItemMarker) ByteColumn() int {
	return m.Start - m.LineStart
}

// Width returns the width of the marker in bytes.
func (m listItemMarker) Width() int {
	return m.End - m.Start
}

// Spacing returns the number of whitespace bytes between the marker and the
// item content on the marker line.
func (m listItemMarker) Spacing() int {
	return m.ContentStart - m.End
}

// HasContent reports whether the marker line holds content after the marker.
func (m listItemMarker) HasContent() bool {
	return m.ContentStart < m.LineEnd
}

// findListItemMarker locates the marker of a list item in the source. The
// full span of a list item starts at its marker (see package mdast), so the
// marker is read from there. It reports false for nodes that are not list
// items or have no source.
func findListItemMarker(file *mdast.FileSnapshot, item *mdast.Node) (listItemMarker, bool) {
	if file == nil || item == nil || item.Kind != mdast.NodeListItem {
		return listItemMarker{}, false
	}
	pos := item.SourcePosition()
	if !pos.IsValid() || pos.StartLine > len(file.Lines) {
		return listItemMarker{}, false
	}
	line := file.Lines[pos.StartLine-1]
	textStart := line.StartOffset + lint.LineTextOffset(file, pos.StartLine)
	start := item.SourceRange().StartOffset
	if start < textStart || start > line.NewlineStart || !markerBoundaryBefore(file.Content, textStart, start) {
		return listItemMarker{}, false
	}
	marker, ok := readListMarker(file.Content, line, start)
	if !ok {
		return listItemMarker{}, false
	}
	marker.Line = pos.StartLine
	marker.TextStart = textStart
	return marker, true
}

// readListMarker reads a list marker starting exactly at start. The marker
// must be followed by whitespace or the end of the line.
func readListMarker(content []byte, line mdast.LineInfo, start int) (listItemMarker, bool) {
	if start >= line.NewlineStart {
		return listItemMarker{}, false
	}
	marker := listItemMarker{LineStart: line.StartOffset, TextStart: line.StartOffset, LineEnd: line.NewlineStart, Start: start}
	switch content[start] {
	case '-', '+', '*':
		marker.Bullet = content[start]
		marker.End = start + 1
	default:
		end := start
		for end < line.NewlineStart && isASCIIDigit(content[end]) {
			end++
		}
		digits := end - start
		if digits == 0 || digits > maxOrderedMarkerDigits || end >= line.NewlineStart ||
			(content[end] != '.' && content[end] != ')') {
			return listItemMarker{}, false
		}
		number, err := strconv.Atoi(string(content[start:end]))
		if err != nil {
			return listItemMarker{}, false
		}
		marker.Ordered = true
		marker.NumberText = string(content[start:end])
		marker.Number = number
		marker.Delimiter = content[end]
		marker.End = end + 1
	}
	if marker.End < line.NewlineStart && !isMarkerSpace(content[marker.End]) {
		return listItemMarker{}, false
	}
	marker.ContentStart = marker.End
	for marker.ContentStart < line.NewlineStart && isMarkerSpace(content[marker.ContentStart]) {
		marker.ContentStart++
	}
	return marker, true
}

// markerBoundaryBefore reports whether a marker may start at offset: at the
// line start or after whitespace or a blockquote marker.
func markerBoundaryBefore(content []byte, textStart, offset int) bool {
	if offset == textStart {
		return true
	}
	prev := content[offset-1]
	return isMarkerSpace(prev) || prev == '>'
}

func isMarkerSpace(char byte) bool {
	return char == ' ' || char == '\t'
}

func isASCIIDigit(char byte) bool {
	return char >= '0' && char <= '9'
}
