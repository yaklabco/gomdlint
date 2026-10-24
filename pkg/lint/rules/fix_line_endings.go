package rules

import (
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Fix checks and line endings.
//
// A carriage return that no line feed follows ends a line, as a line feed
// does, but goldmark and the checks' own line scans end lines only at line
// feeds. So the fix checks read a document with such carriage returns
// written as line feeds, which keeps every offset and every line, and read
// the text of fixes the same way.
//
// A rule writes "\n" for each newline of its fix text, and the engine writes
// it in the file's line endings (see fix.LineEndingWriter), keeping it a line
// ending of its own. Against the document as the checks read it, the "\n"
// is a line ending of its own too, never joined to a carriage return before
// it, so the checks see the lines the engine writes.

// checkedContent returns a document's content as the fix checks read it.
func checkedContent(content []byte) []byte {
	return mdast.LineFeedContent(content)
}

// checkedEdit returns edit with its text read as the fix checks read text.
func checkedEdit(edit fix.TextEdit) fix.TextEdit {
	edit.NewText = mdast.LineFeedText(edit.NewText)
	return edit
}

// checkedEdits returns edits with their text read as the fix checks read
// text: edits itself when no text holds a carriage return, and a copy
// otherwise.
func checkedEdits(edits []fix.TextEdit) []fix.TextEdit {
	for idx, edit := range edits {
		if strings.Contains(edit.NewText, "\r") {
			checked := slices.Clone(edits)
			for at := idx; at < len(checked); at++ {
				checked[at] = checkedEdit(checked[at])
			}
			return checked
		}
	}
	return edits
}
