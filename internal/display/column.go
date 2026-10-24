// Package display converts the byte-based positions and absolute paths that
// lint diagnostics carry into the forms reporters show to people and tools.
package display

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// markSpacing is the distance in bytes between the marks a Columns keeps on
// a long line. A lookup decodes at most about two spacings of text it has
// decoded before, and a line no longer than one spacing gets no marks.
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

	var units int
	if len(text) <= markSpacing {
		units = c.decode(text, mark{}, offset).units
	} else {
		units = c.lineIndex(pos.Line, text).unitsBefore(c, offset)
	}

	if offset > len(text) {
		units += offset - len(text)
	}

	return units + 1
}

// lineIndex returns the index of a long line, whose text is text, creating
// it on first use.
func (c *Columns) lineIndex(line int, text []byte) *lineIndex {
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
// offset, or of the whole text when offset is past its end, decoding with c.
func (l *lineIndex) unitsBefore(c *Columns, offset int) int {
	if offset < l.reach.offset {
		// A mark after offset is the end of a character that starts before
		// the mark's multiple of markSpacing and contains offset. That
		// character counts in full, so decode returns the mark as it is.
		return c.decode(l.text, l.marks[offset/markSpacing], offset).units
	}

	for l.reach.offset < len(l.text) && l.reach.offset < offset {
		l.reach = c.decode(l.text, l.reach, min(offset, (l.reach.offset/markSpacing+1)*markSpacing))
		if l.reach.offset >= len(l.marks)*markSpacing {
			l.marks = append(l.marks, l.reach)
		}
	}

	return l.reach.units
}

// decode decodes text from the character boundary from until it reaches or
// passes byte offset or the end of text, and returns where it stopped. A
// character that offset points inside counts in full.
func (c *Columns) decode(text []byte, from mark, offset int) mark {
	for from.offset < len(text) && from.offset < offset {
		size, units := nextCharacter(text[from.offset:])
		from.offset += size
		from.units += units
		c.decoded += size
	}

	return from
}

// ByteOffset returns the byte offset in text after its first units UTF-16
// code units, counted as Columns counts them. A count that ends inside a
// character outside the Basic Multilingual Plane covers that character, and
// a count past the end of text returns its length.
func ByteOffset(text []byte, units int) int {
	offset, counted := 0, 0
	for offset < len(text) && counted < units {
		size, charUnits := nextCharacter(text[offset:])
		offset += size
		counted += charUnits
	}

	return offset
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

// nextCharacter returns the length in bytes of the character at the start of
// text, which must not be empty, and the UTF-16 code units it occupies once
// decoded as Node decodes UTF-8 (the WHATWG UTF-8 decoder): a well-formed
// character keeps its own code units, and a maximal ill-formed sequence
// becomes one U+FFFD, one code unit.
func nextCharacter(text []byte) (int, int) {
	char, size := utf8.DecodeRune(text)
	if char != utf8.RuneError || size > 1 {
		return size, utf16.RuneLen(char)
	}

	return maximalSubpart(text), 1
}

// maximalSubpart returns the length of the ill-formed sequence at the start
// of text that a UTF-8 decoder replaces with one U+FFFD: the lead byte and
// the continuation bytes after it that still form the start of a well-formed
// character, at least one byte. Unicode calls this the maximal subpart.
func maximalSubpart(text []byte) int {
	lead := text[0]
	low, high := byte(0x80), byte(0xBF)
	var continuations int
	switch {
	case lead >= 0xC2 && lead <= 0xDF:
		continuations = 1
	case lead == 0xE0:
		continuations, low = 2, 0xA0
	case lead == 0xED:
		continuations, high = 2, 0x9F
	case lead >= 0xE1 && lead <= 0xEF:
		continuations = 2
	case lead == 0xF0:
		continuations, low = 3, 0x90
	case lead == 0xF4:
		continuations, high = 3, 0x8F
	case lead >= 0xF1 && lead <= 0xF3:
		continuations = 3
	default:
		return 1
	}

	size := 1
	for size <= continuations && size < len(text) {
		next := text[size]
		if next < low || next > high {
			break
		}
		size++
		low, high = 0x80, 0xBF
	}

	return size
}
