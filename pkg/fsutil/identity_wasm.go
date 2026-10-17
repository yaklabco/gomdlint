//go:build js || wasip1

package fsutil

import (
	"os"
	"syscall"
)

// openNoFollow is not used on WebAssembly hosts.
const openNoFollow = 0

// syncDirectories is false: WebAssembly hosts do not sync directories.
const syncDirectories = false

// hasMultipleLinks reports whether the open file has more than one hard
// link, from the host's link count. When the link count cannot be read it
// reports true, so the caller rewrites the file in place and keeps its
// identity.
func hasMultipleLinks(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return true
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || stat.Nlink > 1
}

// restoreOwner is a no-op on WebAssembly hosts.
func restoreOwner(_ ownerFile, _ os.FileInfo) (bool, error) {
	return true, nil
}
