package rules

import (
	"bytes"

	"github.com/clipperhouse/displaywidth"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// applyMarkdownlintStyle selects the allowed style with the fewest errors for
// each parsed table. Native MD056 separately checks malformed table-like text.
func (r *TableColumnStyleRule) applyMarkdownlintStyle(ctx *lint.RuleContext, style ColumnStyle) ([]lint.Diagnostic, error) {
	var diags []lint.Diagnostic
	for _, table := range ctx.Tables() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		pos := table.SourcePosition()
		var best []lint.Diagnostic
		selected := false
		for _, candidate := range []ColumnStyle{ColumnStyleAligned, ColumnStyleCompact, ColumnStyleTight} {
			if style != ColumnStyleAny && style != candidate {
				continue
			}
			var errors []lint.Diagnostic
			for line := pos.StartLine; line <= pos.EndLine; line++ {
				if candidate == ColumnStyleAligned {
					if line > pos.StartLine {
						errors = append(errors, r.checkDividerAlignment(ctx, pos.StartLine, line)...)
					}
				} else {
					if ctx.OptionBool("aligned_delimiter", false) && line == pos.StartLine+1 {
						errors = append(errors, r.checkDividerAlignment(ctx, pos.StartLine, line)...)
					}
					errors = append(errors, r.checkDividerSpacing(ctx, line, candidate)...)
				}
			}
			if !selected || len(errors) < len(best) {
				best, selected = errors, true
			}
		}
		diags = append(diags, best...)
	}
	return diags, nil
}

func (r *TableColumnStyleRule) checkDividerSpacing(ctx *lint.RuleContext, line int, style ColumnStyle) []lint.Diagnostic {
	content := bytes.TrimRight(lint.LineContent(ctx.File, line), " \t")
	textStart := min(lint.LineTextOffset(ctx.File, line), len(content))
	_, contentStart := linePrefix(string(content[textStart:]))
	contentStart += textStart
	columns := tableDividerOffsets(content)
	var diags []lint.Diagnostic
	for index, column := range columns {
		start, end := contentStart, len(content)
		if index > 0 {
			start = columns[index-1] + 1
		}
		if index+1 < len(columns) {
			end = columns[index+1]
		}
		if column > start && (index > 0 || len(bytes.TrimSpace(content[start:column])) > 0) {
			padding := len(content[start:column]) - len(bytes.TrimRight(content[start:column], " \t"))
			if !tablePaddingMatches(padding, style) {
				diags = append(diags, r.dividerSpacingDiagnostic(ctx, line, column, style))
			}
		}
		if end > column+1 {
			padding := len(content[column+1:end]) - len(bytes.TrimLeft(content[column+1:end], " \t"))
			if !tablePaddingMatches(padding, style) {
				diags = append(diags, r.dividerSpacingDiagnostic(ctx, line, column, style))
			}
		}
	}
	return diags
}

func tablePaddingMatches(padding int, style ColumnStyle) bool {
	return (style == ColumnStyleCompact && padding == 1) || (style == ColumnStyleTight && padding == 0)
}

func (r *TableColumnStyleRule) dividerSpacingDiagnostic(
	ctx *lint.RuleContext, line, column int, style ColumnStyle,
) lint.Diagnostic {
	pos := mdast.SourcePosition{StartLine: line, EndLine: line, StartColumn: column + 1, EndColumn: column + 2}
	return lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, "Table pipe spacing does not match style "+string(style)).Build()
}

func (r *TableColumnStyleRule) checkDividerAlignment(ctx *lint.RuleContext, headerLine, rowLine int) []lint.Diagnostic {
	header := lint.LineContent(ctx.File, headerLine)
	remaining := make(map[int]bool)
	for _, column := range tableDividerOffsets(header) {
		remaining[displaywidth.String(string(header[:column]))] = true
	}
	var diags []lint.Diagnostic
	content := lint.LineContent(ctx.File, rowLine)
	for _, column := range tableDividerOffsets(content) {
		if len(remaining) == 0 {
			break
		}
		effective := displaywidth.String(string(content[:column]))
		if remaining[effective] {
			delete(remaining, effective)
			continue
		}
		pos := mdast.SourcePosition{StartLine: rowLine, EndLine: rowLine, StartColumn: column + 1, EndColumn: column + 2}
		diags = append(diags, lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			"Table pipe does not align with header").Build())
	}
	return diags
}

func tableDividerOffsets(content []byte) []int {
	var columns []int
	escaped := false
	for offset, ch := range content {
		if ch == '|' && !escaped {
			columns = append(columns, offset)
		}
		if ch == '\\' {
			escaped = !escaped
		} else {
			escaped = false
		}
	}
	return columns
}
