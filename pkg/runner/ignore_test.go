package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// makeTree creates empty files under dir from slash-separated paths, or
// files with content when the path maps to one.
func makeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

// discoverRel runs discovery and returns the found files relative to dir,
// with forward slashes, sorted.
func discoverRel(t *testing.T, dir string, opts runner.Options) []string {
	t.Helper()

	files, err := runner.Discover(context.Background(), opts)
	require.NoError(t, err)

	rel := make([]string, 0, len(files))
	for _, file := range files {
		path, err := filepath.Rel(dir, file)
		require.NoError(t, err)
		rel = append(rel, filepath.ToSlash(path))
	}
	slices.Sort(rel)
	return rel
}

func TestDiscover_DoubleStarMatchesWholeSegments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makeTree(t, dir, map[string]string{
		"a.md": "", "data.md": "", "docs/a.md": "", "docs/data.md": "",
		"docs/sub/a.md": "", "docs/sub/schema.md": "",
	})

	got := discoverRel(t, dir, runner.Options{
		Paths:        []string{"."},
		WorkingDir:   dir,
		ExcludeGlobs: []string{"**/a.md"},
	})

	assert.Equal(t, []string{"data.md", "docs/data.md", "docs/sub/schema.md"}, got)
}

func TestDiscover_ExcludeGlobForms(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"top.md": "", "CHANGELOG.md": "", "docs/a.md": "", "docs/sub/b.md": "",
		"pkg/CHANGELOG.md": "", "drafts/c.md": "", "notes/drafts/d.md": "",
	}

	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{
			name:     "recursive glob under a directory",
			patterns: []string{"docs/**/*.md"},
			want:     []string{"CHANGELOG.md", "drafts/c.md", "notes/drafts/d.md", "pkg/CHANGELOG.md", "top.md"},
		},
		{
			name:     "name without a slash matches at any depth",
			patterns: []string{"CHANGELOG.md"},
			want:     []string{"docs/a.md", "docs/sub/b.md", "drafts/c.md", "notes/drafts/d.md", "top.md"},
		},
		{
			name:     "directory name prunes at any depth",
			patterns: []string{"drafts/"},
			want:     []string{"CHANGELOG.md", "docs/a.md", "docs/sub/b.md", "pkg/CHANGELOG.md", "top.md"},
		},
		{
			name:     "single star stays in one segment",
			patterns: []string{"docs/*.md"},
			want:     []string{"CHANGELOG.md", "docs/sub/b.md", "drafts/c.md", "notes/drafts/d.md", "pkg/CHANGELOG.md", "top.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			makeTree(t, dir, files)

			got := discoverRel(t, dir, runner.Options{
				Paths:        []string{"."},
				WorkingDir:   dir,
				ExcludeGlobs: tt.patterns,
			})
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDiscover_ExcludeGlobsUseBaseDir covers patterns from a config file,
// which are relative to its directory rather than the working directory.
func TestDiscover_ExcludeGlobsUseBaseDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makeTree(t, dir, map[string]string{
		"proj/docs/skip.md": "", "proj/keep.md": "", "docs/outside.md": "",
	})

	t.Run("walking from above the base", func(t *testing.T) {
		t.Parallel()

		got := discoverRel(t, dir, runner.Options{
			Paths:         []string{"."},
			WorkingDir:    dir,
			ExcludeGlobs:  []string{"docs/**"},
			IgnoreBaseDir: filepath.Join(dir, "proj"),
		})
		assert.Equal(t, []string{"docs/outside.md", "proj/keep.md"}, got)
	})

	t.Run("walking from below the base", func(t *testing.T) {
		t.Parallel()

		got := discoverRel(t, dir, runner.Options{
			Paths:         []string{"."},
			WorkingDir:    filepath.Join(dir, "proj", "docs"),
			ExcludeGlobs:  []string{"docs/**"},
			IgnoreBaseDir: filepath.Join(dir, "proj"),
		})
		assert.Empty(t, got)
	})

	t.Run("named file below an excluded directory", func(t *testing.T) {
		t.Parallel()

		got := discoverRel(t, dir, runner.Options{
			Paths:         []string{"proj/docs/skip.md"},
			WorkingDir:    dir,
			ExcludeGlobs:  []string{"docs"},
			IgnoreBaseDir: filepath.Join(dir, "proj"),
		})
		assert.Empty(t, got)
	})
}

func TestDiscover_Gitignore(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) string {
		t.Helper()

		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
		makeTree(t, dir, map[string]string{
			".gitignore":      "build/\n*.gen.md\n",
			"docs/.gitignore": "local.md\n",
			"ok.md":           "", "build/out.md": "", "api.gen.md": "",
			"docs/local.md": "", "docs/guide.md": "", "local.md": "",
		})
		return dir
	}

	t.Run("skips ignored files while walking", func(t *testing.T) {
		t.Parallel()

		dir := setup(t)
		got := discoverRel(t, dir, runner.Options{Paths: []string{"."}, WorkingDir: dir, Gitignore: true})
		assert.Equal(t, []string{"docs/guide.md", "local.md", "ok.md"}, got)
	})

	t.Run("applies parent .gitignore files below the root", func(t *testing.T) {
		t.Parallel()

		dir := setup(t)
		makeTree(t, dir, map[string]string{"docs/x.gen.md": ""})
		got := discoverRel(t, dir, runner.Options{Paths: []string{"docs"}, WorkingDir: dir, Gitignore: true})
		assert.Equal(t, []string{"docs/guide.md"}, got)
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		dir := setup(t)
		got := discoverRel(t, dir, runner.Options{Paths: []string{"."}, WorkingDir: dir})
		assert.Len(t, got, 6)
	})

	t.Run("named paths are linted anyway", func(t *testing.T) {
		t.Parallel()

		dir := setup(t)
		got := discoverRel(t, dir, runner.Options{
			Paths: []string{"api.gen.md", "build"}, WorkingDir: dir, Gitignore: true,
		})
		assert.Equal(t, []string{"api.gen.md", "build/out.md"}, got)
	})
}

