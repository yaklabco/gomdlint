package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

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
// Path is relative to the working directory when the file lies inside it,
// and absolute otherwise.
type JSONFileResult struct {
	Path        string           `json:"path"`
	Diagnostics []JSONDiagnostic `json:"diagnostics"`
	Modified    bool             `json:"modified,omitempty"`

	// Error is set when the file could not be linted at all, for example
	// because it could not be read.
	Error string `json:"error,omitempty"`

	// RuleErrors lists the rules that failed on the file, in rule ID order.
	// The file was linted, but its findings or fixes may be incomplete.
	RuleErrors []JSONRuleError `json:"ruleErrors,omitempty"`

	// Notices lists the problems with the file's markdownlint comments that
	// change neither its findings nor the exit code, in the order of the
	// comments and their words. The key is absent for a file without
	// notices.
	Notices []JSONNotice `json:"notices,omitempty"`
}

// JSONNotice describes a problem with a markdownlint comment that changes
// neither the findings nor the exit code: a word of the comment, or a key
// of a markdownlint-configure-file comment, that names no rule or tag, which
// the comment ignores, as markdownlint does; a word that is one of
// gomdlint's deprecated tags, which still work but will stop working in a
// later release; an option in a configure-file setting that its rules do
// not read; or a configure-file comment that markdownlint ignores because
// it holds no object of settings.
type JSONNotice struct {
	// Line is the 1-based line on which the comment starts.
	Line int `json:"line"`

	// Kind is "unknown-rule" for a word or key that names no rule or tag,
	// "deprecated-tag" for a deprecated tag, "unknown-option" for an option
	// that the rules of a configure-file setting do not read, and
	// "invalid-configuration" for a configure-file comment that holds no
	// object of settings.
	Kind string `json:"kind"`

	// Directive is the kind of comment, such as "markdownlint-disable".
	Directive string `json:"directive"`

	// Name is the word, key or option as the comment writes it. The key is
	// absent for an "invalid-configuration" notice.
	Name string `json:"name,omitempty"`

	// Suggestions are, for an unknown word, the rules and tags it most
	// likely means, any one of which may be meant, and, for an unknown
	// option of one rule, the option names it most likely means. The key is
	// absent when none is close enough. Rules are named as markdownlint
	// names them, such as ul-style for unordered-list-style.
	Suggestions []string `json:"suggestions,omitempty"`

	// Rules are, for a deprecated tag, the names of the rules it covers,
	// which the comment can name instead, and, for an unknown option, the
	// rules whose setting holds it, none of which reads it, named as
	// markdownlint names them. The key is absent otherwise.
	Rules []string `json:"rules,omitempty"`

	// Message describes the problem, starting with the kind of comment.
	Message string `json:"message"`
}

// JSONRuleError describes a rule that failed on a file: it returned an
// error, panicked, or proposed an invalid fix.
type JSONRuleError struct {
	RuleID   string `json:"ruleId"`
	RuleName string `json:"ruleName,omitempty"`
	Message  string `json:"message"`
}

// JSONDiagnostic represents a single diagnostic.
// Lines are 1-based. Columns are 1-based character columns (Unicode code
// points), as markdownlint reports them, and EndColumn is exclusive.
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
// StartOffset and EndOffset are byte offsets into the file content.
type JSONFix struct {
	StartOffset int    `json:"startOffset"`
	EndOffset   int    `json:"endOffset"`
	NewText     string `json:"newText"`
}

// JSONSummary contains aggregate statistics.
type JSONSummary struct {
	FilesChecked    int `json:"filesChecked"`
	FilesWithIssues int `json:"filesWithIssues"`
	FilesModified   int `json:"filesModified"`

	// FilesErrored counts the files that could not be linted at all (the
	// files with an error). Files on which a rule failed count in
	// RuleErrors instead.
	FilesErrored int `json:"filesErrored"`

	// RuleErrors counts the rules that failed, once per rule and file.
	RuleErrors int `json:"ruleErrors"`

	TotalIssues int            `json:"totalIssues"`
	BySeverity  map[string]int `json:"bySeverity"`
}

// JSONReporter formats results as JSON.
type JSONReporter struct {
	opts Options
	out  io.Writer
}

// NewJSONReporter creates a new JSON reporter.
func NewJSONReporter(opts Options) *JSONReporter {
	return &JSONReporter{
		opts: opts,
		out:  opts.Writer,
	}
}

// Report implements Reporter.
func (r *JSONReporter) Report(_ context.Context, result *runner.Result) (int, error) {
	output := r.buildOutput(result)

	encoder := json.NewEncoder(r.out)
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

	shown := displayResult(result, r.opts.WorkingDir)

	// Pre-allocate if we have files
	if len(shown.Files) > 0 {
		output.Files = make([]JSONFileResult, 0, len(shown.Files))
	}

	for _, file := range shown.Files {
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
				// The same failures SARIF reports as notifications.
				for _, ruleErr := range file.Result.RuleErrorList() {
					fileResult.RuleErrors = append(fileResult.RuleErrors, JSONRuleError{
						RuleID:   ruleErr.RuleID,
						RuleName: ruleErr.RuleName,
						Message:  ruleFailureCause(ruleErr).Error(),
					})
					output.Summary.RuleErrors++
				}

				// The same notices SARIF reports as configuration notifications.
				for _, notice := range file.Notices {
					fileResult.Notices = append(fileResult.Notices, JSONNotice{
						Line:        notice.Line,
						Kind:        string(notice.Kind),
						Directive:   notice.Directive,
						Name:        notice.Name,
						Suggestions: notice.Suggestions,
						Rules:       notice.Rules,
						Message:     notice.Message,
					})
				}

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
