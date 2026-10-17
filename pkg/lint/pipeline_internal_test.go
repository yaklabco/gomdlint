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
