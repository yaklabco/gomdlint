package fix

import (
	"slices"
	"strings"
)

// CombineEdits merges edits of content that do not overlap into one edit
// that also writes the text between them back unchanged, so that they are
// applied together or not at all. Applied, the edit gives the same text as
// the edits. It reports false when there are no edits or two of them
// overlap. Edits are taken in the order of SortEdits, so an insertion at
// the start of a replacement goes first and insertions at one offset keep
// their order. The caller's edits keep their order.
func CombineEdits(content []byte, edits []TextEdit) (TextEdit, bool) {
	if len(edits) == 0 {
		return TextEdit{}, false
	}
	sorted := slices.Clone(edits)
	SortEdits(sorted)
	var text strings.Builder
	for idx, edit := range sorted {
		if idx > 0 {
			prev := sorted[idx-1]
			if edit.StartOffset < prev.EndOffset {
				return TextEdit{}, false
			}
			text.Write(content[prev.EndOffset:edit.StartOffset])
		}
		text.WriteString(edit.NewText)
	}
	return TextEdit{
		StartOffset: sorted[0].StartOffset,
		EndOffset:   sorted[len(sorted)-1].EndOffset,
		NewText:     text.String(),
	}, true
}
