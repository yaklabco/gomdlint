package reporter_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

func TestDiffReporter_PreservesSubdirectoryInPath(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	subDir := filepath.Join(workDir, "docs", "guide")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	absPath := filepath.Join(subDir, "file.md")
	require.NoError(t, os.WriteFile(absPath, []byte("# Test\n\nold line\n"), 0o644))

	original := []byte("# Test\n\nold line\n")
	modified := []byte("# Test\n\nnew line\n")
	diff := fix.GenerateDiff(absPath, original, modified)
	require.NotNil(t, diff)

	var buf bytes.Buffer
	rep := reporter.NewDiffReporter(reporter.Options{
		Writer:     &buf,
		Color:      "never",
		WorkingDir: workDir,
	})

	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: absPath,
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{},
				Diff:       diff,
			},
		}},
	}

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	output := buf.String()
	assert.Contains(t, output, "docs/guide/file.md",
		"diff output should preserve subdirectory path")
	assert.Contains(t, output, "diff --git a/docs/guide/file.md b/docs/guide/file.md")
	assert.Contains(t, output, "--- a/docs/guide/file.md")
	assert.Contains(t, output, "+++ b/docs/guide/file.md")
}

func TestDiffReporter_FallsBackToFullPathNotBasename(t *testing.T) {
	t.Parallel()

	// When the file is outside the working directory, the full path should be
	// preserved rather than being stripped to just the basename.
	workDir := t.TempDir()
	otherDir := t.TempDir()
	subDir := filepath.Join(otherDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	absPath := filepath.Join(subDir, "file.md")
	require.NoError(t, os.WriteFile(absPath, []byte("# Test\n\nold line\n"), 0o644))

	original := []byte("# Test\n\nold line\n")
	modified := []byte("# Test\n\nnew line\n")
	diff := fix.GenerateDiff(absPath, original, modified)
	require.NotNil(t, diff)

	var buf bytes.Buffer
	rep := reporter.NewDiffReporter(reporter.Options{
		Writer:     &buf,
		Color:      "never",
		WorkingDir: workDir,
	})

	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: absPath,
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{},
				Diff:       diff,
			},
		}},
	}

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	output := buf.String()
	// Must NOT strip to just "file.md" — should retain enough path to be unambiguous.
	assert.NotContains(t, output, "diff --git a/file.md b/file.md",
		"should not strip path to bare filename")
	assert.Contains(t, output, "sub/file.md",
		"should retain at least the parent directory")
}

func TestDiffReporter_SymlinkedWorkingDir(t *testing.T) {
	t.Parallel()

	// Create a real dir and a symlink to it.
	realDir := t.TempDir()
	docsDir := filepath.Join(realDir, "docs")
	require.NoError(t, os.MkdirAll(docsDir, 0o755))
	absPath := filepath.Join(docsDir, "file.md")
	require.NoError(t, os.WriteFile(absPath, []byte("# Test\n\nold line\n"), 0o644))

	symlinkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skip("cannot create symlinks:", err)
	}

	original := []byte("# Test\n\nold line\n")
	modified := []byte("# Test\n\nnew line\n")
	diff := fix.GenerateDiff(absPath, original, modified)
	require.NotNil(t, diff)

	var buf bytes.Buffer
	// WorkingDir is the symlink, but the file path uses the real directory.
	rep := reporter.NewDiffReporter(reporter.Options{
		Writer:     &buf,
		Color:      "never",
		WorkingDir: symlinkDir,
	})

	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: absPath,
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{},
				Diff:       diff,
			},
		}},
	}

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	output := buf.String()
	assert.Contains(t, output, "docs/file.md",
		"should resolve symlinks and produce correct relative path")
}

func TestDiffReporter_EmptyWorkingDir(t *testing.T) {
	t.Parallel()

	absPath := "/some/absolute/path/file.md"

	original := []byte("# Test\n\nold line\n")
	modified := []byte("# Test\n\nnew line\n")
	diff := fix.GenerateDiff(absPath, original, modified)
	require.NotNil(t, diff)

	var buf bytes.Buffer
	rep := reporter.NewDiffReporter(reporter.Options{
		Writer:     &buf,
		Color:      "never",
		WorkingDir: "", // empty — should keep full path
	})

	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: absPath,
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{},
				Diff:       diff,
			},
		}},
	}

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	output := buf.String()
	// With no WorkingDir, should use the path as-is (minus leading slash for diff prefix).
	assert.Contains(t, output, "some/absolute/path/file.md",
		"with empty WorkingDir, should preserve the full path")
}

func TestDiffReporter_KeepsHunkLinesThatLookLikeFileHeaders(t *testing.T) {
	t.Parallel()

	// A thematic break "---" removed and a line starting with "++" added: in
	// the hunk they become "----" and "+++ plus", which look like the file
	// header lines but are content.
	original := []byte("# Title\n\n---\n\ntext\n")
	modified := []byte("# Title\n\n***\n\n++ plus\n")
	diff := fix.GenerateDiff("doc.md", original, modified)
	require.NotNil(t, diff)

	var buf bytes.Buffer
	rep := reporter.NewDiffReporter(reporter.Options{Writer: &buf, Color: "never"})

	_, err := rep.Report(context.Background(), &runner.Result{
		Files: []runner.FileOutcome{{
			Path:   "doc.md",
			Result: &lint.PipelineResult{FileResult: &lint.FileResult{}, Diff: diff},
		}},
	})
	require.NoError(t, err)

	lines := strings.Split(buf.String(), "\n")
	assert.Contains(t, lines, "----", "the removed thematic break is part of the patch")
	assert.Contains(t, lines, "+***")
	assert.Contains(t, lines, "-text")
	assert.Contains(t, lines, "+++ plus", "the added line is part of the patch")
	assert.Equal(t, 1, strings.Count(buf.String(), "--- a/doc.md\n"), "the file header appears once")
	assert.Equal(t, 1, strings.Count(buf.String(), "+++ b/doc.md\n"), "the file header appears once")
}

func TestDiffReporter_RelativePath(t *testing.T) {
	t.Parallel()

	// When the path is already relative, it should be used as-is.
	diff := fix.GenerateDiff("docs/file.md", []byte("old\n"), []byte("new\n"))
	require.NotNil(t, diff)

	var buf bytes.Buffer
	rep := reporter.NewDiffReporter(reporter.Options{
		Writer:     &buf,
		Color:      "never",
		WorkingDir: "/whatever",
	})

	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: "docs/file.md",
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{},
				Diff:       diff,
			},
		}},
	}

	count, err := rep.Report(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Contains(t, buf.String(), "docs/file.md")
}
