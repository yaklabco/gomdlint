package fix

import (
	"bytes"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Line endings the fix engine writes.
const (
	// LineEndingLF is a Unix line ending.
	LineEndingLF = "\n"

	// LineEndingCRLF is a Windows line ending.
	LineEndingCRLF = "\r\n"

	// LineEndingCR is a carriage return that no line feed follows, the line
	// ending of classic Mac OS files. CommonMark and markdownlint read it as
	// a line ending.
	LineEndingCR = "\r"
)

// DetectLineEnding returns the dominant line ending of content, the one that
// ends the most lines: LineEndingLF, LineEndingCRLF, or LineEndingCR for a
// carriage return that no line feed follows. A tie goes to the tied ending
// that ends a line first. Content without a line ending is treated as LF.
func DetectLineEnding(content []byte) string {
	crlf := bytes.Count(content, []byte(LineEndingCRLF))
	lf := bytes.Count(content, []byte(LineEndingLF)) - crlf
	cr := bytes.Count(content, []byte(LineEndingCR)) - crlf
	most := max(lf, crlf, cr)
	if most == 0 {
		return LineEndingLF
	}

	for pos := 0; pos < len(content); {
		idx := bytes.IndexAny(content[pos:], LineEndingCRLF)
		if idx < 0 {
			break
		}
		idx += pos
		ending, count := LineEndingLF, lf
		switch {
		case content[idx] == '\n':
		case idx+1 < len(content) && content[idx+1] == '\n':
			ending, count = LineEndingCRLF, crlf
		default:
			ending, count = LineEndingCR, cr
		}
		if count == most {
			return ending
		}
		pos = idx + len(ending)
	}
	return LineEndingLF
}

// ConvertNewlines returns text with every "\n" that is not already part of a
// "\r\n" pair written as lineEnding, LineEndingCRLF or LineEndingCR. Rules
// build fix text with "\n"; the engine converts it so fixes keep the line
// endings of a CRLF or CR file. With any other lineEnding, LineEndingLF
// included, the text is returned unchanged.
func ConvertNewlines(text, lineEnding string) string {
	if (lineEnding != LineEndingCRLF && lineEnding != LineEndingCR) || !strings.Contains(text, LineEndingLF) {
		return text
	}

	var builder strings.Builder
	builder.Grow(len(text) + strings.Count(text, LineEndingLF))
	for idx := range len(text) {
		if text[idx] == '\n' && (idx == 0 || text[idx-1] != '\r') {
			builder.WriteString(lineEnding)
			continue
		}
		builder.WriteByte(text[idx])
	}
	return builder.String()
}

// LineEndingWriter writes the newlines of fix text in the line endings of
// one file's content.
type LineEndingWriter struct {
	content []byte
	ending  string
	// keep reports whether line endings need keeping apart (see KeepsApart).
	keep bool
}

// NewLineEndingWriter returns a LineEndingWriter for content and lineEnding,
// the line ending fixes are written with: the dominant line ending of the
// file content was fixed from (see DetectLineEnding), which a pass of the
// fix loop may have left without any line that ends with it.
func NewLineEndingWriter(content []byte, lineEnding string) *LineEndingWriter {
	keep := lineEnding == LineEndingCR || mdast.HasBareCarriageReturn(content)
	return &LineEndingWriter{content: content, ending: lineEnding, keep: keep}
}

// KeepsApart reports whether a carriage return that ends a line can come
// right before a line feed once fixes are applied, so that fixes must keep
// the two apart: when the content holds a carriage return that no line feed
// follows, or when fixes are written with such carriage returns. Otherwise
// fixes are written as before line endings were kept apart.
func (w *LineEndingWriter) KeepsApart() bool {
	return w.keep
}

// WriteNewlines writes the newlines of fix text in the line endings of the
// content, in place, and reports whether the edits keep every line ending of
// the content and of their text apart. edits are the edits of one fix,
// sorted and without overlaps, as Validator.Validate returns them.
//
// Each "\n" of an edit's text that is not part of a "\r\n" pair is written
// as the dominant line ending (see ConvertNewlines).
//
// Where KeepsApart reports true, a carriage return that ends a line and a
// line feed after it would make one CRLF line ending, so WriteNewlines keeps
// them apart. An edit whose text holds as many line endings as the content
// it replaces, as an edit that rewrites lines does, writes them as the line
// endings it replaces, in order, so that each line it rewrites keeps its own
// ending, unless that would join a line ending at either end of the edit.
// Otherwise, when the dominant ending is LF and the text starts with a
// newline right after a carriage return, or the dominant ending is CR and
// the text ends with a newline right before a line feed, the text's
// newlines are written as carriage returns or as line feeds instead.
// WriteNewlines reports false when the edits still put a carriage return
// that ends a line right before a line feed, as a deletion of the text
// between the two does: applied, such edits would join two line endings
// into one. Where KeepsApart reports false, the edits get the dominant
// ending throughout, and WriteNewlines reports true.
func (w *LineEndingWriter) WriteNewlines(edits []TextEdit) bool {
	for idx := range edits {
		edit := &edits[idx]
		if !w.keep {
			edit.NewText = ConvertNewlines(edit.NewText, w.ending)
			continue
		}
		if text, ok := keepLineEndings(edit.NewText, w.content[edit.StartOffset:edit.EndOffset]); ok &&
			!w.joinsAround(*edit, text) {
			edit.NewText = text
			continue
		}
		ending := w.ending
		switch {
		case w.ending == LineEndingLF && strings.HasPrefix(edit.NewText, LineEndingLF) && w.afterCarriageReturn(*edit):
			ending = LineEndingCR
		case w.ending == LineEndingCR && endsWithNewline(edit.NewText) && w.beforeLineFeed(*edit):
			ending = LineEndingLF
		}
		edit.NewText = ConvertNewlines(edit.NewText, ending)
	}
	return !w.keep || !JoinsLineEndings(w.content, edits)
}

// joinsAround reports whether text, written for edit, would join a carriage
// return that ends a line to a line feed at either end of edit.
func (w *LineEndingWriter) joinsAround(edit TextEdit, text string) bool {
	return (strings.HasPrefix(text, LineEndingLF) && w.afterCarriageReturn(edit)) ||
		(strings.HasSuffix(text, LineEndingCR) && w.beforeLineFeed(edit))
}

// afterCarriageReturn reports whether edit starts right after a carriage
// return of the content.
func (w *LineEndingWriter) afterCarriageReturn(edit TextEdit) bool {
	return edit.StartOffset > 0 && w.content[edit.StartOffset-1] == '\r'
}

// beforeLineFeed reports whether edit ends right before a line feed of the
// content.
func (w *LineEndingWriter) beforeLineFeed(edit TextEdit) bool {
	return edit.EndOffset < len(w.content) && w.content[edit.EndOffset] == '\n'
}

// keepLineEndings returns text with its line endings written as those of
// replaced, in order, when both hold the same number of line endings, one or
// more. ok is false otherwise.
func keepLineEndings(text string, replaced []byte) (string, bool) {
	want := lineEndingsOf(replaced)
	have := lineEndingsOf([]byte(text))
	if len(want) == 0 || len(have) != len(want) {
		return "", false
	}
	var builder strings.Builder
	builder.Grow(len(text) + len(replaced))
	pos := 0
	for idx, ending := range have {
		builder.WriteString(text[pos:ending.start])
		builder.WriteString(want[idx].text)
		pos = ending.start + len(ending.text)
	}
	builder.WriteString(text[pos:])
	return builder.String(), true
}

// endingAt is a line ending of a text, LineEndingLF, LineEndingCRLF or
// LineEndingCR, and where it starts.
type endingAt struct {
	text  string
	start int
}

// lineEndingsOf returns the line endings of text, in order.
func lineEndingsOf(text []byte) []endingAt {
	var endings []endingAt
	for idx := 0; idx < len(text); idx++ {
		switch {
		case text[idx] == '\n':
			endings = append(endings, endingAt{text: LineEndingLF, start: idx})
		case text[idx] != '\r':
		case idx+1 < len(text) && text[idx+1] == '\n':
			endings = append(endings, endingAt{text: LineEndingCRLF, start: idx})
			idx++
		default:
			endings = append(endings, endingAt{text: LineEndingCR, start: idx})
		}
	}
	return endings
}

// endsWithNewline reports whether text ends with a "\n" that is not part of
// a "\r\n" pair.
func endsWithNewline(text string) bool {
	return strings.HasSuffix(text, LineEndingLF) && !strings.HasSuffix(text, LineEndingCRLF)
}

// JoinsLineEndings reports whether applying edits, sorted and without
// overlaps, to content puts a carriage return right before a line feed that
// did not follow it (see JoinAt).
func JoinsLineEndings(content []byte, edits []TextEdit) bool {
	_, _, joined := JoinAt(content, edits)
	return joined
}

// JoinAt finds the first place where applying edits, sorted and without
// overlaps, to content puts a carriage return right before a line feed that
// did not follow it: where the text of an edit, or the content an edit
// leaves, meets the text of the next edit or the content after it. The
// edits must not split a CRLF pair of content, as Validator.Validate
// ensures, so every such pair is a new one, which joins two line endings
// into one. from and to are the content offsets where the piece that ends
// with the carriage return ends and where the piece that starts with the
// line feed starts; every edit between the two pieces lies in [from, to].
func JoinAt(content []byte, edits []TextEdit) (int, int, bool) {
	// last is the last byte of the fixed content so far and lastEnd the
	// content offset where the piece that holds it ends; has reports
	// whether there is such a byte. joins adds a piece of the fixed content
	// that starts with first and ends with end, at content offset stop.
	var last byte
	lastEnd := 0
	has := false
	joins := func(first, end byte, stop int) bool {
		joined := has && last == '\r' && first == '\n'
		if !joined {
			last, lastEnd, has = end, stop, true
		}
		return joined
	}

	pos := 0
	for _, edit := range edits {
		if pos < edit.StartOffset && joins(content[pos], content[edit.StartOffset-1], edit.StartOffset) {
			return lastEnd, pos, true
		}
		if text := edit.NewText; text != "" && joins(text[0], text[len(text)-1], edit.EndOffset) {
			return lastEnd, edit.StartOffset, true
		}
		pos = edit.EndOffset
	}
	if pos < len(content) && joins(content[pos], content[len(content)-1], len(content)) {
		return lastEnd, pos, true
	}
	return 0, 0, false
}
