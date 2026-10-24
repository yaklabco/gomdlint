package fix_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

func TestDetectLineEnding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty", content: "", want: fix.LineEndingLF},
		{name: "no line breaks", content: "# Title", want: fix.LineEndingLF},
		{name: "lf", content: "a\nb\n", want: fix.LineEndingLF},
		{name: "crlf", content: "a\r\nb\r\n", want: fix.LineEndingCRLF},
		{name: "mostly crlf", content: "a\r\nb\r\nc\n", want: fix.LineEndingCRLF},
		{name: "mostly lf", content: "a\nb\nc\r\n", want: fix.LineEndingLF},
		{name: "tie goes to first crlf", content: "a\r\nb\n", want: fix.LineEndingCRLF},
		{name: "tie goes to first lf", content: "a\nb\r\n", want: fix.LineEndingLF},
		{name: "cr", content: "a\rb\r", want: fix.LineEndingCR},
		{name: "mostly cr", content: "a\rb\rc\n", want: fix.LineEndingCR},
		{name: "mostly lf with cr", content: "a\nb\rc\n", want: fix.LineEndingLF},
		{name: "mostly crlf with cr", content: "a\r\nb\rc\r\n", want: fix.LineEndingCRLF},
		{name: "tie goes to first cr", content: "a\rb\n", want: fix.LineEndingCR},
		{name: "tie goes to first lf after cr", content: "a\nb\r", want: fix.LineEndingLF},
		{name: "three-way tie goes to first", content: "a\r\nb\rc\n", want: fix.LineEndingCRLF},
		{name: "tie goes to the first of the most common", content: "a\rb\nc\r\nd\ne\r\n", want: fix.LineEndingLF},
		{name: "carriage return before crlf", content: "a\r\r\n", want: fix.LineEndingCR},
		{name: "final carriage return", content: "a\r", want: fix.LineEndingCR},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, fix.DetectLineEnding([]byte(tt.content)))
		})
	}
}

func TestConvertNewlines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		text   string
		ending string
		want   string
	}{
		{name: "lf keeps text", text: "a\nb\n", ending: fix.LineEndingLF, want: "a\nb\n"},
		{name: "lf keeps existing crlf", text: "a\r\nb\n", ending: fix.LineEndingLF, want: "a\r\nb\n"},
		{name: "crlf converts bare newlines", text: "\n", ending: fix.LineEndingCRLF, want: "\r\n"},
		{name: "crlf converts every newline", text: "a\n\nb\n", ending: fix.LineEndingCRLF, want: "a\r\n\r\nb\r\n"},
		{name: "crlf leaves existing pairs", text: "a\r\nb\n", ending: fix.LineEndingCRLF, want: "a\r\nb\r\n"},
		{name: "crlf without newlines", text: "abc", ending: fix.LineEndingCRLF, want: "abc"},
		{name: "empty", text: "", ending: fix.LineEndingCRLF, want: ""},
		{name: "cr converts bare newlines", text: "\n", ending: fix.LineEndingCR, want: "\r"},
		{name: "cr converts every newline", text: "a\n\nb\n", ending: fix.LineEndingCR, want: "a\r\rb\r"},
		{name: "cr leaves existing pairs", text: "a\r\nb\n", ending: fix.LineEndingCR, want: "a\r\nb\r"},
		{name: "cr keeps carriage returns", text: "a\rb\n", ending: fix.LineEndingCR, want: "a\rb\r"},
		{name: "cr without newlines", text: "abc", ending: fix.LineEndingCR, want: "abc"},
		{name: "unknown ending keeps text", text: "a\nb", ending: "", want: "a\nb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, fix.ConvertNewlines(tt.text, tt.ending))
		})
	}
}

