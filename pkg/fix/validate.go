package fix

import (
	"fmt"
	"sort"
)

// ValidationError describes an invalid edit.
type ValidationError struct {
	Edit    TextEdit
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid edit [%d:%d]: %s", e.Edit.StartOffset, e.Edit.EndOffset, e.Message)
}

// ConflictError describes overlapping edits.
type ConflictError struct {
	Edit1 TextEdit
	Edit2 TextEdit
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("overlapping edits: [%d:%d] and [%d:%d]",
		e.Edit1.StartOffset, e.Edit1.EndOffset,
		e.Edit2.StartOffset, e.Edit2.EndOffset)
}

// ValidateEdits checks that all edits have valid ranges for the given content length.
// Returns nil if all edits are valid, or the first validation error encountered.
func ValidateEdits(edits []TextEdit, contentLen int) error {
	for _, edit := range edits {
		if edit.StartOffset < 0 {
			return &ValidationError{Edit: edit, Message: "start offset is negative"}
		}
		if edit.EndOffset < edit.StartOffset {
			return &ValidationError{Edit: edit, Message: "end offset is before start offset"}
		}
		if edit.EndOffset > contentLen {
			return &ValidationError{
				Edit:    edit,
				Message: fmt.Sprintf("end offset %d exceeds content length %d", edit.EndOffset, contentLen),
			}
		}
	}
	return nil
}

// SortEdits sorts edits by start offset, then by end offset.
// This produces a deterministic order for edit application.
func SortEdits(edits []TextEdit) {
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].StartOffset != edits[j].StartOffset {
			return edits[i].StartOffset < edits[j].StartOffset
		}
		return edits[i].EndOffset < edits[j].EndOffset
	})
}

// DetectConflicts checks for overlapping edits in a sorted slice.
// Returns nil if no conflicts, or the first conflict found.
// Edits must be sorted by SortEdits before calling.
func DetectConflicts(edits []TextEdit) error {
	for i := 1; i < len(edits); i++ {
		prev := edits[i-1]
		curr := edits[i]
		// Overlap if current starts before previous ends.
		if curr.StartOffset < prev.EndOffset {
			return &ConflictError{Edit1: prev, Edit2: curr}
		}
	}
	return nil
}

// PrepareEdits validates, sorts, and checks for conflicts.
// Returns the sorted edits and any error encountered.
func PrepareEdits(edits []TextEdit, contentLen int) ([]TextEdit, error) {
	if len(edits) == 0 {
		return edits, nil
	}

	if err := ValidateEdits(edits, contentLen); err != nil {
		return nil, err
	}

	result := make([]TextEdit, len(edits))
	copy(result, edits)
	SortEdits(result)

	if err := DetectConflicts(result); err != nil {
		return nil, err
	}

	return result, nil
}

// canMerge checks if two overlapping edits can be safely merged.
// Only pure deletions (empty NewText) can be merged.
func canMerge(a, b TextEdit) bool {
	return a.NewText == "" && b.NewText == ""
}

// mergeEdits merges two overlapping deletion edits into one.
// Both edits must have empty NewText (deletions only).
// Returns the merged edit covering the union of both ranges.
func mergeEdits(a, b TextEdit) TextEdit {
	return TextEdit{
		StartOffset: min(a.StartOffset, b.StartOffset),
		EndOffset:   max(a.EndOffset, b.EndOffset),
		NewText:     "",
	}
}

// FilterConflicts filters out overlapping edits from a sorted slice.
// Returns the non-conflicting edits (accepted) and the conflicting edits (skipped).
// Edits must be sorted by SortEdits before calling.
// Uses greedy algorithm: earlier edits (by start position) take precedence.
func FilterConflicts(edits []TextEdit) ([]TextEdit, []TextEdit) {
	if len(edits) == 0 {
		return nil, nil
	}

	// Pre-allocate with estimated capacity
	accepted := make([]TextEdit, 0, len(edits))
	skipped := make([]TextEdit, 0)

	// Accept the first edit
	accepted = append(accepted, edits[0])
	lastAcceptedEnd := edits[0].EndOffset

	// Process remaining edits
	for i := 1; i < len(edits); i++ {
		edit := edits[i]
		if edit.StartOffset >= lastAcceptedEnd {
			// No conflict - accept this edit
			accepted = append(accepted, edit)
			lastAcceptedEnd = edit.EndOffset
		} else {
			// Conflict - skip this edit
			skipped = append(skipped, edit)
		}
	}

	return accepted, skipped
}

// MergeAndFilterConflicts attempts to merge overlapping deletions, then filters
// any remaining conflicts. This is safer than pure filtering because overlapping
// deletions can be combined into a single deletion covering the union.
//
// Edits must be sorted by SortEdits before calling.
//
// Returns:
//   - accepted: edits to apply (merged where possible)
//   - skipped: edits that couldn't be merged or applied
//   - merged: count of edits that were merged (for reporting)
func MergeAndFilterConflicts(edits []TextEdit) ([]TextEdit, []TextEdit, int) {
	if len(edits) == 0 {
		return nil, nil, 0
	}

	accepted := make([]TextEdit, 0, len(edits))
	skipped := make([]TextEdit, 0)
	merged := 0

	// Start with first edit
	current := edits[0]

	for i := 1; i < len(edits); i++ {
		edit := edits[i]

		if edit.StartOffset >= current.EndOffset {
			// No overlap - accept current and move on
			accepted = append(accepted, current)
			current = edit
		} else {
			// Overlap detected - try to merge
			if canMerge(current, edit) {
				// Both are deletions - merge them
				current = mergeEdits(current, edit)
				merged++
			} else {
				// Can't merge - skip the later edit
				skipped = append(skipped, edit)
			}
		}
	}

	// Don't forget the last edit
	accepted = append(accepted, current)

	return accepted, skipped, merged
}

// PrepareEditsFiltered validates, sorts, merges, and filters conflicting edits.
// Unlike PrepareEdits, it does not error on conflicts - it merges deletions
// and filters remaining conflicts.
// Returns (accepted edits, skipped edits, merged count, error).
// Error only for validation failures.
func PrepareEditsFiltered(edits []TextEdit, contentLen int) ([]TextEdit, []TextEdit, int, error) {
	if len(edits) == 0 {
		return nil, nil, 0, nil
	}

	if err := ValidateEdits(edits, contentLen); err != nil {
		return nil, nil, 0, err
	}

	sorted := make([]TextEdit, len(edits))
	copy(sorted, edits)
	SortEdits(sorted)

	accepted, skipped, merged := MergeAndFilterConflicts(sorted)
	return accepted, skipped, merged, nil
}
