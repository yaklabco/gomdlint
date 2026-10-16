package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// ListIndentRule checks for inconsistent indentation of list items at the same level.
type ListIndentRule struct {
	lint.BaseRule
}

// NewListIndentRule creates a new list-indent rule.
func NewListIndentRule() *ListIndentRule {
	return &ListIndentRule{
		BaseRule: lint.NewBaseRule(
			"MD005",
			"list-indent",
			"Inconsistent indentation for list items at the same level",
			[]string{"bullet", "indentation", "ul"},
			true,
		),
	}
}

// Apply checks for inconsistent list item indentation.
//
// Like markdownlint, every item of a list must start in the column of the
// first item's marker, and ordered lists may instead right-align their
// numbers so that the markers end in the same column. Columns count from the
// content of the enclosing blockquote. Unordered lists are reported without a
// fix, because MD007 moves their items to the configured indentation; ordered
// items are moved only when that keeps the structure.
func (r *ListIndentRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	plan := newListShiftPlan(ctx.File)
	var mismatches []indentMismatch
	for _, list := range ctx.Lists() {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}
		mismatches = append(mismatches, listIndentMismatches(ctx.File, plan, list)...)
	}
	safeShifts := plan.resolve()

	diags := make([]lint.Diagnostic, 0, len(mismatches))
	for _, mismatch := range mismatches {
		msg := fmt.Sprintf("List item indentation %d does not match expected %d", mismatch.actual, mismatch.expected)
		suggestion := fmt.Sprintf("Indent list item by %d spaces", mismatch.expected)
		diags = append(diags, indentDiagnostic(ctx, r.ID(), mismatch, safeShifts, msg, suggestion))
	}
	return diags, nil
}

// indentMismatch is a list item whose marker is not in the expected column.
// The columns are relative to the content of the enclosing blockquote, which
// starts at byte column base of the marker line.
type indentMismatch struct {
	item     *mdast.Node
	marker   listItemMarker
	base     int
	actual   int
	expected int
}

// relativeListMarker locates an item's marker and returns it with its column
// relative to the content of the enclosing blockquote and that content's
// byte column on the marker line.
func relativeListMarker(file *mdast.FileSnapshot, item *mdast.Node) (listItemMarker, int, int, bool) {
	marker, found := findListItemMarker(file, item)
	if !found {
		return listItemMarker{}, 0, 0, false
	}
	base, ok := blockquoteContentBase(file, item.Parent, marker.Line)
	if !ok {
		return listItemMarker{}, 0, 0, false
	}
	return marker, marker.Column() - base, base, true
}

// listIndentMismatches finds the misaligned items of one list and proposes
// moving ordered items to the expected column.
func listIndentMismatches(file *mdast.FileSnapshot, plan *listShiftPlan, list *mdast.Node) []indentMismatch {
	items := lint.ListItems(list)
	if len(items) < 2 {
		return nil
	}
	first, expectedIndent, _, found := relativeListMarker(file, items[0])
	if !found {
		return nil
	}
	ordered := lint.IsOrderedList(list)
	expectedEnd := expectedIndent + first.Width()
	endMatching := false

	var mismatches []indentMismatch
	for _, item := range items[1:] {
		marker, actual, base, found := relativeListMarker(file, item)
		if !found {
			continue
		}
		mismatch := indentMismatch{item: item, marker: marker, base: base, actual: actual, expected: expectedIndent}
		if !ordered {
			if actual != expectedIndent {
				mismatches = append(mismatches, mismatch)
			}
			continue
		}
		if actual == expectedIndent && !endMatching {
			continue
		}
		if actual+marker.Width() == expectedEnd {
			endMatching = true
			continue
		}
		if endMatching {
			mismatch.expected = expectedEnd - marker.Width()
		}
		mismatches = append(mismatches, mismatch)
		proposeIndentShift(plan, mismatch)
	}
	return mismatches
}

// proposeIndentShift proposes moving an item's marker, and with it its
// content, to the expected column.
func proposeIndentShift(plan *listShiftPlan, mismatch indentMismatch) {
	if mismatch.expected < 0 {
		return
	}
	layout, ok := plan.layout(mismatch.item)
	if !ok {
		return
	}
	plan.propose(mismatch.item, listShift{
		markerCol:  mismatch.expected,
		contentCol: layout.contentCol + mismatch.expected - layout.markerCol,
	})
}

