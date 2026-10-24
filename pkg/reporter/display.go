package reporter

import (
	"github.com/yaklabco/gomdlint/internal/display"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// displayResult returns a copy of result in the form the text, table and JSON
// reporters print. File paths are relative to workDir when they lie inside it
// (see display.Path), and diagnostic columns count UTF-16 code units instead
// of the bytes lint uses internally, as markdownlint reports them (see
// display.Columns). result is not modified.
func displayResult(result *runner.Result, workDir string) *runner.Result {
	if result == nil {
		return nil
	}

	shown := *result
	shown.Files = make([]runner.FileOutcome, len(result.Files))
	for idx, file := range result.Files {
		shown.Files[idx] = displayOutcome(file, workDir)
	}

	return &shown
}

// displayOutcome returns a copy of file with a display path and UTF-16
// columns. The pipeline and lint results are copied rather than modified.
func displayOutcome(file runner.FileOutcome, workDir string) runner.FileOutcome {
	shownPath := display.Path(file.Path, workDir)

	shown := file
	shown.Path = shownPath
	if file.Result == nil || file.Result.FileResult == nil {
		return shown
	}

	pipelineResult := *file.Result
	fileResult := *file.Result.FileResult
	if file.Result.Diagnostics != nil {
		columns := display.NewColumns(fileResult.Snapshot)
		fileResult.Diagnostics = make([]lint.Diagnostic, len(file.Result.Diagnostics))
		for idx, diag := range file.Result.Diagnostics {
			fileResult.Diagnostics[idx] = displayDiagnostic(diag, columns, file.Path, shownPath, workDir)
		}
	}

	pipelineResult.FileResult = &fileResult
	shown.Result = &pipelineResult

	return shown
}

// displayDiagnostic returns diag with its path in display form and its
// columns converted from bytes to UTF-16 code units by columns, the start
// column including the code units into its character. filePath and
// shownPath are the outcome's path before and after conversion, reused for
// the common case of a diagnostic that names its own file.
func displayDiagnostic(
	diag lint.Diagnostic,
	columns *display.Columns,
	filePath, shownPath, workDir string,
) lint.Diagnostic {
	if diag.FilePath == "" || diag.FilePath == filePath {
		diag.FilePath = shownPath
	} else {
		diag.FilePath = display.Path(diag.FilePath, workDir)
	}

	start := mdast.Position{Line: diag.StartLine, Column: diag.StartColumn}
	diag.StartColumn = columns.ColumnWithin(start, diag.StartColumnUTF16Offset)
	diag.StartColumnUTF16Offset = 0 // Now part of StartColumn.
	diag.EndColumn = columns.Column(mdast.Position{Line: diag.EndLine, Column: diag.EndColumn})

	return diag
}

// sourceLine returns the text of a 1-based line for context display. The text
// matches the columns of displayResult: no line terminator and no byte order
// mark.
func sourceLine(snapshot *mdast.FileSnapshot, line int) string {
	return string(display.LineText(snapshot, line))
}
