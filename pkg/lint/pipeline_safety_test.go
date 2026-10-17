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
	assert.Equal(t, 3, result.TotalFixesApplied)
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

func TestPipeline_NeverWritesFixedContentThatCannotBeParsed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("x and y"), 0o644))

	errUnparsable := errors.New("unparsable")
	parser := &mockParser{}
	strict := &mockParser{parseFunc: func(ctx context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
		if bytes.Contains(content, []byte("BAD")) {
			return nil, errUnparsable
		}
		return parser.Parse(ctx, path, content)
	}}
	registry := lint.NewRegistry()
	registry.Register(replaceFirstRule("TEST001", "y", "z"))
	registry.Register(replaceFirstRule("TEST002", "z", "BAD"))
	pipeline := lint.NewPipeline(lint.NewEngine(strict, registry))

	result, err := pipeline.ProcessFile(context.Background(), path, fixConfig(), lint.PipelineOptions{Fix: true})
	require.NoError(t, err)

	assert.True(t, result.Skipped)
	assert.Contains(t, result.SkipReason, "unparsable")
	assert.False(t, result.Modified)
	assert.False(t, result.Written)
	assert.Zero(t, result.FixPasses, "nothing was applied to the file")
	require.Len(t, result.Diagnostics, 1, "diagnostics describe the unchanged file")
	assert.Equal(t, "TEST001", result.Diagnostics[0].RuleID)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "x and y", string(got))
}

func TestPipeline_UnparsableFixLeavesNoPendingEdits(t *testing.T) {
	t.Parallel()

	errUnparsable := errors.New("unparsable")
	parser := &mockParser{}
	strict := &mockParser{parseFunc: func(ctx context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
		if bytes.Contains(content, []byte("BAD")) {
			return nil, errUnparsable
		}
		return parser.Parse(ctx, path, content)
	}}
	registry := lint.NewRegistry()
	registry.Register(replaceFirstRule("TEST001", "y", "z"))
	registry.Register(replaceFirstRule("TEST002", "z", "BAD"))
	// Conflicts with TEST001 on the first pass, so that pass skips it.
	registry.Register(replaceFirstRule("TEST003", "y", "w"))
	pipeline := lint.NewPipeline(lint.NewEngine(strict, registry))

	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte("x and y"), fixConfig(),
		lint.PipelineOptions{Fix: true})
	require.NoError(t, err)

	require.True(t, result.Skipped)
	assert.Empty(t, result.Edits, "no fix will be applied to a skipped file")
	assert.Empty(t, result.SkippedEdits, "skipped edits only describe a loop that stopped early")
	assert.False(t, result.EditConflicts)
	assert.Len(t, result.Diagnostics, 2, "the unchanged file's findings are still reported")
}

func TestPipeline_PanicErrorCarriesTheStack(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("parser bug")
	parser := &mockParser{
		parseFunc: func(context.Context, string, []byte) (*mdast.FileSnapshot, error) {
			panic(sentinel)
		},
	}
	pipeline := lint.NewPipeline(lint.NewEngine(parser, lint.NewRegistry()))

	_, err := pipeline.ProcessContent(context.Background(), "doc.md", []byte("hello"), config.NewConfig(),
		lint.DefaultPipelineOptions())

	require.ErrorIs(t, err, lint.ErrInternal)
	require.ErrorIs(t, err, sentinel, "an error panic value stays matchable")
	var internalErr *lint.InternalError
	require.ErrorAs(t, err, &internalErr)
	assert.Equal(t, "doc.md", internalErr.Path)
	assert.Contains(t, string(internalErr.Stack), "goroutine", "the stack is kept for library callers")
}

