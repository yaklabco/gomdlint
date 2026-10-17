package rules

import (
	"bytes"
	"cmp"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// maxListItemSpacing is the widest whitespace after a list marker that still
// belongs to the item prefix; wider runs make the content an indented code
// block that starts one column after the marker.
const maxListItemSpacing = 4

// maxBlockIndent is the deepest indentation, relative to its container, at
// which a line can still start a list item or other block.
const maxBlockIndent = 3

// listItemLayout is the column layout of a list item on its marker line.
//
// Columns are relative to the content of the nearest enclosing blockquote
// (or to the start of the line outside blockquotes), so that items on lines
// with different blockquote markers compare correctly. They are byte
// columns, which equal visual columns because a layout is only built when
// the marker prefix holds no tabs.
type listItemLayout struct {
	marker     listItemMarker
	base       int
	markerCol  int
	contentCol int
}

// listShift is a change of the marker and content columns of a list item,
// relative to its blockquote content like listItemLayout.
type listShift struct {
	markerCol  int
	contentCol int
}

// listShiftPlan collects the column changes one rule wants to make to list
// items in a single pass, keeps only those that preserve the document
// structure, and turns them into fixes that stay safe whichever of them are
// applied.
type listShiftPlan struct {
	file    *mdast.FileSnapshot
	layouts map[*mdast.Node]listItemLayout
	missing map[*mdast.Node]bool
	shifts  map[*mdast.Node]listShift

	// independent is true when every accepted shift must be safe on its own,
	// whatever happens to the others. It is set by resolve.
	independent bool
}

func newListShiftPlan(file *mdast.FileSnapshot) *listShiftPlan {
	return &listShiftPlan{
		file:    file,
		layouts: make(map[*mdast.Node]listItemLayout),
		missing: make(map[*mdast.Node]bool),
		shifts:  make(map[*mdast.Node]listShift),
	}
}

// layout returns the column layout of a list item, or false when its marker
// cannot be located or its prefix holds tabs.
func (p *listShiftPlan) layout(item *mdast.Node) (listItemLayout, bool) {
	if layout, ok := p.layouts[item]; ok {
		return layout, true
	}
	if p.missing[item] {
		return listItemLayout{}, false
	}
	layout, ok := computeListItemLayout(p.file, item)
	if !ok {
		p.missing[item] = true
		return listItemLayout{}, false
	}
	p.layouts[item] = layout
	return layout, true
}

// propose records that the rule wants to move an item's marker and content to
// the given columns.
func (p *listShiftPlan) propose(item *mdast.Node, shift listShift) {
	p.shifts[item] = shift
}

// final returns an item's marker and content columns after the accepted
// shifts are applied.
func (p *listShiftPlan) final(item *mdast.Node) (listShift, bool) {
	if shift, ok := p.shifts[item]; ok {
		return shift, true
	}
	return p.current(item)
}

// current returns an item's marker and content columns before any shift.
func (p *listShiftPlan) current(item *mdast.Node) (listShift, bool) {
	layout, ok := p.layout(item)
	if !ok {
		return listShift{}, false
	}
	return listShift{markerCol: layout.markerCol, contentCol: layout.contentCol}, true
}

// resolve drops every proposed shift that would change the document structure
// and returns the accepted shifts. Shifts are checked in document order,
// repeatedly, because dropping one shift changes the final columns the
// others are checked against.
//
// Every shift is checked against the final columns of the other items, and
// fixes then apply the shifts of related items together. When a directive
// can suppress some of the rule's diagnostics but not others, a shift must
// also be safe while every other item keeps its current columns, because the
// shifts it was checked with may never be applied.
func (p *listShiftPlan) resolve() map[*mdast.Node]listShift {
	p.independent = hasLineDirectives(p.file)
	for changed := true; changed; {
		changed = false
		for _, item := range p.orderedShiftItems() {
			shift := p.shifts[item]
			if !p.keepsStructure(item, shift, p.final) ||
				(p.independent && !p.keepsStructure(item, shift, p.current)) {
				delete(p.shifts, item)
				changed = true
			}
		}
	}
	return p.shifts
}

// fixes turns the edits that make the accepted shifts, one per item, into
// the edits the diagnostics carry. Any fix may be dropped while others are
// applied: a directive can suppress a diagnostic, and a conflict with another
// rule's edit can skip a fix. So shifts that are only safe together form a
// group whose changes are made by a single edit, on the diagnostic of the
// group's first item, and the other items of the group get no fix. Shifts
// that are safe on their own keep one fix each. A group with an item that has
// no edit gets no fix at all.
func (p *listShiftPlan) fixes(edits map[*mdast.Node]fix.TextEdit) map[*mdast.Node]fix.TextEdit {
	result := make(map[*mdast.Node]fix.TextEdit)
	for _, group := range p.groups() {
		groupEdits := make([]fix.TextEdit, 0, len(group))
		for _, item := range group {
			if edit, ok := edits[item]; ok {
				groupEdits = append(groupEdits, edit)
			}
		}
		if len(groupEdits) != len(group) {
			continue
		}
		if combined, ok := combineEdits(p.file.Content, groupEdits); ok {
			result[group[0]] = combined
		}
	}
	return result
}

// groups partitions the items with accepted shifts into groups that must be
// applied together, each in document order. An item joins the group of a
// related item, one whose columns its check reads, when its shift is only
// safe if that item moves too. Every check reads one other item at a time,
// so items in different groups are safe whether or not the other group is
// applied: each is safe with the other at its current columns and, as
// resolve checked, at its final ones. When shifts are independent every item
// is a group of its own.
func (p *listShiftPlan) groups() [][]*mdast.Node {
	items := p.orderedShiftItems()
	order := make(map[*mdast.Node]int, len(items))
	leader := make(map[*mdast.Node]*mdast.Node, len(items))
	for idx, item := range items {
		order[item] = idx
		leader[item] = item
	}
	find := func(item *mdast.Node) *mdast.Node {
		for leader[item] != item {
			leader[item] = leader[leader[item]]
			item = leader[item]
		}
		return item
	}
	if !p.independent {
		for _, item := range items {
			for _, other := range checkedItems(item) {
				if _, ok := leader[other]; !ok || p.keepsStructure(item, p.shifts[item], p.finalExcept(other)) {
					continue
				}
				first, second := find(item), find(other)
				if order[second] < order[first] {
					first, second = second, first
				}
				leader[second] = first
			}
		}
	}
	var groups [][]*mdast.Node
	index := make(map[*mdast.Node]int)
	for _, item := range items {
		root := find(item)
		idx, ok := index[root]
		if !ok {
			idx = len(groups)
			index[root] = idx
			groups = append(groups, nil)
		}
		groups[idx] = append(groups[idx], item)
	}
	return groups
}

// finalExcept returns the final columns of every item but kept, which keeps
// its current columns.
func (p *listShiftPlan) finalExcept(kept *mdast.Node) func(*mdast.Node) (listShift, bool) {
	return func(item *mdast.Node) (listShift, bool) {
		if item == kept {
			return p.current(item)
		}
		return p.final(item)
	}
}

// checkedItems returns the items whose columns the structure check of item
// reads: its parent item, its neighbours in its list, the items of its
// nested lists and, for a first item, the last item of a list right before
// its own.
func checkedItems(item *mdast.Node) []*mdast.Node {
	var checked []*mdast.Node
	list := item.Parent
	if parent := list.Parent; parent != nil && parent.Kind == mdast.NodeListItem {
		checked = append(checked, parent)
	}
	if prev := previousListItem(item); prev != nil {
		checked = append(checked, prev)
	} else if before := list.Prev; before != nil && before.Kind == mdast.NodeList {
		if last := lastListItem(before); last != nil {
			checked = append(checked, last)
		}
	}
	if next := nextListItem(item); next != nil {
		checked = append(checked, next)
	}
	for child := item.FirstChild; child != nil; child = child.Next {
		if child.Kind == mdast.NodeList {
			checked = append(checked, lint.ListItems(child)...)
		}
	}
	return checked
}

// combineEdits merges edits that do not overlap into one edit that also
// rewrites the text between them unchanged, so that they are applied
// together or not at all.
func combineEdits(content []byte, edits []fix.TextEdit) (fix.TextEdit, bool) {
	if len(edits) == 0 {
		return fix.TextEdit{}, false
	}
	sorted := slices.Clone(edits)
	slices.SortFunc(sorted, func(a, b fix.TextEdit) int { return cmp.Compare(a.StartOffset, b.StartOffset) })
	var text strings.Builder
	for idx, edit := range sorted {
		if idx > 0 {
			prev := sorted[idx-1]
			if edit.StartOffset < prev.EndOffset {
				return fix.TextEdit{}, false
			}
			text.Write(content[prev.EndOffset:edit.StartOffset])
		}
		text.WriteString(edit.NewText)
	}
	return fix.TextEdit{
		StartOffset: sorted[0].StartOffset,
		EndOffset:   sorted[len(sorted)-1].EndOffset,
		NewText:     text.String(),
	}, true
}

// lineDirectivePattern matches the start of an inline directive that turns
// rules on or off for part of a file.
var lineDirectivePattern = regexp.MustCompile(`(?i)<!--\s*markdownlint-(?:disable|enable|capture|restore)`)

// hasLineDirectives reports whether a file holds an inline directive that
// can turn rules on or off for some lines only, so that some diagnostics of
// a rule may be suppressed while others are kept. It looks where the engine
// looks for directives, in HTML blocks and inline HTML.
func hasLineDirectives(file *mdast.FileSnapshot) bool {
	return mdast.FindFirst(file.Root, func(node *mdast.Node) bool {
		return (node.Kind == mdast.NodeHTMLBlock || node.Kind == mdast.NodeHTMLInline) &&
			lineDirectivePattern.Match(node.Text())
	}) != nil
}

func (p *listShiftPlan) orderedShiftItems() []*mdast.Node {
	items := make([]*mdast.Node, 0, len(p.shifts))
	for item := range p.shifts {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].SourceRange().StartOffset < items[j].SourceRange().StartOffset
	})
	return items
}

