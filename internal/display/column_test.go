package display

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

func TestColumns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		line    int
		column  int
		want    int
	}{
		{name: "ascii is unchanged", content: "plain trail   \n", line: 1, column: 12, want: 12},
		{name: "two-byte letters are one unit each", content: "# Title\n\nÄÖÜ trail   \n", line: 3, column: 13, want: 10},
		{name: "exclusive end column", content: "# Title\n\nÄÖÜ trail   \n", line: 3, column: 16, want: 13},
		{name: "three-byte letters are one unit each", content: "中文 x\n", line: 1, column: 8, want: 4},
		{name: "astral character is two units", content: "\U0001F600 x\n", line: 1, column: 6, want: 4},
		{name: "two astral characters are four units", content: "\U0001F600\U0001F600 tab\there\n", line: 1, column: 13, want: 9},
		{name: "combining mark is a unit of its own", content: "é combining   \n", line: 1, column: 14, want: 13},
		{name: "astral character on a CRLF line", content: "# Title\r\n\r\n\U0001F600 smile   \r\n", line: 3, column: 11, want: 9},
		{name: "astral character on a bare CR line", content: "# Title\r\r\U0001F600 smile   \r", line: 3, column: 11, want: 9},
		{name: "column past content counts terminator bytes", content: "é\r\nnext\n", line: 1, column: 4, want: 3},
		{name: "column at end of last line without newline", content: "ab", line: 1, column: 3, want: 3},
		{name: "leading byte order mark is not a column", content: "\xef\xbb\xbf# Title\n", line: 1, column: 4, want: 1},
		{name: "column after byte order mark", content: "\xef\xbb\xbf# Title\n", line: 1, column: 6, want: 3},
		{name: "column inside byte order mark", content: "\xef\xbb\xbf# Title\n", line: 1, column: 2, want: 1},
		{name: "zero width no-break space on later line counts", content: "a\n\xef\xbb\xbfb\n", line: 2, column: 4, want: 2},
		{name: "column inside a character covers it", content: "中x\n", line: 1, column: 2, want: 2},
		{name: "column inside an astral character covers both units", content: "\U0001F600x\n", line: 1, column: 3, want: 3},
		// Node decodes each maximal ill-formed sequence to one U+FFFD.
		{name: "invalid bytes count one each", content: "\xff\xfex\n", line: 1, column: 3, want: 3},
		{name: "truncated three-byte character is one unit", content: "\xe2\x82x\n", line: 1, column: 3, want: 2},
		{name: "truncated four-byte character is one unit", content: "\xf0\x9f\x98x\n", line: 1, column: 4, want: 2},
		{name: "encoded surrogate is three units", content: "\xed\xa0\x80x\n", line: 1, column: 4, want: 4},
		{name: "overlong encoding is two units", content: "\xc0\xafx\n", line: 1, column: 3, want: 3},
		{name: "out of range four-byte lead is units of its own", content: "\xf4\x90\x80x\n", line: 1, column: 4, want: 4},
		{name: "replacement character itself is one unit", content: "�x\n", line: 1, column: 4, want: 2},
		{name: "column past a CRLF terminator", content: "\U0001F600\r\nx\n", line: 1, column: 8, want: 6},
		{name: "zero column means unknown", content: "abc\n", line: 1, column: 0, want: 0},
		{name: "negative column means unknown", content: "abc\n", line: 1, column: -2, want: 0},
		{name: "negative column on unknown line means unknown", content: "abc\n", line: 9, column: -2, want: 0},
		{name: "line zero is unchanged", content: "ÄÖÜ\n", line: 0, column: 5, want: 5},
		{name: "line past end is unchanged", content: "ÄÖÜ\n", line: 9, column: 5, want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			snapshot := mdast.NewFileSnapshot("doc.md", []byte(tt.content))
			got := NewColumns(snapshot).Column(mdast.Position{Line: tt.line, Column: tt.column})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestColumns_NilSnapshot(t *testing.T) {
	t.Parallel()

	columns := NewColumns(nil)
	assert.Equal(t, 13, columns.Column(mdast.Position{Line: 3, Column: 13}))
	assert.Zero(t, columns.Column(mdast.Position{Line: 3, Column: -1}), "a negative column is unknown")
}

// lookupOrderContent mixes short lines with a long line (longer than
// markSpacing) of emoji, two-byte letters, a combining mark and ill-formed
// UTF-8, after a byte order mark, with CRLF, CR and LF line endings.
var lookupOrderContent = "\xef\xbb\xbf\U0001F600 Ä\tx\r\n" +
	strings.Repeat("é \U0001F600Ä\xe2\x82 中 ", 40) + "end   \r" +
	"plain\n" +
	"中 \U0001F600 z"

// TestColumns_AnyLookupOrder checks that one converter gives every position
// the column a fresh converter gives it, whatever order the lookups come in:
// forwards and backwards on one line, across lines, and back again.
func TestColumns_AnyLookupOrder(t *testing.T) {
	t.Parallel()

	snapshot := mdast.NewFileSnapshot("doc.md", []byte(lookupOrderContent))
	require.Greater(t, len(snapshot.LineContent(2)), 2*markSpacing, "the second line is long")

	var positions []mdast.Position
	for line := 1; line <= len(snapshot.Lines); line++ {
		info := snapshot.Lines[line-1]
		for column := 1; column <= info.EndOffset-info.StartOffset+2; column++ {
			positions = append(positions, mdast.Position{Line: line, Column: column})
		}
	}

	backwards := slices.Clone(positions)
	slices.Reverse(backwards)
	orders := map[string][]mdast.Position{
		"forwards":  positions,
		"backwards": backwards,
		"mixed":     interleaved(positions),
		"strided":   strided(positions, 97),
	}

	for name, order := range orders {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			columns := NewColumns(snapshot)
			for _, pos := range order {
				want := NewColumns(snapshot).Column(pos)
				assert.Equal(t, want, columns.Column(pos), "line %d, byte column %d", pos.Line, pos.Column)
			}
		})
	}
}

