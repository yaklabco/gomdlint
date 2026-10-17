//go:build unix

package fsutil

import (
	"fmt"
	"os"
	"syscall"
)

// openNoFollow makes opening a path fail if its last element is a symbolic
// link, so an in-place rewrite never writes through a link that appeared
// after the path was checked.
const openNoFollow = syscall.O_NOFOLLOW

// hasMultipleLinks reports whether the file described by info has more than
// one hard link.
func hasMultipleLinks(_ string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink > 1
}

// restoreOwner gives tmp the owner and group of existing. It reports false,
// without an error, when that fails for any reason: the current user may not
// give files away (EPERM), the ID has no mapping in a user namespace
// (EINVAL), or the file system has no owners (ENOSYS, EOPNOTSUPP).
func restoreOwner(tmp ownerFile, existing os.FileInfo) (bool, error) {
	want, ok := existing.Sys().(*syscall.Stat_t)
	if !ok {
		return true, nil
	}

	info, err := tmp.Stat()
	if err != nil {
		return false, fmt.Errorf("stat temp file: %w", err)
	}
	have, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (have.Uid == want.Uid && have.Gid == want.Gid) {
		return true, nil
	}

	return tmp.Chown(int(want.Uid), int(want.Gid)) == nil, nil
}

// syncDir flushes a directory entry change, such as a rename, to disk. It is
// best-effort: a directory that cannot be opened (no read permission) or
// synced (some file systems do not support it) is left as it is.
func syncDir(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = handle.Sync() //nolint:errcheck // Best effort: the rename stands whether or not the directory can be synced.
	_ = handle.Close()
}