// keepsStructure reports whether moving an item's marker and content to the
// shifted columns, with every other item at the columns that columns
// returns, leaves the item in the same list at the same depth and keeps
// every surrounding line where it is. Only items whose content is a
// paragraph on the marker line followed by nested lists are moved; anything
// else may depend on the content column in ways this check does not model.
// A paragraph line that looks like the start of a block is paragraph text
// only because of where it sits, so an item holding one keeps its content
// column.
func (p *listShiftPlan) keepsStructure(
	item *mdast.Node,
	shift listShift,
	columns func(*mdast.Node) (listShift, bool),
) bool {
	layout, ok := p.layout(item)
	if !ok || !isSimpleListItem(item, layout.marker.Line) {
		return false
	}
	if shift.markerCol != layout.markerCol && !markerIndentEditable(p.file, layout) {
		return false
	}
	if shift.markerCol < 0 || shift.contentCol <= shift.markerCol {
		return false
	}
	if shift.contentCol != layout.contentCol && hasDormantBlockStart(p.file, item.FirstChild, layout.marker.Line) {
		return false
	}

	list := item.Parent
	if parent := list.Parent; parent != nil && parent.Kind == mdast.NodeListItem {
		parentShift, ok := columns(parent)
		if !ok || !sameBlockquote(parent, item) ||
			shift.markerCol < parentShift.contentCol || shift.markerCol > parentShift.contentCol+maxBlockIndent {
			return false
		}
	} else if shift.markerCol > maxBlockIndent {
		return false
	}

	if prev := previousListItem(item); prev != nil {
		prevShift, ok := columns(prev)
		if !ok || shift.markerCol >= prevShift.contentCol {
			return false
		}
	} else if before := list.Prev; before != nil && before.Kind == mdast.NodeList {
		// The first item must not move under the last item of the list
		// before it, which would take it in.
		lastShift, ok := columns(lastListItem(before))
		if !ok || shift.markerCol >= lastShift.contentCol {
			return false
		}
	}
	next := nextListItem(item)
	if next != nil {
		nextShift, ok := columns(next)
		if !ok || nextShift.markerCol >= shift.contentCol {
			return false
		}
	}

	for child := item.FirstChild; child != nil; child = child.Next {
		if child.Kind != mdast.NodeList {
			continue
		}
		for _, nested := range lint.ListItems(child) {
			nestedShift, ok := columns(nested)
			if !ok || nestedShift.markerCol < shift.contentCol || nestedShift.markerCol > shift.contentCol+maxBlockIndent {
				return false
			}
		}
	}

	if next == nil {
		if col, found := p.firstColumnAfter(list, nodeLastLine(item)); found && col >= shift.contentCol {
			return false
		}
	}
	return true
}

