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

// TablePipeStyleRule checks for consistent leading/trailing pipe style in tables.
type TablePipeStyleRule struct {
	lint.BaseRule
}

// NewTablePipeStyleRule creates a new table pipe style rule.
func NewTablePipeStyleRule() *TablePipeStyleRule {
	return &TablePipeStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD055",
			"table-pipe-style",
			"Table pipe style should be consistent",
			[]string{"table"},
			false, // Not auto-fixable (complex).
		),
	}
}

// PipeStyle represents the pipe style of tables.
type PipeStyle string

const (
	// PipeStyleConsistent uses whatever style is first encountered.
	PipeStyleConsistent PipeStyle = "consistent"
	// PipeStyleLeadingAndTrailing requires pipes at both ends.
	PipeStyleLeadingAndTrailing PipeStyle = "leading_and_trailing"
	// PipeStyleLeadingOnly requires pipe at start only.
	PipeStyleLeadingOnly PipeStyle = "leading_only"
	// PipeStyleTrailingOnly requires pipe at end only.
	PipeStyleTrailingOnly PipeStyle = "trailing_only"
	// PipeStyleNoLeadingOrTrailing requires no pipes at ends.
	PipeStyleNoLeadingOrTrailing PipeStyle = "no_leading_or_trailing"
)

// Apply checks table pipe style consistency.
func (r *TablePipeStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// Skip if not GFM flavor.
	if ctx.Config != nil && ctx.Config.Flavor != config.FlavorGFM {
		return nil, nil
	}

	configStyle := PipeStyle(ctx.OptionString("style", string(PipeStyleConsistent)))

	var diags []lint.Diagnostic
	var expectedStyle PipeStyle

	if configStyle != PipeStyleConsistent {
		expectedStyle = configStyle
	}

	// Find tables by looking for delimiter rows.
	lineNum := 1
	for lineNum <= len(ctx.File.Lines) {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		content := lint.LineContent(ctx.File, lineNum)
		if !isTableDelimiterRow(content) {
			lineNum++
			continue
		}

		// Found a table. Check all rows including header.
		tableStart := lineNum - 1 // Header row
		if tableStart < 1 {
			tableStart = lineNum
		}

		tableEnd := lineNum
		for tableEnd+1 <= len(ctx.File.Lines) {
			nextContent := lint.LineContent(ctx.File, tableEnd+1)
			if !isTableRow(nextContent) {
				break
			}
			tableEnd++
		}

		// Check all rows in the table
		for rowNum := tableStart; rowNum <= tableEnd; rowNum++ {
			rowContent := lint.LineContent(ctx.File, rowNum)
			trimmed := bytes.TrimSpace(rowContent)
			if len(trimmed) == 0 {
				continue
			}

			hasLeading := len(trimmed) > 0 && trimmed[0] == '|'
			hasTrailing := len(trimmed) > 0 && trimmed[len(trimmed)-1] == '|'

			var detectedStyle PipeStyle
			switch {
			case hasLeading && hasTrailing:
				detectedStyle = PipeStyleLeadingAndTrailing
			case hasLeading:
				detectedStyle = PipeStyleLeadingOnly
			case hasTrailing:
				detectedStyle = PipeStyleTrailingOnly
			default:
				detectedStyle = PipeStyleNoLeadingOrTrailing
			}

			// Set expected style from first row if consistent mode
			if expectedStyle == "" {
				expectedStyle = detectedStyle
				continue
			}

			// Check for style mismatch
			if detectedStyle != expectedStyle {
				pos := mdast.SourcePosition{
					StartLine:   rowNum,
					StartColumn: 1,
					EndLine:     rowNum,
					EndColumn:   len(rowContent),
				}
				diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
					fmt.Sprintf("Table row pipe style '%s' does not match expected '%s'", detectedStyle, expectedStyle)).
					WithSeverity(config.SeverityWarning).
					WithSuggestion(fmt.Sprintf("Use %s pipe style for all table rows", expectedStyle)).
					Build()
				diags = append(diags, diag)
			}
		}

		lineNum = tableEnd + 1
	}

	return diags, nil
}

