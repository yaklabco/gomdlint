package cli

import "github.com/jamesainslie/gomdlint/pkg/runner"

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

// ExitCodeFromResult determines the exit code based on result and strict mode.
func ExitCodeFromResult(result *runner.Result, strict bool) int {
	if result == nil {
		return ExitSuccess
	}

	errors := result.Stats.DiagnosticsBySeverity["error"]
	warnings := result.Stats.DiagnosticsBySeverity["warning"]

	if errors > 0 {
		return ExitLintErrors
	}

	if strict && warnings > 0 {
		return ExitLintWarnings
	}

	return ExitSuccess
}
