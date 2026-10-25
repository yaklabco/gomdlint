package lint

import (
	"slices"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

// fixGroups gathers the fix groups of one rule's diagnostics in a file (see
// Diagnostic.FixGroup) and makes each group one fix.
type fixGroups struct {
	// sizes counts the diagnostics the rule reported in each group.
	sizes map[int]int
	// slots holds, for each group, the index among the pending fixes that
	// its fix takes: the place of its first member with a valid fix.
	slots map[int]int
}

// newFixGroups counts the members of each fix group among the diagnostics a
// rule reported, before any is dropped.
func newFixGroups(diags []Diagnostic) *fixGroups {
	groups := &fixGroups{sizes: make(map[int]int), slots: make(map[int]int)}
	for _, diag := range diags {
		if diag.FixGroup != 0 {
			groups.sizes[diag.FixGroup]++
		}
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
// that the engine keeps, whose fixes are valid. A group is fixed when every
// diagnostic the rule reported in it is kept with a fix, and their edits do
// not overlap: they become one edit that also writes the text between them
// back unchanged, which is offered like any other fix and takes the place
// that reserve kept, fixing each member. Each member keeps its own edits,
// written with the file's line endings. Otherwise every member loses its fix
// and the group's place among fixes is dropped.
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
// their edits overlap.
func (g *fixGroups) combine(diags []Diagnostic, group int, members []int, content []byte) ([]fix.TextEdit, bool) {
	if len(members) != g.sizes[group] {
		return nil, false
	}
	var edits []fix.TextEdit
	for _, idx := range members {
		if len(diags[idx].FixEdits) == 0 {
			return nil, false
		}
		edits = append(edits, diags[idx].FixEdits...)
	}
	combined, ok := fix.CombineEdits(content, edits)
	if !ok {
		return nil, false
	}
	return []fix.TextEdit{combined}, true
}
