//go:build linux

package fsutil

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openExchangeDir opens the directory of root as a file, whose descriptor
// exchangeFiles uses. It is the same directory as root, opened through it.
func openExchangeDir(root *os.Root) (*os.File, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, fmt.Errorf("open directory: %w", err)
	}
	return dir, nil
}

// exchangeFiles atomically swaps the directory entries first and second in
// dir with renameat2(RENAME_EXCHANGE). It returns errExchangeUnsupported
// when the kernel or the file system cannot exchange.
func exchangeFiles(dir *os.File, first, second string) error {
	if dir == nil {
		return errExchangeUnsupported
	}
	fd := int(dir.Fd()) //nolint:gosec // A file descriptor is a small non-negative integer.
	err := unix.Renameat2(fd, first, fd, second, unix.RENAME_EXCHANGE)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EINVAL), errors.Is(err, unix.ENOSYS), errors.Is(err, unix.EOPNOTSUPP):
		return errExchangeUnsupported
	default:
		return fmt.Errorf("exchange %s and %s: %w", first, second, err)
	}
}

// renameNoReplace moves the entry from in dir to the name given by to,
// failing with an error that matches fs.ErrExist rather than replacing an
// entry that already has that name.
func renameNoReplace(dir *os.File, from, to string) error {
	fd := int(dir.Fd()) //nolint:gosec // A file descriptor is a small non-negative integer.
	if err := unix.Renameat2(fd, from, fd, to, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("rename %s to %s: %w", from, to, err)
	}
	return nil
}
