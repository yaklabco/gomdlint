package runner

import "github.com/yaklabco/gomdlint/pkg/lint"

// FileOutcome wraps PipelineResult with resolved path metadata.
type FileOutcome struct {
	// Path is the file path that was processed.
	Path string

	// Result contains the pipeline result for this file.
	// May be nil if the file encountered an error during processing.
	Result *lint.PipelineResult

	// Error is set if the file could not be processed.
	Error error
}

// Stats captures aggregate information about a run.
type Stats struct {
	// FilesDiscovered is the total number of files found during discovery.
	FilesDiscovered int

	// FilesProcessed is the number of files successfully processed.
	FilesProcessed int

	// FilesSkipped is the number of files skipped (e.g., due to concurrent modification).
	FilesSkipped int

	// FilesErrored is the number of files that encountered errors.
	FilesErrored int

	// DiagnosticsTotal is the total number of diagnostics across all files.
	DiagnosticsTotal int

	// DiagnosticsFixable is the number of diagnostics that have auto-fixes.
	DiagnosticsFixable int

	// DiagnosticsBySeverity maps severity levels to counts.
	DiagnosticsBySeverity map[string]int

	// FilesWithIssues is the number of files with at least one diagnostic.
	FilesWithIssues int

	// FilesModified is the number of files that were modified by fixes.
	FilesModified int

	// DiagnosticsFixed is the total number of issues fixed across all files.
	DiagnosticsFixed int
}

// Result is the overall runner result.
type Result struct {
	// Files contains the outcome for each processed file.
	// Files are ordered deterministically (by path).
	Files []FileOutcome

	// Stats contains aggregate statistics for the run.
	Stats Stats

	// Errors contains any non-file-specific errors encountered.
	Errors []error
}

// HasFailures reports whether any diagnostics with error severity occurred.
func (r *Result) HasFailures() bool {
	if r == nil {
		return false
	}
	return r.Stats.DiagnosticsBySeverity["error"] > 0
}

// HasIssues reports whether any diagnostics were found.
func (r *Result) HasIssues() bool {
	if r == nil {
		return false
	}
	return r.Stats.DiagnosticsTotal > 0
}

// newStats creates a new Stats with initialized maps.
func newStats() Stats {
	return Stats{
		DiagnosticsBySeverity: make(map[string]int),
	}
}

// accumulate updates the result with a file outcome.
func (r *Result) accumulate(outcome FileOutcome) {
	r.Files = append(r.Files, outcome)

	if outcome.Error != nil {
		r.Stats.FilesErrored++
		return
	}

	if outcome.Result == nil {
		return
	}

	r.Stats.FilesProcessed++

	if outcome.Result.Skipped {
		r.Stats.FilesSkipped++
	}

	if outcome.Result.Written {
		r.Stats.FilesModified++
	}

	// Track total edits applied (issues fixed).
	r.Stats.DiagnosticsFixed += outcome.Result.TotalEditsApplied

	if outcome.Result.FileResult != nil {
		diagCount := len(outcome.Result.Diagnostics)
		r.Stats.DiagnosticsTotal += diagCount
		r.Stats.DiagnosticsFixable += outcome.Result.FixableCount()

		if diagCount > 0 {
			r.Stats.FilesWithIssues++
		}

		for _, diag := range outcome.Result.Diagnostics {
			severity := string(diag.Severity)
			if severity == "" {
				severity = "warning"
			}
			r.Stats.DiagnosticsBySeverity[severity]++
		}
	}
}