// firstColumnAfter returns the column, relative to the blockquote content of
// list, of the first non-blank line after the given line. Lines outside that
// blockquote, such as lazy continuation lines, cannot join the list and are
// not reported. A line whose indentation holds a tab reports an unreachable
// column, so that callers treat it as unsafe.
func (p *listShiftPlan) firstColumnAfter(list *mdast.Node, lineNum int) (int, bool) {
	for next := lineNum + 1; next <= len(p.file.Lines); next++ {
		if lint.IsBlankLineInContainer(p.file, list, next) {
			continue
		}
		base, ok := blockquoteContentBase(p.file, list, next)
		if !ok {
			return 0, false
		}
		content := lint.RawLineContent(p.file, next)
		indentEnd := len(content) - len(bytes.TrimLeft(content[base:], " \t"))
		if bytes.IndexByte(content[:indentEnd], '\t') >= 0 {
			return math.MaxInt, true
		}
		return indentEnd - base, true
	}
	return 0, false
}

// computeListItemLayout measures an item's marker and content columns.
func computeListItemLayout(file *mdast.FileSnapshot, item *mdast.Node) (listItemLayout, bool) {
	marker, ok := findListItemMarker(file, item)
	if !ok {
		return listItemLayout{}, false
	}
	base, ok := blockquoteContentBase(file, item.Parent, marker.Line)
	// A tab anywhere before the content, even inside the blockquote prefix,
	// expands to a tab stop, so byte columns would not match the columns
	// CommonMark uses.
	if !ok || base > marker.Column() || bytes.IndexByte(file.Content[marker.LineStart:marker.ContentStart], '\t') >= 0 {
		return listItemLayout{}, false
	}
	spacing := marker.Spacing()
	if !marker.HasContent() || spacing > maxListItemSpacing {
		spacing = 1
	}
	markerCol := marker.Column() - base
	return listItemLayout{
		marker:     marker,
		base:       base,
		markerCol:  markerCol,
		contentCol: markerCol + marker.Width() + spacing,
	}, true
}