// TableColumnCountRule checks for consistent column counts in GFM tables.
type TableColumnCountRule struct {
	lint.BaseRule
}

// NewTableColumnCountRule creates a new table column count rule.
func NewTableColumnCountRule() *TableColumnCountRule {
	return &TableColumnCountRule{
		BaseRule: lint.NewBaseRule(
			"MD056",
			"table-column-count",
			"Table rows should have consistent column counts",
			[]string{"table"},
			false, // Not auto-fixable.
		),
	}
}

// DefaultEnabled returns true only for GFM flavor.
func (r *TableColumnCountRule) DefaultEnabled() bool {
	return true
}

// Apply checks table column consistency. Skipped if not GFM flavor.
func (r *TableColumnCountRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// Skip if not GFM flavor.
	if ctx.Config != nil && ctx.Config.Flavor != config.FlavorGFM {
		return nil, nil
	}

	var diags []lint.Diagnostic

	// Find table-like structures by looking for delimiter rows.
	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		content := lint.LineContent(ctx.File, lineNum)
		if !isTableDelimiterRow(content) {
			continue
		}

		// Found delimiter row, check header and data rows.
		delimColCount := countTableColumns(content)

		// Check header row (line before delimiter).
		if lineNum > 1 {
			headerContent := lint.LineContent(ctx.File, lineNum-1)
			if isTableRow(headerContent) {
				headerColCount := countTableColumns(headerContent)
				if headerColCount != delimColCount {
					pos := mdast.SourcePosition{
						StartLine:   lineNum - 1,
						StartColumn: 1,
						EndLine:     lineNum - 1,
						EndColumn:   len(headerContent),
					}
					diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
						fmt.Sprintf("Table header has %d columns, delimiter has %d", headerColCount, delimColCount)).
						WithSeverity(config.SeverityWarning).
						WithSuggestion("Ensure all rows have the same number of columns").
						Build()
					diags = append(diags, diag)
				}
			}
		}

		// Check data rows (lines after delimiter).
		for dataLine := lineNum + 1; dataLine <= len(ctx.File.Lines); dataLine++ {
			dataContent := lint.LineContent(ctx.File, dataLine)
			if !isTableRow(dataContent) {
				break
			}

			dataColCount := countTableColumns(dataContent)
			if dataColCount != delimColCount {
				pos := mdast.SourcePosition{
					StartLine:   dataLine,
					StartColumn: 1,
					EndLine:     dataLine,
					EndColumn:   len(dataContent),
				}
				diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
					fmt.Sprintf("Table row has %d columns, expected %d", dataColCount, delimColCount)).
					WithSeverity(config.SeverityWarning).
					WithSuggestion("Ensure all rows have the same number of columns").
					Build()
				diags = append(diags, diag)
			}
		}
	}

	return diags, nil
}

// TableAlignmentRule validates table delimiter row format.
type TableAlignmentRule struct {
	lint.BaseRule
}

// NewTableAlignmentRule creates a new table alignment rule.
func NewTableAlignmentRule() *TableAlignmentRule {
	return &TableAlignmentRule{
		BaseRule: lint.NewBaseRule(
			"MDL003",
			"table-alignment",
			"Table delimiter row should be properly formatted",
			[]string{"tables", "gfm"},
			true, // Auto-fixable.
		),
	}
}

// DefaultEnabled returns true only for GFM flavor.
func (r *TableAlignmentRule) DefaultEnabled() bool {
	return true
}