// TestColumns_LookupsOnALongLineDecodeItOnce guards the work of converting
// the columns of many findings on one long line. Lookups in column order
// continue from the previous one, and lookups behind the furthest one so far,
// such as the ends of short findings after a finding that runs to the end of
// the line, resume from a nearby mark. The work grows with the line plus a
// constant per lookup, not with the line times the findings.
func TestColumns_LookupsOnALongLineDecodeItOnce(t *testing.T) {
	t.Parallel()

	const repeats = 20000
	unit := "\U0001F600 Ä "
	line := strings.Repeat(unit, repeats)
	snapshot := mdast.NewFileSnapshot("doc.md", []byte(line+"\nnext\n"))
	unitsPerRepeat := 2 + 1 + 1 + 1
	// A lookup behind the furthest one decodes from the mark at or before its
	// multiple of markSpacing: at most one spacing and one character.
	perLookup := markSpacing + utf8.UTFMax

	t.Run("in column order", func(t *testing.T) {
		t.Parallel()

		columns := NewColumns(snapshot)
		for idx := range repeats {
			got := columns.Column(mdast.Position{Line: 1, Column: idx*len(unit) + 1})
			assert.Equal(t, idx*unitsPerRepeat+1, got)
		}
		assert.LessOrEqual(t, columns.decoded, len(line), "the line is decoded once")
	})

	t.Run("alternating long and short findings", func(t *testing.T) {
		t.Parallel()

		columns := NewColumns(snapshot)
		for idx := range repeats {
			from := idx*len(unit) + 1
			start := columns.Column(mdast.Position{Line: 1, Column: from})
			end := columns.Column(mdast.Position{Line: 1, Column: len(line) + 1})
			shortEnd := columns.Column(mdast.Position{Line: 1, Column: from + len("\U0001F600")})
			assert.Equal(t, idx*unitsPerRepeat+1, start)
			assert.Equal(t, repeats*unitsPerRepeat+1, end)
			assert.Equal(t, idx*unitsPerRepeat+3, shortEnd)
		}
		assert.LessOrEqual(t, columns.decoded, len(line)+2*repeats*perLookup,
			"the line is decoded once, plus a bounded amount per lookup")
	})

	t.Run("in reverse order", func(t *testing.T) {
		t.Parallel()

		columns := NewColumns(snapshot)
		for idx := repeats - 1; idx >= 0; idx-- {
			got := columns.Column(mdast.Position{Line: 1, Column: idx*len(unit) + 1})
			assert.Equal(t, idx*unitsPerRepeat+1, got)
		}
		assert.LessOrEqual(t, columns.decoded, len(line)+repeats*perLookup,
			"the line is decoded once, plus a bounded amount per lookup")
	})
}

