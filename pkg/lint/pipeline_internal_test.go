package lint

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// stubParser returns a minimal snapshot of the content.
type stubParser struct{}

func (stubParser) Parse(_ context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
	return &mdast.FileSnapshot{
		Path:    path,
		Content: content,
		Lines:   mdast.BuildLines(content),
		Tokens:  []mdast.Token{{Kind: mdast.TokText, StartOffset: 0, EndOffset: len(content)}},
		Root:    mdast.NewNode(mdast.NodeDocument),
	}, nil
}

// xToYRule fixes the first "x" to "y".
type xToYRule struct {
	BaseRule
}

func (r *xToYRule) Apply(ctx *RuleContext) ([]Diagnostic, error) {
	idx := bytes.IndexByte(ctx.File.Content, 'x')
	if idx < 0 {
		return nil, nil
	}
	return []Diagnostic{{
		RuleID: "TEST001", Message: "x", StartLine: 1, StartColumn: idx + 1,
		FixEdits: []fix.TextEdit{{StartOffset: idx, EndOffset: idx + 1, NewText: "y"}},
	}}, nil
}

// processWithHook runs a fixing pipeline on path, with beforeWrite run after
// every check and right before the write.
func processWithHook(t *testing.T, path string, roots []string, beforeWrite func()) *PipelineResult {
	t.Helper()

	registry := NewRegistry()
	registry.Register(&xToYRule{BaseRule: NewBaseRule("TEST001", "x-to-y", "", nil, true)})
	pipeline := NewPipeline(NewEngine(stubParser{}, registry))
	pipeline.beforeWrite = beforeWrite

	cfg := config.NewConfig()
	cfg.Fix = true
	result, err := pipeline.ProcessFile(context.Background(), path, cfg, PipelineOptions{Fix: true, WriteRoots: roots})
	require.NoError(t, err)
	return result
}

func readString(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

// requireRefused checks that a write was refused and that the result
// describes the unfixed file.
func requireRefused(t *testing.T, result *PipelineResult) {
	t.Helper()
	assert.True(t, result.Skipped)
	assert.False(t, result.Written)
	assert.Zero(t, result.TotalEditsApplied)
	require.Len(t, result.Diagnostics, 1, "the unfixed finding is reported")
}

func TestPipeline_RefusesAParentDirectoryReplacedBeforeTheWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "doc.md"), []byte("x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "doc.md"), []byte("outside x\n"), 0o644))

	result := processWithHook(t, filepath.Join(root, "docs", "doc.md"), []string{root}, func() {
		require.NoError(t, os.Rename(filepath.Join(root, "docs"), filepath.Join(root, "docs.aside")))
		if err := os.Symlink(outside, filepath.Join(root, "docs")); err != nil {
			t.Skipf("symlinks not supported here: %v", err)
		}
	})

	requireRefused(t, result)
	assert.Equal(t, "outside x\n", readString(t, filepath.Join(outside, "doc.md")),
		"a replaced parent directory must not redirect the write")
	assert.Equal(t, "x\n", readString(t, filepath.Join(root, "docs.aside", "doc.md")))
}

func TestPipeline_RefusesAFileReplacedByAHardLinkBeforeTheWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	victim := filepath.Join(t.TempDir(), "victim.md")
	require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o644))
	require.NoError(t, os.WriteFile(victim, []byte("victim x\n"), 0o644))

	result := processWithHook(t, path, []string{root}, func() {
		require.NoError(t, os.Remove(path))
		if err := os.Link(victim, path); err != nil {
			t.Skipf("hard links not supported here: %v", err)
		}
	})

	requireRefused(t, result)
	assert.Equal(t, "victim x\n", readString(t, victim), "a file linked in after the checks must not be rewritten")
}

func TestPipeline_KeepsAFileWrittenConcurrentlyBeforeTheWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o644))

	result := processWithHook(t, path, []string{root}, func() {
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.WriteFile(path, []byte("concurrent x\n"), 0o644))
	})

	requireRefused(t, result)
	assert.Equal(t, "concurrent x\n", readString(t, path), "another writer's file must not be replaced")
}

func TestPipeline_WritesThePinnedFileWhenALinkMovesBeforeTheWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outside := t.TempDir()
	inside := filepath.Join(dir, "inside.md")
	victim := filepath.Join(outside, "victim.md")
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.WriteFile(inside, []byte("x\n"), 0o644))
	require.NoError(t, os.WriteFile(victim, []byte("unrelated x\n"), 0o644))
	if err := os.Symlink(inside, link); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	registry := NewRegistry()
	registry.Register(&xToYRule{BaseRule: NewBaseRule("TEST001", "x-to-y", "", nil, true)})
	pipeline := NewPipeline(NewEngine(stubParser{}, registry))
	// Point the link outside after every check, right before the write.
	pipeline.beforeWrite = func() {
		require.NoError(t, os.Remove(link))
		require.NoError(t, os.Symlink(victim, link))
	}

	cfg := config.NewConfig()
	cfg.Fix = true
	result, err := pipeline.ProcessFile(context.Background(), link, cfg,
		PipelineOptions{Fix: true, WriteRoots: []string{dir}})
	require.NoError(t, err)

	got, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "unrelated x\n", string(got), "the write must not follow the moved link")
	got, err = os.ReadFile(inside)
	require.NoError(t, err)
	assert.Equal(t, "y\n", string(got), "the file that was read and checked is the one fixed")
	assert.True(t, result.Written)
}
