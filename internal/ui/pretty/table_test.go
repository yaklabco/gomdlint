package pretty_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/clipperhouse/displaywidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// tableRuleColumn returns the RULE column of the rows of a table: the
// identifier at the offset of the RULE header in each line between the
// heavy separators.
func tableRuleColumn(t *testing.T, table string) []string {
	t.Helper()

	lines := strings.Split(table, "\n")
	require.Greater(t, len(lines), 2, "table:\n%s", table)
	start := strings.Index(lines[0], "RULE")
	require.GreaterOrEqual(t, start, 0, "no RULE header in:\n%s", table)

	var rules []string
	for _, line := range lines[2:] {
		if strings.HasPrefix(line, "=") {
			break
		}
		require.Greater(t, len(line), start, "row %q ends before the RULE column", line)
		rules = append(rules, strings.Fields(line[start:])[0])
	}
	return rules
}

func TestTableFormatter_RuleColumnFollowsRuleFormat(t *testing.T) {
	file := runner.FileOutcome{
		Path: "doc.md",
		Result: &lint.PipelineResult{FileResult: &lint.FileResult{Diagnostics: []lint.Diagnostic{
			{
				RuleID: "MD009", RuleName: "no-trailing-spaces", Message: "Trailing whitespace",
				Severity: config.SeverityWarning, StartLine: 3, StartColumn: 22,
			},
			{
				RuleID: "MD053", RuleName: "link-image-reference-definitions", Message: "Unused reference definition [x]",
				Severity: config.SeverityWarning, StartLine: 17, StartColumn: 1,
			},
		}}},
	}
	result := &runner.Result{Files: []runner.FileOutcome{file}}

	tests := []struct {
		format config.RuleFormat
		want   []string
	}{
		{format: config.RuleFormatName, want: []string{"no-trailing-spaces", "link-image-reference-definitions"}},
		{format: config.RuleFormatID, want: []string{"MD009", "MD053"}},
		{format: config.RuleFormatCombined, want: []string{"MD009/no-trailing-spaces", "MD053/link-image-reference-definitions"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			// A terminal of 80 columns shortens messages and paths, not rules.
			for _, width := range []int{200, 80} {
				formatter := pretty.NewTableFormatter(pretty.NewStyles(false), width, tt.format)

				assert.Equal(t, tt.want, tableRuleColumn(t, formatter.FormatTable(result)), "table at width %d", width)
				assert.Equal(t, tt.want, tableRuleColumn(t, formatter.FormatFileTable(file)), "per-file table at width %d", width)
			}

			// Only a terminal too narrow for the other columns at their
			// floors cuts rules, at their end, so their start stays.
			formatter := pretty.NewTableFormatter(pretty.NewStyles(false), 50, tt.format)
			for _, table := range []string{formatter.FormatTable(result), formatter.FormatFileTable(file)} {
				for idx, rule := range tableRuleColumn(t, table) {
					cut := strings.TrimSuffix(rule, "...")
					assert.True(t, rule == tt.want[idx] || (cut != rule && strings.HasPrefix(tt.want[idx], cut)),
						"rule %q at width 50 is neither %q nor its start", rule, tt.want[idx])
				}
			}
		})
	}
}

// tableLines returns the lines of a formatted table, without the newline
// that ends the last one.
func tableLines(table string) []string {
	return strings.Split(strings.TrimSuffix(table, "\n"), "\n")
}

// assertTableFits checks that every line of a table is valid UTF-8 and at
// most width terminal cells wide, and that every row is as wide as the
// header, so the columns line up.
func assertTableFits(t *testing.T, table string, width int, name string) {
	t.Helper()

	lines := tableLines(table)
	header := displaywidth.String(lines[0])
	inRows := false
	for _, line := range lines {
		if !assert.True(t, utf8.ValidString(line), "%s: line %q is not valid UTF-8", name, line) {
			continue
		}
		assert.LessOrEqual(t, displaywidth.String(line), width, "%s: line %q is wider than the terminal", name, line)

		switch {
		case strings.HasPrefix(line, "="):
			inRows = !inRows // The heavy separators enclose the rows.
		case inRows && !strings.HasPrefix(line, "-"):
			assert.Equal(t, header, displaywidth.String(line), "%s: row %q does not line up with the header", name, line)
		}
	}
}

