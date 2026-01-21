// Package logging provides a structured logging wrapper around charmbracelet/log.
package logging

// Field name constants for structured logging.
// Using constants prevents typos and enables IDE autocomplete.
const (
	// Common fields.
	FieldError      = "error"
	FieldPath       = "path"
	FieldPaths      = "paths"
	FieldFiles      = "files"
	FieldInput      = "input"
	FieldOutput     = "output"
	FieldWorkingDir = "working_dir"

	// Configuration fields.
	FieldFlavor = "flavor"
	FieldFix    = "fix"
	FieldDryRun = "dry_run"
	FieldJobs   = "jobs"

	// Statistics fields.
	FieldFilesDiscovered  = "files_discovered"
	FieldFilesProcessed   = "files_processed"
	FieldFilesWithIssues  = "files_with_issues"
	FieldDiagnosticsTotal = "diagnostics_total"
	FieldFilesModified    = "files_modified"

	// Version fields.
	FieldVersion = "version"
	FieldCommit  = "commit"
	FieldBuilt   = "built"

	// Rule fields.
	FieldName        = "name"
	FieldSeverity    = "severity"
	FieldFixable     = "fixable"
	FieldDescription = "description"
)
