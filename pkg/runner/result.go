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

	// Exhausted is true if the fix loop exhausted MaxFixPasses with edits remaining.
	Exhausted bool

	// RemainingEdits is the count of edits not applied due to exhaustion.
	RemainingEdits int

	// FixPasses is the number of fix passes performed.
	FixPasses int

	// Oscillated is true if the fix loop stopped because the remaining fixes
	// would only recreate content from an earlier pass.
	Oscillated bool

	// RuleErrors lists the rules that failed on this file, sorted by rule
	// ID: errors returned by a rule, recovered panics (errors.Is
	// lint.ErrRulePanic) and invalid fix edits (errors.Is lint.ErrInvalidFix).
	// The file was processed, but its findings or fixes may be incomplete,
	// so the file counts as failed (see Failed).
	RuleErrors []*lint.RuleError

	// SkippedEdits lists the fix edits of the final lint pass that were not
	// applied because their fix conflicted with another fix. It is empty
	// unless the fix loop stopped early (Exhausted or Oscillated).
	SkippedEdits []lint.SkippedEdit
}

// Failed reports whether the file could not be linted completely, which
// fails the run: it could not be processed at all (Error, counted in
// Stats.FilesErrored), or a rule failed on it (RuleErrors, counted in
// Stats.FilesWithRuleErrors).
//
// Every per-file failure belongs here, so that new kinds fail the run
// without changes elsewhere.
func (o *FileOutcome) Failed() bool {
	return o.Error != nil || len(o.RuleErrors) > 0
}

// Stats captures aggregate information about a run.
type Stats struct {
	// FilesDiscovered is the total number of files found during discovery.
	FilesDiscovered int

	// FilesProcessed is the number of files successfully processed.
	FilesProcessed int

	// FilesSkipped is the number of files skipped (e.g., due to concurrent modification).
	FilesSkipped int

	// FilesErrored is the number of files that could not be processed at
	// all, such as files that could not be read or written. Files on which
	// a rule failed are counted in FilesWithRuleErrors instead.
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

	// FilesWithPendingFixes is the number of files whose fixes were
	// computed but not written, as in a dry run.
	FilesWithPendingFixes int

	// DiagnosticsFixed is the number of fix edits applied across all files,
	// in memory for a dry run. The pipeline reports edits, not the
	// diagnostics they fixed, so one diagnostic may count more than once.
	DiagnosticsFixed int

	// FilesWithRuleErrors is the number of processed files on which at least
	// one rule failed. These files are not counted in FilesErrored.
	FilesWithRuleErrors int

	// RuleErrors is the total number of rule failures, at most one per rule
	// and file.
	RuleErrors int

	// EditsSkipped is the total number of fix edits left unapplied because
	// of conflicts in the final lint pass.
	EditsSkipped int
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

// HasRuleErrors reports whether any rule failed on any file.
func (r *Result) HasRuleErrors() bool {
	if r == nil {
		return false
	}
	return r.Stats.FilesWithRuleErrors > 0
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
	}

	if outcome.Result == nil {
		return
	}

	r.Stats.FilesProcessed++

	if len(outcome.RuleErrors) > 0 {
		r.Stats.FilesWithRuleErrors++
		r.Stats.RuleErrors += len(outcome.RuleErrors)
	}
	r.Stats.EditsSkipped += len(outcome.SkippedEdits)

	if outcome.Result.Skipped {
		r.Stats.FilesSkipped++
	}

	// Fixes count once written, or as pending in a dry run. A skipped file's
	// fixes never reached the disk, so they do not count.
	switch {
	case outcome.Result.Written:
		r.Stats.FilesModified++
		r.Stats.DiagnosticsFixed += outcome.Result.TotalEditsApplied
	case outcome.Result.Modified && !outcome.Result.Skipped:
		r.Stats.FilesWithPendingFixes++
		r.Stats.DiagnosticsFixed += outcome.Result.TotalEditsApplied
	}

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
