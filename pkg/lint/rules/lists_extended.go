package rules

import (
	"bytes"
	"fmt"
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
func (r *ListIndentRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	// Track indentation levels within each list.
	lists := ctx.Lists()
	for _, list := range lists {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		listDiags := r.checkList(ctx, list)
		diags = append(diags, listDiags...)
	}

	return diags, nil
}

func (r *ListIndentRule) checkList(ctx *lint.RuleContext, list *mdast.Node) []lint.Diagnostic {
	var diags []lint.Diagnostic
	items := lint.ListItems(list)
	if len(items) < 2 {
		return diags
	}

	// Get the indentation of the first item to use as reference.
	firstItem := items[0]
	firstPos := firstItem.SourcePosition()
	if !firstPos.IsValid() {
		return diags
	}

	referenceIndent := r.getItemIndent(ctx.File, firstPos.StartLine)

	// Check remaining items.
	for i := 1; i < len(items); i++ {
		item := items[i]
		pos := item.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		indent := r.getItemIndent(ctx.File, pos.StartLine)
		if indent != referenceIndent {
			line := ctx.File.Lines[pos.StartLine-1]

			// Build fix.
			builder := fix.NewEditBuilder()
			lineContent := lint.LineContent(ctx.File, pos.StartLine)
			trimmed := bytes.TrimLeft(lineContent, " \t")

			newLine := strings.Repeat(" ", referenceIndent) + string(trimmed)
			builder.ReplaceRange(line.StartOffset, line.NewlineStart, newLine)

			diag := lint.NewDiagnostic(r.ID(), item,
				fmt.Sprintf("List item indentation %d does not match expected %d", indent, referenceIndent)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Indent list item by %d spaces", referenceIndent)).
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags
}

func (r *ListIndentRule) getItemIndent(file *mdast.FileSnapshot, lineNum int) int {
	content := lint.LineText(file, lineNum)
	indent := 0
	for _, ch := range content {
		switch ch {
		case ' ':
			indent++
		case '\t':
			indent += 4 // Count tab as 4 spaces.
		default:
			return indent
		}
	}
	return indent
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
func (r *ULIndentRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	indent := ctx.OptionInt("indent", 2)
	startIndented := ctx.OptionBool("start_indented", false)
	startIndent := ctx.OptionInt("start_indent", indent)

	var diags []lint.Diagnostic

	// Only process top-level lists (direct children of document).
	for child := ctx.Root.FirstChild; child != nil; child = child.Next {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		if child.Kind != mdast.NodeList {
			continue
		}

		// Skip ordered lists.
		if lint.IsOrderedList(child) {
			continue
		}

		listDiags := r.checkULIndent(ctx, child, 0, indent, startIndented, startIndent)
		diags = append(diags, listDiags...)
	}

	return diags, nil
}

func (r *ULIndentRule) checkULIndent(
	ctx *lint.RuleContext,
	list *mdast.Node,
	depth int,
	indent int,
	startIndented bool,
	startIndent int,
) []lint.Diagnostic {
	var diags []lint.Diagnostic

	// Calculate expected indentation.
	var expectedIndent int
	if depth == 0 {
		if startIndented {
			expectedIndent = startIndent
		} else {
			expectedIndent = 0
		}
	} else {
		if startIndented {
			expectedIndent = startIndent + (depth * indent)
		} else {
			expectedIndent = depth * indent
		}
	}

	items := lint.ListItems(list)
	for _, item := range items {
		pos := item.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		actualIndent := r.getItemIndent(ctx.File, pos.StartLine)
		if actualIndent != expectedIndent {
			line := ctx.File.Lines[pos.StartLine-1]

			// Build fix.
			builder := fix.NewEditBuilder()
			lineContent := lint.LineText(ctx.File, pos.StartLine)
			trimmed := bytes.TrimLeft(lineContent, " \t")

			newLine := strings.Repeat(" ", expectedIndent) + string(trimmed)
			builder.ReplaceRange(line.StartOffset+lint.LineTextOffset(ctx.File, pos.StartLine), line.NewlineStart, newLine)

			diag := lint.NewDiagnostic(r.ID(), item,
				fmt.Sprintf("Unordered list indentation %d does not match expected %d", actualIndent, expectedIndent)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Indent list item by %d spaces", expectedIndent)).
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}

		// Check nested lists.
		for child := item.FirstChild; child != nil; child = child.Next {
			if child.Kind == mdast.NodeList && !lint.IsOrderedList(child) {
				childDiags := r.checkULIndent(ctx, child, depth+1, indent, startIndented, startIndent)
				diags = append(diags, childDiags...)
			}
		}
	}

	return diags
}

func (r *ULIndentRule) getItemIndent(file *mdast.FileSnapshot, lineNum int) int {
	content := lint.LineText(file, lineNum)
	indent := 0
	for _, ch := range content {
		switch ch {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent
		}
	}
	return indent
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
