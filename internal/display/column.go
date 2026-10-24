// Package display converts the byte-based positions and absolute paths that
// lint diagnostics carry into the forms reporters show to people and tools.
package display

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// utf8BOM is the UTF-8 encoding of a byte order mark. Editors hide it, and
// SARIF forbids it from affecting columns, so it never counts as a column.
const utf8BOM = "\xef\xbb\xbf"

// BOMLength returns the length in bytes of the byte order mark at the start
// of content, or 0 when content has none.
func BOMLength(content []byte) int {
	if bytes.HasPrefix(content, []byte(utf8BOM)) {
		return len(utf8BOM)
	}
	return 0
}

// Columns converts the byte columns of one file snapshot into the columns
// that markdownlint, markdownlint-cli2 and SARIF report: 1-based and counted
// in UTF-16 code units, the indices of a JavaScript string. A character
// outside the Basic Multilingual Plane, such as an emoji, occupies two
// columns, and every other character one.
//
// A Columns remembers how far it has decoded the line of its last lookup, so
// lookups in column order on one line decode that line once. A lookup before
// the previous one on its line, or on another line, decodes its line from the
// start again. Use one Columns for the start positions of a file's sorted
// findings and another for their end positions. A Columns is not safe for
// concurrent use.
type Columns struct {
	snapshot *mdast.FileSnapshot

	// line is the 1-based line that text holds, or 0 before the first lookup.
	line int

	// text is the text of line without its line terminator and, for the
	// first line, without a byte order mark.
	text []byte

	// skipped is the length of the byte order mark removed from text.
	skipped int

	// offset is the byte offset in text that the decoding has reached, and
	// units the UTF-16 code units of the text before it.
	offset int
	units  int

	// decoded counts the bytes decoded over all lookups, so tests can check
	// that the work grows with the text rather than with the lookups.
	decoded int
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

	offset := pos.Column - 1
	if pos.Line != c.line || offset-c.skipped < c.offset {
		c.startLine(pos.Line)
	}
	offset = max(offset-c.skipped, 0)

	return c.unitsBefore(offset) + 1
}

// startLine points the converter at the start of line.
func (c *Columns) startLine(line int) {
	c.line = line
	c.text = c.snapshot.LineContent(line)
	c.skipped = 0
	if line == 1 {
		c.skipped = BOMLength(c.text)
		c.text = c.text[c.skipped:]
	}
	c.offset = 0
	c.units = 0
}

// unitsBefore counts the UTF-16 code units of the line text that precede byte
// offset, which is at or after the offset decoded so far. Offsets past the
// end of the text add one unit per byte. Invalid bytes decode to
// utf8.RuneError and count one unit each.
func (c *Columns) unitsBefore(offset int) int {
	for c.offset < len(c.text) && c.offset < offset {
		char, size := utf8.DecodeRune(c.text[c.offset:])
		c.units += utf16.RuneLen(char)
		c.offset += size
		c.decoded += size
	}

	if offset > len(c.text) {
		return c.units + offset - len(c.text)
	}

	return c.units
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