func TestLineEndingWriter(t *testing.T) {
	t.Parallel()

	insert := func(offset int, text string) fix.TextEdit {
		return fix.TextEdit{StartOffset: offset, EndOffset: offset, NewText: text}
	}
	tests := []struct {
		name    string
		content string
		edits   []fix.TextEdit
		want    []string
		wantOK  bool
	}{
		{name: "lf", content: "a\nb\n", edits: []fix.TextEdit{insert(2, "\n")}, want: []string{"\n"}, wantOK: true},
		{name: "crlf", content: "a\r\nb\r\n", edits: []fix.TextEdit{insert(3, ">\n")}, want: []string{">\r\n"}, wantOK: true},
		{name: "cr", content: "a\rb\r", edits: []fix.TextEdit{insert(2, "\n"), insert(4, "\n")}, want: []string{"\r", "\r"}, wantOK: true},
		{name: "text without newlines", content: "a\rb\r", edits: []fix.TextEdit{{StartOffset: 0, EndOffset: 1, NewText: "x"}}, want: []string{"x"}, wantOK: true},
		{
			// A line feed would make a CRLF pair with the carriage return
			// before it, so the newlines are written as carriage returns.
			name: "newlines after a carriage return in a line feed file", content: "a\rb\nc\n",
			edits: []fix.TextEdit{insert(2, "\n\n")}, want: []string{"\r\r"}, wantOK: true,
		},
		{
			name: "prefixed newline after a carriage return in a line feed file", content: "> a\rb\nc\n",
			edits: []fix.TextEdit{insert(4, ">\n")}, want: []string{">\n"}, wantOK: true,
		},
		{
			// A carriage return would make a CRLF pair with the line feed
			// after it, so the newlines are written as line feeds.
			name: "newlines before a line feed in a carriage return file", content: "a\n\nb\rc\rd\r",
			edits: []fix.TextEdit{insert(2, "x\ny\n")}, want: []string{"x\ny\n"}, wantOK: true,
		},
		{
			name: "newlines after a carriage return in a crlf file", content: "a\rb\r\nc\r\n",
			edits: []fix.TextEdit{insert(2, "\n")}, want: []string{"\r\n"}, wantOK: true,
		},
		{
			name: "deletion between a carriage return and a line feed", content: "a\r\r\n\nb",
			edits: []fix.TextEdit{{StartOffset: 2, EndOffset: 4}}, wantOK: false,
		},
		{
			name: "newline between a carriage return and a line feed", content: "a\rX\nb",
			edits: []fix.TextEdit{{StartOffset: 2, EndOffset: 3, NewText: "\n"}}, wantOK: false,
		},
		{
			name: "adjacent edits that join a carriage return and a line feed", content: "ab\n\n\rc",
			edits:  []fix.TextEdit{{StartOffset: 0, EndOffset: 1, NewText: "\r"}, {StartOffset: 1, EndOffset: 2, NewText: "\n"}},
			wantOK: false,
		},
		{
			// Without a carriage return alone in the content, nothing is
			// withheld, as before line endings were kept apart.
			name: "adjacent edits in content without carriage returns alone", content: "ab",
			edits:  []fix.TextEdit{{StartOffset: 0, EndOffset: 1, NewText: "\r"}, {StartOffset: 1, EndOffset: 2, NewText: "\n"}},
			want:   []string{"\r", "\n"},
			wantOK: true,
		},
		{
			// The edit rewrites one line, but keeping its line feed would
			// join it to the carriage return before the edit.
			name: "line rewritten after a carriage return", content: "a\rb\nc\rd\r",
			edits:  []fix.TextEdit{{StartOffset: 2, EndOffset: 4, NewText: "\nb"}},
			want:   []string{"\rb"},
			wantOK: true,
		},
		{
			name: "deletion between line feeds", content: "a\n\n\nb",
			edits: []fix.TextEdit{{StartOffset: 2, EndOffset: 3}}, want: []string{""}, wantOK: true,
		},
		{
			// The edit rewrites lines it copies with their line endings, so
			// each keeps its own, a line feed in a carriage return file.
			name: "lines rewritten with their own endings", content: "* a\n* b\r* c\r",
			edits: []fix.TextEdit{{StartOffset: 0, EndOffset: 9, NewText: "- a\n- b\r-"}}, want: []string{"- a\n- b\r-"}, wantOK: true,
		},
		{
			name: "lines rewritten keep their endings in order", content: "a\rb\nc\r",
			edits: []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "x\ny\nz"}}, want: []string{"x\ry\nz"}, wantOK: true,
		},
		{
			name: "lines added take the dominant ending", content: "a\rb\r",
			edits: []fix.TextEdit{{StartOffset: 0, EndOffset: 3, NewText: "x\ny\nz"}}, want: []string{"x\ry\rz"}, wantOK: true,
		},
		{
			// Without a carriage return alone, a CRLF file's newlines are all
			// written as CRLF, as before line endings were kept.
			name: "lines rewritten in a CRLF file without carriage returns alone", content: "a\r\nb\nc\r\nd\r\n",
			edits: []fix.TextEdit{{StartOffset: 3, EndOffset: 6, NewText: "B\nC"}}, want: []string{"B\r\nC"}, wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			content := []byte(tt.content)
			edits := append([]fix.TextEdit(nil), tt.edits...)
			ok := fix.NewLineEndingWriter(content, fix.DetectLineEnding(content)).WriteNewlines(edits)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			got := make([]string, len(edits))
			for idx, edit := range edits {
				got[idx] = edit.NewText
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestJoinAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		edits    []fix.TextEdit
		from, to int
		joined   bool
	}{
		{name: "no edits", content: "a\rb\n"},
		{name: "deletion between a carriage return and a line feed", content: "a\rxy\nb",
			edits: []fix.TextEdit{{StartOffset: 2, EndOffset: 4}}, from: 2, to: 4, joined: true},
		{name: "two deletions that empty the text between them", content: "a\rxy\nb",
			edits: []fix.TextEdit{{StartOffset: 2, EndOffset: 3}, {StartOffset: 3, EndOffset: 4}}, from: 2, to: 4, joined: true},
		{name: "carriage return written before a line feed", content: "ab\nc",
			edits: []fix.TextEdit{{StartOffset: 1, EndOffset: 2, NewText: "\r"}}, from: 2, to: 2, joined: true},
		{name: "line feed written after a carriage return", content: "a\rb",
			edits: []fix.TextEdit{{StartOffset: 2, EndOffset: 2, NewText: "\n"}}, from: 2, to: 2, joined: true},
		{name: "carriage returns kept apart", content: "a\rb\n",
			edits: []fix.TextEdit{{StartOffset: 2, EndOffset: 2, NewText: "\r"}}},
		{name: "CRLF pair of the content", content: "a\r\nb", edits: []fix.TextEdit{{StartOffset: 3, EndOffset: 4, NewText: "c"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			from, to, joined := fix.JoinAt([]byte(tt.content), tt.edits)
			assert.Equal(t, tt.joined, joined)
			assert.Equal(t, tt.joined, fix.JoinsLineEndings([]byte(tt.content), tt.edits))
			if tt.joined {
				assert.Equal(t, []int{tt.from, tt.to}, []int{from, to})
			}
		})
	}
}