// mixedResult is a run whose paths, messages and rule names mix ASCII with
// wide characters, emoji, combining marks, control characters and long
// text: each text of mixedTexts is the path of a file whose diagnostics
// carry every text as a message.
func mixedResult() *runner.Result {
	rules := []struct{ id, name string }{
		{id: "MD009", name: "no-trailing-spaces"},
		{id: "MD053", name: "link-image-reference-definitions"},
		{id: "XX001", name: "规则名称很长很长的自定义规则"}, // A custom rule named with wide characters.
	}
	messages := append(mixedTexts(), "a tab\tand a newline\nin one message", "x")

	result := &runner.Result{}
	for fileIdx, path := range mixedTexts() {
		diagnostics := make([]lint.Diagnostic, 0, len(messages))
		for msgIdx, message := range messages {
			rule := rules[(fileIdx+msgIdx)%len(rules)]
			diagnostics = append(diagnostics, lint.Diagnostic{
				RuleID: rule.id, RuleName: rule.name, Message: message, Severity: config.SeverityWarning,
				StartLine: 1 + msgIdx*101, StartColumn: 1 + fileIdx*7,
			})
		}
		result.Files = append(result.Files, runner.FileOutcome{
			Path:   path,
			Result: &lint.PipelineResult{FileResult: &lint.FileResult{Diagnostics: diagnostics}},
		})
	}
	return result
}

// TestTableFormatter_FitsTerminal covers combined and per-file tables over
// mixed text, terminal widths and rule formats: every line is valid UTF-8
// and fits the terminal, and the columns line up. Columns shrink to fit and
// cut their text on character boundaries.
func TestTableFormatter_FitsTerminal(t *testing.T) {
	result := mixedResult()

	for _, width := range []int{60, 80, 100, 120} {
		for _, format := range []config.RuleFormat{config.RuleFormatName, config.RuleFormatID, config.RuleFormatCombined} {
			formatter := pretty.NewTableFormatter(pretty.NewStyles(false), width, format)

			assertTableFits(t, formatter.FormatTable(result), width, fmt.Sprintf("table, %s rules, width %d", format, width))
			for _, file := range result.Files {
				name := fmt.Sprintf("per-file table of %q, %s rules, width %d", file.Path, format, width)
				assertTableFits(t, formatter.FormatFileTable(file), width, name)
				title := formatter.FormatFileTitle(file.Path)
				assert.True(t, utf8.ValidString(title), "%s: title %q", name, title)
				assert.LessOrEqual(t, displaywidth.String(title), width, "%s: title %q", name, title)
			}
		}
	}
}

// TestTableFormatter_KeepsWidePathsWhole covers a path of wide characters
// that fits its column in terminal cells but not in bytes. It used to be
// cut by bytes, in the middle of a character.
func TestTableFormatter_KeepsWidePathsWhole(t *testing.T) {
	file := runner.FileOutcome{
		Path: "docs/开发使用指南.md",
		Result: &lint.PipelineResult{FileResult: &lint.FileResult{Diagnostics: []lint.Diagnostic{{
			RuleID: "MD053", RuleName: "link-image-reference-definitions", Message: "Unused reference definition [unused]",
			Severity: config.SeverityWarning, StartLine: 3, StartColumn: 1,
		}}}},
	}
	formatter := pretty.NewTableFormatter(pretty.NewStyles(false), 100, config.RuleFormatName)

	table := formatter.FormatTable(&runner.Result{Files: []runner.FileOutcome{file}})

	assertTableFits(t, table, 100, "table")
	assert.Contains(t, table, " docs/开发使用指南.md ")
}

