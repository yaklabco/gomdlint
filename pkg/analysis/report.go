package analysis

import "time"

// Report contains pre-computed views of lint results.
// Computed once by Analyze(), used by all renderers.
type Report struct {
	// Diagnostics is the flat list for detailed output.
	Diagnostics []DiagnosticEntry `json:"diagnostics,omitempty"`

	// ByFile groups diagnostics by file path.
	ByFile []FileAnalysis `json:"byFile,omitempty"`

	// ByRule groups diagnostics by rule.
	ByRule []RuleAnalysis `json:"byRule,omitempty"`

	// Totals contains aggregate statistics.
	Totals Totals `json:"summary"`

	// Version is the report format version.
	Version string `json:"version"`

	// Timestamp is when the analysis was performed.
	Timestamp time.Time `json:"timestamp"`
}

// DiagnosticEntry represents a single diagnostic in the report.
type DiagnosticEntry struct {
	FilePath    string     `json:"filePath"`
	RuleID      string     `json:"ruleId"`
	RuleName    string     `json:"ruleName"`
	Severity    string     `json:"severity"`
	Message     string     `json:"message"`
	StartLine   int        `json:"startLine"`
	StartColumn int        `json:"startColumn"`
	EndLine     int        `json:"endLine"`
	EndColumn   int        `json:"endColumn"`
	Suggestion  string     `json:"suggestion,omitempty"`
	Fixable     bool       `json:"fixable"`
	Fixes       []FixEntry `json:"fixes,omitempty"`
}

// FixEntry represents a text edit fix.
type FixEntry struct {
	StartOffset int    `json:"startOffset"`
	EndOffset   int    `json:"endOffset"`
	NewText     string `json:"newText"`
}

// Totals contains aggregate statistics for the report.
type Totals struct {
	Files           int `json:"filesChecked"`
	FilesWithIssues int `json:"filesWithIssues"`
	Issues          int `json:"totalIssues"`
	Errors          int `json:"errors"`
	Warnings        int `json:"warnings"`
	Infos           int `json:"infos"`
	Fixable         int `json:"fixable"`
}

// HasIssues returns true if there are any issues.
func (t Totals) HasIssues() bool {
	return t.Issues > 0
}

// HasErrors returns true if there are any errors.
func (t Totals) HasErrors() bool {
	return t.Errors > 0
}

// FileAnalysis contains aggregated data for a single file.
type FileAnalysis struct {
	Path     string   `json:"path"`
	Issues   int      `json:"issues"`
	Errors   int      `json:"errors"`
	Warnings int      `json:"warnings"`
	Infos    int      `json:"infos"`
	Rules    []string `json:"rules,omitempty"`
}

// RuleAnalysis contains aggregated data for a single rule.
type RuleAnalysis struct {
	RuleID   string   `json:"ruleId"`
	RuleName string   `json:"ruleName"`
	Issues   int      `json:"issues"`
	Errors   int      `json:"errors"`
	Warnings int      `json:"warnings"`
	Infos    int      `json:"infos"`
	Fixable  bool     `json:"fixable"`
	Files    []string `json:"files,omitempty"`
}
