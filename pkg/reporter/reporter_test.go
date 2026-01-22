package reporter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    reporter.Format
		wantErr bool
	}{
		{name: "empty defaults to text", input: "", want: reporter.FormatText},
		{name: "text", input: "text", want: reporter.FormatText},
		{name: "json", input: "json", want: reporter.FormatJSON},
		{name: "diff", input: "diff", want: reporter.FormatDiff},
		{name: "unknown format", input: "xml", wantErr: true},
		{name: "sarif", input: "sarif", want: reporter.FormatSARIF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reporter.ParseFormat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormat_IsValid(t *testing.T) {
	tests := []struct {
		format reporter.Format
		want   bool
	}{
		{reporter.FormatText, true},
		{reporter.FormatJSON, true},
		{reporter.FormatSARIF, true},
		{reporter.FormatDiff, true},
		{reporter.Format("unknown"), false},
		{reporter.Format(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.format.IsValid())
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		format  reporter.Format
		wantErr bool
	}{
		{name: "text reporter", format: reporter.FormatText},
		{name: "json reporter", format: reporter.FormatJSON},
		{name: "sarif reporter", format: reporter.FormatSARIF},
		{name: "diff reporter", format: reporter.FormatDiff},
		{name: "empty defaults to text", format: ""},
		{name: "unknown format", format: "xml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			opts := reporter.Options{
				Writer: &buf,
				Format: tt.format,
				Color:  "never",
			}

			rep, err := reporter.New(opts)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, rep)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, rep)
		})
	}
}

func TestTextReporter_NilResult(t *testing.T) {
	var buf bytes.Buffer
	rep := reporter.NewTextReporter(reporter.Options{
		Writer:      &buf,
		Color:       "never",
		ShowSummary: true,
	})

	count, err := rep.Report(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, buf.String(), "No files to check")
}

func TestTextReporter_EmptyResult(t *testing.T) {
	var buf bytes.Buffer
	rep := reporter.NewTextReporter(reporter.Options{
		Writer:      &buf,
		Color:       "never",
		ShowSummary: true,
	})

	result := &runner.Result{
		Files: []runner.FileOutcome{},
		Stats: runner.Stats{
			DiagnosticsBySeverity: make(map[string]int),
		},
	}

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestTextReporter_WithDiagnostics(t *testing.T) {
	var buf bytes.Buffer
	rep := reporter.NewTextReporter(reporter.Options{
		Writer:      &buf,
		Color:       "never",
		ShowSummary: true,
		ShowContext: false,
		GroupByFile: true,
	})

	result := createTestResult()

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	output := buf.String()
	assert.Contains(t, output, "test.md")
	assert.Contains(t, output, "MD001")
	assert.Contains(t, output, "error")
	assert.Contains(t, output, "2 issues") // One-line summary format
}

func TestJSONReporter_NilResult(t *testing.T) {
	var buf bytes.Buffer
	rep := reporter.NewJSONReporter(reporter.Options{
		Writer: &buf,
		Color:  "never",
	})

	count, err := rep.Report(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Should still produce valid JSON
	var output reporter.JSONOutput
	err = json.Unmarshal(buf.Bytes(), &output)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", output.Version)
	assert.Empty(t, output.Files)
}

func TestJSONReporter_WithDiagnostics(t *testing.T) {
	var buf bytes.Buffer
	rep := reporter.NewJSONReporter(reporter.Options{
		Writer: &buf,
		Color:  "never",
	})

	result := createTestResult()

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	var output reporter.JSONOutput
	err = json.Unmarshal(buf.Bytes(), &output)
	require.NoError(t, err)

	assert.Equal(t, "1.0.0", output.Version)
	assert.Len(t, output.Files, 1)
	assert.Len(t, output.Files[0].Diagnostics, 2)
	assert.Equal(t, 2, output.Summary.TotalIssues)
	assert.Equal(t, 1, output.Summary.FilesWithIssues)
}

func TestJSONReporter_Compact(t *testing.T) {
	var buf bytes.Buffer
	rep := reporter.NewJSONReporter(reporter.Options{
		Writer:  &buf,
		Color:   "never",
		Compact: true,
	})

	result := createTestResult()

	_, err := rep.Report(context.Background(), result)
	require.NoError(t, err)

	// Compact output should be a single line
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 1)
}

func TestDiffReporter_NilResult(t *testing.T) {
	var buf bytes.Buffer
	rep := reporter.NewDiffReporter(reporter.Options{
		Writer: &buf,
		Color:  "never",
	})

	count, err := rep.Report(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, buf.String())
}

func TestDiffReporter_NoDiffs(t *testing.T) {
	var buf bytes.Buffer
	rep := reporter.NewDiffReporter(reporter.Options{
		Writer: &buf,
		Color:  "never",
	})

	result := createTestResult()

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 0, count) // No diffs in test result
}