// Apply checks table delimiter row formatting.
func (r *TableAlignmentRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// Skip if not GFM flavor.
	if ctx.Config != nil && ctx.Config.Flavor != config.FlavorGFM {
		return nil, nil
	}

	minDashes := ctx.OptionInt("min_dashes", 3)

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		content := lint.LineContent(ctx.File, lineNum)
		if !isTableDelimiterRow(content) {
			continue
		}

		// Check each cell in the delimiter row.
		cells := splitTableCells(content)
		for _, cell := range cells {
			cell = bytes.TrimSpace(cell)
			if len(cell) == 0 {
				continue
			}

			// Count dashes.
			dashes := 0
			for _, ch := range cell {
				if ch == '-' {
					dashes++
				}
			}

			if dashes < minDashes {
				pos := mdast.SourcePosition{
					StartLine:   lineNum,
					StartColumn: 1,
					EndLine:     lineNum,
					EndColumn:   len(content),
				}

				// Build fix.
				builder := r.buildAlignmentFix(ctx.File, lineNum, minDashes)

				diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
					fmt.Sprintf("Table delimiter has fewer than %d dashes", minDashes)).
					WithSeverity(config.SeverityWarning).
					WithSuggestion(fmt.Sprintf("Use at least %d dashes in delimiter cells", minDashes))

				if builder != nil {
					diagBuilder = diagBuilder.WithFix(builder)
				}

				diags = append(diags, diagBuilder.Build())
				break // One diagnostic per line.
			}
		}
	}

	return diags, nil
}

func (r *TableAlignmentRule) buildAlignmentFix(
	file *mdast.FileSnapshot,
	lineNum int,
	minDashes int,
) *fix.EditBuilder {
	if file == nil || lineNum < 1 || lineNum > len(file.Lines) {
		return nil
	}

	content := lint.LineContent(file, lineNum)
	cells := splitTableCells(content)

	newCells := make([]string, 0, len(cells))
	for _, cell := range cells {
		cell = bytes.TrimSpace(cell)
		if len(cell) == 0 {
			newCells = append(newCells, strings.Repeat("-", minDashes))
			continue
		}

		// Preserve alignment markers.
		leftAlign := cell[0] == ':'
		rightAlign := cell[len(cell)-1] == ':'

		dashes := strings.Repeat("-", minDashes)
		var newCell string
		switch {
		case leftAlign && rightAlign:
			newCell = ":" + dashes + ":"
		case leftAlign:
			newCell = ":" + dashes
		case rightAlign:
			newCell = dashes + ":"
		default:
			newCell = dashes
		}
		newCells = append(newCells, newCell)
	}

	newContent := "| " + strings.Join(newCells, " | ") + " |"
	line := file.Lines[lineNum-1]

	builder := fix.NewEditBuilder()
	builder.ReplaceRange(line.StartOffset, line.NewlineStart, newContent)

	return builder
}

// TableBlankLinesRule ensures blank lines around tables.
type TableBlankLinesRule struct {
	lint.BaseRule
}

// NewTableBlankLinesRule creates a new table blank lines rule.
func NewTableBlankLinesRule() *TableBlankLinesRule {
	return &TableBlankLinesRule{
		BaseRule: lint.NewBaseRule(
			"MD058",
			"blanks-around-tables",
			"Tables should be surrounded by blank lines",
			[]string{"table"},
			true, // Auto-fixable.
		),
	}
}

// DefaultEnabled returns true only for GFM flavor.
func (r *TableBlankLinesRule) DefaultEnabled() bool {
	return true
}

// DefaultSeverity returns info level for this rule.
func (r *TableBlankLinesRule) DefaultSeverity() config.Severity {
	return config.SeverityInfo
}

