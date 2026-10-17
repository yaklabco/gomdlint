package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// TestRunner_Run_PreviewFixesWithFixRecordsTheWrittenDiff covers
// PreviewFixes in a run with Fix: the fixes are written, and each fixed
// file carries the diff of what was written.
func TestRunner_Run_PreviewFixesWithFixRecordsTheWrittenDiff(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	registry := lint.NewRegistry()
	registry.Register(newContentRule("TEST001", func(content []byte) ([]lint.Diagnostic, error) {
		if !strings.HasPrefix(string(content), "hello") {
			return nil, nil
		}
		return []lint.Diagnostic{{
			RuleID: "TEST001", Message: "fix needed", StartLine: 1, StartColumn: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "world"}},
		}}, nil
	}))
	lintRunner := runner.New(lint.NewPipeline(lint.NewEngine(&mockParser{}, registry)))

	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.NoBackups = true
	result, err := lintRunner.Run(context.Background(), runner.Options{
		Paths:        []string{"."},
		WorkingDir:   dir,
		Config:       cfg,
		PreviewFixes: true,
	})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "world", string(content), "the fix is written")

	require.Len(t, result.Files, 1)
	outcome := result.Files[0]
	require.NotNil(t, outcome.Result)
	assert.True(t, outcome.Result.Written)
	require.NotNil(t, outcome.Result.Diff, "a written file carries the diff of what was written")
	assert.Equal(t, fix.GenerateDiff(path, []byte("hello"), []byte("world")).String(), outcome.Result.Diff.String())
	assert.Zero(t, result.Stats.DiagnosticsTotal, "diagnostics describe the fixed file")
}
