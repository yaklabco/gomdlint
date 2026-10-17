package fsutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// replaceWithFIFO moves the file at path to saved and puts a FIFO in its
// place. Opening a FIFO for reading blocks until a writer opens it.
func replaceWithFIFO(t *testing.T, path, saved string) {
	t.Helper()
	assert.NoError(t, os.Rename(path, saved))
	assert.NoError(t, unix.Mkfifo(path, 0o644))
}

// assertFIFOKept checks that the FIFO is still at path and the moved file
// is untouched.
func assertFIFOKept(t *testing.T, dir, path, saved string) {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeNamedPipe, "the FIFO is not replaced")
	assert.Equal(t, "x\n", readFileForTest(t, saved), "the moved file is not touched")
	assertNoStrayFiles(t, dir, "doc.md", "saved.md")
}

func TestPinnedFile_RefusesAFIFOPutInPlaceOfTheFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	saved := filepath.Join(dir, "saved.md")
	writeFileForTest(t, path, "x\n")
	pinned := pinForTest(t, dir, "doc.md")
	replaceWithFIFO(t, path, saved)

	require.ErrorIs(t, pinned.Write([]byte("y\n")), ErrFileChanged, "the write must neither block nor replace the FIFO")
	assertFIFOKept(t, dir, path, saved)
}

func TestPinnedFile_RefusesAFIFOPutInPlaceOfTheFileJustBeforeTheCommit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	saved := filepath.Join(dir, "saved.md")
	writeFileForTest(t, path, "x\n")
	pinned := pinForTest(t, dir, "doc.md")
	pinned.beforeCommit = func() { replaceWithFIFO(t, path, saved) }

	require.ErrorIs(t, pinned.Write([]byte("y\n")), ErrFileChanged, "the write must neither block nor keep the FIFO swapped out")
	assertFIFOKept(t, dir, path, saved)
}
