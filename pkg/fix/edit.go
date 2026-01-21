// Package fix provides text edit types and application logic for auto-fixing.
package fix

// TextEdit represents a single text replacement in a file.
type TextEdit struct {
	// StartOffset is the byte index where the edit begins (inclusive).
	StartOffset int

	// EndOffset is the byte index where the edit ends (exclusive).
	EndOffset int

	// NewText is the replacement text.
	NewText string
}

// EditBuilder accumulates text edits for a file.
// This is a stub that will be expanded in later phases.
type EditBuilder struct {
	Edits []TextEdit
}

// NewEditBuilder creates a new EditBuilder.
func NewEditBuilder() *EditBuilder {
	return &EditBuilder{
		Edits: make([]TextEdit, 0),
	}
}

// ReplaceRange adds an edit that replaces bytes [start, end) with newText.
func (b *EditBuilder) ReplaceRange(start, end int, newText string) {
	b.Edits = append(b.Edits, TextEdit{
		StartOffset: start,
		EndOffset:   end,
		NewText:     newText,
	})
}

// Insert adds an edit that inserts text at the given offset.
func (b *EditBuilder) Insert(offset int, text string) {
	b.ReplaceRange(offset, offset, text)
}

// Delete adds an edit that deletes bytes [start, end).
func (b *EditBuilder) Delete(start, end int) {
	b.ReplaceRange(start, end, "")
}
