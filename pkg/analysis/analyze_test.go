package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

func TestAnalyze_EmptyResult(t *testing.T) {
	t.Parallel()

	result := &runner.Result{
		Files: []runner.FileOutcome{},
	}

	report := Analyze(result, DefaultOptions())

	require.NotNil(t, report)
	assert.Equal(t, 0, report.Totals.Issues)
	assert.Empty(t, report.Diagnostics)
	assert.Empty(t, report.ByFile)
	assert.Empty(t, report.ByRule)
}

func TestAnalyze_CountsTotals(t *testing.T) {
	t.Parallel()

	result := &runner.Result{
		Files: []runner.FileOutcome{
			{
				Path: "file1.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{
							{RuleID: "MD001", RuleName: "heading-increment", Severity: config.SeverityError},
							{RuleID: "MD001", RuleName: "heading-increment", Severity: config.SeverityError},
							{RuleID: "MD009", RuleName: "no-trailing-spaces", Severity: config.SeverityWarning},
						},
					},
				},
			},
			{
				Path: "file2.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{
							{RuleID: "MD009", RuleName: "no-trailing-spaces", Severity: config.SeverityWarning},
						},
					},
				},
			},
		},
	}

	report := Analyze(result, DefaultOptions())

	assert.Equal(t, 4, report.Totals.Issues)
	assert.Equal(t, 2, report.Totals.Errors)
	assert.Equal(t, 2, report.Totals.Warnings)
	assert.Equal(t, 2, report.Totals.Files)
	assert.Equal(t, 2, report.Totals.FilesWithIssues)
}

func TestAnalyze_GroupsByRule(t *testing.T) {
	t.Parallel()

	result := &runner.Result{
		Files: []runner.FileOutcome{
			{
				Path: "file1.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{
							{RuleID: "MD001", RuleName: "heading-increment", Severity: config.SeverityError},
							{RuleID: "MD009", RuleName: "no-trailing-spaces", Severity: config.SeverityWarning, FixEdits: []fix.TextEdit{{}}},
						},
					},
				},
			},
			{
				Path: "file2.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{
							{RuleID: "MD009", RuleName: "no-trailing-spaces", Severity: config.SeverityWarning, FixEdits: []fix.TextEdit{{}}},
						},
					},
				},
			},
		},
	}

	report := Analyze(result, DefaultOptions())

	require.Len(t, report.ByRule, 2)

	// Sorted by count descending, MD009 has 2, MD001 has 1
	assert.Equal(t, "MD009", report.ByRule[0].RuleID)
	assert.Equal(t, 2, report.ByRule[0].Issues)
	assert.True(t, report.ByRule[0].Fixable)
	assert.ElementsMatch(t, []string{"file1.md", "file2.md"}, report.ByRule[0].Files)

	assert.Equal(t, "MD001", report.ByRule[1].RuleID)
	assert.Equal(t, 1, report.ByRule[1].Issues)
	assert.False(t, report.ByRule[1].Fixable)
}

func TestAnalyze_GroupsByFile(t *testing.T) {
	t.Parallel()

	result := &runner.Result{
		Files: []runner.FileOutcome{
			{
				Path: "a.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{
							{RuleID: "MD001", Severity: config.SeverityError},
						},
					},
				},
			},
			{
				Path: "b.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{
							{RuleID: "MD001", Severity: config.SeverityError},
							{RuleID: "MD009", Severity: config.SeverityWarning},
							{RuleID: "MD010", Severity: config.SeverityWarning},
						},
					},
				},
			},
		},
	}

	report := Analyze(result, DefaultOptions())

	require.Len(t, report.ByFile, 2)

	// Sorted by count descending, b.md has 3, a.md has 1
	assert.Equal(t, "b.md", report.ByFile[0].Path)
	assert.Equal(t, 3, report.ByFile[0].Issues)
	assert.Equal(t, 1, report.ByFile[0].Errors)
	assert.Equal(t, 2, report.ByFile[0].Warnings)

	assert.Equal(t, "a.md", report.ByFile[1].Path)
	assert.Equal(t, 1, report.ByFile[1].Issues)
}

func TestAnalyze_SortByAlpha(t *testing.T) {
	t.Parallel()

	result := &runner.Result{
		Files: []runner.FileOutcome{
			{
				Path: "z.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{{RuleID: "MD001"}},
					},
				},
			},
			{
				Path: "a.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{{RuleID: "MD001"}, {RuleID: "MD001"}},
					},
				},
			},
		},
	}

	opts := DefaultOptions()
	opts.SortBy = SortByAlpha

	report := Analyze(result, opts)

	require.Len(t, report.ByFile, 2)
	assert.Equal(t, "a.md", report.ByFile[0].Path)
	assert.Equal(t, "z.md", report.ByFile[1].Path)
}

func TestAnalyze_ExcludeViews(t *testing.T) {
	t.Parallel()

	result := &runner.Result{
		Files: []runner.FileOutcome{
			{
				Path: "file.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{{RuleID: "MD001"}},
					},
				},
			},
		},
	}

	opts := Options{
		IncludeDiagnostics: false,
		IncludeByFile:      false,
		IncludeByRule:      true,
		SortBy:             SortByCount,
		SortDesc:           true,
	}

	report := Analyze(result, opts)

	assert.Empty(t, report.Diagnostics, "diagnostics should be excluded")
	assert.Empty(t, report.ByFile, "byFile should be excluded")
	assert.NotEmpty(t, report.ByRule, "byRule should be included")
	assert.Equal(t, 1, report.Totals.Issues, "totals always computed")
}