func TestPipeline_WritesSymlinkTargetsOnlyInsideTheRoots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		roots       func(linkDir, targetDir string) []string
		wantWritten bool
	}{
		{name: "no roots means no restriction", roots: func(string, string) []string { return nil }, wantWritten: true},
		{name: "target inside a root", roots: func(l, d string) []string { return []string{l, d} }, wantWritten: true},
		{name: "target outside every root", roots: func(l, _ string) []string { return []string{l} }, wantWritten: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			linkDir := t.TempDir()
			targetDir := t.TempDir()
			target := filepath.Join(targetDir, "target.md")
			link := filepath.Join(linkDir, "link.md")
			require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlinks not supported here: %v", err)
			}

			pipeline := newTestPipeline(replaceFirstRule("TEST001", "x", "y"))
			opts := lint.PipelineOptions{Fix: true, WriteRoots: tt.roots(linkDir, targetDir)}

			result, err := pipeline.ProcessFile(context.Background(), link, fixConfig(), opts)
			require.NoError(t, err)

			got, err := os.ReadFile(target)
			require.NoError(t, err)
			info, err := os.Lstat(link)
			require.NoError(t, err)
			assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link must stay a link")

			if tt.wantWritten {
				assert.True(t, result.Written)
				assert.Equal(t, "y", string(got))
				return
			}
			assert.False(t, result.Written)
			assert.True(t, result.Skipped)
			assert.Contains(t, result.SkipReason, "outside the linted paths")
			assert.Equal(t, "x", string(got), "a target outside the linted paths must not be written")

			// The result describes the file as it is, not a fix that was refused.
			assert.Equal(t, "x", string(result.Snapshot.Content))
			require.Len(t, result.Diagnostics, 1, "the unfixed finding is still reported")
			assert.False(t, result.Modified)
			assert.Nil(t, result.ModifiedContent)
			assert.Zero(t, result.FixPasses)
			assert.Zero(t, result.TotalEditsApplied)
			assert.Zero(t, result.TotalFixesApplied)
			assert.Empty(t, result.Edits)
		})
	}
}

func TestPipeline_DoesNotWriteThroughSymlinkedDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "doc.md")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	pipeline := newTestPipeline(replaceFirstRule("TEST001", "x", "y"))
	opts := lint.PipelineOptions{Fix: true, WriteRoots: []string{root}}

	// The file itself is not a link; a directory on its path is.
	result, err := pipeline.ProcessFile(context.Background(), filepath.Join(root, "sub", "doc.md"), fixConfig(), opts)
	require.NoError(t, err)

	assert.True(t, result.Skipped)
	assert.False(t, result.Written)
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "x", string(got), "a file reached through a directory link outside the roots must not be written")
}

func TestPipeline_RefusesWhenThePathIsRetargetedDuringProcessing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	other := filepath.Join(dir, "other.md")
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.WriteFile(first, []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(other, []byte("other x"), 0o644))
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	// Point the link at another file while the fixed content is re-linted,
	// after the file was read and before it is written.
	parses := 0
	parser := &mockParser{}
	retargeting := &mockParser{parseFunc: func(ctx context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
		parses++
		if parses == 2 {
			require.NoError(t, os.Remove(link))
			require.NoError(t, os.Symlink(other, link))
		}
		return parser.Parse(ctx, path, content)
	}}
	registry := lint.NewRegistry()
	registry.Register(replaceFirstRule("TEST001", "x", "y"))
	pipeline := lint.NewPipeline(lint.NewEngine(retargeting, registry))

	result, err := pipeline.ProcessFile(context.Background(), link, fixConfig(),
		lint.PipelineOptions{Fix: true, WriteRoots: []string{dir}})
	require.NoError(t, err)

	assert.True(t, result.Skipped)
	assert.False(t, result.Written)
	assert.Zero(t, result.TotalEditsApplied, "a refused write reports no applied fixes")
	assert.Zero(t, result.TotalFixesApplied, "a refused write reports no applied fixes")
	got, err := os.ReadFile(other)
	require.NoError(t, err)
	assert.Equal(t, "other x", string(got), "the fix of one file must never land in another")
	got, err = os.ReadFile(first)
	require.NoError(t, err)
	assert.Equal(t, "x", string(got))
}

func TestWriteRoots(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	docs := filepath.Join(workDir, "docs")
	require.NoError(t, os.MkdirAll(docs, 0o755))
	target := filepath.Join(workDir, "target.md")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))
	link := filepath.Join(workDir, "link.md")
	if err := os.Symlink("target.md", link); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	resolved := func(path string) string {
		resolvedPath, err := filepath.EvalSymlinks(path)
		require.NoError(t, err)
		return resolvedPath
	}

	assert.Equal(t, []string{resolved(workDir)}, lint.WriteRoots(workDir, []string{"."}))
	assert.Equal(t, []string{resolved(docs), resolved(target)}, lint.WriteRoots(workDir, []string{"docs", link}),
		"relative paths resolve from the working directory and a named link resolves to its target")
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