// Apply checks for blank lines around tables.
func (r *TableBlankLinesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// Skip if not GFM flavor.
	if ctx.Config != nil && ctx.Config.Flavor != config.FlavorGFM {
		return nil, nil
	}

	var diags []lint.Diagnostic

	// Find tables by looking for delimiter rows.
	lineNum := 1
	for lineNum <= len(ctx.File.Lines) {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		content := lint.LineContent(ctx.File, lineNum)
		if !isTableDelimiterRow(content) {
			lineNum++
			continue
		}

		// Found a table. Determine its extent.
		tableStart := lineNum - 1 // Header row.
		if tableStart < 1 {
			tableStart = lineNum
		}

		tableEnd := lineNum
		for tableEnd+1 <= len(ctx.File.Lines) {
			nextContent := lint.LineContent(ctx.File, tableEnd+1)
			if !isTableRow(nextContent) {
				break
			}
			tableEnd++
		}

		// Check blank line before.
		if tableStart > 1 && !lint.IsBlankLine(ctx.File, tableStart-1) {
			pos := mdast.SourcePosition{
				StartLine:   tableStart,
				StartColumn: 1,
				EndLine:     tableStart,
				EndColumn:   1,
			}

			builder := fix.NewEditBuilder()
			line := ctx.File.Lines[tableStart-1]
			builder.Insert(line.StartOffset, "\n")

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				"Missing blank line before table").
				WithSeverity(config.SeverityInfo).
				WithSuggestion("Add a blank line before the table").
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}

		// Check blank line after.
		if tableEnd < len(ctx.File.Lines) && !lint.IsBlankLine(ctx.File, tableEnd+1) {
			pos := mdast.SourcePosition{
				StartLine:   tableEnd,
				StartColumn: 1,
				EndLine:     tableEnd,
				EndColumn:   1,
			}

			builder := fix.NewEditBuilder()
			line := ctx.File.Lines[tableEnd-1]
			builder.Insert(line.EndOffset, "\n")

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				"Missing blank line after table").
				WithSeverity(config.SeverityInfo).
				WithSuggestion("Add a blank line after the table").
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}

		lineNum = tableEnd + 1
	}

	return diags, nil
}

// isTableDelimiterRow checks if a line is a table delimiter row (| --- | --- |).
func isTableDelimiterRow(content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return false
	}

	// Must contain pipes and dashes.
	hasPipe := bytes.Contains(trimmed, []byte("|"))
	hasDash := bytes.Contains(trimmed, []byte("-"))
	if !hasPipe || !hasDash {
		return false
	}

	// Check that it only contains valid delimiter characters.
	for _, ch := range trimmed {
		switch ch {
		case '|', '-', ':', ' ', '\t':
			continue
		default:
			return false
		}
	}

	return true
}

// isTableRow checks if a line looks like a table row.
func isTableRow(content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return false
	}

	// Must start and end with pipe (or start/end with content and have pipes).
	return bytes.Contains(trimmed, []byte("|"))
}

// countTableColumns counts the number of columns in a table row.
func countTableColumns(content []byte) int {
	cells := splitTableCells(content)
	return len(cells)
}

// splitTableCells splits a table row into cells.
func splitTableCells(content []byte) [][]byte {
	trimmed := bytes.TrimSpace(content)

	// Remove leading and trailing pipes.
	if len(trimmed) > 0 && trimmed[0] == '|' {
		trimmed = trimmed[1:]
	}
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '|' {
		trimmed = trimmed[:len(trimmed)-1]
	}

	if len(trimmed) == 0 {
		return nil
	}

	return bytes.Split(trimmed, []byte("|"))
}

// TableColumnStyleRule checks for consistent column spacing style in tables.
type TableColumnStyleRule struct {
	lint.BaseRule
}

// NewTableColumnStyleRule creates a new table column style rule.
func NewTableColumnStyleRule() *TableColumnStyleRule {
	return &TableColumnStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD060",
			"table-column-style",
			"Table column style should be consistent",
			[]string{"table"},
			false, // Not auto-fixable (style preference).
		),
	}
}

// DefaultEnabled returns false - this is an optional style rule.
func (r *TableColumnStyleRule) DefaultEnabled() bool {
	return false
}

// ColumnStyle represents the column spacing style of tables.
type ColumnStyle string

