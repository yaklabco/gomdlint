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

func TestReplaceFile_ReplacesReadOnlyFileWhoseOwnerCannotBeRestored(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root can restore any owner and write any file")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("old content\n"), 0o444))

	before, err := os.Stat(path)
	require.NoError(t, err)

	// A read-only file of another user, such as one a container left in a
	// repository: it can be neither given back its owner nor written in
	// place, so it is replaced as before, keeping its mode.
	other := *statT(t, before)
	other.Uid++

	require.NoError(t, replaceFile(path, []byte("new\n"), 0o444, ownerOverride{FileInfo: before, stat: &other}))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got))

	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o444), after.Mode().Perm())
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

// fakeOwnerFile is an ownerFile whose Chown fails with a fixed error.
type fakeOwnerFile struct {
	info     os.FileInfo
	chownErr error
}

func (f fakeOwnerFile) Stat() (os.FileInfo, error) { return f.info, nil }
func (f fakeOwnerFile) Chown(_, _ int) error       { return f.chownErr }

func TestRestoreOwner_TreatsAnyChownFailureAsNotRestored(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
	info, err := os.Stat(path)
	require.NoError(t, err)
	other := *statT(t, info)
	other.Uid++

	// EINVAL: an ID with no mapping in a user namespace (rootless
	// containers); ENOSYS: a file system without chown support.
	for _, chownErr := range []error{syscall.EINVAL, syscall.ENOSYS, syscall.EPERM} {
		restored, err := restoreOwner(fakeOwnerFile{info: info, chownErr: chownErr}, ownerOverride{FileInfo: info, stat: &other})
		require.NoError(t, err, "chown failing with %v must not fail the write", chownErr)
		assert.False(t, restored, "chown failing with %v means the owner was not restored", chownErr)
	}
}

func TestWriteAtomic_DirectorySyncIsBestEffort(t *testing.T) {
	t.Parallel()

	// Without read permission the directory cannot be opened for syncing,
	// but files can still be created and renamed in it.
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o333))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0o755)) })

	path := filepath.Join(dir, "doc.md")
	require.NoError(t, WriteAtomic(context.Background(), path, []byte("new\n"), 0o644),
		"a file that was written must not be reported as a failed write")

	require.NoError(t, os.Chmod(dir, 0o755))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got))
}

func assertNoTempFilesInternal(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the target file should remain in %s", dir)
}