// blockquoteContentBase returns the byte column where the content of the
// nearest blockquote enclosing node starts on a line: the length of the
// container prefix up to and including that blockquote's marker. It is 0
// when node is not inside a blockquote, and false when the line does not
// continue the blockquote.
func blockquoteContentBase(file *mdast.FileSnapshot, node *mdast.Node, lineNum int) (int, bool) {
	child := node
	depth := 0
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind != mdast.NodeBlockquote {
			if depth == 0 {
				child = parent
			}
			continue
		}
		depth++
	}
	if depth == 0 {
		return 0, true
	}
	prefix := lint.ContainerPrefix(file, child, lineNum)
	if strings.Count(string(prefix), ">") < depth {
		return 0, false
	}
	return len(prefix), true
}

// sameBlockquote reports whether two nodes have the same nearest enclosing
// blockquote, so that their relative columns are comparable.
func sameBlockquote(first, second *mdast.Node) bool {
	return nearestBlockquote(first) == nearestBlockquote(second)
}

func nearestBlockquote(node *mdast.Node) *mdast.Node {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeBlockquote {
			return parent
		}
	}
	return nil
}

// isSimpleListItem reports whether an item holds a paragraph that starts on
// the marker line, optionally followed by nested lists and nothing else.
func isSimpleListItem(item *mdast.Node, markerLine int) bool {
	first := item.FirstChild
	if first == nil || first.Kind != mdast.NodeParagraph || first.SourcePosition().StartLine != markerLine {
		return false
	}
	for child := first.Next; child != nil; child = child.Next {
		if child.Kind != mdast.NodeList {
			return false
		}
	}
	return true
}

// markerIndentEditable reports whether the whitespace between the blockquote
// content and a marker is plain indentation, so that changing it moves only
// this marker.
func markerIndentEditable(file *mdast.FileSnapshot, layout listItemLayout) bool {
	for offset := layout.marker.LineStart + layout.base; offset < layout.marker.Start; offset++ {
		if file.Content[offset] != ' ' {
			return false
		}
	}
	return true
}

// previousListItem returns the list item before item in its list.
func previousListItem(item *mdast.Node) *mdast.Node {
	for sibling := item.Prev; sibling != nil; sibling = sibling.Prev {
		if sibling.Kind == mdast.NodeListItem {
			return sibling
		}
	}
	return nil
}

// lastListItem returns the last item of a list, or nil.
func lastListItem(list *mdast.Node) *mdast.Node {
	for child := list.LastChild; child != nil; child = child.Prev {
		if child.Kind == mdast.NodeListItem {
			return child
		}
	}
	return nil
}

// hasDormantBlockStart reports whether a paragraph line after the first, on
// the lines that follow startLine, begins like a block (a list item, heading,
// quote, fence, thematic break or HTML block) once its container prefix and
// indentation are removed.
func hasDormantBlockStart(file *mdast.FileSnapshot, paragraph *mdast.Node, startLine int) bool {
	for lineNum := startLine + 1; lineNum <= nodeLastLine(paragraph); lineNum++ {
		content := lint.RawLineContent(file, lineNum)
		rest := bytes.TrimLeft(content[len(lint.ContainerPrefix(file, paragraph, lineNum)):], " \t")
		if len(rest) > 0 && wrapContinuationStartsBlock(string(rest)) {
			return true
		}
	}
	return false
}

// nextListItem returns the list item after item in its list.
func nextListItem(item *mdast.Node) *mdast.Node {
	for sibling := item.Next; sibling != nil; sibling = sibling.Next {
		if sibling.Kind == mdast.NodeListItem {
			return sibling
		}
	}
	return nil
}

// nodeLastLine returns the last line a node's span covers, treating a span
// that ends at the start of a line as ending on the line before.
func nodeLastLine(node *mdast.Node) int {
	pos := node.SourcePosition()
	if pos.EndColumn == 1 && pos.EndLine > pos.StartLine {
		return pos.EndLine - 1
	}
	return pos.EndLine
}