const (
	// ColumnStyleAny allows any consistent style.
	ColumnStyleAny ColumnStyle = "any"
	// ColumnStyleAligned requires columns to be aligned with padding.
	ColumnStyleAligned ColumnStyle = "aligned"
	// ColumnStyleCompact uses minimal spacing (single space padding).
	ColumnStyleCompact ColumnStyle = "compact"
	// ColumnStyleTight uses no extra spacing.
	ColumnStyleTight ColumnStyle = "tight"
)

// Apply checks table column spacing style.
func (r *TableColumnStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// Skip if not GFM flavor.
	if ctx.Config != nil && ctx.Config.Flavor != config.FlavorGFM {
		return nil, nil
	}

	configStyle := ColumnStyle(ctx.OptionString("style", string(ColumnStyleAny)))
	if configStyle == ColumnStyleAny {
		return nil, nil // Any style is allowed
	}

	var diags []lint.Diagnostic

	// Find tables by looking for delimiter rows.
	lineNum := 1
	for lineNum <= len(ctx.File.Lines) {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		content := lint.LineContent(ctx.File, lineNum)
		if !isTableDelimiterRow(content) {
			lineNum++
			continue
		}

		// Found a table. Check all rows.
		tableStart := lineNum - 1 // Header row
		if tableStart < 1 {
			tableStart = lineNum
		}

		tableEnd := lineNum
		for tableEnd+1 <= len(ctx.File.Lines) {
			nextContent := lint.LineContent(ctx.File, tableEnd+1)
			if !isTableRow(nextContent) {
				break
			}
			tableEnd++
		}

		// Check style of each row
		for rowNum := tableStart; rowNum <= tableEnd; rowNum++ {
			rowContent := lint.LineContent(ctx.File, rowNum)
			detectedStyle := r.detectColumnStyle(rowContent)

			if detectedStyle != configStyle {
				pos := mdast.SourcePosition{
					StartLine:   rowNum,
					StartColumn: 1,
					EndLine:     rowNum,
					EndColumn:   len(rowContent),
				}
				diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
					fmt.Sprintf("Table column style '%s' does not match expected '%s'", detectedStyle, configStyle)).
					WithSeverity(config.SeverityWarning).
					WithSuggestion(fmt.Sprintf("Use %s column style", configStyle)).
					Build()
				diags = append(diags, diag)
			}
		}

		lineNum = tableEnd + 1
	}

	return diags, nil
}

func (r *TableColumnStyleRule) detectColumnStyle(content []byte) ColumnStyle {
	cells := splitTableCells(content)
	if len(cells) == 0 {
		return ColumnStyleCompact
	}

	// Check if all cells have consistent padding
	hasLeadingSpace := true
	hasTrailingSpace := true
	allPaddedSame := true
	firstPadding := -1

	for _, cell := range cells {
		if len(cell) == 0 {
			continue
		}

		leadingSpaces := 0
		for _, ch := range cell {
			if ch != ' ' {
				break
			}
			leadingSpaces++
		}

		trailingSpaces := 0
		for i := len(cell) - 1; i >= 0; i-- {
			if cell[i] != ' ' {
				break
			}
			trailingSpaces++
		}

		if leadingSpaces == 0 {
			hasLeadingSpace = false
		}
		if trailingSpaces == 0 {
			hasTrailingSpace = false
		}

		totalPadding := leadingSpaces + trailingSpaces
		if firstPadding < 0 {
			firstPadding = totalPadding
		} else if totalPadding != firstPadding {
			allPaddedSame = false
		}
	}

	switch {
	case !hasLeadingSpace && !hasTrailingSpace:
		return ColumnStyleTight
	case hasLeadingSpace && hasTrailingSpace && allPaddedSame:
		if firstPadding == 2 { // Single space on each side
			return ColumnStyleCompact
		}
		return ColumnStyleAligned
	default:
		return ColumnStyleCompact
	}
}
