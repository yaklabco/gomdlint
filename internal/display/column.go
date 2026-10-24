// Package display converts the byte-based positions and absolute paths that
// lint diagnostics carry into the forms reporters show to people and tools.
package display

import (
	"bytes"

	"github.com/yaklabco/gomdlint/internal/jstext"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// markSpacing is the distance in bytes between the marks a Columns keeps on
// a long line. A lookup decodes at most one spacing and one character of text
// it has decoded before, and a line no longer than one spacing gets no marks.
const markSpacing = 256

// BOMLength returns the length in bytes of the byte order mark at the start
// of content, or 0 when content has none. Editors hide the mark, and SARIF
// forbids it from affecting columns, so it never counts as a column.
func BOMLength(content []byte) int {
	if bytes.HasPrefix(content, []byte(mdast.ByteOrderMark)) {
		return len(mdast.ByteOrderMark)
	}
	return 0
}

// Columns converts the byte columns of one file snapshot into the columns
// that markdownlint, markdownlint-cli2 and SARIF report: 1-based and counted
// in UTF-16 code units, the indices of a JavaScript string. A character
// outside the Basic Multilingual Plane, such as an emoji, occupies two
// columns, and every other character one. Text that is not valid UTF-8 counts
// as Node decodes it: each maximal ill-formed sequence becomes one U+FFFD,
// one column.
//
// Lookups may come in any order. A line no longer than markSpacing bytes is
// decoded from its start for each lookup. A longer line is decoded once, as
// far as lookups reach, and the converter keeps a mark every markSpacing
// bytes, so a lookup behind the furthest one resumes from the mark before it.
// The work is therefore linear in the text the lookups reach plus a constant
// per lookup, and the marks take memory in proportion to the long lines
// looked up. A Columns is not safe for concurrent use.
type Columns struct {
	snapshot *mdast.FileSnapshot

	// long indexes the long lines looked up so far by their 1-based number.
	long map[int]*lineIndex

	// decoded counts the bytes decoded over all lookups. It exists for tests,
	// which check that the work grows with the text rather than with the
	// lookups, and nothing else reads it.
	decoded int
}

// lineIndex holds how far the decoding of one long line has reached, with
// marks along the way.
type lineIndex struct {
	// text is the text of the line as LineText returns it.
	text []byte

	// marks[i] is the first character boundary at or after byte offset
	// i*markSpacing, with the UTF-16 code units before it. There is a mark for
	// every multiple of markSpacing up to reach.
	marks []mark

	// reach is the furthest character boundary decoded so far.
	reach mark
}

// mark is a byte offset at a character boundary of a line and the UTF-16
// code units of the text before it.
type mark struct {
	offset int
	units  int
}

// NewColumns returns a converter for the byte columns of snapshot.
func NewColumns(snapshot *mdast.FileSnapshot) *Columns {
	return &Columns{snapshot: snapshot}
}

// Column converts pos, a 1-based line and byte column of the snapshot, into a
// 1-based column counted in UTF-16 code units.
//
// A character whose encoding starts before the byte column counts in full, so
// a column that points inside a multibyte character lands just after it. Bytes
// past the end of the line text (its line terminator) count one column each. A
// byte order mark at the start of the file is not counted.
//
// A column below 1 is unknown and converts to 0. Any other column is returned
// unchanged when it cannot be converted: for a nil snapshot or a line outside
// the snapshot.
func (c *Columns) Column(pos mdast.Position) int {
	if pos.Column < 1 {
		return 0
	}
	if c.snapshot == nil || pos.Line < 1 || pos.Line > len(c.snapshot.Lines) {
		return pos.Column
	}

	text := LineText(c.snapshot, pos.Line)
	skipped := len(c.snapshot.LineContent(pos.Line)) - len(text)
	offset := max(pos.Column-1-skipped, 0)

	var units, decoded int
	if len(text) <= markSpacing {
		reached := decode(text, mark{}, offset)
		units, decoded = reached.units, reached.offset
	} else {
		units, decoded = c.longLine(pos.Line, text).unitsBefore(offset)
	}
	c.decoded += decoded

	if offset > len(text) {
		units += offset - len(text)
	}

	return units + 1
}

// ColumnWithin converts pos as Column does, where pos points at the start of
// a character and the position lies within code units into it, and returns
// the column of that code unit. A finding that markdownlint starts on the
// second half of a surrogate pair has within 1 (see
// lint.Diagnostic.StartColumnUTF16Offset). An unknown column stays 0, and a
// negative within counts as 0.
func (c *Columns) ColumnWithin(pos mdast.Position, within int) int {
	column := c.Column(pos)
	if column == 0 {
		return 0
	}

	return column + max(within, 0)
}

// longLine returns the index of a long line, whose text is text, creating it
// on first use.
func (c *Columns) longLine(line int, text []byte) *lineIndex {
	if index, ok := c.long[line]; ok {
		return index
	}

	if c.long == nil {
		c.long = make(map[int]*lineIndex)
	}
	index := &lineIndex{text: text, marks: []mark{{}}}
	c.long[line] = index

	return index
}

// unitsBefore returns the UTF-16 code units of the line text before byte
// offset, or of the whole text when offset is past its end, and the number of
// bytes it decoded to find them.
func (l *lineIndex) unitsBefore(offset int) (int, int) {
	if offset < l.reach.offset {
		// A mark after offset is the end of a character that starts before
		// the mark's multiple of markSpacing and contains offset. That
		// character counts in full, so decode returns the mark as it is.
		from := l.marks[offset/markSpacing]
		reached := decode(l.text, from, offset)
		return reached.units, reached.offset - from.offset
	}

	start := l.reach.offset
	for l.reach.offset < len(l.text) && l.reach.offset < offset {
		l.reach = decode(l.text, l.reach, min(offset, (l.reach.offset/markSpacing+1)*markSpacing))
		if l.reach.offset >= len(l.marks)*markSpacing {
			l.marks = append(l.marks, l.reach)
		}
	}

	return l.reach.units, l.reach.offset - start
}

// decode decodes text from the character boundary from until it reaches or
// passes byte offset or the end of text, and returns where it stopped. A
// character that offset points inside counts in full.
func decode(text []byte, from mark, offset int) mark {
	for from.offset < len(text) && from.offset < offset {
		size, units := jstext.Next(text[from.offset:])
		from.offset += size
		from.units += units
	}

	return from
}

// LineText returns the text of a 1-based line of snapshot without its line
// terminator and, for the first line, without a byte order mark. Columns
// produced by Columns index into this text. It returns nil for a nil snapshot
// or a line outside it.
func LineText(snapshot *mdast.FileSnapshot, line int) []byte {
	if snapshot == nil {
		return nil
	}

	text := snapshot.LineContent(line)
	if line == 1 {
		text = text[BOMLength(text):]
	}

	return text
}
