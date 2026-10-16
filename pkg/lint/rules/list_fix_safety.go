package rules

import (
	"bytes"
	"sort"

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
// Columns are 0-based byte columns, which equal visual columns because the
// layout is only built when the marker prefix holds no tabs.
type listItemLayout struct {
	marker     listItemMarker
	markerCol  int
	contentCol int
}

// listShift is a change of the marker and content columns of a list item.
type listShift struct {
	markerCol  int
	contentCol int
}

// listShiftPlan collects the column changes one rule wants to make to list
// items in a single pass, and keeps only those that preserve the document
// structure when they are applied together.
type listShiftPlan struct {
	file    *mdast.FileSnapshot
	layouts map[*mdast.Node]listItemLayout
	missing map[*mdast.Node]bool
	shifts  map[*mdast.Node]listShift
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
	layout, ok := p.layout(item)
	if !ok {
		return listShift{}, false
	}
	return listShift{markerCol: layout.markerCol, contentCol: layout.contentCol}, true
}

// resolve drops every proposed shift that would change the document structure
// and returns the shifts that are safe to apply together. Shifts are checked
// in document order, repeatedly, because dropping one shift changes the
// final columns the others are checked against.
func (p *listShiftPlan) resolve() map[*mdast.Node]listShift {
	for changed := true; changed; {
		changed = false
		for _, item := range p.orderedShiftItems() {
			if !p.keepsStructure(item, p.shifts[item]) {
				delete(p.shifts, item)
				changed = true
			}
		}
	}
	return p.shifts
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
// shifted columns, together with the other accepted shifts, leaves the item
// in the same list at the same depth and keeps every surrounding line where
// it is. Only items whose content is a paragraph on the marker line followed
// by nested lists are moved; anything else may depend on the content column
// in ways this check does not model.
func (p *listShiftPlan) keepsStructure(item *mdast.Node, shift listShift) bool {
	layout, ok := p.layout(item)
	if !ok || !isSimpleListItem(item, layout.marker.Line) || insideBlockquote(item) {
		return false
	}
	if shift.markerCol != layout.markerCol && !markerIndentEditable(p.file, layout.marker) {
		return false
	}
	if shift.contentCol <= shift.markerCol {
		return false
	}

	list := item.Parent
	if parent := list.Parent; parent != nil && parent.Kind == mdast.NodeListItem {
		parentShift, ok := p.final(parent)
		if !ok || shift.markerCol < parentShift.contentCol || shift.markerCol > parentShift.contentCol+maxBlockIndent {
			return false
		}
	} else if shift.markerCol > maxBlockIndent {
		return false
	}

	if prev := previousListItem(item); prev != nil {
		prevShift, ok := p.final(prev)
		if !ok || shift.markerCol >= prevShift.contentCol {
			return false
		}
	}
	next := nextListItem(item)
	if next != nil {
		nextShift, ok := p.final(next)
		if !ok || nextShift.markerCol >= shift.contentCol {
			return false
		}
	}

	for child := item.FirstChild; child != nil; child = child.Next {
		if child.Kind != mdast.NodeList {
			continue
		}
		for _, nested := range lint.ListItems(child) {
			nestedShift, ok := p.final(nested)
			if !ok || nestedShift.markerCol < shift.contentCol || nestedShift.markerCol > shift.contentCol+maxBlockIndent {
				return false
			}
		}
	}

	if next == nil {
		if col, found := firstColumnAfter(p.file, nodeLastLine(item)); found && col >= shift.contentCol {
			return false
		}
	}
	return true
}

// computeListItemLayout measures an item's marker and content columns.
func computeListItemLayout(file *mdast.FileSnapshot, item *mdast.Node) (listItemLayout, bool) {
	marker, ok := findListItemMarker(file, item)
	if !ok {
		return listItemLayout{}, false
	}
	if bytes.IndexByte(file.Content[marker.LineStart:marker.ContentStart], '\t') >= 0 {
		return listItemLayout{}, false
	}
	spacing := marker.Spacing()
	if !marker.HasContent() || spacing > maxListItemSpacing {
		spacing = 1
	}
	return listItemLayout{
		marker:     marker,
		markerCol:  marker.Column(),
		contentCol: marker.Column() + marker.Width() + spacing,
	}, true
}

// isSimpleListItem reports whether an item holds a paragraph that starts on
// the marker line, optionally followed by nested lists and nothing else.
func isSimpleListItem(item *mdast.Node, markerLine int) bool {
	first := item.FirstChild
	if !isParagraphLike(first) || first.SourcePosition().StartLine != markerLine {
		return false
	}
	for child := first.Next; child != nil; child = child.Next {
		if child.Kind != mdast.NodeList {
			return false
		}
	}
	return true
}

// insideBlockquote reports whether a node has a blockquote ancestor.
func insideBlockquote(node *mdast.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeBlockquote {
			return true
		}
	}
	return false
}

// markerIndentEditable reports whether the whitespace before a marker is
// plain indentation from the start of the line, so that changing it moves
// only this marker.
func markerIndentEditable(file *mdast.FileSnapshot, marker listItemMarker) bool {
	for offset := marker.LineStart; offset < marker.Start; offset++ {
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

// firstColumnAfter returns the column of the first non-whitespace byte on the
// first non-blank line after the given line.
func firstColumnAfter(file *mdast.FileSnapshot, lineNum int) (int, bool) {
	for next := lineNum + 1; next <= len(file.Lines); next++ {
		content := lint.LineContent(file, next)
		col := 0
		for _, char := range content {
			switch char {
			case ' ':
				col++
			case '\t':
				col += 4 - col%4
			default:
				return col, true
			}
		}
	}
	return 0, false
}
