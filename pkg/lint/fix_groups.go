package lint

import (
	"cmp"
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

// fixGroups gathers the fix groups of one rule's diagnostics in a file (see
// Diagnostic.FixGroup) and makes each group one fix. Its maps are nil for a
// rule that reports no group.
type fixGroups struct {
	// sizes counts the diagnostics the rule reported in each group.
	sizes map[int]int
	// slots holds, for each group, the index among the pending fixes that
	// its fix takes: the place of its first member with a valid fix, when
	// the run applies fixes.
	slots map[int]int
}

// newFixGroups counts the members of each fix group among the diagnostics a
// rule reported, before any is dropped.
func newFixGroups(diags []Diagnostic) *fixGroups {
	groups := &fixGroups{}
	for _, diag := range diags {
		if diag.FixGroup == 0 {
			continue
		}
		if groups.sizes == nil {
			groups.sizes = make(map[int]int)
			groups.slots = make(map[int]int)
		}
		groups.sizes[diag.FixGroup]++
	}
	return groups
}

// reserve keeps a place among fixes for the fix of group, at its first
// member, so that the group's fix is resolved where a fix that member
// carried would be.
func (g *fixGroups) reserve(group int, fixes []pendingFix, ruleID string) []pendingFix {
	if _, ok := g.slots[group]; ok {
		return fixes
	}
	g.slots[group] = len(fixes)
	return append(fixes, pendingFix{ruleID: ruleID})
}

// offer decides the fix of each group among diags, the rule's diagnostics
// that the engine keeps, with their validated fixes, or none where a fix
// was withheld or invalid. A group is fixed when every diagnostic the rule
// reported in it is kept with a fix, and their edits do not overlap: they
// become one edit that also writes the text between them back unchanged,
// which is offered like any other fix, and so is checked for line endings
// as a whole, and takes the place that reserve kept, fixing each member.
// Each member keeps its own edits, rewritten where the engine writes the
// group's edit in other line endings (see writeMembers), so that the
// members' edits together make exactly that edit. Where they cannot, as
// when a member's boundary falls inside a line ending that writing changed
// or two members' edits are written alike, each member reports the whole
// written edit, which a consumer applies once. A group that is not fixed
// leaves every member without its fix and its group, and the group's place
// among fixes is dropped.
func (g *fixGroups) offer(
	diags []Diagnostic,
	fixes []pendingFix,
	content []byte,
	offers *fixOffers,
	disabled disabledLines,
) []pendingFix {
	if len(g.sizes) == 0 {
		return fixes
	}
	members := make(map[int][]int, len(g.sizes))
	var order []int
	for idx := range diags {
		group := diags[idx].FixGroup
		if group == 0 {
			continue
		}
		if _, seen := members[group]; !seen {
			order = append(order, group)
		}
		members[group] = append(members[group], idx)
	}
	for _, group := range order {
		pieces, ok := g.pieces(diags, group, members[group])
		var combined fix.TextEdit
		if ok {
			combined, ok = fix.CombineEdits(content, pieceEdits(pieces))
		}
		written := []fix.TextEdit{combined}
		ok = ok && offers.offer(written, disabled)
		if ok && written[0].NewText != combined.NewText &&
			!writeMembers(diags, pieces, content, combined.NewText, written[0].NewText) {
			// The members' own edits cannot show the written edit, so each
			// reports all of it, which a consumer applies once.
			for _, idx := range members[group] {
				diags[idx].FixEdits = []fix.TextEdit{written[0]}
			}
		}
		if !ok {
			for _, idx := range members[group] {
				diags[idx].FixEdits = nil
				diags[idx].FixGroup = 0
			}
			continue
		}
		if slot, reserved := g.slots[group]; reserved {
			fixes[slot].edits = written
			fixes[slot].diagnostics = len(members[group])
		}
	}
	return slices.DeleteFunc(fixes, func(pending pendingFix) bool { return len(pending.edits) == 0 })
}

// groupPiece is one edit of a group's members and the members that carry
// it, as indexes into the rule's diagnostics.
type groupPiece struct {
	edit   fix.TextEdit
	owners []int
}

// pieces returns the distinct edits of a group's members in the order of
// fix.SortEdits, or false when a member of the group was dropped or has no
// fix. An edit that several members carry is one piece, made once, as a fix
// that several diagnostics share is applied once.
func (g *fixGroups) pieces(diags []Diagnostic, group int, members []int) ([]groupPiece, bool) {
	if len(members) != g.sizes[group] {
		return nil, false
	}
	var pieces []groupPiece
	index := make(map[fix.TextEdit]int)
	for _, idx := range members {
		if len(diags[idx].FixEdits) == 0 {
			return nil, false
		}
		for _, edit := range diags[idx].FixEdits {
			if at, seen := index[edit]; seen {
				pieces[at].owners = append(pieces[at].owners, idx)
				continue
			}
			index[edit] = len(pieces)
			pieces = append(pieces, groupPiece{edit: edit, owners: []int{idx}})
		}
	}
	slices.SortStableFunc(pieces, func(a, b groupPiece) int {
		if c := cmp.Compare(a.edit.StartOffset, b.edit.StartOffset); c != 0 {
			return c
		}
		return cmp.Compare(a.edit.EndOffset, b.edit.EndOffset)
	})
	return pieces, true
}

// pieceEdits returns the edits of pieces, in order.
func pieceEdits(pieces []groupPiece) []fix.TextEdit {
	edits := make([]fix.TextEdit, len(pieces))
	for idx, piece := range pieces {
		edits[idx] = piece.edit
	}
	return edits
}

// writeMembers rewrites the edits of a group's members after the engine
// wrote the text of the group's edit, raw, as written, which has other line
// endings in places. Writing changes only line endings, one for one, so
// each piece's text and each stretch of content between two pieces has its
// place in written. Each piece gets its written text, and a stretch whose line
// endings changed becomes one more edit, from its first changed byte to
// its last, of the member that carries the piece before it. Applied
// together, the members' edits then make written. It reports false when
// raw and written do not line up so, which leaves the group without a fix.
func writeMembers(diags []Diagnostic, pieces []groupPiece, content []byte, raw, written string) bool {
	// bounds holds where each piece's text starts and ends in raw, which is
	// checked to hold the pieces in this order.
	bounds := make([]int, 0, 2*len(pieces))
	var built strings.Builder
	for idx, piece := range pieces {
		if idx > 0 {
			built.Write(content[pieces[idx-1].edit.EndOffset:piece.edit.StartOffset])
		}
		bounds = append(bounds, built.Len())
		built.WriteString(piece.edit.NewText)
		bounds = append(bounds, built.Len())
	}
	if built.String() != raw {
		return false
	}
	at, ok := alignLineEndings(raw, written, bounds)
	if !ok {
		return false
	}

	edits := make(map[int][]fix.TextEdit)
	derived := len(pieces)
	for idx, piece := range pieces {
		edit := piece.edit
		edit.NewText = written[at[2*idx]:at[2*idx+1]]
		for _, owner := range piece.owners {
			edits[owner] = append(edits[owner], edit)
		}
		if idx+1 == len(pieces) {
			continue
		}
		start, end := edit.EndOffset, pieces[idx+1].edit.StartOffset
		if gap, changed := changedStretch(content[start:end], written[at[2*idx+1]:at[2*idx+2]], start); changed {
			edits[piece.owners[0]] = append(edits[piece.owners[0]], gap)
			derived++
		}
	}
	// Edits that writing made alike would be applied once by a consumer
	// that applies an edit several findings carry once.
	distinct := make(map[fix.TextEdit]bool)
	for _, ownerEdits := range edits {
		for _, edit := range ownerEdits {
			distinct[edit] = true
		}
	}
	if len(distinct) != derived {
		return false
	}
	for owner, ownerEdits := range edits {
		diags[owner].FixEdits = ownerEdits
	}
	return true
}

// alignLineEndings maps offsets of raw, given in ascending order, to the
// offsets in written where the same text falls. written is raw with some
// line endings ("\r\n", "\r" or "\n") written as other line endings. It
// reports false when the two differ otherwise, or an offset inside a line
// ending of raw falls in one that written changed.
func alignLineEndings(raw, written string, offsets []int) ([]int, bool) {
	at := make([]int, len(offsets))
	next := 0
	rawPos, writtenPos := 0, 0
	for {
		for next < len(offsets) && offsets[next] == rawPos {
			at[next] = writtenPos
			next++
		}
		if rawPos == len(raw) {
			break
		}
		rawEnding := lineEndingLength(raw, rawPos)
		if rawEnding == 0 {
			if writtenPos == len(written) || written[writtenPos] != raw[rawPos] {
				return nil, false
			}
			rawPos++
			writtenPos++
			continue
		}
		if writtenPos == len(written) {
			return nil, false
		}
		writtenEnding := lineEndingLength(written, writtenPos)
		if writtenEnding == 0 {
			return nil, false
		}
		for rawEnding == 2 && next < len(offsets) && offsets[next] == rawPos+1 {
			// An offset between "\r" and "\n" keeps its place only when the
			// line ending stays as it was.
			if raw[rawPos:rawPos+2] != written[writtenPos:writtenPos+writtenEnding] {
				return nil, false
			}
			at[next] = writtenPos + 1
			next++
		}
		rawPos += rawEnding
		writtenPos += writtenEnding
	}
	return at, next == len(offsets) && writtenPos == len(written)
}

// lineEndingLength returns the length of the line ending that starts at
// pos in text: 2 for "\r\n", 1 for "\r" or "\n", and 0 when none does.
func lineEndingLength(text string, pos int) int {
	switch {
	case text[pos] == '\n':
		return 1
	case text[pos] != '\r':
		return 0
	case pos+1 < len(text) && text[pos+1] == '\n':
		return 2
	default:
		return 1
	}
}

// changedStretch returns the edit that turns content, the file at offset,
// into written, from the first byte that differs to the last, or false
// when the two are the same. The edit never starts or ends between the
// "\r" and "\n" of a line ending.
func changedStretch(content []byte, written string, offset int) (fix.TextEdit, bool) {
	if string(content) == written {
		return fix.TextEdit{}, false
	}
	old := string(content)
	prefix := 0
	for prefix < len(old) && prefix < len(written) && old[prefix] == written[prefix] {
		prefix++
	}
	for prefix > 0 && (splitsLineEnding(old, prefix) || splitsLineEnding(written, prefix)) {
		prefix--
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(written)-prefix &&
		old[len(old)-1-suffix] == written[len(written)-1-suffix] {
		suffix++
	}
	for suffix > 0 && (splitsLineEnding(old, len(old)-suffix) || splitsLineEnding(written, len(written)-suffix)) {
		suffix--
	}
	return fix.TextEdit{
		StartOffset: offset + prefix,
		EndOffset:   offset + len(old) - suffix,
		NewText:     written[prefix : len(written)-suffix],
	}, true
}

// splitsLineEnding reports whether pos lies between the "\r" and "\n" of a
// line ending of text.
func splitsLineEnding(text string, pos int) bool {
	return pos > 0 && pos < len(text) && text[pos-1] == '\r' && text[pos] == '\n'
}
