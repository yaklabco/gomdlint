package analysis

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
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

func TestAnalyze_ReportsCharacterColumns(t *testing.T) {
	t.Parallel()

	content := []byte("# Title\n\nÄÖÜ trail   \n")
	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: "doc.md",
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{
					Snapshot: mdast.NewFileSnapshot("doc.md", content),
					Diagnostics: []lint.Diagnostic{{
						RuleID:      "MD009",
						Severity:    config.SeverityWarning,
						StartLine:   3,
						StartColumn: 13,
						EndLine:     3,
						EndColumn:   16,
						FixEdits:    []fix.TextEdit{{StartOffset: 21, EndOffset: 24}},
					}},
				},
			},
		}},
	}

	report := Analyze(result, DefaultOptions())

	require.Len(t, report.Diagnostics, 1)
	entry := report.Diagnostics[0]
	assert.Equal(t, 10, entry.StartColumn, "start column counts characters, not bytes")
	assert.Equal(t, 13, entry.EndColumn, "end column counts characters, not bytes")
	require.Len(t, entry.Fixes, 1)
	assert.Equal(t, 21, entry.Fixes[0].StartOffset, "fix offsets stay byte offsets into the file")
	assert.Equal(t, 24, entry.Fixes[0].EndOffset)
}

func TestAnalyze_PathsRelativeOnlyInsideWorkingDir(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	inside := filepath.Join(workDir, "docs", "in.md")
	outside := filepath.Join(t.TempDir(), "out.md")

	result := &runner.Result{
		Files: []runner.FileOutcome{
			{
				Path: inside,
				Result: &lint.PipelineResult{FileResult: &lint.FileResult{
					Diagnostics: []lint.Diagnostic{{RuleID: "MD001", FilePath: inside}},
				}},
			},
			{
				Path: outside,
				Result: &lint.PipelineResult{FileResult: &lint.FileResult{
					Diagnostics: []lint.Diagnostic{{RuleID: "MD001", FilePath: outside}},
				}},
			},
		},
	}

	opts := DefaultOptions()
	opts.WorkingDir = workDir
	opts.SortBy = SortByAlpha

	report := Analyze(result, opts)

	paths := make([]string, 0, len(report.ByFile))
	for _, file := range report.ByFile {
		paths = append(paths, file.Path)
	}
	assert.ElementsMatch(t, []string{filepath.Join("docs", "in.md"), outside}, paths)

	require.Len(t, report.Diagnostics, 2)
	assert.Equal(t, filepath.Join("docs", "in.md"), report.Diagnostics[0].FilePath)
	assert.Equal(t, outside, report.Diagnostics[1].FilePath, "files outside the working directory keep absolute paths")
}

func TestAnalyze_CountsFilesThatCouldNotBeLintedAndRuleFailures(t *testing.T) {
	t.Parallel()

	result := &runner.Result{
		Files: []runner.FileOutcome{
			{Path: "unreadable.md", Error: errors.New("permission denied")},
			{
				Path: "failed.md",
				Result: &lint.PipelineResult{FileResult: &lint.FileResult{RuleErrors: map[string]error{
					"MD001": errors.New("boom"),
					"MD009": &lint.RuleError{RuleID: "MD009", Err: lint.ErrRulePanic},
					"MD010": nil,
				}}},
			},
		},
	}

	report := Analyze(result, DefaultOptions())

	assert.Equal(t, 2, report.Totals.Files)
	assert.Equal(t, 1, report.Totals.FilesErrored)
	assert.Equal(t, 2, report.Totals.RuleErrors, "rules recorded without an error do not count")
	assert.Zero(t, report.Totals.Issues)
}
