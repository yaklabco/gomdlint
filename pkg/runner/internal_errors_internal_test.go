package runner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestAccumulateCountsInternalErrorsApart covers the kinds of files that
// could not be linted: every one counts in FilesErrored, and those whose
// processing panicked (lint.ErrInternal) also in FilesWithInternalErrors.
func TestAccumulateCountsInternalErrorsApart(t *testing.T) {
	t.Parallel()

	result := &Result{Stats: newStats()}
	result.accumulate(FileOutcome{Path: "a.md", Error: &lint.InternalError{Path: "a.md", Value: "bug"}})
	result.accumulate(FileOutcome{Path: "b.md", Error: fmt.Errorf("preview fixes: %w", &lint.InternalError{Path: "b.md", Value: "bug"})})
	result.accumulate(FileOutcome{Path: "c.md", Error: fmt.Errorf("%w: c.md", lint.ErrPermissionDenied)})

	if result.Stats.FilesErrored != 3 || result.Stats.FilesWithInternalErrors != 2 {
		t.Errorf("FilesErrored = %d, FilesWithInternalErrors = %d, want 3 and 2",
			result.Stats.FilesErrored, result.Stats.FilesWithInternalErrors)
	}
}

// TestProcessFileRecoversPanicsAsInternalErrors covers a panic in the
// runner itself, outside the pipeline, which recovers its own panics: the
// file fails with an internal error rather than crashing the run.
func TestProcessFileRecoversPanicsAsInternalErrors(t *testing.T) {
	t.Parallel()

	var broken *Runner // Processing dereferences the nil runner and panics.
	outcome := broken.processFile(context.Background(), "a.md", nil, lint.PipelineOptions{}, false)

	if !errors.Is(outcome.Error, lint.ErrInternal) {
		t.Fatalf("Error = %v, want an internal error", outcome.Error)
	}
	if outcome.Path != "a.md" || outcome.Result != nil {
		t.Errorf("outcome = %+v, want only the path and the error", outcome)
	}
}