func TestDiscover_SkipDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makeTree(t, dir, map[string]string{
		"ok.md": "", "node_modules/pkg/readme.md": "", "vendor/mod/doc.md": "",
		"web/node_modules/dep/readme.md": "", "web/page.md": "",
	})

	got := discoverRel(t, dir, runner.Options{
		Paths: []string{"."}, WorkingDir: dir, SkipDirs: runner.DefaultSkipDirs(),
	})
	assert.Equal(t, []string{"ok.md", "web/page.md"}, got)

	// A directory named by the caller is walked, but nested ones are not.
	makeTree(t, dir, map[string]string{"node_modules/pkg/node_modules/x/readme.md": ""})
	got = discoverRel(t, dir, runner.Options{
		Paths: []string{"node_modules"}, WorkingDir: dir, SkipDirs: runner.DefaultSkipDirs(),
	})
	assert.Equal(t, []string{"node_modules/pkg/readme.md"}, got)
}

func TestRunner_Run_UnreadableDirectoryFails(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("permission bits do not stop root or Windows from reading directories")
	}

	dir := t.TempDir()
	makeTree(t, dir, map[string]string{"ok.md": "# Test\n", "locked/hidden.md": "# Test\n"})
	locked := filepath.Join(dir, "locked")
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { assert.NoError(t, os.Chmod(locked, 0o755)) })

	lintRunner := runner.New(lint.NewPipeline(lint.NewEngine(&mockParser{}, lint.NewRegistry())))
	result, err := lintRunner.Run(context.Background(), runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     config.NewConfig(),
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.Stats.FilesProcessed)
	assert.Equal(t, 1, result.Stats.FilesErrored)

	var failed []string
	for idx := range result.Files {
		if result.Files[idx].Failed() {
			failed = append(failed, result.Files[idx].Path)
		}
	}
	assert.Equal(t, []string{locked}, failed)
}
