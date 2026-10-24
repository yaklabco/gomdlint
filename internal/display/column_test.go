package display

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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
		{name: "invalid bytes count one each", content: "\xff\xfex\n", line: 1, column: 3, want: 3},
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

// TestColumns_AnyLookupOrder checks that one converter gives every position
// the column a fresh converter gives it, whatever order the lookups come in:
// forwards and backwards on one line, across lines, and back again.
func TestColumns_AnyLookupOrder(t *testing.T) {
	t.Parallel()

	content := "\xef\xbb\xbf\U0001F600 Ä\tx\r\né \U0001F600\U0001F600 y\rplain\n中 \U0001F600 z"
	snapshot := mdast.NewFileSnapshot("doc.md", []byte(content))

	var positions []mdast.Position
	for line := 1; line <= len(snapshot.Lines); line++ {
		info := snapshot.Lines[line-1]
		for column := 1; column <= info.EndOffset-info.StartOffset+1; column++ {
			positions = append(positions, mdast.Position{Line: line, Column: column})
		}
	}

	orders := map[string][]mdast.Position{
		"forwards":  positions,
		"backwards": reversed(positions),
		"mixed":     interleaved(positions),
	}

	for name, order := range orders {
		columns := NewColumns(snapshot)
		for _, pos := range order {
			want := NewColumns(snapshot).Column(pos)
			assert.Equal(t, want, columns.Column(pos), "%s: line %d, byte column %d", name, pos.Line, pos.Column)
		}
	}
}

// TestColumns_SortedLookupsDecodeEachLineOnce guards the work of converting
// the columns of many findings on one long line: lookups in column order
// continue from the previous one instead of decoding the line from its
// start again, so the work grows with the line, not with the line times the
// findings.
func TestColumns_SortedLookupsDecodeEachLineOnce(t *testing.T) {
	t.Parallel()

	const repeats = 20000
	line := strings.Repeat("\U0001F600 Ä ", repeats)
	snapshot := mdast.NewFileSnapshot("doc.md", []byte(line+"\nnext\n"))

	columns := NewColumns(snapshot)
	unitsPerRepeat := 2 + 1 + 1 + 1
	bytesPerRepeat := len("\U0001F600 Ä ")
	for idx := range repeats {
		got := columns.Column(mdast.Position{Line: 1, Column: idx*bytesPerRepeat + 1})
		assert.Equal(t, idx*unitsPerRepeat+1, got)
	}

	assert.LessOrEqual(t, columns.decoded, len(line), "the line is decoded once")
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

// reversed returns a reversed copy of positions.
func reversed(positions []mdast.Position) []mdast.Position {
	out := make([]mdast.Position, len(positions))
	for idx, pos := range positions {
		out[len(positions)-1-idx] = pos
	}
	return out
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
