package rules

import (
	"strconv"

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

// Column returns the 0-based byte column of the marker within its line.
func (m listItemMarker) Column() int {
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

// findListItemMarker locates the marker of a list item in the source.
//
// When the item's span starts at its marker (optionally preceded by
// indentation), the marker is read from there. Otherwise the span starts at the
// item's first content, which is how the parser currently records list items:
// the marker is then found by walking back over the markers of nested lists
// that start on the same content. It reports false when the marker cannot be
// located, for example for an item whose first line holds no content.
func findListItemMarker(file *mdast.FileSnapshot, item *mdast.Node) (listItemMarker, bool) {
	if file == nil || item == nil || item.Kind != mdast.NodeListItem {
		return listItemMarker{}, false
	}
	pos := item.SourcePosition()
	if !pos.IsValid() || pos.StartLine > len(file.Lines) {
		return listItemMarker{}, false
	}
	line := file.Lines[pos.StartLine-1]
	content := file.Content
	spanStart := item.SourceRange().StartOffset
	if spanStart < line.StartOffset || spanStart > line.NewlineStart {
		return listItemMarker{}, false
	}

	// The span starts at the marker.
	start := spanStart
	for start < line.NewlineStart && isMarkerSpace(content[start]) {
		start++
	}
	if marker, ok := readListMarker(content, line, start); ok && markerBoundaryBefore(content, line, start) {
		marker.Line = pos.StartLine
		return marker, true
	}

	// The span starts at the first content: walk back over the markers of
	// nested lists that begin on the same content, then read this item's.
	depth := sameContentNesting(item, spanStart)
	end := spanStart
	var marker listItemMarker
	for range depth + 1 {
		var ok bool
		marker, ok = listMarkerBefore(content, line, end)
		if !ok {
			return listItemMarker{}, false
		}
		end = marker.Start
	}
	marker.Line = pos.StartLine
	return marker, true
}

// sameContentNesting counts the nested lists that begin, through first
// children, at the same offset as the item's content.
func sameContentNesting(item *mdast.Node, offset int) int {
	depth := 0
	for node := item; ; {
		list := node.FirstChild
		if list == nil || list.Kind != mdast.NodeList {
			return depth
		}
		first := list.FirstChild
		if first == nil || first.Kind != mdast.NodeListItem || first.SourceRange().StartOffset != offset {
			return depth
		}
		depth++
		node = first
	}
}

// listMarkerBefore finds the list marker that ends before offset on the line,
// separated from it by at least one whitespace byte.
func listMarkerBefore(content []byte, line mdast.LineInfo, offset int) (listItemMarker, bool) {
	end := offset
	for end > line.StartOffset && isMarkerSpace(content[end-1]) {
		end--
	}
	if end == offset || end == line.StartOffset {
		return listItemMarker{}, false
	}
	start := end - 1
	switch content[start] {
	case '-', '+', '*':
	case '.', ')':
		for start > line.StartOffset && isASCIIDigit(content[start-1]) && end-1-(start-1) <= maxOrderedMarkerDigits {
			start--
		}
		if start == end-1 {
			return listItemMarker{}, false
		}
	default:
		return listItemMarker{}, false
	}
	if !markerBoundaryBefore(content, line, start) {
		return listItemMarker{}, false
	}
	return readListMarker(content, line, start)
}

// readListMarker reads a list marker starting exactly at start. The marker
// must be followed by whitespace or the end of the line.
func readListMarker(content []byte, line mdast.LineInfo, start int) (listItemMarker, bool) {
	if start >= line.NewlineStart {
		return listItemMarker{}, false
	}
	marker := listItemMarker{LineStart: line.StartOffset, LineEnd: line.NewlineStart, Start: start}
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
func markerBoundaryBefore(content []byte, line mdast.LineInfo, offset int) bool {
	if offset == line.StartOffset {
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
