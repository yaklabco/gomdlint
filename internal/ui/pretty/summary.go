package pretty

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/runner"
)

const (
	summaryDividerWidth = 40
	wordFile            = "file"
	wordFiles           = "files"
)

// filesNoun returns "file" or "files" to agree with count.
func filesNoun(count int) string {
	if count == 1 {
		return wordFile
	}
	return wordFiles
}

// formatErrored describes the files a run could not lint.
func (s *Styles) formatErrored(stats runner.Stats) string {
	return s.Failure.Render(fmt.Sprintf("%d %s could not be linted", stats.FilesErrored, filesNoun(stats.FilesErrored)))
}

// FormatSummaryOneLine formats run statistics as a single line.
// Example: "12 issues (8 errors, 4 warnings) in 3 files, 6 fixable".
func (s *Styles) FormatSummaryOneLine(stats runner.Stats) string {
	checked := fmt.Sprintf("%d %s checked", stats.FilesProcessed, filesNoun(stats.FilesProcessed))

	if stats.DiagnosticsTotal == 0 && stats.FilesErrored > 0 {
		return s.formatErrored(stats) + s.Dim.Render(" ("+checked+", no issues found)") + "\n"
	}

	if stats.DiagnosticsTotal == 0 {
		head := "No issues found"
		if stats.FilesWithPendingFixes > 0 {
			// A dry run: the issues are still in the files.
			head = "No issues would remain"
		}
		msg := s.Success.Render(head) + s.Dim.Render(" ("+checked+")")
		// Show fixes applied even when no issues remain
		if stats.DiagnosticsFixed > 0 {
			msg += ", " + s.formatFixed(stats)
		}
		return msg + "\n"
	}

	var parts []string

	// Total issues
	issueWord := "issues"
	if stats.DiagnosticsTotal == 1 {
		issueWord = "issue"
	}

	// Build severity breakdown
	var severityParts []string
	if errors := stats.DiagnosticsBySeverity["error"]; errors > 0 {
		severityParts = append(severityParts, s.Error.Render(fmt.Sprintf("%d errors", errors)))
	}
	if warnings := stats.DiagnosticsBySeverity["warning"]; warnings > 0 {
		severityParts = append(severityParts, s.Warning.Render(fmt.Sprintf("%d warnings", warnings)))
	}
	if infos := stats.DiagnosticsBySeverity["info"]; infos > 0 {
		severityParts = append(severityParts, s.Info.Render(fmt.Sprintf("%d info", infos)))
	}

	// Main count with severity breakdown
	if len(severityParts) > 0 {
		parts = append(parts, fmt.Sprintf("%d %s (%s)", stats.DiagnosticsTotal, issueWord, strings.Join(severityParts, ", ")))
	} else {
		parts = append(parts, fmt.Sprintf("%d %s", stats.DiagnosticsTotal, issueWord))
	}

	// Files with issues
	fileWord := wordFiles
	if stats.FilesWithIssues == 1 {
		fileWord = wordFile
	}
	parts = append(parts, fmt.Sprintf("in %d %s", stats.FilesWithIssues, fileWord))

	// Fixable count
	if stats.DiagnosticsFixable > 0 {
		parts = append(parts, s.Success.Render(fmt.Sprintf("%d fixable", stats.DiagnosticsFixable)))
	}

	// Issues fixed (if any)
	if stats.DiagnosticsFixed > 0 {
		parts = append(parts, s.formatFixed(stats))
	}

	if stats.FilesErrored > 0 {
		parts = append(parts, s.formatErrored(stats))
	}

	return strings.Join(parts, ", ") + "\n"
}

// formatFixed describes the fixes of a run: those written as applied, and
// those a dry run computed without writing as fixes that would be applied.
func (s *Styles) formatFixed(stats runner.Stats) string {
	noun := "fixes"
	if stats.DiagnosticsFixed == 1 {
		noun = "fix"
	}
	if stats.FilesWithPendingFixes > 0 {
		return s.Success.Render(fmt.Sprintf("%d %s would be applied in %d %s",
			stats.DiagnosticsFixed, noun, stats.FilesWithPendingFixes, filesNoun(stats.FilesWithPendingFixes)))
	}
	return s.Success.Render(fmt.Sprintf("%d %s applied in %d %s",
		stats.DiagnosticsFixed, noun, stats.FilesModified, filesNoun(stats.FilesModified)))
}

// FormatSummary formats run statistics as a summary block.
func (s *Styles) FormatSummary(stats runner.Stats) string {
	var builder strings.Builder

	builder.WriteString("\n")
	builder.WriteString(s.SummaryTitle.Render("Summary"))
	builder.WriteString("\n")
	builder.WriteString(strings.Repeat("-", summaryDividerWidth))
	builder.WriteString("\n")

	// Files
	builder.WriteString("  Files checked:     " +
		s.SummaryValue.Render(strconv.Itoa(stats.FilesProcessed)) + "\n")

	if stats.FilesWithIssues > 0 {
		builder.WriteString("  Files with issues: " +
			s.Failure.Render(strconv.Itoa(stats.FilesWithIssues)) + "\n")
	}

	if stats.FilesModified > 0 {
		builder.WriteString("  Files modified:    " +
			s.Success.Render(strconv.Itoa(stats.FilesModified)) + "\n")
	}

	builder.WriteString("\n")

	// Diagnostics by severity
	builder.WriteString("  Total issues:      " +
		s.SummaryValue.Render(strconv.Itoa(stats.DiagnosticsTotal)) + "\n")

	if errors := stats.DiagnosticsBySeverity["error"]; errors > 0 {
		builder.WriteString("    Errors:          " +
			s.Error.Render(strconv.Itoa(errors)) + "\n")
	}
	if warnings := stats.DiagnosticsBySeverity["warning"]; warnings > 0 {
		builder.WriteString("    Warnings:        " +
			s.Warning.Render(strconv.Itoa(warnings)) + "\n")
	}
	if infos := stats.DiagnosticsBySeverity["info"]; infos > 0 {
		builder.WriteString("    Info:            " +
			s.Info.Render(strconv.Itoa(infos)) + "\n")
	}

	builder.WriteString("\n")

	// Overall status
	switch {
	case stats.DiagnosticsBySeverity["error"] > 0:
		builder.WriteString(s.Failure.Render("Lint failed with errors"))
	case stats.DiagnosticsBySeverity["warning"] > 0:
		builder.WriteString(s.Warning.Render("Lint completed with warnings"))
	default:
		builder.WriteString(s.Success.Render("Lint passed"))
	}
	builder.WriteString("\n")

	return builder.String()
}