// indentDiagnostic reports a misaligned marker. Like markdownlint, the range
// covers the line from its start through the marker. A fix rewrites the
// indentation between the blockquote content and the marker when the plan
// accepted the move.
func indentDiagnostic(
	ctx *lint.RuleContext,
	ruleID string,
	mismatch indentMismatch,
	safeShifts map[*mdast.Node]listShift,
	msg, suggestion string,
) lint.Diagnostic {
	marker := mismatch.marker
	pos := mdast.SourcePosition{
		StartLine:   marker.Line,
		StartColumn: 1,
		EndLine:     marker.Line,
		EndColumn:   marker.End - marker.LineStart + 1,
	}
	diagBuilder := lint.NewDiagnosticAt(ruleID, ctx.File.Path, pos, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(suggestion)
	if shift, ok := safeShifts[mismatch.item]; ok && shift.markerCol == mismatch.expected {
		builder := fix.NewEditBuilder()
		builder.ReplaceRange(marker.LineStart+mismatch.base, marker.End,
			strings.Repeat(" ", mismatch.expected)+string(ctx.File.Content[marker.Start:marker.End]))
		diagBuilder = diagBuilder.WithFix(builder)
	}
	return diagBuilder.Build()
}

// ULIndentRule checks unordered list indentation.
type ULIndentRule struct {
	lint.BaseRule
}

// NewULIndentRule creates a new ul-indent rule.
func NewULIndentRule() *ULIndentRule {
	return &ULIndentRule{
		BaseRule: lint.NewBaseRule(
			"MD007",
			"ul-indent",
			"Unordered list indentation",
			[]string{"bullet", "indentation", "ul"},
			true,
		),
	}
}

// Apply checks unordered list indentation.
//
// Like markdownlint, the rule covers unordered lists whose enclosing lists,
// up to the nearest blockquote, are all unordered; the nesting depth restarts
// inside each blockquote and columns count from the blockquote content. All
// moves are planned together, and only those that keep every item under the
// same parent are fixed.
func (r *ULIndentRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	indent := ctx.OptionInt("indent", 2)
	baseIndent := 0
	if ctx.OptionBool("start_indented", false) {
		baseIndent = ctx.OptionInt("start_indent", indent)
	}

	plan := newListShiftPlan(ctx.File)
	var mismatches []indentMismatch
	for _, list := range ctx.Lists() {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}
		if lint.IsOrderedList(list) {
			continue
		}
		nesting, unordered := unorderedListNesting(list)
		if !unordered {
			continue
		}
		expected := baseIndent + nesting*indent
		for _, item := range lint.ListItems(list) {
			marker, actual, base, found := relativeListMarker(ctx.File, item)
			if !found || actual == expected {
				continue
			}
			mismatch := indentMismatch{item: item, marker: marker, base: base, actual: actual, expected: expected}
			mismatches = append(mismatches, mismatch)
			proposeIndentShift(plan, mismatch)
		}
	}
	safeShifts := plan.resolve()

	sort.Slice(mismatches, func(i, j int) bool {
		return mismatches[i].marker.Start < mismatches[j].marker.Start
	})
	diags := make([]lint.Diagnostic, 0, len(mismatches))
	for _, mismatch := range mismatches {
		msg := fmt.Sprintf("Unordered list indentation %d does not match expected %d", mismatch.actual, mismatch.expected)
		suggestion := fmt.Sprintf("Indent list item by %d spaces", mismatch.expected)
		diags = append(diags, indentDiagnostic(ctx, r.ID(), mismatch, safeShifts, msg, suggestion))
	}
	return diags, nil
}

// unorderedListNesting counts the lists enclosing a list up to the nearest
// blockquote. It reports false when one of them is ordered, because MD007
// only covers lists whose parents are all unordered.
func unorderedListNesting(list *mdast.Node) (int, bool) {
	nesting := 0
	for parent := list.Parent; parent != nil; parent = parent.Parent {
		switch parent.Kind {
		case mdast.NodeBlockquote:
			return nesting, true
		case mdast.NodeList:
			if lint.IsOrderedList(parent) {
				return 0, false
			}
			nesting++
		default:
		}
	}
	return nesting, true
}

