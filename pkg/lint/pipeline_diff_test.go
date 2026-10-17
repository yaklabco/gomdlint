package lint_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// helloToWorld is a pipeline whose only rule fixes "hello" to "world".
func helloToWorld() *lint.Pipeline {
	registry := lint.NewRegistry()
	registry.Register(newFuncRule("TEST001", func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		if string(ctx.File.Content) != "hello" {
			return nil, nil
		}
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: 0, EndOffset: 5, NewText: "world"})}, nil
	}))
	return lint.NewPipeline(lint.NewEngine(&mockParser{}, registry))
}

// TestPipeline_ProcessFile_DiffOfWrittenFixes covers PipelineOptions.Diff:
// a write records the diff of what was written, and without it a write
// records no diff.
func TestPipeline_ProcessFile_DiffOfWrittenFixes(t *testing.T) {
	t.Parallel()

	for _, withDiff := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "test.md")
		require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))
		cfg := config.NewConfig()
		cfg.Fix = true

		result, err := helloToWorld().ProcessFile(context.Background(), path, cfg, lint.PipelineOptions{Fix: true, Diff: withDiff})

		require.NoError(t, err)
		require.True(t, result.Written)
		if !withDiff {
			assert.Nil(t, result.Diff, "without Diff a write records no diff")
			continue
		}
		require.NotNil(t, result.Diff)
		assert.Equal(t, fix.GenerateDiff(path, []byte("hello"), []byte("world")).String(), result.Diff.String())
	}
}

// TestPipeline_ProcessContent_Diff covers PipelineOptions.Diff for content
// fixed in memory: the diff is recorded as in a dry run.
func TestPipeline_ProcessContent_Diff(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cfg.Fix = true

	result, err := helloToWorld().ProcessContent(context.Background(), "test.md", []byte("hello"), cfg,
		lint.PipelineOptions{Fix: true, Diff: true})

	require.NoError(t, err)
	require.NotNil(t, result.Diff)
	assert.Equal(t, fix.GenerateDiff("test.md", []byte("hello"), []byte("world")).String(), result.Diff.String())
}
