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
		// A leading "./" or "/" is the base directory.
		{pattern: "./docs/**", path: "docs/a.md", want: true},
		{pattern: "/docs/*.md", path: "docs/a.md", want: true},
		{pattern: "/docs/*.md", path: "x/docs/a.md", want: false},
		{pattern: "/CHANGELOG.md", path: "pkg/CHANGELOG.md", want: false},
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

// TestSetMatchAnchoredPatterns covers patterns tied to the base directory:
// a leading slash anchors a pattern there, as in .gitignore, and an
// absolute path inside the base directory means the same location.
func TestSetMatchAnchoredPatterns(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	set, err := ignore.NewSet(base, []string{
		"/docs/**",
		filepath.Join(base, "drafts") + string(filepath.Separator) + "**",
	})
	require.NoError(t, err)

	assert.True(t, set.Match(filepath.Join(base, "docs", "a.md"), false))
	assert.False(t, set.Match(filepath.Join(base, "sub", "docs", "a.md"), false), "anchored to the base")
	assert.True(t, set.Match(filepath.Join(base, "drafts", "x.md"), false))
	assert.False(t, set.Match(filepath.Join(base, "sub", "drafts", "x.md"), false))
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

	for _, malformed := range []string{"foo[", "[[:bogus:]].md", "[[:alpha:].md", `a\`} {
		require.Error(t, ignore.ValidatePattern(malformed), malformed)
	}
	for _, valid := range []string{"generated[[:digit:]].md", "[]a].md", `[\]].md`, "[a-].md", `\*.md`} {
		require.NoError(t, ignore.ValidatePattern(valid), valid)
	}
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

// TestGitignoreMoreRules holds more patterns to `git check-ignore`'s
// answers (with the global excludes file disabled).
func TestGitignoreMoreRules(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o755))
	writeTree(t, repo, map[string]string{
		".gitignore": "foo/**/bar\n**/deep\n*.md\n!keep*.md\n\\!bang.md\na\\ b.md\ntrail\\ \n/only/**\n",
	})
	rules, err := ignore.GitignoreFor(repo)
	require.NoError(t, err)

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{path: "foo/bar", want: true},
		{path: "foo/x/bar", want: true},
		{path: "foo/x/y/bar", want: true},
		{path: "foo/barx", want: false},
		{path: "deep", want: true},
		{path: "x/deep", want: true},
		{path: "a.md", want: true},
		{path: "keep1.md", want: false},
		{path: "!bang.md", want: true},
		{path: "a b.md", want: true},
		{path: "trail ", want: true},
		{path: "only", isDir: true, want: false},
		{path: "only/z", isDir: true, want: true},
		{path: "only/z/q.md", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, rules.Ignored(filepath.Join(repo, filepath.FromSlash(tt.path)), tt.isDir))
		})
	}
}

// TestGitignoreWildmatch holds the pattern language to Git's wildmatch, with
// each verdict taken from `git -c core.excludesFile=/dev/null check-ignore`
// (one pattern per directory, so cases cannot interact).
func TestGitignoreWildmatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		ignored []string
		kept    []string
	}{
		{pattern: "generated[[:digit:]].md", ignored: []string{"generated1.md"}, kept: []string{"generatedx.md"}},
		{pattern: "[]a].md", ignored: []string{"].md", "a.md"}, kept: []string{"b.md"}},
		{pattern: "?.md", ignored: []string{"x.md"}, kept: []string{"界.md", "ab.md"}},
		{pattern: "???.md", ignored: []string{"界.md", "abc.md"}},
		{pattern: "[!a].md", ignored: []string{"b.md"}, kept: []string{"a.md"}},
		{pattern: "[^a].md", ignored: []string{"b.md"}, kept: []string{"a.md"}},
		{pattern: "[a-c].md", ignored: []string{"b.md"}, kept: []string{"d.md"}},
		{pattern: `[\]].md`, ignored: []string{"].md"}, kept: []string{"x.md"}},
		{pattern: `\*.md`, ignored: []string{"*.md"}, kept: []string{"x.md"}},
		{pattern: "foo[", kept: []string{"foo["}},
		{pattern: "[[:bogus:]].md", kept: []string{"a.md"}},
		{pattern: "[[:alpha:]].md", ignored: []string{"a.md"}, kept: []string{"1.md"}},
		{pattern: "[[:upper:]].md", ignored: []string{"A.md"}, kept: []string{"a.md"}},
		{pattern: "[[:punct:]].md", ignored: []string{"_.md"}, kept: []string{"a.md"}},
		// Git's ctype counts tab, newline, carriage return and space as
		// space, but not vertical tab or form feed.
		{pattern: "[[:space:]]x.md", ignored: []string{" x.md", "\tx.md", "\rx.md"}, kept: []string{"ax.md", "\vx.md", "\fx.md"}},
		{pattern: "[[:xdigit:]].md", ignored: []string{"f.md"}, kept: []string{"g.md"}},
		{pattern: "[[:graph:]].md", ignored: []string{"a.md", "~.md"}, kept: []string{" .md"}},
		{pattern: "[[:print:]].md", ignored: []string{" .md", "a.md"}, kept: []string{"\x7f.md"}},
		{pattern: "[[:cntrl:]].md", ignored: []string{"\x01.md"}, kept: []string{"a.md"}},
		{pattern: "[[:blank:]].md", ignored: []string{"\t.md", " .md"}, kept: []string{"\v.md"}},
		{pattern: "[[:lower:]].md", ignored: []string{"a.md"}, kept: []string{"A.md"}},
		{pattern: "[[:alnum:]].md", ignored: []string{"a.md", "1.md"}, kept: []string{"_.md"}},
		{pattern: "[a-].md", ignored: []string{"-.md", "a.md"}, kept: []string{"b.md"}},
		{pattern: "[[:digit:]-z].md", ignored: []string{"5.md", "-.md", "z.md"}, kept: []string{"y.md"}},
		{pattern: "[--0].md", ignored: []string{"-.md", "0.md"}, kept: []string{"1.md"}},
		{pattern: `[a-\z].md`, ignored: []string{"m.md"}},
		// A reversed range matches nothing, but its first character
		// still counts on its own.
		{pattern: "[z-a].md", ignored: []string{"z.md"}, kept: []string{"m.md", "a.md"}},
		{pattern: "[[:]x.md", ignored: []string{"[x.md", ":x.md"}, kept: []string{"ax.md"}},
		{pattern: "[!]a]x", ignored: []string{"bx"}, kept: []string{"ax", "]x"}},
		{pattern: "[[:alpha:].md", kept: []string{"a.md", "[.md", ":.md"}},
		{pattern: "[界].md", kept: []string{"界.md"}},
		{pattern: `a\`, kept: []string{`a\`, "a"}},
		{pattern: `\#foo`, ignored: []string{"#foo"}},
		{pattern: "*.MD", ignored: []string{"x.MD"}, kept: []string{"x.md"}},
		// A slash inside a bracket expression does not split the pattern,
		// and can never match.
		{pattern: "x[a/b]y", ignored: []string{"xay", "xby"}, kept: []string{"xcy"}},
		// An escaped slash separates directories like a plain one.
		{pattern: `a\/b`, ignored: []string{"a/b"}},
		{pattern: "a/***/b", ignored: []string{"a/b", "a/x/b", "a/x/y/b"}},
		// Before an escaped slash, "**" must match at least one directory.
		{pattern: `**\/b`, ignored: []string{"x/b"}, kept: []string{"b"}},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeTree(t, dir, map[string]string{".gitignore": tt.pattern + "\n"})
			rules, err := ignore.GitignoreFor(dir)
			require.NoError(t, err)

			for _, name := range tt.ignored {
				assert.True(t, rules.Ignored(filepath.Join(dir, name), false), "%q should be ignored", name)
			}
			for _, name := range tt.kept {
				assert.False(t, rules.Ignored(filepath.Join(dir, name), false), "%q should be kept", name)
			}
		})
	}
}

// TestSetMatchWildmatch applies the same pattern language to gomdlint's own
// ignore patterns.
func TestSetMatchWildmatch(t *testing.T) {
	t.Parallel()

	base := filepath.FromSlash("/base")
	set, err := ignore.NewSet(base, []string{"generated[[:digit:]].md", "?.md", "[]a]-x.md"})
	require.NoError(t, err)

	assert.True(t, set.Match(filepath.Join(base, "generated7.md"), false))
	assert.True(t, set.Match(filepath.Join(base, "q.md"), false))
	assert.False(t, set.Match(filepath.Join(base, "界.md"), false), "? matches one byte, as in Git")
	assert.True(t, set.Match(filepath.Join(base, "]-x.md"), false))
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
