package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pinForTest(t *testing.T, dir, name string) *PinnedFile {
	t.Helper()
	pinned, err := Pin(dir, name)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pinned.Close()) })
	return pinned
}

func writeFileForTest(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

// requireExchange skips a test that needs the race-free commit, which only
// Linux file systems with RENAME_EXCHANGE provide.
func requireExchange(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("committing by exchange needs Linux")
	}
}

func assertNoStrayFiles(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.ElementsMatch(t, want, names, "unexpected files in %s", dir)
}

func TestPinnedFile_RefusesAFileReplacedJustBeforeTheCommit(t *testing.T) {
	t.Parallel()
	requireExchange(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	writeFileForTest(t, path, "x\n")
	pinned := pinForTest(t, dir, "doc.md")
	pinned.beforeCommit = func() {
		require.NoError(t, os.Remove(path))
		writeFileForTest(t, path, "concurrent\n")
	}

	require.ErrorIs(t, pinned.Write([]byte("y\n")), ErrFileChanged)
	assert.Equal(t, "concurrent\n", readFileForTest(t, path), "a file saved in the last moment must survive")
	assertNoStrayFiles(t, dir, "doc.md")
}

func TestPinnedFile_RefusesAHardLinkCreatedJustBeforeTheCommit(t *testing.T) {
	t.Parallel()
	requireExchange(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	other := filepath.Join(dir, "other.md")
	writeFileForTest(t, path, "x\n")
	pinned := pinForTest(t, dir, "doc.md")
	pinned.beforeCommit = func() {
		require.NoError(t, os.Link(path, other))
	}

	require.ErrorIs(t, pinned.Write([]byte("y\n")), ErrFileChanged)
	first, err := os.Stat(path)
	require.NoError(t, err)
	second, err := os.Stat(other)
	require.NoError(t, err)
	assert.True(t, os.SameFile(first, second), "a hard link created in the last moment must not be split")
	assert.Equal(t, "x\n", readFileForTest(t, path))
	assertNoStrayFiles(t, dir, "doc.md", "other.md")
}

func TestPinnedFile_RefusesContentSavedJustBeforeTheCommit(t *testing.T) {
	t.Parallel()
	requireExchange(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	writeFileForTest(t, path, "x\n")
	pinned := pinForTest(t, dir, "doc.md")
	pinned.beforeCommit = func() {
		// Saved into the same file, as an editor that rewrites in place does.
		require.NoError(t, os.WriteFile(path, []byte("concurrent\n"), 0o644))
	}

	require.ErrorIs(t, pinned.Write([]byte("y\n")), ErrFileChanged)
	assert.Equal(t, "concurrent\n", readFileForTest(t, path))
	assertNoStrayFiles(t, dir, "doc.md")
}

func TestPinnedFile_CommitsByRenameWhereExchangeIsUnavailable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	writeFileForTest(t, path, "x\n")
	require.NoError(t, os.Chmod(path, 0o600))
	pinned := pinForTest(t, dir, "doc.md")
	pinned.noExchange = true

	require.NoError(t, pinned.Write([]byte("y\n")))
	assert.Equal(t, "y\n", readFileForTest(t, path))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assertNoStrayFiles(t, dir, "doc.md")
}

func TestPinnedFile_BackupNeverOverwritesABackupCreatedMeanwhile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	backup := path + BackupSuffix
	writeFileForTest(t, path, "x\n")
	pinned := pinForTest(t, dir, "doc.md")
	pinned.beforeCommit = func() {
		writeFileForTest(t, backup, "an earlier original\n")
	}

	created, err := pinned.Backup(BackupModeSidecar)
	require.NoError(t, err)
	assert.False(t, created, "a backup that appeared meanwhile counts as an existing backup")
	assert.Equal(t, "an earlier original\n", readFileForTest(t, backup), "an existing backup must never be overwritten")
	assertNoStrayFiles(t, dir, "doc.md", "doc.md"+BackupSuffix)
}

func TestPinnedFile_BackupRefusesAChangedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	writeFileForTest(t, path, "x\n")
	pinned := pinForTest(t, dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("concurrent\n"), 0o644))

	created, err := pinned.Backup(BackupModeSidecar)
	require.ErrorIs(t, err, ErrFileChanged)
	assert.False(t, created)
	assertNoStrayFiles(t, dir, "doc.md")
}

func TestPinnedFile_BackupStaysWithThePinnedDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFileForTest(t, filepath.Join(root, "docs", "doc.md"), "x\n")
	writeFileForTest(t, filepath.Join(root, "other", "doc.md"), "other\n")
	pinned := pinForTest(t, root, filepath.Join("docs", "doc.md"))

	// Move the directory aside and link another directory of the tree in its place.
	require.NoError(t, os.Rename(filepath.Join(root, "docs"), filepath.Join(root, "docs.aside")))
	if err := os.Symlink("other", filepath.Join(root, "docs")); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	created, err := pinned.Backup(BackupModeSidecar)
	require.ErrorIs(t, err, ErrFileChanged)
	assert.False(t, created)
	assertNoStrayFiles(t, filepath.Join(root, "other"), "doc.md")
	assertNoStrayFiles(t, filepath.Join(root, "docs.aside"), "doc.md")
	require.ErrorIs(t, pinned.Write([]byte("y\n")), ErrFileChanged)
	assert.Equal(t, "other\n", readFileForTest(t, filepath.Join(root, "other", "doc.md")))
}

func TestPinnedFile_TempFilesAreHidden(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFileForTest(t, filepath.Join(dir, "doc.md"), "x\n")
	pinned := pinForTest(t, dir, "doc.md")

	var seen []string
	pinned.beforeCommit = func() {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, entry := range entries {
			seen = append(seen, entry.Name())
		}
	}
	require.NoError(t, pinned.Write([]byte("y\n")))

	require.Len(t, seen, 2, "the temp file exists while the write commits")
	for _, name := range seen {
		if name != "doc.md" {
			assert.True(t, strings.HasPrefix(name, ".doc.md.tmp."), "temp file %q should be hidden and named after the file", name)
		}
	}
}
