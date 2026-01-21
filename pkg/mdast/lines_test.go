package mdast_test

import (
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

func TestBuildLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected []mdast.LineInfo
	}{
		{
			name:     "empty content",
			content:  "",
			expected: []mdast.LineInfo{},
		},
		{
			name:    "single line no newline",
			content: "hello",
			expected: []mdast.LineInfo{
				{StartOffset: 0, NewlineStart: 5, EndOffset: 5},
			},
		},
		{
			name:    "single line with LF",
			content: "hello\n",
			expected: []mdast.LineInfo{
				{StartOffset: 0, NewlineStart: 5, EndOffset: 6},
				{StartOffset: 6, NewlineStart: 6, EndOffset: 6},
			},
		},
		{
			name:    "single line with CRLF",
			content: "hello\r\n",
			expected: []mdast.LineInfo{
				{StartOffset: 0, NewlineStart: 5, EndOffset: 7},
				{StartOffset: 7, NewlineStart: 7, EndOffset: 7},
			},
		},
		{
			name:    "multiple lines LF",
			content: "line1\nline2\nline3",
			expected: []mdast.LineInfo{
				{StartOffset: 0, NewlineStart: 5, EndOffset: 6},
				{StartOffset: 6, NewlineStart: 11, EndOffset: 12},
				{StartOffset: 12, NewlineStart: 17, EndOffset: 17},
			},
		},
		{
			name:    "multiple lines CRLF",
			content: "line1\r\nline2\r\n",
			expected: []mdast.LineInfo{
				{StartOffset: 0, NewlineStart: 5, EndOffset: 7},
				{StartOffset: 7, NewlineStart: 12, EndOffset: 14},
				{StartOffset: 14, NewlineStart: 14, EndOffset: 14},
			},
		},
		{
			name:    "single character",
			content: "x",
			expected: []mdast.LineInfo{
				{StartOffset: 0, NewlineStart: 1, EndOffset: 1},
			},
		},
		{
			name:    "only newline",
			content: "\n",
			expected: []mdast.LineInfo{
				{StartOffset: 0, NewlineStart: 0, EndOffset: 1},
				{StartOffset: 1, NewlineStart: 1, EndOffset: 1},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			lines := mdast.BuildLines([]byte(testCase.content))

			if len(lines) != len(testCase.expected) {
				t.Fatalf("expected %d lines, got %d", len(testCase.expected), len(lines))
			}

			for i, exp := range testCase.expected {
				got := lines[i]
				if got.StartOffset != exp.StartOffset ||
					got.NewlineStart != exp.NewlineStart ||
					got.EndOffset != exp.EndOffset {
					t.Errorf("line %d: expected %+v, got %+v", i, exp, got)
				}
			}
		})
	}
}

func TestFileSnapshot_LineAt(t *testing.T) {
	t.Parallel()

	content := "line1\nline2\nline3"
	snapshot := mdast.NewFileSnapshot("test.md", []byte(content))

	tests := []struct {
		name         string
		offset       int
		expectedLine int
		expectedCol  int
	}{
		{"start of file", 0, 1, 1},
		{"middle of line 1", 2, 1, 3},
		{"end of line 1 content", 4, 1, 5},
		{"newline of line 1", 5, 1, 6},
		{"start of line 2", 6, 2, 1},
		{"middle of line 2", 8, 2, 3},
		{"start of line 3", 12, 3, 1},
		{"end of file", 16, 3, 5},
		{"past end of file", 17, 3, 6},
		{"negative offset", -1, 0, 0},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			line, col := snapshot.LineAt(testCase.offset)
			if line != testCase.expectedLine || col != testCase.expectedCol {
				t.Errorf("LineAt(%d): expected (%d, %d), got (%d, %d)",
					testCase.offset, testCase.expectedLine, testCase.expectedCol, line, col)
			}
		})
	}
}

func TestFileSnapshot_Offset(t *testing.T) {
	t.Parallel()

	content := "line1\nline2\nline3"
	snapshot := mdast.NewFileSnapshot("test.md", []byte(content))

	tests := []struct {
		name           string
		line           int
		col            int
		expectedOffset int
		expectedOK     bool
	}{
		{"start of file", 1, 1, 0, true},
		{"middle of line 1", 1, 3, 2, true},
		{"start of line 2", 2, 1, 6, true},
		{"start of line 3", 3, 1, 12, true},
		{"end of line 3", 3, 5, 16, true},
		{"invalid line 0", 0, 1, 0, false},
		{"invalid line 4", 4, 1, 0, false},
		{"invalid col 0", 1, 0, 0, false},
		{"col past line end", 1, 10, 0, false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			offset, ok := snapshot.Offset(testCase.line, testCase.col)
			if ok != testCase.expectedOK {
				t.Errorf("Offset(%d, %d): expected ok=%v, got ok=%v",
					testCase.line, testCase.col, testCase.expectedOK, ok)
			}
			if ok && offset != testCase.expectedOffset {
				t.Errorf("Offset(%d, %d): expected %d, got %d",
					testCase.line, testCase.col, testCase.expectedOffset, offset)
			}
		})
	}
}

func TestLineAtAndOffsetAreInverses(t *testing.T) {
	t.Parallel()

	content := "first\nsecond\nthird line\n"
	snapshot := mdast.NewFileSnapshot("test.md", []byte(content))

	// For each valid offset, LineAt -> Offset should return the same offset.
	for offset := range len(content) {
		line, col := snapshot.LineAt(offset)
		if line == 0 {
			t.Errorf("LineAt(%d) returned invalid position", offset)
			continue
		}

		gotOffset, ok := snapshot.Offset(line, col)
		if !ok {
			t.Errorf("Offset(%d, %d) returned not ok for offset %d", line, col, offset)
			continue
		}

		if gotOffset != offset {
			t.Errorf("roundtrip failed: offset %d -> (%d, %d) -> %d",
				offset, line, col, gotOffset)
		}
	}
}

func TestFileSnapshot_LineCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{"empty", "", 0},
		{"single line no newline", "hello", 1},
		{"single line with newline", "hello\n", 2},
		{"three lines", "a\nb\nc", 3},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snapshot := mdast.NewFileSnapshot("test.md", []byte(testCase.content))
			if snapshot.LineCount() != testCase.expected {
				t.Errorf("expected %d lines, got %d", testCase.expected, snapshot.LineCount())
			}
		})
	}
}

func TestFileSnapshot_LineContent(t *testing.T) {
	t.Parallel()

	content := "first\nsecond\nthird"
	snapshot := mdast.NewFileSnapshot("test.md", []byte(content))

	tests := []struct {
		line     int
		expected string
	}{
		{1, "first"},
		{2, "second"},
		{3, "third"},
		{0, ""},  // invalid
		{4, ""},  // invalid
		{-1, ""}, // invalid
	}

	for _, testCase := range tests {
		lineContent := snapshot.LineContent(testCase.line)
		got := ""
		if lineContent != nil {
			got = string(lineContent)
		}

		if got != testCase.expected {
			t.Errorf("LineContent(%d): expected %q, got %q", testCase.line, testCase.expected, got)
		}
	}
}
