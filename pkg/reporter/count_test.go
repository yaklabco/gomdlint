package reporter_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// TestTextReporter_ExhaustedCountsAgree covers the warning about a fix
// loop that stopped with edits left, grouped and flat, after one and two
// passes with one and two edits left.
func TestTextReporter_ExhaustedCountsAgree(t *testing.T) {
	tests := []struct {
		passes, edits int
		want          string
	}{
		{passes: 1, edits: 1, want: "fix loop exhausted after 1 pass with 1 edit remaining"},
		{passes: 2, edits: 2, want: "fix loop exhausted after 2 passes with 2 edits remaining"},
	}

	for _, tt := range tests {
		for _, grouped := range []bool{true, false} {
			result := createTestResult()
			result.Files[0].Exhausted = true
			result.Files[0].FixPasses = tt.passes
			result.Files[0].RemainingEdits = tt.edits

			var buf bytes.Buffer
			rep := reporter.NewTextReporter(reporter.Options{Writer: &buf, Color: "never", GroupByFile: grouped})
			_, err := rep.Report(context.Background(), result)
			require.NoError(t, err)

			assert.Contains(t, buf.String(), tt.want, "grouped %t", grouped)
		}
	}
}

// TestTableReporter_PassedCountsAgree covers the line under "All files
// passed!" for one and two files checked.
func TestTableReporter_PassedCountsAgree(t *testing.T) {
	for files, want := range map[int]string{1: "1 file checked\n", 2: "2 files checked\n"} {
		result := &runner.Result{Stats: runner.Stats{FilesProcessed: files, DiagnosticsBySeverity: map[string]int{}}}
		for range files {
			result.Files = append(result.Files, runner.FileOutcome{Path: "clean.md"})
		}

		var buf bytes.Buffer
		rep := reporter.NewTableReporter(reporter.Options{Writer: &buf, Color: "never", ShowSummary: true})
		_, err := rep.Report(context.Background(), result)
		require.NoError(t, err)

		assert.Equal(t, "\nAll files passed!\n"+want, buf.String())
	}
}
