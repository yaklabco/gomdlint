package ignore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/ignore"
)

func TestSetMatch(t *testing.T) {
	t.Parallel()

	base := filepath.FromSlash("/base")

	tests := []struct {
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		// "**" matches whole segments only.
		{pattern: "**/a.md", path: "a.md", want: true},
		{pattern: "**/a.md", path: "docs/a.md", want: true},
		{pattern: "**/a.md", path: "docs/sub/a.md", want: true},
		{pattern: "**/a.md", path: "data.md", want: false},
		{pattern: "**/a.md", path: "docs/data.md", want: false},
		{pattern: "docs/**/*.md", path: "docs/a.md", want: true},
		{pattern: "docs/**/*.md", path: "docs/x/y/a.md", want: true},
		{pattern: "docs/**/*.md", path: "mydocs/a.md", want: false},
		{pattern: "docs/**/*.md", path: "a.md", want: false},
		// A trailing "**" also matches the directory itself, so walks prune it.
		{pattern: "vendor/**", path: "vendor", isDir: true, want: true},
		{pattern: "vendor/**", path: "vendor/pkg/doc.md", want: true},
		{pattern: "vendor/**", path: "vendors/doc.md", want: false},
		// "*" stays within one segment.
		{pattern: "docs/*.md", path: "docs/a.md", want: true},
		{pattern: "docs/*.md", path: "docs/sub/a.md", want: false},
		// Patterns without a slash match a name at any depth.
		{pattern: "CHANGELOG.md", path: "CHANGELOG.md", want: true},
		{pattern: "CHANGELOG.md", path: "pkg/CHANGELOG.md", want: true},
		{pattern: "*.draft.md", path: "docs/x.draft.md", want: true},
		{pattern: "node_modules", path: "web/node_modules", isDir: true, want: true},
		// A trailing slash matches directories only.
		{pattern: "build/", path: "build", isDir: true, want: true},
		{pattern: "build/", path: "build", isDir: false, want: false},
		// A leading "./" is the base directory.
		{pattern: "./docs/**", path: "docs/a.md", want: true},
		// Paths outside the base directory never match relative patterns.
		{pattern: "**/a.md", path: "../other/a.md", want: false},
		// Character classes.
		{pattern: "draft-[0-9].md", path: "draft-1.md", want: true},
		{pattern: "draft-[0-9].md", path: "draft-x.md", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+" "+tt.path, func(t *testing.T) {
			t.Parallel()

			set, err := ignore.NewSet(base, []string{tt.pattern})
			require.NoError(t, err)

			path := filepath.Join(base, filepath.FromSlash(tt.path))
			assert.Equal(t, tt.want, set.Match(path, tt.isDir))
		})
	}
}

func TestSetMatchAbsolutePattern(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	set, err := ignore.NewSet(filepath.Join(dir, "elsewhere"), []string{filepath.ToSlash(dir) + "/docs/**"})
	require.NoError(t, err)

	assert.True(t, set.Match(filepath.Join(dir, "docs", "a.md"), false))
	assert.False(t, set.Match(filepath.Join(dir, "a.md"), false))
}

func TestSetMatchWithParents(t *testing.T) {
	t.Parallel()

	base := filepath.FromSlash("/base")
	set, err := ignore.NewSet(base, []string{"drafts", "build/"})
	require.NoError(t, err)

	assert.True(t, set.MatchWithParents(filepath.Join(base, "docs", "drafts", "a.md"), false))
	assert.True(t, set.MatchWithParents(filepath.Join(base, "build", "out", "a.md"), false))
	assert.False(t, set.MatchWithParents(filepath.Join(base, "docs", "a.md"), false))
	assert.False(t, set.Match(filepath.Join(base, "docs", "drafts", "a.md"), false),
		"Match checks the path itself only")
}

