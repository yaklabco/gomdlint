package lint

import (
	"slices"

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
// which is offered like any other fix and takes the place that reserve
// kept, fixing each member. Each member keeps its own edits, written with
// the file's line endings. Otherwise every member loses its fix and its
// group, and the group's place among fixes is dropped.
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
		edits, ok := g.combine(diags, group, members[group], content)
		for _, idx := range members[group] {
			ok = ok && offers.endings.WriteNewlines(diags[idx].FixEdits)
		}
		ok = ok && offers.offer(edits, disabled)
		if !ok {
			for _, idx := range members[group] {
				diags[idx].FixEdits = nil
				diags[idx].FixGroup = 0
			}
			continue
		}
		if slot, reserved := g.slots[group]; reserved {
			fixes[slot].edits = edits
			fixes[slot].diagnostics = len(members[group])
		}
	}
	return slices.DeleteFunc(fixes, func(pending pendingFix) bool { return len(pending.edits) == 0 })
}

// combine returns the one edit that makes the fixes of a group's members,
// or false when a member of the group was dropped or has no fix, or two of
// their edits overlap. An edit that several members carry is made once, as
// a fix that several diagnostics share is applied once.
func (g *fixGroups) combine(diags []Diagnostic, group int, members []int, content []byte) ([]fix.TextEdit, bool) {
	if len(members) != g.sizes[group] {
		return nil, false
	}
	var edits []fix.TextEdit
	seen := make(map[fix.TextEdit]bool)
	for _, idx := range members {
		if len(diags[idx].FixEdits) == 0 {
			return nil, false
		}
		for _, edit := range diags[idx].FixEdits {
			if !seen[edit] {
				seen[edit] = true
				edits = append(edits, edit)
			}
		}
	}
	combined, ok := fix.CombineEdits(content, edits)
	if !ok {
		return nil, false
	}
	return []fix.TextEdit{combined}, true
}