// diagnostic returns a finding of rule MD009 with severity on line, with a
// fix when fixable.
func diagnostic(severity config.Severity, line int, fixable bool) lint.Diagnostic {
	diag := lint.Diagnostic{
		RuleID: "MD009", RuleName: "no-trailing-spaces", Message: "Trailing whitespace",
		Severity: severity, StartLine: line, StartColumn: 1,
	}
	if fixable {
		diag.FixEdits = []fix.TextEdit{{StartOffset: 0, EndOffset: 1}}
	}
	return diag
}

// resultOf returns a run with one file per list of diagnostics, named
// a.md, b.md and so on.
func resultOf(files ...[]lint.Diagnostic) *runner.Result {
	result := &runner.Result{}
	for idx, diagnostics := range files {
		result.Files = append(result.Files, runner.FileOutcome{
			Path:   string(rune('a'+idx)) + ".md",
			Result: &lint.PipelineResult{FileResult: &lint.FileResult{Diagnostics: diagnostics}},
		})
	}
	return result
}

// tableRows returns the rows of a table without color: the lines between
// its heavy separators, without the light separators between files.
func tableRows(table string) []string {
	var rows []string
	inRows := false
	for _, line := range tableLines(ansiSequence.ReplaceAllString(table, "")) {
		switch {
		case strings.HasPrefix(line, "="):
			inRows = !inRows
		case inRows && !strings.HasPrefix(line, "-"):
			rows = append(rows, line)
		}
	}
	return rows
}

// TestTableFormatter_SeverityMarkers covers the marker that leads every
// row, E, W or I for its severity, in combined and per-file tables, with
// and without color. Without color, the marker is the only sign of a row's
// severity. A finding without a severity counts as a warning, as in the
// summary counts.
func TestTableFormatter_SeverityMarkers(t *testing.T) {
	result := resultOf(
		[]lint.Diagnostic{
			diagnostic(config.SeverityError, 1, false),
			diagnostic(config.SeverityWarning, 2, true),
			diagnostic(config.SeverityInfo, 3, false),
		},
		[]lint.Diagnostic{diagnostic("", 4, false)},
	)

	for _, colored := range []bool{false, true} {
		formatter := pretty.NewTableFormatter(pretty.NewStyles(colored), 100, config.RuleFormatID)

		var markers []string
		for _, row := range tableRows(formatter.FormatTable(result)) {
			markers = append(markers, strings.Fields(row)[0])
		}
		assert.Equal(t, []string{"E", "W", "I", "W"}, markers, "combined table, colored %t", colored)

		markers = nil
		for _, row := range tableRows(formatter.FormatFileTable(result.Files[0])) {
			markers = append(markers, strings.Fields(row)[0])
		}
		assert.Equal(t, []string{"E", "W", "I"}, markers, "per-file table, colored %t", colored)
	}
}

