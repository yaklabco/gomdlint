//go:build windows

package fsutil

import (
	"os"
	"syscall"
)

// openNoFollow is not available on Windows.
const openNoFollow = 0

// openNonBlock is not needed on Windows, where opening does not wait.
const openNonBlock = 0

// syncDirectories is false: Windows cannot flush a directory handle.
const syncDirectories = false

// hasMultipleLinks reports whether the open file has more than one hard
// link. When the link count cannot be read it reports true, so the caller
// rewrites the file in place and keeps its identity.
func hasMultipleLinks(file *os.File) bool {
	var data syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &data); err != nil {
		return true
	}
	return data.NumberOfLinks > 1
}

// restoreOwner is a no-op on Windows: a replacement file inherits its
// security descriptor from the directory.
func restoreOwner(_ ownerFile, _ os.FileInfo) (bool, error) {
	return true, nil
}
