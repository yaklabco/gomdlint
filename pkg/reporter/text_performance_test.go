package reporter_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// lineCounter counts the lines written to it and discards them.
type lineCounter struct {
	lines int
}

func (w *lineCounter) Write(p []byte) (int, error) {
	w.lines += bytes.Count(p, []byte("\n"))
	return len(p), nil
}

// manyFindings returns a result for a large document with a trailing-space
// finding on every line.
func manyFindings(path string, lines int) *runner.Result {
	var doc strings.Builder
	diags := make([]lint.Diagnostic, 0, lines)
	for idx := range lines {
		text := fmt.Sprintf("Line %d of a large document with trailing spaces", idx+1)
		doc.WriteString(text + "   \n")
		diags = append(diags, lint.Diagnostic{
			RuleID: "MD009", RuleName: "no-trailing-spaces", Message: "Trailing whitespace",
			Severity: config.SeverityWarning, FilePath: path,
			StartLine: idx + 1, StartColumn: len(text) + 1, EndLine: idx + 1, EndColumn: len(text) + 4,
		})
	}

	snapshot := mdast.NewFileSnapshot(path, []byte(doc.String()))
	return &runner.Result{
		Files: []runner.FileOutcome{{
			Path:   path,
			Result: &lint.PipelineResult{Path: path, FileResult: &lint.FileResult{Snapshot: snapshot, Diagnostics: diags}},
		}},
		Stats: runner.Stats{
			FilesDiscovered: 1, FilesProcessed: 1, FilesWithIssues: 1,
			DiagnosticsTotal: lines, DiagnosticsBySeverity: map[string]int{"warning": lines},
		},
	}
}

// TestTextReporter_SourceContextScalesWithTheOutput guards the source
// context of the text report against work that grows with the document for
// every finding. Looking up each finding's line by splitting the whole
// document took over ten seconds for a 400 KB document with 5000 findings.
// Rendering this 1 MB document with 20000 findings now takes a fraction of
// a second, so the generous limit holds on slow CI machines and under -race
// while still catching a scan of the document per finding.
func TestTextReporter_SourceContextScalesWithTheOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("large document")
	}
	const (
		findings = 20000
		limit    = 10 * time.Second
	)

	workDir := t.TempDir()
	result := manyFindings(filepath.Join(workDir, "large.md"), findings)

	for _, grouped := range []bool{true, false} {
		t.Run(fmt.Sprintf("grouped %v", grouped), func(t *testing.T) {
			out := &lineCounter{}
			rep, err := reporter.New(reporter.Options{
				Writer:      out,
				Format:      reporter.FormatText,
				Color:       "never",
				ShowContext: true,
				ShowSummary: true,
				GroupByFile: grouped,
				WorkingDir:  workDir,
			})
			require.NoError(t, err)

			type report struct {
				count int
				err   error
			}
			done := make(chan report, 1)
			start := time.Now()
			go func() {
				count, err := rep.Report(context.Background(), result)
				done <- report{count: count, err: err}
			}()

			select {
			case got := <-done:
				require.NoError(t, got.err)
				assert.Equal(t, findings, got.count)
				assert.GreaterOrEqual(t, out.lines, 3*findings, "every finding prints its line and a caret")
				t.Logf("rendered %d findings in %v", findings, time.Since(start))
			case <-time.After(limit):
				t.Fatalf("rendering %d findings took longer than %v", findings, limit)
			}
		})
	}
}