// ListMarkerSpaceRule checks for correct spaces after list markers.
type ListMarkerSpaceRule struct {
	lint.BaseRule
}

// NewListMarkerSpaceRule creates a new list-marker-space rule.
func NewListMarkerSpaceRule() *ListMarkerSpaceRule {
	return &ListMarkerSpaceRule{
		BaseRule: lint.NewBaseRule(
			"MD030",
			"list-marker-space",
			"Spaces after list markers",
			[]string{"ol", "ul", "whitespace"},
			true,
		),
	}
}

// spacingMismatch is a list item whose marker is followed by the wrong
// number of spaces.
type spacingMismatch struct {
	item     *mdast.Node
	marker   listItemMarker
	actual   int
	expected int
}

// Apply checks for correct spaces after list markers.
//
// Like markdownlint, a list uses the single-line setting when every item
// occupies exactly one line, and the multi-line setting otherwise. Markers are
// located through the AST, so lists inside blockquotes and lists nested on a
// shared line are checked too. The fix changes the item's content column, so
// it is only offered where that keeps the list structure.
func (r *ListMarkerSpaceRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	ulSingle := ctx.OptionInt("ul_single", 1)
	ulMulti := ctx.OptionInt("ul_multi", 1)
	olSingle := ctx.OptionInt("ol_single", 1)
	olMulti := ctx.OptionInt("ol_multi", 1)

	plan := newListShiftPlan(ctx.File)
	var mismatches []spacingMismatch
	for _, list := range ctx.Lists() {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}

		items := lint.ListItems(list)
		single := listItemsAreSingleLines(ctx.File, list, items)
		expected := ulMulti
		switch {
		case lint.IsOrderedList(list) && single:
			expected = olSingle
		case lint.IsOrderedList(list):
			expected = olMulti
		case single:
			expected = ulSingle
		}

		for _, item := range items {
			marker, found := findListItemMarker(ctx.File, item)
			if !found || !marker.HasContent() {
				continue
			}
			actual := marker.Spacing()
			if actual > maxListItemSpacing {
				// The content is indented code that starts one space after
				// the marker.
				actual = 1
			}
			if actual == expected {
				continue
			}
			mismatches = append(mismatches, spacingMismatch{item: item, marker: marker, actual: actual, expected: expected})
			if expected < 1 || expected > maxListItemSpacing || marker.Spacing() > maxListItemSpacing {
				continue
			}
			if layout, ok := plan.layout(item); ok {
				plan.propose(item, listShift{
					markerCol:  layout.markerCol,
					contentCol: layout.markerCol + marker.Width() + expected,
				})
			}
		}
	}
	safeShifts := plan.resolve()

	diags := make([]lint.Diagnostic, 0, len(mismatches))
	for _, mismatch := range mismatches {
		marker := mismatch.marker
		pos := mdast.SourcePosition{
			StartLine:   marker.Line,
			StartColumn: marker.End - marker.LineStart + 1,
			EndLine:     marker.Line,
			EndColumn:   marker.ContentStart - marker.LineStart + 1,
		}
		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("List marker space %d does not match expected %d", mismatch.actual, mismatch.expected)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Use %d space(s) after the list marker", mismatch.expected))
		if _, safe := safeShifts[mismatch.item]; safe {
			// The edit covers the marker too, so that no other list rule
			// changes this marker in the same pass.
			builder := fix.NewEditBuilder()
			builder.ReplaceRange(marker.Start, marker.ContentStart,
				string(ctx.File.Content[marker.Start:marker.End])+strings.Repeat(" ", mismatch.expected))
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return diags, nil
}

// listItemsAreSingleLines reports whether every item of a list occupies one
// line, which markdownlint checks by comparing the list's line count with its
// number of items.
func listItemsAreSingleLines(file *mdast.FileSnapshot, list *mdast.Node, items []*mdast.Node) bool {
	if len(items) == 0 {
		return true
	}
	firstLine := list.SourcePosition().StartLine
	if marker, found := findListItemMarker(file, items[0]); found {
		firstLine = marker.Line
	}
	return nodeLastLine(list)-firstLine+1 == len(items)
}

// BlanksAroundListsRule checks that lists are surrounded by blank lines.
type BlanksAroundListsRule struct {
	lint.BaseRule
}

// NewBlanksAroundListsRule creates a new blanks-around-lists rule.
func NewBlanksAroundListsRule() *BlanksAroundListsRule {
	return &BlanksAroundListsRule{
		BaseRule: lint.NewBaseRule(
			"MD032",
			"blanks-around-lists",
			"Lists should be surrounded by blank lines",
			[]string{"blank_lines", "bullet", "ol", "ul"},
			true,
		),
	}
}

// Apply checks that lists are surrounded by blank lines.
func (r *BlanksAroundListsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	// Find top-level lists only (not nested).
	for child := ctx.Root.FirstChild; child != nil; child = child.Next {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		if child.Kind != mdast.NodeList {
			continue
		}

		pos := child.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		// Check for blank line before.
		if diag := r.checkBlankBefore(ctx, child, pos); diag != nil {
			diags = append(diags, *diag)
		}

		// Check for blank line after.
		if diag := r.checkBlankAfter(ctx, child, pos); diag != nil {
			diags = append(diags, *diag)
		}
	}

	return diags, nil
}

// checkBlankBefore checks if there's a missing blank line before the list.
func (r *BlanksAroundListsRule) checkBlankBefore(
	ctx *lint.RuleContext,
	list *mdast.Node,
	pos mdast.SourcePosition,
) *lint.Diagnostic {
	if pos.StartLine <= 1 || lint.IsBlankLine(ctx.File, pos.StartLine-1) {
		return nil
	}

	// Check if previous sibling exists.
	if list.Prev == nil {
		return nil
	}
	// A preceding list reports this same boundary from its trailing edge.
	if list.Prev.Kind == mdast.NodeList {
		return nil
	}

	line := ctx.File.Lines[pos.StartLine-1]
	builder := fix.NewEditBuilder()
	builder.Insert(line.StartOffset, "\n")

	diagPos := mdast.SourcePosition{
		StartLine:   pos.StartLine,
		StartColumn: 1,
		EndLine:     pos.StartLine,
		EndColumn:   1,
	}

	diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, diagPos,
		"Missing blank line before list").
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Add a blank line before the list").
		WithFix(builder).
		Build()
	return &diag
}

