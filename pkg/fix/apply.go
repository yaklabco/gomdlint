package fix

import "bytes"

// ApplyEdits applies a sorted, validated slice of edits to content.
// Edits must be prepared with PrepareEdits before calling.
// Returns the modified content.
func ApplyEdits(content []byte, edits []TextEdit) []byte {
	if len(edits) == 0 {
		return content
	}

	// Estimate result size.
	delta := 0
	for _, e := range edits {
		delta += len(e.NewText) - (e.EndOffset - e.StartOffset)
	}

	var out bytes.Buffer
	out.Grow(len(content) + delta)

	cursor := 0
	for _, e := range edits {
		// Copy content before this edit.
		out.Write(content[cursor:e.StartOffset])
		// Write replacement text.
		out.WriteString(e.NewText)
		cursor = e.EndOffset
	}
	// Copy remaining content.
	out.Write(content[cursor:])

	return out.Bytes()
}
