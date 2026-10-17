package reporter_test

import (
	"bytes"
	"context"
	"fmt"
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