// checkBlankAfter checks the boundary after the complete list, including continuations.
func (r *BlanksAroundListsRule) checkBlankAfter(
	ctx *lint.RuleContext,
	list *mdast.Node,
	pos mdast.SourcePosition,
) *lint.Diagnostic {
	return r.checkBlankAfterNormal(ctx, list, pos)
}

// checkBlankAfterNormal checks for missing blank line after list using the list's end position.
// We check the line immediately after the list ends (pos.EndLine + 1), mirroring
// how checkBlankBefore checks pos.StartLine - 1. This avoids false positives when
// the next sibling's SourcePosition excludes structural markers (e.g., fenced code
// blocks report content start, not the opening fence line).
func (r *BlanksAroundListsRule) checkBlankAfterNormal(
	ctx *lint.RuleContext,
	list *mdast.Node,
	pos mdast.SourcePosition,
) *lint.Diagnostic {
	if list.Next == nil {
		return nil
	}

	// Check the line immediately after the list ends.
	checkLine := pos.EndLine + 1
	if checkLine > len(ctx.File.Lines) || lint.IsBlankLine(ctx.File, checkLine) {
		return nil
	}

	// Find the diagnostic line - use the last list item's start line (the actual marker).
	diagLine := pos.EndLine
	if lastItem := list.LastChild; lastItem != nil {
		if itemPos := lastItem.SourcePosition(); itemPos.IsValid() {
			diagLine = itemPos.StartLine
		}
	}

	return r.createAfterDiagnostic(ctx, diagLine, pos.EndLine)
}

// createAfterDiagnostic creates a diagnostic for missing blank line after list.
func (r *BlanksAroundListsRule) createAfterDiagnostic(ctx *lint.RuleContext, diagLine, insertionLine int) *lint.Diagnostic {
	line := ctx.File.Lines[insertionLine-1]
	builder := fix.NewEditBuilder()
	builder.Insert(line.EndOffset, "\n")

	diagPos := mdast.SourcePosition{
		StartLine:   diagLine,
		StartColumn: 1,
		EndLine:     diagLine,
		EndColumn:   1,
	}

	diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, diagPos,
		"Missing blank line after list").
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Add a blank line after the list").
		WithFix(builder).
		Build()
	return &diag
}