func TestDefaultOptions(t *testing.T) {
	opts := reporter.DefaultOptions()

	assert.NotNil(t, opts.Writer)
	assert.NotNil(t, opts.ErrorWriter)
	assert.Equal(t, reporter.FormatText, opts.Format)
	assert.Equal(t, "auto", opts.Color)
	assert.True(t, opts.ShowContext)
	assert.True(t, opts.ShowSummary)
	assert.True(t, opts.GroupByFile)
	assert.False(t, opts.Compact)
	assert.Equal(t, config.RuleFormatName, opts.RuleFormat)
}

func TestSARIFReporter_IncludesRuleName(t *testing.T) {
	var buf bytes.Buffer
	opts := reporter.DefaultOptions()
	opts.Writer = &buf

	rep := reporter.NewSARIFReporter(opts)

	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: "test.md",
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{
					Diagnostics: []lint.Diagnostic{{
						RuleID:    "MD009",
						RuleName:  "no-trailing-spaces",
						Message:   "Test",
						FilePath:  "test.md",
						StartLine: 1,
					}},
				},
			},
		}},
	}

	_, err := rep.Report(context.Background(), result)
	require.NoError(t, err)

	// SARIF should contain the rule name in the rule's name field
	output := buf.String()
	assert.Contains(t, output, "no-trailing-spaces")
	assert.Contains(t, output, "MD009")
}

func TestJSONReporter_IncludesRuleName(t *testing.T) {
	var buf bytes.Buffer
	opts := reporter.DefaultOptions()
	opts.Writer = &buf
	opts.Format = reporter.FormatJSON

	rep := reporter.NewJSONReporter(opts)

	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: "test.md",
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{
					Diagnostics: []lint.Diagnostic{{
						RuleID:    "MD009",
						RuleName:  "no-trailing-spaces",
						Message:   "Test",
						FilePath:  "test.md",
						StartLine: 1,
					}},
				},
			},
		}},
	}

	_, err := rep.Report(context.Background(), result)
	require.NoError(t, err)

	// JSON should contain both ruleId and ruleName
	assert.Contains(t, buf.String(), `"ruleId": "MD009"`)
	assert.Contains(t, buf.String(), `"ruleName": "no-trailing-spaces"`)
}

func TestTextReporter_RuleFormat(t *testing.T) {
	var buf bytes.Buffer
	opts := reporter.DefaultOptions()
	opts.Writer = &buf
	opts.RuleFormat = config.RuleFormatName
	opts.ShowContext = false
	opts.ShowSummary = false

	rep := reporter.NewTextReporter(opts)

	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: "test.md",
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{
					Diagnostics: []lint.Diagnostic{{
						RuleID:    "MD009",
						RuleName:  "no-trailing-spaces",
						Message:   "Trailing whitespace",
						Severity:  config.SeverityWarning,
						FilePath:  "test.md",
						StartLine: 1,
					}},
				},
			},
		}},
	}

	_, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "no-trailing-spaces")
	assert.NotContains(t, buf.String(), "MD009")
}

// createTestResult creates a test runner.Result with sample diagnostics.
func createTestResult() *runner.Result {
	return &runner.Result{
		Files: []runner.FileOutcome{
			{
				Path: "test.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{
							{
								RuleID:      "MD001",
								Message:     "Heading levels should only increment by one level at a time",
								Severity:    config.SeverityError,
								FilePath:    "test.md",
								StartLine:   5,
								StartColumn: 1,
								EndLine:     5,
								EndColumn:   15,
								Suggestion:  "Use ## instead of ###",
							},
							{
								RuleID:      "MD010",
								Message:     "Hard tabs found",
								Severity:    config.SeverityWarning,
								FilePath:    "test.md",
								StartLine:   10,
								StartColumn: 1,
								EndLine:     10,
								EndColumn:   5,
							},
						},
					},
				},
			},
		},
		Stats: runner.Stats{
			FilesDiscovered:       1,
			FilesProcessed:        1,
			FilesWithIssues:       1,
			DiagnosticsTotal:      2,
			DiagnosticsBySeverity: map[string]int{"error": 1, "warning": 1},
		},
	}
}
