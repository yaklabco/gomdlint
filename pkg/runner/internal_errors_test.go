package runner_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// crashingParser panics on content holding "crash", as a parser bug would.
type crashingParser struct{}

func (crashingParser) Parse(ctx context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
	if bytes.Contains(content, []byte("crash")) {
		panic("parser bug")
	}
	return (&mockParser{}).Parse(ctx, path, content)
}

// TestRunner_Run_CountsInternalErrorsApart covers a file whose processing
// panicked: it could not be linted, like an unreadable file, but the cause
// is a bug, which Stats.FilesWithInternalErrors counts apart.
func TestRunner_Run_CountsInternalErrorsApart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("crash\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("fine\n"), 0o644))

	lintRunner := runner.New(lint.NewPipeline(lint.NewEngine(crashingParser{}, lint.NewRegistry())))
	result, err := lintRunner.Run(context.Background(), runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     config.NewConfig(),
		Jobs:       1,
	})
	require.NoError(t, err)

	crashed := outcomeFor(t, result, "a.md")
	require.ErrorIs(t, crashed.Error, lint.ErrInternal)
	assert.True(t, crashed.Failed())
	require.NoError(t, outcomeFor(t, result, "b.md").Error)

	assert.Equal(t, 1, result.Stats.FilesErrored, "the file could not be linted")
	assert.Equal(t, 1, result.Stats.FilesWithInternalErrors)
	assert.Equal(t, 1, result.Stats.FilesProcessed)
}
