package fix

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"unicode/utf8"
)

// Validator checks fixes against the content they will be applied to.
type Validator struct {
	content []byte

	// checkRunes is true when content is valid UTF-8, so an edit boundary
	// inside a multi-byte character can be detected.
	checkRunes bool
}

// NewValidator returns a Validator for content.
func NewValidator(content []byte) *Validator {
	return &Validator{content: content, checkRunes: utf8.Valid(content)}
}

// Validate checks that edits form a well-formed fix: every range lies within
// the content, no two edits overlap, and no edit boundary falls between the
// "\r" and "\n" of a line ending or, in UTF-8 content, inside a multi-byte
// character. It returns a sorted copy of the edits and leaves the input
// unchanged. Errors are *ValidationError or *ConflictError values.
func (v *Validator) Validate(edits []TextEdit) ([]TextEdit, error) {
	if err := ValidateEdits(edits, len(v.content)); err != nil {
		return nil, err
	}

	sorted := slices.Clone(edits)
	SortEdits(sorted)
	if err := DetectConflicts(sorted); err != nil {
		return nil, err
	}

	for _, edit := range sorted {
		for _, offset := range [...]int{edit.StartOffset, edit.EndOffset} {
			if msg := v.splitAt(offset); msg != "" {
				return nil, &ValidationError{Edit: edit, Message: msg}
			}
		}
	}
	return sorted, nil
}

// splitAt describes what an edit boundary at offset would split, or returns
// an empty string when the boundary is safe.
func (v *Validator) splitAt(offset int) string {
	if offset <= 0 || offset >= len(v.content) {
		return ""
	}
	if v.content[offset-1] == '\r' && v.content[offset] == '\n' {
		return fmt.Sprintf("offset %d splits a CRLF line ending", offset)
	}
	if v.checkRunes && !utf8.RuneStart(v.content[offset]) {
		return fmt.Sprintf("offset %d splits a multi-byte UTF-8 character", offset)
	}
	return ""
}

// ResolveFixes chooses the fixes to apply in one pass.
//
// Each fix is a group of edits that must be applied together, so a fix is
// accepted whole or skipped whole and is never half applied. Each fix must
// be valid on its own (see Validator). Fixes are considered in order of their
// first edit (start offset, then end offset), with input order breaking
// ties, so the earlier fix wins a conflict. A fix is skipped when one of its
// edits overlaps an accepted edit or inserts strictly inside one, except
// that overlapping pure deletions merge into a single deletion.
//
// It returns the accepted edits, sorted for ApplyEdits, and the indexes of
// the skipped fixes in ascending order. Insertions at the same offset are
// applied in input order of their fixes.
func ResolveFixes(fixes [][]TextEdit) ([]TextEdit, []int) {
	firsts := make([]TextEdit, len(fixes))
	order := make([]int, 0, len(fixes))
	for idx, edits := range fixes {
		if len(edits) == 0 {
			continue
		}
		firsts[idx] = slices.MinFunc(edits, compareRanges)
		order = append(order, idx)
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return compareRanges(firsts[a], firsts[b])
	})

	var accepted []placedEdit
	var skipped []int
	for _, fixIdx := range order {
		if !fits(accepted, fixes[fixIdx]) {
			skipped = append(skipped, fixIdx)
			continue
		}
		for editIdx, edit := range fixes[fixIdx] {
			accepted = place(accepted, placedEdit{edit: edit, fix: fixIdx, index: editIdx})
		}
	}
	slices.Sort(skipped)

	if len(accepted) == 0 {
		return nil, skipped
	}
	result := make([]TextEdit, len(accepted))
	for idx, placed := range accepted {
		result[idx] = placed.edit
	}
	return result, skipped
}

// placedEdit is an accepted edit together with the position of its fix in
// the input and its position within the fix, which order insertions at the
// same offset.
type placedEdit struct {
	edit  TextEdit
	fix   int
	index int
}

func comparePlaced(left, right placedEdit) int {
	if c := compareRanges(left.edit, right.edit); c != 0 {
		return c
	}
	if c := cmp.Compare(left.fix, right.fix); c != 0 {
		return c
	}
	return cmp.Compare(left.index, right.index)
}

// overlapping returns the half-open index range of accepted edits that
// conflict with edit, meaning start < edit.EndOffset and end >
// edit.StartOffset. Accepted edits are sorted and conflict free, so their
// starts and ends are both non-decreasing and the conflicting edits are
// contiguous.
func overlapping(accepted []placedEdit, edit TextEdit) (int, int) {
	lo := sort.Search(len(accepted), func(idx int) bool {
		return accepted[idx].edit.EndOffset > edit.StartOffset
	})
	hi := sort.Search(len(accepted), func(idx int) bool {
		return accepted[idx].edit.StartOffset >= edit.EndOffset
	})
	return lo, max(lo, hi)
}

// fits reports whether every edit of a fix can join the accepted edits,
// either without conflict or by merging pure deletions.
func fits(accepted []placedEdit, edits []TextEdit) bool {
	for _, edit := range edits {
		lo, hi := overlapping(accepted, edit)
		if lo == hi {
			continue
		}
		if edit.NewText != "" {
			return false
		}
		for _, placed := range accepted[lo:hi] {
			if placed.edit.NewText != "" {
				return false
			}
		}
	}
	return true
}

// place adds an edit that fits to the accepted edits, merging it with any
// deletions it overlaps.
func place(accepted []placedEdit, edit placedEdit) []placedEdit {
	lo, hi := overlapping(accepted, edit.edit)
	if lo == hi {
		idx, _ := slices.BinarySearchFunc(accepted, edit, comparePlaced)
		return slices.Insert(accepted, idx, edit)
	}

	merged := accepted[lo]
	merged.edit = TextEdit{
		StartOffset: min(edit.edit.StartOffset, accepted[lo].edit.StartOffset),
		EndOffset:   max(edit.edit.EndOffset, accepted[hi-1].edit.EndOffset),
	}
	return slices.Replace(accepted, lo, hi, merged)
}
