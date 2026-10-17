package runner

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestAccumulateCountsFixes covers how fixes count: written files as
// modified, unwritten ones from a dry run as pending, and skipped files not
// at all, since their fixes never reached the disk. A fix is one fixed
// diagnostic, however many edits it took.
func TestAccumulateCountsFixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		result      lint.PipelineResult
		wantFixed   int
		wantWritten int
		wantPending int
		wantSkipped int
	}{
		{
			name:        "written",
			result:      lint.PipelineResult{Modified: true, Written: true, TotalEditsApplied: 3, TotalFixesApplied: 2},
			wantFixed:   2,
			wantWritten: 1,
		},
		{
			name:        "dry run",
			result:      lint.PipelineResult{Modified: true, TotalEditsApplied: 3, TotalFixesApplied: 2},
			wantFixed:   2,
			wantPending: 1,
		},
		{
			name:        "skipped",
			result:      lint.PipelineResult{Modified: true, Skipped: true, SkipReason: "file modified during processing", TotalEditsApplied: 3, TotalFixesApplied: 2},
			wantSkipped: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := &Result{Stats: newStats()}
			pipelineResult := tt.result
			result.accumulate(FileOutcome{Path: "a.md", Result: &pipelineResult})

			got := result.Stats
			if got.DiagnosticsFixed != tt.wantFixed || got.FilesModified != tt.wantWritten ||
				got.FilesWithPendingFixes != tt.wantPending || got.FilesSkipped != tt.wantSkipped {
				t.Errorf("stats = %+v, want fixed %d, written %d, pending %d, skipped %d",
					got, tt.wantFixed, tt.wantWritten, tt.wantPending, tt.wantSkipped)
			}
		})
	}
}
