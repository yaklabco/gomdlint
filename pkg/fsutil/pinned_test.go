package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fsutil"
)

func pinFile(t *testing.T, dir, name string) *fsutil.PinnedFile {
	t.Helper()
	pinned, err := fsutil.Pin(dir, name)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pinned.Close()) })
	return pinned
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func TestPin_ReadsTheFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "docs", "a.md"), "x\n", 0o640)

	pinned := pinFile(t, dir, filepath.Join("docs", "a.md"))

	assert.Equal(t, "x\n", string(pinned.Content()))
	info := pinned.Info()
	assert.Equal(t, filepath.Join(dir, "docs", "a.md"), info.Path)
	assert.Equal(t, int64(2), info.Size)
	assert.Equal(t, os.FileMode(0o640), info.Mode.Perm())
}

func TestPin_RejectsMissingFilesDirectoriesAndEscapes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	outside := filepath.Join(t.TempDir(), "outside.md")
	writeTestFile(t, outside, "outside\n", 0o644)

	_, err := fsutil.Pin(dir, "missing.md")
	require.ErrorIs(t, err, fsutil.ErrNotFound)

	_, err = fsutil.Pin(dir, "sub")
	require.ErrorIs(t, err, fsutil.ErrIsDirectory)

	_, err = fsutil.Pin(dir, filepath.Join("..", filepath.Base(filepath.Dir(outside)), "outside.md"))
	require.Error(t, err, "a name must not leave the directory")

	symlinkOrSkip(t, outside, filepath.Join(dir, "link.md"))
	_, err = fsutil.Pin(dir, "link.md")
	require.Error(t, err, "a link must not lead out of the directory")
}

func TestPinnedFile_WriteReplacesTheContentAndKeepsTheMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeTestFile(t, path, "x\n", 0o600)

	pinned := pinFile(t, dir, "a.md")
	require.NoError(t, pinned.Write([]byte("y\n")))

	assert.Equal(t, "y\n", readTestFile(t, path))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assertNoTempFiles(t, dir)
}

func TestPinnedFile_WriteKeepsHardLinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "second.md")
	writeTestFile(t, first, "a much longer original\n", 0o644)
	hardLinkOrSkip(t, first, second)

	pinned := pinFile(t, dir, "first.md")
	require.NoError(t, pinned.Write([]byte("new\n")))

	assert.Equal(t, "new\n", readTestFile(t, second))
	firstInfo, err := os.Stat(first)
	require.NoError(t, err)
	secondInfo, err := os.Stat(second)
	require.NoError(t, err)
	assert.True(t, os.SameFile(firstInfo, secondInfo), "the hard link must not be split")
	assertNoTempFiles(t, dir)
}

func TestPinnedFile_WriteRefusesAReplacedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeTestFile(t, path, "x\n", 0o644)
	pinned := pinFile(t, dir, "a.md")

	// Another writer replaced the file after it was read.
	require.NoError(t, os.Remove(path))
	writeTestFile(t, path, "concurrent\n", 0o644)

	require.ErrorIs(t, pinned.Write([]byte("y\n")), fsutil.ErrFileChanged)
	assert.Equal(t, "concurrent\n", readTestFile(t, path), "the other writer's content must survive")
	assertNoTempFiles(t, dir)
}

func TestPinnedFile_WriteRefusesAHardLinkToAnotherFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	victim := filepath.Join(t.TempDir(), "victim.md")
	writeTestFile(t, path, "x\n", 0o644)
	writeTestFile(t, victim, "victim\n", 0o644)
	pinned := pinFile(t, dir, "a.md")

	// The checked name now refers to another file through a hard link.
	require.NoError(t, os.Remove(path))
	hardLinkOrSkip(t, victim, path)

	require.ErrorIs(t, pinned.Write([]byte("y\n")), fsutil.ErrFileChanged)
	assert.Equal(t, "victim\n", readTestFile(t, victim), "another file must never be rewritten")
}

func TestPinnedFile_WriteRefusesAParentDirectoryReplacedByALink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(root, "docs", "a.md"), "x\n", 0o644)
	writeTestFile(t, filepath.Join(outside, "a.md"), "outside\n", 0o644)
	pinned := pinFile(t, root, filepath.Join("docs", "a.md"))

	// Move the checked directory aside and put a link to another directory,
	// holding a file of the same name, in its place.
	require.NoError(t, os.Rename(filepath.Join(root, "docs"), filepath.Join(root, "docs.aside")))
	symlinkOrSkip(t, outside, filepath.Join(root, "docs"))

	require.ErrorIs(t, pinned.Write([]byte("y\n")), fsutil.ErrFileChanged)
	assert.Equal(t, "outside\n", readTestFile(t, filepath.Join(outside, "a.md")),
		"a write must never leave the directory it was pinned in")
	assert.Equal(t, "x\n", readTestFile(t, filepath.Join(root, "docs.aside", "a.md")))
}

func TestPinnedFile_Check(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeTestFile(t, path, "x\n", 0o644)
	pinned := pinFile(t, dir, "a.md")

	require.NoError(t, pinned.Check(true))

	writeTestFile(t, path, "z\n", 0o644)
	require.ErrorIs(t, pinned.Check(true), fsutil.ErrFileChanged, "changed content must be noticed")

	require.NoError(t, os.Remove(path))
	writeTestFile(t, path, "x\n", 0o644)
	require.ErrorIs(t, pinned.Check(true), fsutil.ErrFileChanged, "a replaced file must be noticed")
}

func TestPinnedFile_BackupSavesTheContentThatWasRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeTestFile(t, path, "original\n", 0o640)
	pinned := pinFile(t, dir, "a.md")

	created, err := pinned.Backup(fsutil.BackupModeNone)
	require.NoError(t, err)
	assert.False(t, created)

	created, err = pinned.Backup(fsutil.BackupModeSidecar)
	require.NoError(t, err)
	assert.True(t, created)
	backup := path + fsutil.BackupSuffix
	assert.Equal(t, "original\n", readTestFile(t, backup))
	info, err := os.Stat(backup)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())

	created, err = pinned.Backup(fsutil.BackupModeSidecar)
	require.NoError(t, err)
	assert.False(t, created, "an existing backup is kept")
}
