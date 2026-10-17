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

// openNonBlock makes opening a FIFO return at once instead of waiting for
// a writer. It does not change reading or writing a regular file.
const openNonBlock = syscall.O_NONBLOCK

// syncDirectories is true where a directory can be synced to make a rename
// in it durable.
const syncDirectories = true

// hasMultipleLinks reports whether the open file has more than one hard
// link. When the link count cannot be read it reports true, so the caller
// rewrites the file in place and keeps its identity.
func hasMultipleLinks(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return true
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || stat.Nlink > 1
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