func TestNewSetRejectsMalformedPatterns(t *testing.T) {
	t.Parallel()

	_, err := ignore.NewSet("/base", []string{"docs/[a-"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docs/[a-")

	require.Error(t, ignore.ValidatePattern("[z-a"))
	require.NoError(t, ignore.ValidatePattern("docs/**/*.md"))
}

// writeTree creates files under dir from a map of slash-separated paths.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

func TestGitignoreRules(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o755))
	writeTree(t, repo, map[string]string{
		".gitignore": "# build output\nbuild/\n*.tmp.md\n/rooted.md\n!keep.tmp.md\n" +
			"logs/**\n!logs/keep.md\n\\#hash.md\nspaced.md   \nnot-[!a]*.md\n",
		"sub/.gitignore": "local.md\n!/rooted.md\n",
	})

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{path: "build", isDir: true, want: true},
		{path: "sub/build", isDir: true, want: true},
		{path: "build.md", want: false},
		{path: "a.tmp.md", want: true},
		{path: "sub/a.tmp.md", want: true},
		{path: "keep.tmp.md", want: false},
		{path: "rooted.md", want: true},
		{path: "other/rooted.md", want: false},
		{path: "logs", isDir: true, want: false},
		{path: "logs/x.md", want: true},
		{path: "logs/keep.md", want: false},
		{path: "#hash.md", want: true},
		{path: "spaced.md", want: true},
		{path: "not-b.md", want: true},
		{path: "not-a.md", want: false},
		{path: "ok.md", want: false},
		// Nested .gitignore files apply below their directory only.
		{path: "sub/local.md", want: true},
		{path: "local.md", want: false},
		{path: "other/local.md", want: false},
	}

	root, err := ignore.GitignoreFor(repo)
	require.NoError(t, err)
	sub, err := root.Child(filepath.Join(repo, "sub"))
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			rules := root
			if filepath.Dir(filepath.FromSlash(tt.path)) == "sub" {
				rules = sub
			}
			assert.Equal(t, tt.want, rules.Ignored(filepath.Join(repo, filepath.FromSlash(tt.path)), tt.isDir))
		})
	}
}

// TestGitignoreForSubdirectory covers walks that start below the repository
// root, where the .gitignore files of parent directories still apply.
func TestGitignoreForSubdirectory(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o755))
	writeTree(t, repo, map[string]string{
		".gitignore":      "drafts/\n",
		"docs/.gitignore": "*.local.md\n",
		"docs/guide/a.md": "",
	})

	rules, err := ignore.GitignoreFor(filepath.Join(repo, "docs", "guide"))
	require.NoError(t, err)

	assert.True(t, rules.Ignored(filepath.Join(repo, "docs", "guide", "drafts"), true))
	assert.True(t, rules.Ignored(filepath.Join(repo, "docs", "guide", "x.local.md"), false))
	assert.False(t, rules.Ignored(filepath.Join(repo, "docs", "guide", "a.md"), false))
}

// TestGitignoreForStopsAtRepositoryRoot keeps .gitignore files above the
// repository out of its rules.
func TestGitignoreForStopsAtRepositoryRoot(t *testing.T) {
	t.Parallel()

	outer := t.TempDir()
	repo := filepath.Join(outer, "repo")
	writeTree(t, outer, map[string]string{
		".gitignore":   "*.md\n",
		"repo/.git":    "gitdir: elsewhere\n", // A worktree's .git is a file.
		"repo/docs.md": "",
	})

	rules, err := ignore.GitignoreFor(repo)
	require.NoError(t, err)

	assert.False(t, rules.Ignored(filepath.Join(repo, "docs.md"), false))
}

// TestGitignoreForOutsideRepository applies only the directory's own
// .gitignore when no repository encloses it.
func TestGitignoreForOutsideRepository(t *testing.T) {
	t.Parallel()

	outer := t.TempDir()
	dir := filepath.Join(outer, "dir")
	writeTree(t, outer, map[string]string{
		".gitignore":     "*.md\n",
		"dir/.gitignore": "skip.md\n",
	})

	rules, err := ignore.GitignoreFor(dir)
	require.NoError(t, err)

	assert.True(t, rules.Ignored(filepath.Join(dir, "skip.md"), false))
	assert.False(t, rules.Ignored(filepath.Join(dir, "keep.md"), false))
}
