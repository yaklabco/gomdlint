package reporter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaklabco/gomdlint/pkg/runner"
)

// Severity string constants.
const (
	severityWarning = "warning"
)

// JSONOutput is the top-level JSON structure.
type JSONOutput struct {
	Version string           `json:"version"`
	Files   []JSONFileResult `json:"files"`
	Summary JSONSummary      `json:"summary"`
}

// JSONFileResult represents a single file's results.
type JSONFileResult struct {
	Path        string           `json:"path"`
	Diagnostics []JSONDiagnostic `json:"diagnostics"`
	Modified    bool             `json:"modified,omitempty"`
	Error       string           `json:"error,omitempty"`
}

// JSONDiagnostic represents a single diagnostic.
type JSONDiagnostic struct {
	RuleID      string    `json:"ruleId"`
	RuleName    string    `json:"ruleName"`
	Severity    string    `json:"severity"`
	Message     string    `json:"message"`
	StartLine   int       `json:"startLine"`
	StartColumn int       `json:"startColumn"`
	EndLine     int       `json:"endLine"`
	EndColumn   int       `json:"endColumn"`
	Suggestion  string    `json:"suggestion,omitempty"`
	Fixable     bool      `json:"fixable"`
	Fixes       []JSONFix `json:"fixes,omitempty"`
}

// JSONFix represents a proposed fix.
type JSONFix struct {
	StartOffset int    `json:"startOffset"`
	EndOffset   int    `json:"endOffset"`
	NewText     string `json:"newText"`
}

// JSONSummary contains aggregate statistics.
type JSONSummary struct {
	FilesChecked    int            `json:"filesChecked"`
	FilesWithIssues int            `json:"filesWithIssues"`
	FilesModified   int            `json:"filesModified"`
	FilesErrored    int            `json:"filesErrored"`
	TotalIssues     int            `json:"totalIssues"`
	BySeverity      map[string]int `json:"bySeverity"`
}

// JSONReporter formats results as JSON.
type JSONReporter struct {
	opts Options
	bw   *bufio.Writer
}

// NewJSONReporter creates a new JSON reporter.
func NewJSONReporter(opts Options) *JSONReporter {
	return &JSONReporter{
		opts: opts,
		bw:   bufio.NewWriterSize(opts.Writer, bufWriterSize),
	}
}

// Report implements Reporter.
func (r *JSONReporter) Report(_ context.Context, result *runner.Result) (_ int, err error) {
	defer func() {
		if flushErr := r.bw.Flush(); err == nil {
			err = flushErr
		}
	}()

	output := r.buildOutput(result)

	encoder := json.NewEncoder(r.bw)
	if !r.opts.Compact {
		encoder.SetIndent("", "  ")
	}

	if err := encoder.Encode(output); err != nil {
		return 0, fmt.Errorf("encode JSON: %w", err)
	}

	return output.Summary.TotalIssues, nil
}

func (r *JSONReporter) buildOutput(result *runner.Result) *JSONOutput {
	output := &JSONOutput{
		Version: "1.0.0",
		Files:   make([]JSONFileResult, 0),
		Summary: JSONSummary{
			BySeverity: make(map[string]int),
		},
	}

	if result == nil {
		return output
	}

	// Pre-allocate if we have files
	if len(result.Files) > 0 {
		output.Files = make([]JSONFileResult, 0, len(result.Files))
	}

	for _, file := range result.Files {
		fileResult := JSONFileResult{
			Path:        file.Path,
			Diagnostics: make([]JSONDiagnostic, 0),
		}

		if file.Error != nil {
			fileResult.Error = file.Error.Error()
			output.Summary.FilesErrored++
		}

		if file.Result != nil {
			fileResult.Modified = file.Result.Written

			if file.Result.FileResult != nil {
				for _, diag := range file.Result.Diagnostics {
					jsonDiag := JSONDiagnostic{
						RuleID:      diag.RuleID,
						RuleName:    diag.RuleName,
						Severity:    string(diag.Severity),
						Message:     diag.Message,
						StartLine:   diag.StartLine,
						StartColumn: diag.StartColumn,
						EndLine:     diag.EndLine,
						EndColumn:   diag.EndColumn,
						Suggestion:  diag.Suggestion,
						Fixable:     len(diag.FixEdits) > 0,
					}

					for _, edit := range diag.FixEdits {
						jsonDiag.Fixes = append(jsonDiag.Fixes, JSONFix{
							StartOffset: edit.StartOffset,
							EndOffset:   edit.EndOffset,
							NewText:     edit.NewText,
						})
					}

					fileResult.Diagnostics = append(fileResult.Diagnostics, jsonDiag)
					output.Summary.TotalIssues++

					severity := string(diag.Severity)
					if severity == "" {
						severity = severityWarning
					}
					output.Summary.BySeverity[severity]++
				}
			}
		}

		if len(fileResult.Diagnostics) > 0 {
			output.Summary.FilesWithIssues++
		}
		if fileResult.Modified {
			output.Summary.FilesModified++
		}

		output.Files = append(output.Files, fileResult)
		output.Summary.FilesChecked++
	}

	return output
}
