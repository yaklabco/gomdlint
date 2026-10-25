package rules

import (
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// listFixes holds the fixes of one rule's list items, one edit per item.
// Items that are only safe together form a group, such as the bullets of a
// list, where a different bullet starts a new list. While the rule checks
// its fixes, the diagnostic of a group's first item carries one edit that
// makes the whole group, so the group is checked once, as the engine will
// apply it. Then share gives every item of a group that passed its own edit
// and the group's lint.Diagnostic.FixGroup, so each finding the group fixes
// shows its fix, and the engine applies the group as that one edit.
type listFixes struct {
	content []byte
	edits   map[*mdast.Node]fix.TextEdit
	groups  map[*mdast.Node]*listFixGroup
	count   int
}

// listFixGroup is a group of two or more items whose edits are only safe
// together.
type listFixGroup struct {
	id       int
	leader   *mdast.Node
	combined fix.TextEdit
}

func newListFixes(content []byte) *listFixes {
	return &listFixes{
		content: content,
		edits:   make(map[*mdast.Node]fix.TextEdit),
		groups:  make(map[*mdast.Node]*listFixGroup),
	}
}

// add records the edits of items, one each, that are applied together or
// not at all. items[0] leads the group. Items whose edits overlap get no
// fix.
func (f *listFixes) add(items []*mdast.Node, edits []fix.TextEdit) {
	if len(items) == 0 || len(items) != len(edits) {
		return
	}
	if len(items) == 1 {
		f.edits[items[0]] = edits[0]
		return
	}
	combined, ok := fix.CombineEdits(f.content, edits)
	if !ok {
		return
	}
	f.count++
	group := &listFixGroup{id: f.count, leader: items[0], combined: combined}
	for idx, item := range items {
		f.edits[item] = edits[idx]
		f.groups[item] = group
	}
}

// checked returns the edit that the diagnostic of item carries while the
// rule checks its fixes: the item's own edit, or for the first item of a
// group, the edit that makes the group. The other items of a group carry
// none until share gives them theirs.
func (f *listFixes) checked(item *mdast.Node) (fix.TextEdit, bool) {
	if group, ok := f.groups[item]; ok {
		return group.combined, item == group.leader
	}
	edit, ok := f.edits[item]
	return edit, ok
}

// share gives each diagnostic of a group its own edit and the group's
// number, once keepSafeFixes has checked the group's edit on the diagnostic
// of its first item. A group whose edit was withheld keeps no fix. items[i]
// is the item that diags[i] reports, or nil.
func (f *listFixes) share(diags []lint.Diagnostic, items []*mdast.Node) {
	if len(f.groups) == 0 {
		return
	}
	kept := make(map[*listFixGroup]bool)
	for idx, item := range items {
		if group, ok := f.groups[item]; ok && item == group.leader && diags[idx].HasFix() {
			kept[group] = true
		}
	}
	for idx, item := range items {
		group, ok := f.groups[item]
		if !ok {
			continue
		}
		diags[idx].FixEdits = nil
		if kept[group] {
			diags[idx].FixEdits = []fix.TextEdit{f.edits[item]}
			diags[idx].FixGroup = group.id
		}
	}
}
