//go:build unix

package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// hasMultipleLinks reports whether the file described by info has more than
// one hard link.
func hasMultipleLinks(_ string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink > 1
}

// restoreOwner gives tmp the owner and group of existing. It reports false,
// without an error, when the current user is not allowed to do that.
func restoreOwner(tmp *os.File, existing os.FileInfo) (bool, error) {
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

	if err := tmp.Chown(int(want.Uid), int(want.Gid)); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return false, nil
		}
		return false, fmt.Errorf("chown temp file: %w", err)
	}
	return true, nil
}

// syncDir flushes a directory entry change, such as a rename, to disk.
// File systems that cannot sync directories are not treated as an error.
func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}

	syncErr := handle.Sync()
	closeErr := handle.Close()
	if syncErr != nil && !errors.Is(syncErr, syscall.EINVAL) && !errors.Is(syncErr, syscall.ENOTSUP) {
		return fmt.Errorf("sync directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close directory: %w", closeErr)
	}
	return nil
}
