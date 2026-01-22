package reporter_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

func TestReporter_FacadeReturnsIssueCount(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	opts := reporter.Options{
		Writer: &buf,
		Format: reporter.FormatJSON,
	}

	rep, err := reporter.New(opts)
	require.NoError(t, err)

	result := &runner.Result{
		Files: []runner.FileOutcome{
			{
				Path: "test.md",
				Result: &lint.PipelineResult{
					FileResult: &lint.FileResult{
						Diagnostics: []lint.Diagnostic{
							{RuleID: "MD001", Severity: config.SeverityError},
							{RuleID: "MD002", Severity: config.SeverityWarning},
						},
					},
				},
			},
		},
	}

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
