//go:build unix

package fsutil

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ownerOverride reports a different owner for an existing file, so tests can
// exercise owner restoration without root.
type ownerOverride struct {
	os.FileInfo

	stat *syscall.Stat_t
}

func (o ownerOverride) Sys() any { return o.stat }

func statT(t *testing.T, info os.FileInfo) *syscall.Stat_t {
	t.Helper()
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok, "expected *syscall.Stat_t")
	return stat
}

func TestReplaceFile_RestoresOwnerOrRewritesInPlace(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("old content\n"), 0o644))

	before, err := os.Stat(path)
	require.NoError(t, err)

	// Pretend the file belongs to another user.
	other := *statT(t, before)
	other.Uid++

	require.NoError(t, replaceFile(path, []byte("new\n"), 0o644, ownerOverride{FileInfo: before, stat: &other}))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got))

	after, err := os.Stat(path)
	require.NoError(t, err)
	if os.Geteuid() == 0 {
		// Root restores the owner on the replacement file.
		assert.Equal(t, other.Uid, statT(t, after).Uid, "owner must be restored")
		return
	}
	// Anyone else cannot give the file away, so the file is rewritten in
	// place and keeps its inode and owner.
	assert.True(t, os.SameFile(before, after), "file must be rewritten in place when its owner cannot be restored")
	assert.Equal(t, statT(t, before).Uid, statT(t, after).Uid)
	assertNoTempFilesInternal(t, dir)
}

func TestWriteAtomic_PreservesOwnerAsRoot(t *testing.T) {
	t.Parallel()

	if os.Geteuid() != 0 {
		t.Skip("changing a file's owner requires root")
	}

	const nobody = 65534
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("old\n"), 0o644))
	require.NoError(t, os.Chown(path, nobody, nobody))

	require.NoError(t, WriteAtomic(context.Background(), path, []byte("new\n"), 0o644))

	info, err := os.Stat(path)
	require.NoError(t, err)
	stat := statT(t, info)
	assert.Equal(t, uint32(nobody), stat.Uid)
	assert.Equal(t, uint32(nobody), stat.Gid)
}

func TestSyncDir(t *testing.T) {
	t.Parallel()

	require.NoError(t, syncDir(t.TempDir()))
	require.Error(t, syncDir(filepath.Join(t.TempDir(), "missing")))
}

func assertNoTempFilesInternal(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the target file should remain in %s", dir)
}
