package reporter_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/clipperhouse/displaywidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// TestTableReporter_PerFileLegend covers per-file tables, whose rows carry
// severity markers like a combined table's: the legend that explains them
// follows the last table, once, before the overall summary.
func TestTableReporter_PerFileLegend(t *testing.T) {
	finding := func(severity config.Severity) lint.Diagnostic {
		return lint.Diagnostic{
			RuleID: "MD009", RuleName: "no-trailing-spaces", Message: "Trailing whitespace",
			Severity: severity, StartLine: 1, StartColumn: 1,
		}
	}
	result := &runner.Result{Stats: runner.Stats{
		FilesProcessed: 2, FilesWithIssues: 2, DiagnosticsTotal: 2,
		DiagnosticsBySeverity: map[string]int{"error": 1, "warning": 1},
	}}
	for path, severity := range map[string]config.Severity{"a.md": config.SeverityError, "b.md": config.SeverityWarning} {
		result.Files = append(result.Files, runner.FileOutcome{
			Path:   path,
			Result: &lint.PipelineResult{FileResult: &lint.FileResult{Diagnostics: []lint.Diagnostic{finding(severity)}}},
		})
	}
	slices.SortFunc(result.Files, func(a, b runner.FileOutcome) int { return strings.Compare(a.Path, b.Path) })

	var buf bytes.Buffer
	opts := reporter.Options{
		Writer: &buf, Color: "never", Format: reporter.FormatTable,
		PerFile: true, ShowSummary: true, RuleFormat: config.RuleFormatName,
	}

	_, err := reporter.NewTableReporter(opts).Report(context.Background(), result)
	require.NoError(t, err)

	const legend = " Legend: E = error | W = warning\n"
	output := buf.String()
	assert.Equal(t, 1, strings.Count(output, legend), output)
	legendAt, lastTable, summary := strings.Index(output, legend), strings.Index(output, "\nb.md\n"), strings.Index(output, "Overall Summary")
	assert.Less(t, lastTable, legendAt, "the legend follows the last table:\n%s", output)
	assert.Less(t, legendAt, summary, "the legend comes before the overall summary:\n%s", output)
}

// TestTableReporter_FitsTerminal covers whole table reports, combined and
// per file, over paths and messages that mix ASCII with wide characters,
// emoji and combining marks, one path wider than the terminal. A writer
// that is not a terminal gets 100 columns: every line of the report is
// valid UTF-8 and fits them.
func TestTableReporter_FitsTerminal(t *testing.T) {
	const width = 100 // The width the table reporter assumes for a non-terminal writer.

	paths := []string{
		"README.md",
		"docs/开发使用指南.md",
		"docs/" + strings.Repeat("开发使用指南", 10) + ".md",
		"été/résumé.md",
		"emoji/👨‍👩‍👧/notes 🇩🇪.md",
	}
	messages := []string{
		"Trailing whitespace",
		"Unused reference definition [" + strings.Repeat("äöü", 30) + "]",
		"Heading ends with trailing punctuation \"。\" in 开发使用指南的说明",
		"Link text 'café 👨‍👩‍👧' is not descriptive",
	}

	result := &runner.Result{Stats: runner.Stats{
		FilesProcessed: len(paths), FilesWithIssues: len(paths),
		DiagnosticsTotal:      len(paths) * len(messages),
		DiagnosticsBySeverity: map[string]int{"warning": len(paths) * len(messages)},
	}}
	for fileIdx, path := range paths {
		diagnostics := make([]lint.Diagnostic, 0, len(messages))
		for msgIdx, message := range messages {
			diagnostics = append(diagnostics, lint.Diagnostic{
				RuleID: "MD053", RuleName: "link-image-reference-definitions", Message: message,
				Severity: config.SeverityWarning, StartLine: 1 + msgIdx*37, StartColumn: 1 + fileIdx,
			})
		}
		result.Files = append(result.Files, runner.FileOutcome{
			Path:   path,
			Result: &lint.PipelineResult{FileResult: &lint.FileResult{Diagnostics: diagnostics}},
		})
	}

	for _, perFile := range []bool{false, true} {
		for _, format := range []config.RuleFormat{config.RuleFormatName, config.RuleFormatID, config.RuleFormatCombined} {
			name := fmt.Sprintf("per file %t, %s rules", perFile, format)

			var buf bytes.Buffer
			opts := reporter.DefaultOptions()
			opts.Writer = &buf
			opts.Color = "never"
			opts.Format = reporter.FormatTable
			opts.PerFile = perFile
			opts.RuleFormat = format

			_, err := reporter.NewTableReporter(opts).Report(context.Background(), result)
			require.NoError(t, err, name)

			for _, line := range strings.Split(buf.String(), "\n") {
				if assert.True(t, utf8.ValidString(line), "%s: line %q is not valid UTF-8", name, line) {
					assert.LessOrEqual(t, displaywidth.String(line), width, "%s: line %q is wider than the terminal", name, line)
				}
			}
		}
	}
}
