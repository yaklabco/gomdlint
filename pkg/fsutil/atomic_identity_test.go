package fsutil_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fsutil"
)

// symlinkOrSkip creates a symbolic link or skips the test on platforms where
// the current user cannot create one.
func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}
}

// hardLinkOrSkip creates a hard link or skips the test where that is not supported.
func hardLinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Link(oldname, newname); err != nil {
		t.Skipf("hard links not supported here: %v", err)
	}
}

// assertNoTempFiles fails if a temp file from WriteAtomic is left in dir.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".tmp.", "temp file left behind in %s", dir)
	}
}

func TestWriteAtomic_WritesThroughSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.WriteFile(target, []byte("old\n"), 0o644))
	symlinkOrSkip(t, "target.md", link)

	require.NoError(t, fsutil.WriteAtomic(context.Background(), link, []byte("new\n"), 0o644))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link must stay a symbolic link")
	dest, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "target.md", dest, "the link must keep pointing at its target")

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got), "the link target must receive the new content")
	assertNoTempFiles(t, dir)
}

func TestWriteAtomic_WritesThroughSymlinkChainAcrossDirectories(t *testing.T) {
	t.Parallel()

	linkDir := t.TempDir()
	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "target.md")
	first := filepath.Join(linkDir, "first.md")
	second := filepath.Join(linkDir, "second.md")
	require.NoError(t, os.WriteFile(target, []byte("old\n"), 0o600))
	symlinkOrSkip(t, target, first)
	symlinkOrSkip(t, "first.md", second)

	require.NoError(t, fsutil.WriteAtomic(context.Background(), second, []byte("new\n"), 0o600))

	for _, link := range []string{first, second} {
		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "%s must stay a symbolic link", link)
	}
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got))
	assertNoTempFiles(t, linkDir)
	assertNoTempFiles(t, targetDir)
}

func TestWriteAtomic_RejectsDanglingSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	link := filepath.Join(dir, "link.md")
	symlinkOrSkip(t, "missing.md", link)

	err := fsutil.WriteAtomic(context.Background(), link, []byte("new\n"), 0o644)
	require.Error(t, err)

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "a dangling link must not be replaced")
	assertNoTempFiles(t, dir)
}

func TestWriteAtomic_PreservesHardLinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		original string
		content  string
	}{
		{name: "longer content", original: "short\n", content: "much longer content\n"},
		{name: "shorter content", original: "much longer original content\n", content: "short\n"},
		{name: "empty content", original: "something\n", content: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			first := filepath.Join(dir, "first.md")
			second := filepath.Join(dir, "second.md")
			require.NoError(t, os.WriteFile(first, []byte(tt.original), 0o644))
			hardLinkOrSkip(t, first, second)

			require.NoError(t, fsutil.WriteAtomic(context.Background(), first, []byte(tt.content), 0o644))

			for _, path := range []string{first, second} {
				got, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, tt.content, string(got), "%s must see the new content", filepath.Base(path))
			}

			firstInfo, err := os.Stat(first)
			require.NoError(t, err)
			secondInfo, err := os.Stat(second)
			require.NoError(t, err)
			assert.True(t, os.SameFile(firstInfo, secondInfo), "the hard link must not be split")
			assertNoTempFiles(t, dir)
		})
	}
}

func TestWriteAtomic_KeepsModeWhenRewritingHardLinkInPlace(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "second.md")
	require.NoError(t, os.WriteFile(first, []byte("old\n"), 0o600))
	hardLinkOrSkip(t, first, second)

	require.NoError(t, fsutil.WriteAtomic(context.Background(), second, []byte("new\n"), 0o600))

	info, err := os.Stat(first)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestReplaceFile_RefusesASymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.WriteFile(target, []byte("old\n"), 0o644))
	symlinkOrSkip(t, "target.md", link)

	err := fsutil.ReplaceFile(context.Background(), link, []byte("new\n"), 0o644)
	require.ErrorIs(t, err, fsutil.ErrSymlink)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "old\n", string(got), "a link that appeared at a resolved path must not be followed")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	assertNoTempFiles(t, dir)
}

func TestReplaceFile_WritesFilesAndKeepsHardLinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "second.md")
	require.NoError(t, os.WriteFile(first, []byte("old\n"), 0o644))
	hardLinkOrSkip(t, first, second)

	require.NoError(t, fsutil.ReplaceFile(context.Background(), first, []byte("new\n"), 0o644))

	got, err := os.ReadFile(second)
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got))

	fresh := filepath.Join(dir, "fresh.md")
	require.NoError(t, fsutil.ReplaceFile(context.Background(), fresh, []byte("created\n"), 0))
	got, err = os.ReadFile(fresh)
	require.NoError(t, err)
	assert.Equal(t, "created\n", string(got))
}

func TestWriteAtomic_RejectsDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := fsutil.WriteAtomic(context.Background(), dir, []byte("x"), 0o644)
	require.ErrorIs(t, err, fsutil.ErrIsDirectory)
}
