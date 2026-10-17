package pretty_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

func TestFormatSummary_Basic(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        10,
		FilesWithIssues:       3,
		DiagnosticsTotal:      15,
		DiagnosticsBySeverity: map[string]int{"error": 5, "warning": 10},
	}

	result := styles.FormatSummary(stats)

	assert.Contains(t, result, "Summary")
	assert.Contains(t, result, "Files checked:")
	assert.Contains(t, result, "10")
	assert.Contains(t, result, "Files with issues:")
	assert.Contains(t, result, "3")
	assert.Contains(t, result, "Total issues:")
	assert.Contains(t, result, "15")
	assert.Contains(t, result, "Errors:")
	assert.Contains(t, result, "5")
	assert.Contains(t, result, "Warnings:")
}

func TestFormatSummary_NoIssues(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        5,
		FilesWithIssues:       0,
		DiagnosticsTotal:      0,
		DiagnosticsBySeverity: map[string]int{},
	}

	result := styles.FormatSummary(stats)

	assert.Contains(t, result, "Lint passed")
	assert.NotContains(t, result, "Files with issues:")
}

func TestFormatSummary_WithErrors(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        10,
		FilesWithIssues:       2,
		DiagnosticsTotal:      5,
		DiagnosticsBySeverity: map[string]int{"error": 2, "warning": 3},
	}

	result := styles.FormatSummary(stats)

	assert.Contains(t, result, "Lint failed with errors")
}

func TestFormatSummary_WarningsOnly(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        10,
		FilesWithIssues:       2,
		DiagnosticsTotal:      5,
		DiagnosticsBySeverity: map[string]int{"warning": 5},
	}

	result := styles.FormatSummary(stats)

	assert.Contains(t, result, "Lint completed with warnings")
}

func TestFormatSummary_WithModifiedFiles(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        10,
		FilesWithIssues:       2,
		FilesModified:         2,
		DiagnosticsTotal:      5,
		DiagnosticsBySeverity: map[string]int{"warning": 5},
	}

	result := styles.FormatSummary(stats)

	assert.Contains(t, result, "Files modified:")
	assert.Contains(t, result, "2")
}

func TestFormatSummary_InfoOnly(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        10,
		FilesWithIssues:       1,
		DiagnosticsTotal:      3,
		DiagnosticsBySeverity: map[string]int{"info": 3},
	}

	result := styles.FormatSummary(stats)

	assert.Contains(t, result, "Info:")
	assert.Contains(t, result, "3")
	// With only info-level issues, should show "Lint passed"
	assert.Contains(t, result, "Lint passed")
}

func TestFormatSummaryOneLine_NoIssues(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        5,
		FilesWithIssues:       0,
		DiagnosticsTotal:      0,
		DiagnosticsBySeverity: map[string]int{},
	}

	result := styles.FormatSummaryOneLine(stats)

	assert.Contains(t, result, "No issues found")
	assert.Contains(t, result, "5 files checked")
}

func TestFormatSummaryOneLine_WithIssues(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        10,
		FilesWithIssues:       3,
		DiagnosticsTotal:      12,
		DiagnosticsFixable:    8,
		DiagnosticsBySeverity: map[string]int{"error": 4, "warning": 8},
	}

	result := styles.FormatSummaryOneLine(stats)

	assert.Contains(t, result, "12 issues")
	assert.Contains(t, result, "4 errors")
	assert.Contains(t, result, "8 warnings")
	assert.Contains(t, result, "in 3 files")
	assert.Contains(t, result, "8 fixable")
}

func TestFormatSummaryOneLine_SingleIssue(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        1,
		FilesWithIssues:       1,
		DiagnosticsTotal:      1,
		DiagnosticsFixable:    1,
		DiagnosticsBySeverity: map[string]int{"warning": 1},
	}

	result := styles.FormatSummaryOneLine(stats)

	assert.Contains(t, result, "1 issue")
	assert.Contains(t, result, "in 1 file")
	assert.Contains(t, result, "1 fixable")
}

func TestFormatSummaryOneLine_WithModified(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        10,
		FilesWithIssues:       3,
		FilesModified:         2,
		DiagnosticsFixed:      7,
		DiagnosticsTotal:      5,
		DiagnosticsFixable:    5,
		DiagnosticsBySeverity: map[string]int{"warning": 5},
	}

	result := styles.FormatSummaryOneLine(stats)

	assert.Contains(t, result, "5 issues")
	assert.Contains(t, result, "7 fixes applied in 2 files")
}

func TestFormatSummaryOneLine_NoFixable(t *testing.T) {
	styles := pretty.NewStyles(false)

	stats := runner.Stats{
		FilesProcessed:        5,
		FilesWithIssues:       2,
		DiagnosticsTotal:      3,
		DiagnosticsFixable:    0,
		DiagnosticsBySeverity: map[string]int{"error": 3},
	}

	result := styles.FormatSummaryOneLine(stats)

	assert.Contains(t, result, "3 issues")
	assert.Contains(t, result, "3 errors")
	assert.NotContains(t, result, "fixable")
}

