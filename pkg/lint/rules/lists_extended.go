package rules

import (
	"bytes"
	"fmt"
	"regexp"
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
	content := lint.LineContent(file, lineNum)
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
			lineContent := lint.LineContent(ctx.File, pos.StartLine)
			trimmed := bytes.TrimLeft(lineContent, " \t")

			newLine := strings.Repeat(" ", expectedIndent) + string(trimmed)
			builder.ReplaceRange(line.StartOffset, line.NewlineStart, newLine)

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
	content := lint.LineContent(file, lineNum)
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

// listMarkerPattern matches list markers and captures the spaces after.
var listMarkerPattern = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])(\s+)`)

// Apply checks for correct spaces after list markers.
func (r *ListMarkerSpaceRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	ulSingle := ctx.OptionInt("ul_single", 1)
	ulMulti := ctx.OptionInt("ul_multi", 1)
	olSingle := ctx.OptionInt("ol_single", 1)
	olMulti := ctx.OptionInt("ol_multi", 1)

	var diags []lint.Diagnostic

	lists := ctx.Lists()
	for _, list := range lists {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		isOrdered := lint.IsOrderedList(list)
		isTight := lint.IsTightList(list)

		var expectedSpaces int
		if isOrdered {
			if isTight {
				expectedSpaces = olSingle
			} else {
				expectedSpaces = olMulti
			}
		} else {
			if isTight {
				expectedSpaces = ulSingle
			} else {
				expectedSpaces = ulMulti
			}
		}

		items := lint.ListItems(list)
		for _, item := range items {
			pos := item.SourcePosition()
			if !pos.IsValid() {
				continue
			}

			lineContent := lint.LineContent(ctx.File, pos.StartLine)
			match := listMarkerPattern.FindSubmatch(lineContent)
			if match == nil {
				continue
			}

			actualSpaces := len(match[3])
			if actualSpaces == expectedSpaces {
				continue
			}

			line := ctx.File.Lines[pos.StartLine-1]
			indent := match[1]
			marker := match[2]

			// Build fix.
			builder := fix.NewEditBuilder()
			markerEnd := line.StartOffset + len(indent) + len(marker)
			spacesEnd := markerEnd + actualSpaces
			builder.ReplaceRange(markerEnd, spacesEnd, strings.Repeat(" ", expectedSpaces))

			diagPos := mdast.SourcePosition{
				StartLine:   pos.StartLine,
				StartColumn: len(indent) + len(marker) + 1,
				EndLine:     pos.StartLine,
				EndColumn:   len(indent) + len(marker) + actualSpaces + 1,
			}

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, diagPos,
				fmt.Sprintf("List marker space %d does not match expected %d", actualSpaces, expectedSpaces)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %d space(s) after the list marker", expectedSpaces)).
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
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

// checkBlankAfter checks if there's a missing blank line after the list.
// This handles both normal cases and lazy continuation where text following
// a list item without a blank line gets absorbed into the list.
func (r *BlanksAroundListsRule) checkBlankAfter(
	ctx *lint.RuleContext,
	list *mdast.Node,
	pos mdast.SourcePosition,
) *lint.Diagnostic {
	// First, check for lazy continuation: if the list's EndLine extends beyond
	// the last list item's marker StartLine, content was absorbed.
	if diag := r.checkLazyContinuation(ctx, list, pos); diag != nil {
		return diag
	}

	// If no lazy continuation, check normally based on next sibling.
	return r.checkBlankAfterNormal(ctx, list, pos)
}

// checkLazyContinuation detects when text was absorbed into the list via lazy continuation.
func (r *BlanksAroundListsRule) checkLazyContinuation(
	ctx *lint.RuleContext,
	list *mdast.Node,
	pos mdast.SourcePosition,
) *lint.Diagnostic {
	lastItem := list.LastChild
	if lastItem == nil {
		return nil
	}

	itemPos := lastItem.SourcePosition()
	if !itemPos.IsValid() || itemPos.StartLine >= pos.EndLine {
		return nil
	}

	// List absorbed content after the last marker - check if there's a blank line after the marker.
	checkLine := itemPos.StartLine + 1
	if checkLine > len(ctx.File.Lines) || lint.IsBlankLine(ctx.File, checkLine) {
		return nil
	}

	return r.createAfterDiagnostic(ctx, itemPos.StartLine)
}

// checkBlankAfterNormal checks for missing blank line after list using next sibling position.
func (r *BlanksAroundListsRule) checkBlankAfterNormal(
	ctx *lint.RuleContext,
	list *mdast.Node,
	pos mdast.SourcePosition,
) *lint.Diagnostic {
	if list.Next == nil {
		return nil
	}

	// Find the line we need to check for blankness.
	checkLine := r.findCheckLineForAfter(ctx, list)
	if checkLine <= 0 || checkLine > len(ctx.File.Lines) || lint.IsBlankLine(ctx.File, checkLine) {
		return nil
	}

	// Find the diagnostic line - use the last list item's start line (the actual marker).
	diagLine := pos.EndLine
	if lastItem := list.LastChild; lastItem != nil {
		if itemPos := lastItem.SourcePosition(); itemPos.IsValid() {
			diagLine = itemPos.StartLine
		}
	}

	return r.createAfterDiagnostic(ctx, diagLine)
}

// findCheckLineForAfter determines which line to check for blankness after the list.
func (r *BlanksAroundListsRule) findCheckLineForAfter(ctx *lint.RuleContext, list *mdast.Node) int {
	nextPos := list.Next.SourcePosition()
	if nextPos.IsValid() && nextPos.StartLine > 1 {
		// Next sibling has valid position - check line before it.
		return nextPos.StartLine - 1
	}

	// Next sibling has invalid position (e.g., ThematicBreak with L0-L0).
	// Fall back to checking the line after the last list item's marker.
	if lastItem := list.LastChild; lastItem != nil {
		if itemPos := lastItem.SourcePosition(); itemPos.IsValid() && itemPos.StartLine < len(ctx.File.Lines) {
			return itemPos.StartLine + 1
		}
	}

	return 0
}

// createAfterDiagnostic creates a diagnostic for missing blank line after list.
func (r *BlanksAroundListsRule) createAfterDiagnostic(ctx *lint.RuleContext, diagLine int) *lint.Diagnostic {
	line := ctx.File.Lines[diagLine-1]
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