// TestColumns_CharacterAcrossAMark covers a character that straddles a
// multiple of markSpacing on a long line, so the mark for that multiple is
// the end of the character. Lookups behind the furthest one that point
// inside the character must still count it in full.
func TestColumns_CharacterAcrossAMark(t *testing.T) {
	t.Parallel()

	// The emoji takes bytes 254 to 257, across offset 256.
	line := strings.Repeat("a", markSpacing-2) + "\U0001F600" + strings.Repeat("b", markSpacing)
	snapshot := mdast.NewFileSnapshot("doc.md", []byte(line+"\n"))

	columns := NewColumns(snapshot)
	require.Equal(t, (markSpacing-2)+2+markSpacing+1, columns.Column(mdast.Position{Line: 1, Column: len(line) + 1}),
		"the first lookup decodes the whole line and marks it")

	tests := []struct {
		offset int
		want   int
	}{
		{offset: 253, want: 254},
		{offset: 254, want: 255}, // the start of the emoji
		{offset: 255, want: 257}, // inside: the emoji counts in full
		{offset: 256, want: 257}, // inside, at the multiple of markSpacing
		{offset: 257, want: 257},
		{offset: 258, want: 257}, // just after the emoji, two units
		{offset: 259, want: 258},
	}
	for _, tt := range tests {
		got := columns.Column(mdast.Position{Line: 1, Column: tt.offset + 1})
		assert.Equal(t, tt.want, got, "byte offset %d", tt.offset)
		assert.Equal(t, tt.want, NewColumns(snapshot).Column(mdast.Position{Line: 1, Column: tt.offset + 1}),
			"a fresh converter agrees at byte offset %d", tt.offset)
	}
}

func TestLineText(t *testing.T) {
	t.Parallel()

	snapshot := mdast.NewFileSnapshot("doc.md", []byte("\xef\xbb\xbf# Title\r\n\r\nÄÖÜ trail   \r\n"))

	assert.Equal(t, "# Title", string(LineText(snapshot, 1)), "byte order mark and CRLF are not part of the line text")
	assert.Empty(t, LineText(snapshot, 2))
	assert.Equal(t, "ÄÖÜ trail   ", string(LineText(snapshot, 3)))
	assert.Nil(t, LineText(snapshot, 0))
	assert.Nil(t, LineText(snapshot, 99))
	assert.Nil(t, LineText(nil, 1))
}

func TestBOMLength(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 3, BOMLength([]byte("\xef\xbb\xbf# Title\n")))
	assert.Zero(t, BOMLength([]byte("# Title\n")))
	assert.Zero(t, BOMLength([]byte("\xef\xbb")), "a partial mark is not a byte order mark")
	assert.Zero(t, BOMLength(nil))
}

// interleaved returns positions taken alternately from the front and the
// back, so consecutive lookups jump between lines and directions.
func interleaved(positions []mdast.Position) []mdast.Position {
	out := make([]mdast.Position, 0, len(positions))
	for front, back := 0, len(positions)-1; front <= back; front, back = front+1, back-1 {
		out = append(out, positions[front])
		if front != back {
			out = append(out, positions[back])
		}
	}
	return out
}

// strided returns positions in the order of a walk that takes every step-th
// one and wraps around, so consecutive lookups jump far forwards and back.
// step grows until it is coprime with the number of positions, so the walk
// visits each position once.
func strided(positions []mdast.Position, step int) []mdast.Position {
	for gcd(step, len(positions)) != 1 {
		step++
	}
	out := make([]mdast.Position, 0, len(positions))
	for idx := range positions {
		out = append(out, positions[(idx*step)%len(positions)])
	}
	return out
}

// gcd returns the greatest common divisor of a and b.
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func TestColumns_ColumnWithin(t *testing.T) {
	t.Parallel()

	snapshot := mdast.NewFileSnapshot("doc.md", []byte("aa\U0001F600b\n"))
	columns := NewColumns(snapshot)
	emoji := mdast.Position{Line: 1, Column: 3}

	assert.Equal(t, 3, columns.ColumnWithin(emoji, 0), "the start of the emoji")
	assert.Equal(t, 4, columns.ColumnWithin(emoji, 1), "the second half of its surrogate pair")
	assert.Equal(t, 3, columns.ColumnWithin(emoji, -1), "a negative offset counts as 0")
	assert.Zero(t, columns.ColumnWithin(mdast.Position{Line: 1, Column: 0}, 1), "an unknown column stays unknown")
}