func TestFormatSummaryOneLine_ErroredFiles(t *testing.T) {
	styles := pretty.NewStyles(false)

	t.Run("without findings", func(t *testing.T) {
		stats := runner.Stats{
			FilesProcessed:        2,
			FilesErrored:          1,
			DiagnosticsBySeverity: map[string]int{},
		}

		result := styles.FormatSummaryOneLine(stats)

		assert.Contains(t, result, "1 file could not be linted")
		assert.Contains(t, result, "2 files checked")
		assert.NotContains(t, result, "No issues found")
	})

	t.Run("with findings", func(t *testing.T) {
		stats := runner.Stats{
			FilesProcessed:        1,
			FilesErrored:          2,
			FilesWithIssues:       1,
			DiagnosticsTotal:      1,
			DiagnosticsBySeverity: map[string]int{"warning": 1},
		}

		result := styles.FormatSummaryOneLine(stats)

		assert.Contains(t, result, "1 issue")
		assert.Contains(t, result, "2 files could not be linted")
	})
}

// TestFormatSummaryOneLine_DryRun covers fixes computed but not written:
// the summary says what would change instead of what was fixed.
func TestFormatSummaryOneLine_DryRun(t *testing.T) {
	styles := pretty.NewStyles(false)

	t.Run("no issues would remain", func(t *testing.T) {
		result := styles.FormatSummaryOneLine(runner.Stats{
			FilesProcessed:        1,
			DiagnosticsFixed:      1,
			FilesWithPendingFixes: 1,
			DiagnosticsBySeverity: map[string]int{},
		})

		assert.Equal(t, "No issues would remain (1 file checked), 1 fix would be applied in 1 file\n", result)
	})

	t.Run("issues would remain", func(t *testing.T) {
		result := styles.FormatSummaryOneLine(runner.Stats{
			FilesProcessed:        2,
			FilesWithIssues:       1,
			DiagnosticsTotal:      1,
			DiagnosticsFixed:      3,
			FilesWithPendingFixes: 2,
			DiagnosticsBySeverity: map[string]int{"warning": 1},
		})

		assert.Contains(t, result, "3 fixes would be applied in 2 files")
		assert.NotContains(t, result, "fixed in")
	})
}

func TestFormatSummaryOneLine_SkippedFixes(t *testing.T) {
	styles := pretty.NewStyles(false)

	result := styles.FormatSummaryOneLine(runner.Stats{
		FilesProcessed:        2,
		FilesWithIssues:       1,
		FilesSkipped:          1,
		DiagnosticsTotal:      1,
		DiagnosticsBySeverity: map[string]int{"warning": 1},
	})

	assert.Contains(t, result, "fixes not written to 1 file")

	result = styles.FormatSummaryOneLine(runner.Stats{
		FilesProcessed:        1,
		FilesSkipped:          1,
		DiagnosticsBySeverity: map[string]int{},
	})

	assert.Contains(t, result, "fixes not written to 1 file")
}

func TestFormatTableSummary_ErroredFiles(t *testing.T) {
	formatter := pretty.NewTableFormatter(pretty.NewStyles(false), 80, config.RuleFormatName)

	result := formatter.FormatTableSummary(runner.Stats{
		FilesProcessed:        1,
		FilesErrored:          1,
		DiagnosticsBySeverity: map[string]int{},
	}, "")

	assert.Contains(t, result, "1 file checked")
	assert.Contains(t, result, "1 file could not be linted")
}

func TestFormatSummaryOneLine_RuleFailures(t *testing.T) {
	styles := pretty.NewStyles(false)

	t.Run("without findings", func(t *testing.T) {
		result := styles.FormatSummaryOneLine(runner.Stats{
			FilesProcessed:        2,
			FilesWithRuleErrors:   1,
			RuleErrors:            1,
			DiagnosticsBySeverity: map[string]int{},
		})

		assert.Contains(t, result, "1 rule failure")
		assert.Contains(t, result, "2 files checked")
		assert.NotContains(t, result, "No issues found", "the findings may be incomplete")
		assert.NotContains(t, result, "could not be linted", "the files were read")
	})

	t.Run("with findings and a file that could not be linted", func(t *testing.T) {
		result := styles.FormatSummaryOneLine(runner.Stats{
			FilesProcessed:        2,
			FilesErrored:          1,
			FilesWithRuleErrors:   1,
			RuleErrors:            2,
			FilesWithIssues:       1,
			DiagnosticsTotal:      1,
			DiagnosticsBySeverity: map[string]int{"warning": 1},
		})

		assert.Contains(t, result, "1 issue")
		assert.Contains(t, result, "1 file could not be linted, 2 rule failures")
	})
}

func TestFormatTableSummary_RuleFailures(t *testing.T) {
	formatter := pretty.NewTableFormatter(pretty.NewStyles(false), 80, config.RuleFormatName)

	result := formatter.FormatTableSummary(runner.Stats{
		FilesProcessed:        1,
		FilesWithRuleErrors:   1,
		RuleErrors:            1,
		DiagnosticsBySeverity: map[string]int{},
	}, "")

	assert.Contains(t, result, "1 file checked")
	assert.Contains(t, result, "1 rule failure")
}

func TestFormatSummaryOneLine_SingleFileChecked(t *testing.T) {
	styles := pretty.NewStyles(false)

	result := styles.FormatSummaryOneLine(runner.Stats{FilesProcessed: 1, DiagnosticsBySeverity: map[string]int{}})

	assert.Contains(t, result, "(1 file checked)")
}
