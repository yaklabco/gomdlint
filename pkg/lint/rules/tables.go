package rules

import (
	"fmt"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// The table rules work on the tables the parser recognized (GFM), so text
// that only looks like a table is ignored, rows the parser absorbed into a
// table count as rows, and tables inside blockquotes and list items are
// checked. Positions follow markdownlint.

// tableRow is one row of a parsed table as written in the source: the header
// row, the delimiter row, or a body row.
type tableRow struct {
	line      int
	start     int   // offset of the row's first character
	end       int   // offset after the row's last non-whitespace character
	leading   bool  // the row starts with a pipe
	trailing  bool  // the row ends with an unescaped pipe
	pipes     []int // offsets of the unescaped pipes
	delimiter bool
}

// cellCount returns the number of cells in the row, as GFM counts them.
func (row tableRow) cellCount() int {
	count := len(row.pipes) + 1
	if row.leading {
		count--
	}
	if row.trailing && (len(row.pipes) > 1 || !row.leading) {
		count--
	}
	return count
}

// cellStart returns the offset where cell idx begins: the pipe before it, or
// the row start for a first cell without a leading pipe.
func (row tableRow) cellStart(idx int) int {
	if !row.leading {
		idx--
	}
	if idx < 0 {
		return row.start
	}
	return row.pipes[idx]
}

// cells returns the start and end offsets of each cell's text, between pipes.
func (row tableRow) cells() [][2]int {
	bounds := make([]int, 0, len(row.pipes)+2)
	if !row.leading {
		bounds = append(bounds, row.start-1)
	}
	bounds = append(bounds, row.pipes...)
	if !row.trailing {
		bounds = append(bounds, row.end)
	}
	cells := make([][2]int, 0, len(bounds))
	for idx := 0; idx+1 < len(bounds); idx++ {
		cells = append(cells, [2]int{bounds[idx] + 1, bounds[idx+1]})
	}
	return cells
}

// parseTableRowText finds the pipes of a row whose text is content[start:]
// up to the end of the line. A pipe escaped with a backslash is cell content;
// a pipe inside a code span still separates cells, as in GFM.
func parseTableRowText(content []byte, start int) tableRow {
	end := start
	for end < len(content) && content[end] != '\n' && content[end] != '\r' {
		end++
	}
	for end > start && (content[end-1] == ' ' || content[end-1] == '\t') {
		end--
	}
	row := tableRow{start: start, end: end}
	for pos := start; pos < end; pos++ {
		switch content[pos] {
		case '\\':
			pos++
		case '|':
			row.pipes = append(row.pipes, pos)
		}
	}
	row.leading = len(row.pipes) > 0 && row.pipes[0] == start
	row.trailing = len(row.pipes) > 0 && row.pipes[len(row.pipes)-1] == end-1
	return row
}

// tableRows returns the rows of a parsed table in source order, or nil when
// the header and delimiter rows have different cell counts: GFM requires them
// to match, so such text is not a table on GitHub or in markdownlint, though
// the parser accepts a header with fewer cells. The delimiter row has no node
// of its own: it is the line after the header row.
func tableRows(file *mdast.FileSnapshot, table *mdast.Node) []tableRow {
	rows := parsedTableRows(file, table)
	if len(rows) < 2 || rows[0].cellCount() != rows[1].cellCount() {
		return nil
	}
	return rows
}

// isGFMTable reports whether a parsed table is a table under GFM's rules.
func isGFMTable(file *mdast.FileSnapshot, table *mdast.Node) bool {
	return tableRows(file, table) != nil
}

func parsedTableRows(file *mdast.FileSnapshot, table *mdast.Node) []tableRow {
	var rows []tableRow
	for node := table.FirstChild; node != nil; node = node.Next {
		if node.Kind != mdast.NodeTableRow {
			continue
		}
		start := node.SourceRange().StartOffset
		line, _ := file.LineAt(start)
		row := parseTableRowText(file.Content, start)
		row.line = line
		rows = append(rows, row)
		if node != table.FirstChild || line >= len(file.Lines) {
			continue
		}
		delimiterLine := line + 1
		delimiterStart := file.Lines[delimiterLine-1].StartOffset + len(lint.ContainerPrefix(file, table, delimiterLine))
		for delimiterStart < len(file.Content) && (file.Content[delimiterStart] == ' ' || file.Content[delimiterStart] == '\t') {
			delimiterStart++
		}
		delimiter := parseTableRowText(file.Content, delimiterStart)
		delimiter.line = delimiterLine
		delimiter.delimiter = true
		rows = append(rows, delimiter)
	}
	return rows
}

// tablePosition returns the position of one character at offset.
func tablePosition(file *mdast.FileSnapshot, offset int) mdast.SourcePosition {
	line, column := file.LineAt(offset)
	return mdast.SourcePosition{StartLine: line, StartColumn: column, EndLine: line, EndColumn: column + 1}
}

// TablePipeStyleRule checks for consistent leading/trailing pipe style in tables.
type TablePipeStyleRule struct {
	lint.BaseRule
}

// NewTablePipeStyleRule creates a new table pipe style rule.
func NewTablePipeStyleRule() *TablePipeStyleRule {
	return &TablePipeStyleRule{
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD055")),
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

// pipeStyleOf returns the style of a row's outer pipes.
func pipeStyleOf(leading, trailing bool) PipeStyle {
	switch {
	case leading && trailing:
		return PipeStyleLeadingAndTrailing
	case leading:
		return PipeStyleLeadingOnly
	case trailing:
		return PipeStyleTrailingOnly
	default:
		return PipeStyleNoLeadingOrTrailing
	}
}

// Apply checks every row of every table, the delimiter row included, against
// the configured style or, by default, the style of the document's first row.
// A missing or unexpected leading pipe is reported at the start of the row and
// a trailing one at its last character.
func (r *TablePipeStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// Like markdownlint, a style expects each pipe unless it rules that pipe
	// out, so an unknown style expects both.
	expected := PipeStyle(ctx.OptionString("style", string(PipeStyleConsistent)))
	expectLeading := expected != PipeStyleNoLeadingOrTrailing && expected != PipeStyleTrailingOnly
	expectTrailing := expected != PipeStyleNoLeadingOrTrailing && expected != PipeStyleLeadingOnly

	var diags []lint.Diagnostic
	for _, table := range ctx.Tables() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		for _, row := range tableRows(ctx.File, table) {
			actual := pipeStyleOf(row.leading, row.trailing)
			if expected == PipeStyleConsistent {
				expected, expectLeading, expectTrailing = actual, row.leading, row.trailing
			}
			if row.leading != expectLeading {
				diags = append(diags, r.pipeDiagnostic(ctx, row.start, "leading", expectLeading, expected, actual))
			}
			if row.trailing != expectTrailing {
				diags = append(diags, r.pipeDiagnostic(ctx, row.end-1, "trailing", expectTrailing, expected, actual))
			}
		}
	}
	return diags, nil
}

func (r *TablePipeStyleRule) pipeDiagnostic(
	ctx *lint.RuleContext, offset int, side string, wanted bool, expected, actual PipeStyle,
) lint.Diagnostic {
	problem := "Unexpected"
	if wanted {
		problem = "Missing"
	}
	return lint.NewDiagnosticAt(r.ID(), ctx.File.Path, tablePosition(ctx.File, offset),
		fmt.Sprintf("%s %s pipe: expected %s, found %s", problem, side, expected, actual)).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Use %s pipe style for all table rows", expected)).
		Build()
}

// TableColumnCountRule checks for consistent column counts in GFM tables.
type TableColumnCountRule struct {
	lint.BaseRule
}

// NewTableColumnCountRule creates a new table column count rule.
func NewTableColumnCountRule() *TableColumnCountRule {
	return &TableColumnCountRule{
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD056")),
	}
}

// Apply reports rows with a different number of cells than the header row.
// A row with too few cells is reported at its last character; a row with
// too many at the start of its first extra cell.
func (r *TableColumnCountRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	for _, table := range ctx.Tables() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		rows := tableRows(ctx.File, table)
		if len(rows) == 0 {
			continue
		}
		expected := rows[0].cellCount()
		for _, row := range rows {
			actual := row.cellCount()
			var offset int
			var msg string
			switch {
			case actual < expected:
				offset, msg = row.end-1, "Too few cells, row will be missing data"
			case actual > expected:
				offset, msg = row.cellStart(expected), "Too many cells, extra data will be missing"
			default:
				continue
			}
			diags = append(diags, lint.NewDiagnosticAt(r.ID(), ctx.File.Path, tablePosition(ctx.File, offset),
				fmt.Sprintf("%s: expected %d, found %d", msg, expected, actual)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Give every row as many cells as the header row").
				Build())
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
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MDL003")),
	}
}

// Apply reports delimiter rows with a cell of fewer than min_dashes dashes.
// The fix lengthens only those dash runs and keeps the alignment colons,
// padding, pipes, and container prefix as written.
func (r *TableAlignmentRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	minDashes := ctx.OptionInt("min_dashes", 3)
	var diags []lint.Diagnostic
	for _, table := range ctx.Tables() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		var delimiter *tableRow
		for _, row := range tableRows(ctx.File, table) {
			if row.delimiter {
				delimiter = &row
				break
			}
		}
		if delimiter == nil {
			continue
		}
		builder := fix.NewEditBuilder()
		for _, cell := range delimiter.cells() {
			text := string(ctx.File.Content[cell[0]:cell[1]])
			first, last := strings.IndexByte(text, '-'), strings.LastIndexByte(text, '-')
			if first < 0 || last-first+1 >= minDashes {
				continue
			}
			builder.ReplaceRange(cell[0]+first, cell[0]+last+1, strings.Repeat("-", minDashes))
		}
		if len(builder.Edits) == 0 {
			continue
		}
		diags = append(diags, lint.NewDiagnosticAt(r.ID(), ctx.File.Path, tablePosition(ctx.File, delimiter.start),
			fmt.Sprintf("Table delimiter has fewer than %d dashes", minDashes)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Use at least %d dashes in delimiter cells", minDashes)).
			WithFix(builder).
			Build())
	}
	return verifyFixes(ctx, diags, fixAllowance{}), nil
}

// TableBlankLinesRule ensures blank lines around tables.
type TableBlankLinesRule struct {
	lint.BaseRule
}

// NewTableBlankLinesRule creates a new table blank lines rule.
func NewTableBlankLinesRule() *TableBlankLinesRule {
	return &TableBlankLinesRule{
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD058")),
	}
}

// Apply checks the lines before and after each table. Like markdownlint, a
// line holding only blockquote markers, whitespace, or HTML comments is
// blank, and the fix inserts a blank line that keeps the enclosing
// blockquotes whole. Text directly after a table is one of its rows. The
// fix is withheld where the blank line would make a tight list loose (see
// blankLineLoosensList).
func (r *TableBlankLinesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	inserted := blankLineInserts{}
	tightness := listTightness{}
	for _, table := range ctx.Tables() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		pos := table.SourcePosition()
		rows := tableRows(ctx.File, table)
		if !pos.IsValid() || len(rows) == 0 {
			continue
		}
		blankLine := lint.BlankLineText(ctx.File, table)

		if pos.StartLine > 1 && !isMarkdownlintBlankLine(lint.LineContent(ctx.File, pos.StartLine-1)) {
			var builder *fix.EditBuilder
			if !blankLineLoosensList(table, pos.StartLine, tightness) {
				builder = inserted.insert(ctx.File.Lines[pos.StartLine-1].StartOffset, blankLine)
			}
			diags = append(diags, r.blankLineDiagnostic(ctx, rows[0].start, "before", builder))
		}
		if pos.EndLine < len(ctx.File.Lines) && !isMarkdownlintBlankLine(lint.LineContent(ctx.File, pos.EndLine+1)) {
			var builder *fix.EditBuilder
			if !blankLineLoosensList(table, pos.EndLine+1, tightness) {
				builder = inserted.insert(ctx.File.Lines[pos.EndLine].StartOffset, blankLine)
			}
			diags = append(diags, r.blankLineDiagnostic(ctx, rows[len(rows)-1].start, "after", builder))
		}
	}
	return verifyFixes(ctx, diags, fixAllowance{}), nil
}

// blankLineLoosensList reports whether a blank line inserted before line,
// inside the containers of table, would make a tight list loose. The
// innermost list or list item between the table and its innermost
// blockquote that holds both line and the line before it takes the blank
// line between two of its items or two blocks of one item. That makes its
// list loose: the list itself, or the item's list. So the line loosens the
// list when it is tight, and leaves a loose list loose. The lists around
// that list hold the line inside one block of an item, which leaves them as
// they are. The inserted line carries the markers of the table's
// blockquotes, so it stays inside them, and only the lists within the
// innermost one take it for a blank line. A list or item without a position
// counts as tight, and so does a list that tightness finds may be tight.
//
// A tight list made loose changes how every item of the list renders, and
// this guard is what keeps that from happening. The re-parse that checks
// the fix cannot always see it: the region checks for a long list at the
// top level or in a top-level blockquote give a table made from a paragraph
// no record of a blank line before it. It still checks each fix that this
// lets through, such as one in a loose list that would move the table out
// of its item.
func blankLineLoosensList(table *mdast.Node, line int, tightness listTightness) bool {
	for node := table.Parent; node != nil; node = node.Parent {
		switch node.Kind {
		case mdast.NodeBlockquote, mdast.NodeDocument:
			return false
		case mdast.NodeList, mdast.NodeListItem:
			pos := node.SourcePosition()
			if !pos.IsValid() {
				return true
			}
			if pos.StartLine < line && line <= pos.EndLine {
				list := node
				if node.Kind == mdast.NodeListItem {
					list = node.Parent
				}
				return tightness.mayBeTight(list)
			}
		}
	}
	return false
}

// listTightness caches, for each list node, whether a blank line inside it
// may make it loose (see mayBeTight), so that MD058 reads each list once.
type listTightness map[*mdast.Node]bool

// mayBeTight reports whether list may be tight as micromark, markdownlint's
// parser, reads it. It may when the parser reads it as tight or gives it no
// list attributes, and also when an item holds a fenced code block without
// a closing fence or an HTML block. micromark takes the blank lines that
// follow such a block in its item into the block, as CommonMark does for a
// fence, where goldmark gives them to the next item or block, so goldmark
// reads the list as loose where micromark reads it as tight. A node that is
// no list counts as tight too.
func (t listTightness) mayBeTight(list *mdast.Node) bool {
	if list == nil || list.Kind != mdast.NodeList || list.Block == nil || list.Block.List == nil || list.Block.List.Tight {
		return true
	}
	tight, ok := t[list]
	if !ok {
		tight = holdsBlankLineTaker(list)
		t[list] = tight
	}
	return tight
}

// holdsBlankLineTaker reports whether a block in the items of node, or of
// the lists inside them, can take in the blank lines that end its item: a
// fenced code block without a closing fence, or an HTML block, whose kind,
// which says whether its end condition passes blank lines, the parsed tree
// does not keep. A block inside a blockquote cannot, since a blank line
// ends the blockquote first.
func holdsBlankLineTaker(node *mdast.Node) bool {
	for child := node.FirstChild; child != nil; child = child.Next {
		switch child.Kind {
		case mdast.NodeHTMLBlock:
			return true
		case mdast.NodeCodeBlock:
			code := child.Block
			if code == nil || code.CodeBlock == nil || (!code.CodeBlock.Indented && !code.CodeBlock.Closed) {
				return true
			}
		case mdast.NodeList, mdast.NodeListItem:
			if holdsBlankLineTaker(child) {
				return true
			}
		}
	}
	return false
}

func (r *TableBlankLinesRule) blankLineDiagnostic(
	ctx *lint.RuleContext, offset int, side string, builder *fix.EditBuilder,
) lint.Diagnostic {
	diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, tablePosition(ctx.File, offset),
		"Missing blank line "+side+" table").
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Add a blank line " + side + " the table")
	if builder != nil {
		diagBuilder = diagBuilder.WithFix(builder)
	}
	return diagBuilder.Build()
}

// TableColumnStyleRule checks for consistent column spacing style in tables.
type TableColumnStyleRule struct {
	lint.BaseRule
}

// NewTableColumnStyleRule creates a new table column style rule.
func NewTableColumnStyleRule() *TableColumnStyleRule {
	return &TableColumnStyleRule{
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD060")),
	}
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

// Apply checks table column spacing. With style "any", markdownlint checks
// each table against every style and reports the closest one; setting
// markdownlint_compatible to false instead allows any spacing. The aligned,
// compact, and tight styles are always checked like markdownlint.
func (r *TableColumnStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := ColumnStyle(ctx.OptionString("style", string(ColumnStyleAny)))
	if configStyle == ColumnStyleAny && !ctx.OptionBool("markdownlint_compatible", true) {
		return nil, nil
	}
	return r.applyMarkdownlintStyle(ctx, configStyle)
}
