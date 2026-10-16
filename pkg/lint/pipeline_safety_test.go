package lint_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// replaceFirstRule reports the first occurrence of from and fixes it to to,
// so it needs one pass per occurrence, like a real rule that is re-run on
// the fixed content.
func replaceFirstRule(id, from, to string) *funcRule {
	return newFuncRule(id, func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		idx := bytes.Index(ctx.File.Content, []byte(from))
		if idx < 0 {
			return nil, nil
		}
		return []lint.Diagnostic{fixDiag(id, fix.TextEdit{StartOffset: idx, EndOffset: idx + len(from), NewText: to})}, nil
	})
}

func newTestPipeline(rules ...lint.Rule) *lint.Pipeline {
	registry := lint.NewRegistry()
	for _, rule := range rules {
		registry.Register(rule)
	}
	return lint.NewPipeline(lint.NewEngine(&mockParser{}, registry))
}

func fixConfig() *config.Config {
	cfg := config.NewConfig()
	cfg.Fix = true
	return cfg
}

func TestPipeline_RelintsAfterTheFinalPass(t *testing.T) {
	t.Parallel()

	pipeline := newTestPipeline(replaceFirstRule("TEST001", "x", "y"))
	opts := lint.PipelineOptions{Fix: true, MaxFixPasses: 3}

	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte("xxx"), fixConfig(), opts)
	require.NoError(t, err)

	assert.Equal(t, "yyy", string(result.ModifiedContent))
	assert.Equal(t, 3, result.FixPasses)
	assert.False(t, result.Exhausted, "the last pass fixed the last issue")
	assert.Zero(t, result.RemainingEdits)
	assert.Empty(t, result.Diagnostics, "diagnostics must describe the fixed content")
	assert.Equal(t, 3, result.TotalEditsApplied)
}

func TestPipeline_ExhaustedReportsWhatRemainsInTheFixedContent(t *testing.T) {
	t.Parallel()

	pipeline := newTestPipeline(replaceFirstRule("TEST001", "x", "y"))
	opts := lint.PipelineOptions{Fix: true, MaxFixPasses: 3}

	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte("xxxx"), fixConfig(), opts)
	require.NoError(t, err)

	assert.Equal(t, "yyyx", string(result.ModifiedContent))
	assert.True(t, result.Exhausted)
	assert.False(t, result.Oscillated)
	assert.Equal(t, 1, result.RemainingEdits)
	require.Len(t, result.Diagnostics, 1)
	assert.Equal(t, 3, result.Diagnostics[0].FixEdits[0].StartOffset, "the remaining issue is at its position in the fixed content")
}

func TestPipeline_StopsWhenFixesUndoEachOther(t *testing.T) {
	t.Parallel()

	pipeline := newTestPipeline(
		replaceFirstRule("TEST001", "x", "y"),
		replaceFirstRule("TEST002", "y", "x"),
	)

	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte("x"), fixConfig(), lint.PipelineOptions{Fix: true})
	require.NoError(t, err)

	assert.True(t, result.Oscillated)
	assert.False(t, result.Exhausted)
	assert.Equal(t, 1, result.FixPasses, "the loop stops as soon as content repeats")
	assert.Equal(t, 1, result.RemainingEdits)
	assert.Equal(t, "y", string(result.ModifiedContent))
}

func TestPipeline_FixThatChangesNothingDoesNotWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	noop := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: 0, EndOffset: 5, NewText: "hello"})}, nil
	})
	pipeline := newTestPipeline(noop)

	result, err := pipeline.ProcessFile(context.Background(), path, fixConfig(), lint.PipelineOptions{Fix: true})
	require.NoError(t, err)

	assert.False(t, result.Modified, "identical bytes are not a modification")
	assert.False(t, result.Written)
	assert.Nil(t, result.ModifiedContent)
	assert.Zero(t, result.FixPasses)
	assert.True(t, result.Oscillated, "the remaining fix cannot make progress")
}

func TestPipeline_KeepsRuleErrorsFromEarlierPasses(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("cannot handle x")
	picky := newFuncRule("TEST002", func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		if bytes.Contains(ctx.File.Content, []byte("x")) {
			return nil, sentinel
		}
		return nil, nil
	})
	pipeline := newTestPipeline(replaceFirstRule("TEST001", "x", "y"), picky)

	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte("x"), fixConfig(), lint.PipelineOptions{Fix: true})
	require.NoError(t, err)

	assert.Equal(t, "y", string(result.ModifiedContent))
	require.Contains(t, result.RuleErrors, "TEST002", "an error in the first pass must not be lost")
	require.ErrorIs(t, result.RuleErrors["TEST002"], sentinel)
}

func TestPipeline_RecoversFromPanicWhileProcessingAFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	parser := &mockParser{
		parseFunc: func(context.Context, string, []byte) (*mdast.FileSnapshot, error) {
			panic("parser bug")
		},
	}
	pipeline := lint.NewPipeline(lint.NewEngine(parser, lint.NewRegistry()))

	_, err := pipeline.ProcessFile(context.Background(), path, config.NewConfig(), lint.DefaultPipelineOptions())
	require.ErrorIs(t, err, lint.ErrInternal)
	assert.Contains(t, err.Error(), "parser bug")
	assert.True(t, lint.IsPipelineError(err))

	_, err = pipeline.ProcessContent(context.Background(), "test.md", []byte("hello"), config.NewConfig(), lint.DefaultPipelineOptions())
	require.ErrorIs(t, err, lint.ErrInternal)
}

func TestPipeline_KeepsCRLFLineEndings(t *testing.T) {
	t.Parallel()

	// Inserts a blank line before every "Para" line that follows a non-blank line.
	blankBeforePara := newFuncRule("TEST001", func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		content := string(ctx.File.Content)
		idx := strings.Index(content, "Para")
		if idx <= 0 || strings.HasSuffix(content[:idx], "\n\n") || strings.HasSuffix(content[:idx], "\r\n\r\n") {
			return nil, nil
		}
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: idx, EndOffset: idx, NewText: "\n"})}, nil
	})
	pipeline := newTestPipeline(blankBeforePara)

	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte("# Title\r\nPara\r\n"), fixConfig(),
		lint.PipelineOptions{Fix: true})
	require.NoError(t, err)

	assert.Equal(t, "# Title\r\n\r\nPara\r\n", string(result.ModifiedContent))
	assert.False(t, result.Oscillated)
	assert.Empty(t, result.Diagnostics)
}

func TestPipeline_DryRunDiffShowsAddedFinalNewline(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("# Title\n\ntext"), 0o644))

	finalNewline := newFuncRule("TEST001", func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		content := ctx.File.Content
		if bytes.HasSuffix(content, []byte("\n")) {
			return nil, nil
		}
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: len(content), EndOffset: len(content), NewText: "\n"})}, nil
	})
	pipeline := newTestPipeline(finalNewline)

	result, err := pipeline.ProcessFile(context.Background(), path, fixConfig(), lint.PipelineOptions{Fix: true, DryRun: true})
	require.NoError(t, err)

	require.NotNil(t, result.Diff)
	assert.Contains(t, result.Diff.String(), "-text\n"+fix.NoNewlineMarker+"\n+text\n")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "# Title\n\ntext", string(got), "dry run must not write")
}
