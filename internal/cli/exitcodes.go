package cli

import (
	"errors"
	"fmt"

	"github.com/yaklabco/gomdlint/pkg/runner"
)

// Exit codes for gomdlint.
const (
	// ExitSuccess indicates successful execution with no issues.
	ExitSuccess = 0

	// ExitLintErrors indicates lint completed but found errors.
	ExitLintErrors = 1

	// ExitLintWarnings indicates lint completed but found warnings (when strict mode).
	ExitLintWarnings = 2

	// ExitInvalidUsage indicates invalid command-line usage.
	ExitInvalidUsage = 64

	// ExitConfigError indicates configuration file errors.
	ExitConfigError = 65

	// ExitInternalError indicates an internal error.
	ExitInternalError = 70

	// ExitIOError indicates file I/O errors.
	ExitIOError = 74
)

// exitCodeHelp documents the exit codes in command help.
const exitCodeHelp = `Exit codes:
  0   no errors (warnings alone exit 0 unless --strict)
  1   lint errors found
  2   warnings found with --strict
  64  invalid command-line usage
  65  invalid configuration
  70  internal error or interrupted run
  74  a file could not be read or written`

// ErrLintIssuesFound is returned when lint issues are found.
var ErrLintIssuesFound = errors.New("lint issues found")

// ExitError carries a process exit code out of a command.
type ExitError struct {
	// Code is the process exit code.
	Code int

	// Err is the underlying error.
	Err error
}

// Error implements the error interface.
func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *ExitError) Unwrap() error {
	return e.Err
}

// withExitCode attaches an exit code to err unless err already carries one.
func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	return &ExitError{Code: code, Err: err}
}

// usageError reports invalid command-line usage.
func usageError(format string, args ...any) error {
	return &ExitError{Code: ExitInvalidUsage, Err: fmt.Errorf(format, args...)}
}

// ExitCode maps an error returned by command execution to a process exit code.
//
// Command implementations attach codes to every error they return. An error
// without a code therefore comes from Cobra's own argument and flag handling,
// which makes it a usage error.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return ExitInvalidUsage
}

// ExitCodeFromResult determines the exit code based on result and strict mode.
//
// Files that could not be linted outrank findings, because the findings of an
// incomplete run understate the problems. Error findings outrank warnings, and
// warnings only fail the run in strict mode.
func ExitCodeFromResult(result *runner.Result, strict bool) int {
	if result == nil {
		return ExitSuccess
	}

	if result.Stats.FilesErrored > 0 {
		return ExitIOError
	}

	if result.Stats.DiagnosticsBySeverity["error"] > 0 {
		return ExitLintErrors
	}

	if strict && result.Stats.DiagnosticsBySeverity["warning"] > 0 {
		return ExitLintWarnings
	}

	return ExitSuccess
}
