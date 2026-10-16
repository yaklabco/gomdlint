package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

func TestConvertColumn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		line    int
		column  int
		unit    ColumnUnit
		want    int
	}{
		{name: "ascii is unchanged", content: "plain trail   \n", line: 1, column: 12, unit: ColumnCharacters, want: 12},
		{name: "two-byte letters count once", content: "# Title\n\nÄÖÜ trail   \n", line: 3, column: 13, unit: ColumnCharacters, want: 10},
		{name: "exclusive end column", content: "# Title\n\nÄÖÜ trail   \n", line: 3, column: 16, unit: ColumnCharacters, want: 13},
		{name: "bmp letters are one utf16 unit", content: "ÄÖÜ x\n", line: 1, column: 8, unit: ColumnUTF16, want: 5},
		{name: "astral character is one code point", content: "\U0001F600 x\n", line: 1, column: 6, unit: ColumnCharacters, want: 3},
		{name: "astral character is two utf16 units", content: "\U0001F600 x\n", line: 1, column: 6, unit: ColumnUTF16, want: 4},
		{name: "column past content counts terminator bytes", content: "é\r\nnext\n", line: 1, column: 4, unit: ColumnCharacters, want: 3},
		{name: "column at end of last line without newline", content: "ab", line: 1, column: 3, unit: ColumnCharacters, want: 3},
		{name: "leading byte order mark is not a column", content: "\ufeff# Title\n", line: 1, column: 4, unit: ColumnCharacters, want: 1},
		{name: "column after byte order mark", content: "\ufeff# Title\n", line: 1, column: 6, unit: ColumnUTF16, want: 3},
		{name: "column inside byte order mark", content: "\ufeff# Title\n", line: 1, column: 2, unit: ColumnCharacters, want: 1},
		{name: "zero width no-break space on later line counts", content: "a\n\ufeffb\n", line: 2, column: 4, unit: ColumnCharacters, want: 2},
		{name: "column inside a character covers it", content: "中x\n", line: 1, column: 2, unit: ColumnCharacters, want: 2},
		{name: "invalid bytes count one each", content: "\xff\xfex\n", line: 1, column: 3, unit: ColumnCharacters, want: 3},
		{name: "zero column means unknown", content: "abc\n", line: 1, column: 0, unit: ColumnCharacters, want: 0},
		{name: "line zero is unchanged", content: "ÄÖÜ\n", line: 0, column: 5, unit: ColumnCharacters, want: 5},
		{name: "line past end is unchanged", content: "ÄÖÜ\n", line: 9, column: 5, unit: ColumnCharacters, want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			snapshot := mdast.NewFileSnapshot("doc.md", []byte(tt.content))
			got := ConvertColumn(snapshot, tt.line, tt.column, tt.unit)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertColumn_NilSnapshot(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 13, ConvertColumn(nil, 3, 13, ColumnCharacters))
}

func TestLineText(t *testing.T) {
	t.Parallel()

	snapshot := mdast.NewFileSnapshot("doc.md", []byte("\ufeff# Title\r\n\r\nÄÖÜ trail   \r\n"))

	assert.Equal(t, "# Title", string(LineText(snapshot, 1)), "byte order mark and CRLF are not part of the line text")
	assert.Empty(t, LineText(snapshot, 2))
	assert.Equal(t, "ÄÖÜ trail   ", string(LineText(snapshot, 3)))
	assert.Nil(t, LineText(snapshot, 0))
	assert.Nil(t, LineText(snapshot, 99))
	assert.Nil(t, LineText(nil, 1))
}
