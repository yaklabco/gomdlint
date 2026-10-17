package analysis

import (
	"cmp"
	"slices"
	"time"

	"github.com/yaklabco/gomdlint/internal/display"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// ReportVersion is the current report format version.
const ReportVersion = "1.0.0"

// Severity string constants for internal use.
const (
	severityError   = "error"
	severityWarning = "warning"
	severityInfo    = "info"
)

// analysisContext holds temporary state during analysis.
type analysisContext struct {
	ruleMap   map[string]*RuleAnalysis
	fileMap   map[string]*FileAnalysis
	ruleFiles map[string]map[string]bool
	fileRules map[string]map[string]bool
}

// newAnalysisContext creates a new analysis context.
func newAnalysisContext() *analysisContext {
	return &analysisContext{
		ruleMap:   make(map[string]*RuleAnalysis),
		fileMap:   make(map[string]*FileAnalysis),
		ruleFiles: make(map[string]map[string]bool),
		fileRules: make(map[string]map[string]bool),
	}
}

// normalizeSeverity returns the severity string, defaulting to warning.
func normalizeSeverity(sev string) string {
	if sev == "" {
		return severityWarning
	}
	return sev
}

// incrementSeverityCounts updates counts based on severity.
func incrementSeverityCounts(severity string, totals *Totals, fa *FileAnalysis) {
	switch severity {
	case severityError:
		totals.Errors++
		fa.Errors++
	case severityWarning:
		totals.Warnings++
		fa.Warnings++
	case severityInfo:
		totals.Infos++
		fa.Infos++
	}
}

// incrementRuleSeverity updates rule analysis severity counts.
func incrementRuleSeverity(severity string, ra *RuleAnalysis) {
	switch severity {
	case severityError:
		ra.Errors++
	case severityWarning:
		ra.Warnings++
	case severityInfo:
		ra.Infos++
	}
}

// getOrCreateFileAnalysis returns existing or creates new FileAnalysis.
func (ctx *analysisContext) getOrCreateFileAnalysis(path string) *FileAnalysis {
	if _, ok := ctx.fileMap[path]; !ok {
		ctx.fileMap[path] = &FileAnalysis{Path: path}
		ctx.fileRules[path] = make(map[string]bool)
	}
	return ctx.fileMap[path]
}

// getOrCreateRuleAnalysis returns existing or creates new RuleAnalysis.
func (ctx *analysisContext) getOrCreateRuleAnalysis(ruleID, ruleName string) *RuleAnalysis {
	if _, ok := ctx.ruleMap[ruleID]; !ok {
		ctx.ruleMap[ruleID] = &RuleAnalysis{
			RuleID:   ruleID,
			RuleName: ruleName,
		}
		ctx.ruleFiles[ruleID] = make(map[string]bool)
	}
	return ctx.ruleMap[ruleID]
}

// createDiagnosticEntry builds a DiagnosticEntry from a lint diagnostic.
// Columns are converted from bytes to characters using the file snapshot.
func createDiagnosticEntry(path, severity string, snapshot *mdast.FileSnapshot, diag *lint.Diagnostic) DiagnosticEntry {
	entry := DiagnosticEntry{
		FilePath:    path,
		RuleID:      diag.RuleID,
		RuleName:    diag.RuleName,
		Severity:    severity,
		Message:     diag.Message,
		StartLine:   diag.StartLine,
		StartColumn: display.Column(snapshot, mdast.Position{Line: diag.StartLine, Column: diag.StartColumn}, display.CodePoints),
		EndLine:     diag.EndLine,
		EndColumn:   display.Column(snapshot, mdast.Position{Line: diag.EndLine, Column: diag.EndColumn}, display.CodePoints),
		Suggestion:  diag.Suggestion,
		Fixable:     len(diag.FixEdits) > 0,
	}
	for _, edit := range diag.FixEdits {
		entry.Fixes = append(entry.Fixes, FixEntry{
			StartOffset: edit.StartOffset,
			EndOffset:   edit.EndOffset,
			NewText:     edit.NewText,
		})
	}
	return entry
}

// buildByRule constructs the ByRule slice from accumulated data.
func (ctx *analysisContext) buildByRule(opts Options) []RuleAnalysis {
	result := make([]RuleAnalysis, 0, len(ctx.ruleMap))
	for ruleID, ra := range ctx.ruleMap {
		for f := range ctx.ruleFiles[ruleID] {
			ra.Files = append(ra.Files, f)
		}
		slices.Sort(ra.Files)
		result = append(result, *ra)
	}
	sortRuleAnalysis(result, opts.SortBy, opts.SortDesc)
	return result
}

// buildByFile constructs the ByFile slice from accumulated data.
func (ctx *analysisContext) buildByFile(opts Options) []FileAnalysis {
	result := make([]FileAnalysis, 0, len(ctx.fileMap))
	for path, fa := range ctx.fileMap {
		if fa.Issues == 0 {
			continue
		}
		for r := range ctx.fileRules[path] {
			fa.Rules = append(fa.Rules, r)
		}
		slices.Sort(fa.Rules)
		result = append(result, *fa)
	}
	sortFileAnalysis(result, opts.SortBy, opts.SortDesc)
	return result
}

// Analyze transforms a runner.Result into a Report.
// It performs a single pass through diagnostics to compute all views.
func Analyze(result *runner.Result, opts Options) *Report {
	report := &Report{
		Version:   ReportVersion,
		Timestamp: time.Now(),
	}

	if result == nil {
		return report
	}

	ctx := newAnalysisContext()

	for _, file := range result.Files {
		report.Totals.Files++
		if file.Error != nil {
			report.Totals.FilesErrored++
		}
		if file.Result == nil || file.Result.FileResult == nil {
			continue
		}
		report.Totals.RuleErrors += len(file.Result.RuleErrorList())
		if len(file.Result.Diagnostics) > 0 {
			report.Totals.FilesWithIssues++
		}

		displayPath := display.Path(file.Path, opts.WorkingDir)
		fa := ctx.getOrCreateFileAnalysis(displayPath)

		for _, diag := range file.Result.Diagnostics {
			report.Totals.Issues++
			severity := normalizeSeverity(string(diag.Severity))
			isFixable := len(diag.FixEdits) > 0

			incrementSeverityCounts(severity, &report.Totals, fa)
			if isFixable {
				report.Totals.Fixable++
			}

			fa.Issues++
			ctx.fileRules[displayPath][diag.RuleID] = true

			ra := ctx.getOrCreateRuleAnalysis(diag.RuleID, diag.RuleName)
			ra.Issues++
			incrementRuleSeverity(severity, ra)
			if isFixable {
				ra.Fixable = true
			}
			ctx.ruleFiles[diag.RuleID][displayPath] = true

			if opts.IncludeDiagnostics {
				report.Diagnostics = append(report.Diagnostics,
					createDiagnosticEntry(displayPath, severity, file.Result.Snapshot, &diag))
			}
		}
	}

	if opts.IncludeByRule {
		report.ByRule = ctx.buildByRule(opts)
	}
	if opts.IncludeByFile {
		report.ByFile = ctx.buildByFile(opts)
	}

	return report
}

func sortRuleAnalysis(rules []RuleAnalysis, sortBy SortField, desc bool) {
	slices.SortFunc(rules, func(left, right RuleAnalysis) int {
		switch sortBy {
		case SortByAlpha:
			// Alphabetical sorting is always ascending (A-Z)
			return cmp.Compare(left.RuleID, right.RuleID)
		case SortBySeverity:
			// Errors first, then warnings, then infos (always descending by severity)
			result := cmp.Compare(right.Errors, left.Errors)
			if result == 0 {
				result = cmp.Compare(right.Warnings, left.Warnings)
			}
			if result == 0 {
				result = cmp.Compare(right.Issues, left.Issues)
			}
			return result
		default: // SortByCount
			result := cmp.Compare(left.Issues, right.Issues)
			if desc {
				result = -result
			}
			return result
		}
	})
}

func sortFileAnalysis(files []FileAnalysis, sortBy SortField, desc bool) {
	slices.SortFunc(files, func(left, right FileAnalysis) int {
		switch sortBy {
		case SortByAlpha:
			// Alphabetical sorting is always ascending (A-Z)
			return cmp.Compare(left.Path, right.Path)
		case SortBySeverity:
			// Errors first, then warnings, then infos (always descending by severity)
			result := cmp.Compare(right.Errors, left.Errors)
			if result == 0 {
				result = cmp.Compare(right.Warnings, left.Warnings)
			}
			if result == 0 {
				result = cmp.Compare(right.Issues, left.Issues)
			}
			return result
		default: // SortByCount
			result := cmp.Compare(left.Issues, right.Issues)
			if desc {
				result = -result
			}
			return result
		}
	})
}
