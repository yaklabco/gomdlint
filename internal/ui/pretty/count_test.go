package pretty_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// TestCount covers the counted nouns of human output: the singular for a
// count of one and the plural for any other count, zero included.
func TestCount(t *testing.T) {
	assert.Equal(t, "0 errors", pretty.Count(0, "error", "errors"))
	assert.Equal(t, "1 error", pretty.Count(1, "error", "errors"))
	assert.Equal(t, "2 errors", pretty.Count(2, "error", "errors"))
	assert.Equal(t, "2 info", pretty.Count(2, "info", "info"), "info names a severity and has no plural")
}

// severityStats returns run statistics with the given numbers of errors,
// warnings and info findings, in files files, all of them checked.
func severityStats(errors, warnings, infos, files int) runner.Stats {
	return runner.Stats{
		FilesProcessed:        files,
		FilesWithIssues:       files,
		DiagnosticsTotal:      errors + warnings + infos,
		DiagnosticsBySeverity: map[string]int{"error": errors, "warning": warnings, "info": infos},
	}
}

// TestSummaryOneLine_CountsAgree covers the one-line summary of text
// output with zero, one and two findings of each severity. Zero counts are
// left out.
func TestSummaryOneLine_CountsAgree(t *testing.T) {
	tests := []struct {
		name  string
		stats runner.Stats
		want  string
	}{
		{name: "one of each", stats: severityStats(1, 1, 1, 1), want: "3 issues (1 error, 1 warning, 1 info), in 1 file\n"},
		{name: "two of each", stats: severityStats(2, 2, 2, 2), want: "6 issues (2 errors, 2 warnings, 2 info), in 2 files\n"},
		{name: "one error", stats: severityStats(1, 0, 0, 1), want: "1 issue (1 error), in 1 file\n"},
		{name: "one warning", stats: severityStats(0, 1, 0, 1), want: "1 issue (1 warning), in 1 file\n"},
		{name: "no findings", stats: severityStats(0, 0, 0, 2), want: "No issues found (2 files checked)\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pretty.NewStyles(false).FormatSummaryOneLine(tt.stats))
		})
	}
}

// TestTableSummary_CountsAgree covers the summary line under a table with
// zero, one and two files and findings of each severity.
func TestTableSummary_CountsAgree(t *testing.T) {
	formatter := pretty.NewTableFormatter(pretty.NewStyles(false), 100, config.RuleFormatName)

	assert.Equal(t, " 1 file checked | 1 error | 1 warning | 1 info", formatter.FormatTableSummary(severityStats(1, 1, 1, 1), ""))
	assert.Equal(t, " 2 files checked | 2 errors | 2 warnings | 2 info", formatter.FormatTableSummary(severityStats(2, 2, 2, 2), ""))
	assert.Equal(t, " 0 files checked", formatter.FormatTableSummary(severityStats(0, 0, 0, 0), ""))
}

// TestFileTableSummary_CountsAgree covers the summary line under a
// per-file table with one and two findings of each severity.
func TestFileTableSummary_CountsAgree(t *testing.T) {
	formatter := pretty.NewTableFormatter(pretty.NewStyles(false), 100, config.RuleFormatName)
	summary := func(each int) string {
		var diagnostics []lint.Diagnostic
		for _, severity := range []config.Severity{config.SeverityError, config.SeverityWarning, config.SeverityInfo} {
			for range each {
				diagnostics = append(diagnostics, diagnostic(severity, len(diagnostics)+1, true))
			}
		}
		lines := tableLines(formatter.FormatFileTable(resultOf(diagnostics).Files[0]))
		return lines[len(lines)-1]
	}

	assert.Equal(t, " 1 error | 1 warning | 1 info | 3 fixable", summary(1))
	assert.Equal(t, " 2 errors | 2 warnings | 2 info | 6 fixable", summary(2))
}

// TestFileHeader_CountsAgree covers the header of a file's findings in
// text output.
func TestFileHeader_CountsAgree(t *testing.T) {
	styles := pretty.NewStyles(false)

	assert.Equal(t, "a.md", styles.FormatFileHeader("a.md", 0))
	assert.Equal(t, "a.md (1 issue)", styles.FormatFileHeader("a.md", 1))
	assert.Equal(t, "a.md (2 issues)", styles.FormatFileHeader("a.md", 2))
}