// TestTableFormatter_LegendDescribesRows covers the legend of a table: it
// explains exactly the markers the rows show, in the same words with and
// without color. With color, each severity marker in the legend has the
// color of its rows.
func TestTableFormatter_LegendDescribesRows(t *testing.T) {
	tests := []struct {
		name   string
		result *runner.Result
		want   string
	}{
		{
			name: "every marker",
			result: resultOf([]lint.Diagnostic{
				diagnostic(config.SeverityInfo, 1, false),
				diagnostic(config.SeverityWarning, 2, true),
				diagnostic(config.SeverityError, 3, false),
			}),
			want: " Legend: E = error | W = warning | I = info | + = fixable",
		},
		{
			name:   "warnings in two files, none fixable",
			result: resultOf([]lint.Diagnostic{diagnostic(config.SeverityWarning, 1, false)}, []lint.Diagnostic{diagnostic("", 2, false)}),
			want:   " Legend: W = warning",
		},
		{
			name:   "fixable errors",
			result: resultOf([]lint.Diagnostic{diagnostic(config.SeverityError, 1, true)}),
			want:   " Legend: E = error | + = fixable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain := pretty.NewTableFormatter(pretty.NewStyles(false), 100, config.RuleFormatID)
			styles := pretty.NewStyles(true)
			colored := pretty.NewTableFormatter(styles, 100, config.RuleFormatID)

			plainTable := plain.FormatTable(tt.result)
			coloredTable := colored.FormatTable(tt.result)
			lines := tableLines(plainTable)
			assert.Equal(t, tt.want, lines[len(lines)-1], "the legend ends the table")
			assert.Equal(t, plainTable, ansiSequence.ReplaceAllString(coloredTable, ""), "color changes no text")
			assert.Equal(t, tt.want+"\n", plain.FormatLegend(tt.result), "the legend of per-file tables")
			assert.Equal(t, tt.want+"\n", ansiSequence.ReplaceAllString(colored.FormatLegend(tt.result), ""))

			coloredLines := tableLines(coloredTable)
			coloredLegend := coloredLines[len(coloredLines)-1]
			for marker, style := range map[string]lipgloss.Style{
				"E": styles.TableErrorRow, "W": styles.TableWarnRow, "I": styles.TableInfoRow, "+": styles.TableFixable,
			} {
				if strings.Contains(tt.want, " "+marker+" = ") {
					assert.Contains(t, coloredLegend, style.Render(marker), "the %q marker has the color of its rows", marker)
				}
			}
		})
	}
}

// TestTableFormatter_SameTextInBothColorModes covers whole combined and
// per-file tables: color only adds escape sequences.
func TestTableFormatter_SameTextInBothColorModes(t *testing.T) {
	result := mixedResult()
	for _, format := range []config.RuleFormat{config.RuleFormatName, config.RuleFormatCombined} {
		plain := pretty.NewTableFormatter(pretty.NewStyles(false), 80, format)
		colored := pretty.NewTableFormatter(pretty.NewStyles(true), 80, format)

		assert.Equal(t, plain.FormatTable(result), ansiSequence.ReplaceAllString(colored.FormatTable(result), ""))
		for _, file := range result.Files {
			assert.Equal(t, plain.FormatFileTable(file), ansiSequence.ReplaceAllString(colored.FormatFileTable(file), ""))
		}
	}
}

// TestTableFormatter_CutsMessagesOnCharacterBoundaries covers a message of
// two-byte letters that is too long for its column. It used to be cut by
// bytes, in the middle of a letter.
func TestTableFormatter_CutsMessagesOnCharacterBoundaries(t *testing.T) {
	message := "Unused reference definition [" + strings.Repeat("äöü", 30) + "]"
	file := runner.FileOutcome{
		Path: "doc.md",
		Result: &lint.PipelineResult{FileResult: &lint.FileResult{Diagnostics: []lint.Diagnostic{{
			RuleID: "MD053", RuleName: "link-image-reference-definitions", Message: message,
			Severity: config.SeverityWarning, StartLine: 5, StartColumn: 1,
		}}}},
	}

	for _, format := range []config.RuleFormat{config.RuleFormatName, config.RuleFormatID} {
		formatter := pretty.NewTableFormatter(pretty.NewStyles(false), 100, format)
		for name, table := range map[string]string{
			"table":          formatter.FormatTable(&runner.Result{Files: []runner.FileOutcome{file}}),
			"per-file table": formatter.FormatFileTable(file),
		} {
			name = fmt.Sprintf("%s, %s rules", name, format)
			assertTableFits(t, table, 100, name)

			row := tableLines(table)[2]
			start := strings.Index(row, "Unused")
			if assert.GreaterOrEqual(t, start, 0, "%s: row %q", name, row) {
				cell, _, found := strings.Cut(row[start:], "...")
				assert.True(t, found, "%s: the message is cut, got %q", name, row)
				assert.True(t, strings.HasPrefix(message, cell), "%s: %q is not the start of the message", name, cell)
			}
		}
	}
}
