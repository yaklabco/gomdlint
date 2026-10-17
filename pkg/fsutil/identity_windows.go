//go:build windows

package fsutil

import (
	"os"
	"syscall"
)

// hasMultipleLinks reports whether the file at path has more than one hard
// link. If the link count cannot be read, the file is treated as having one.
func hasMultipleLinks(path string, _ os.FileInfo) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	var data syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &data); err != nil {
		return false
	}
	return data.NumberOfLinks > 1
}

// restoreOwner is a no-op on Windows: a replacement file inherits its
// security descriptor from the directory.
func restoreOwner(_ ownerFile, _ os.FileInfo) (bool, error) {
	return true, nil
}

// syncDir is a no-op on Windows, which cannot flush a directory handle.
func syncDir(_ string) {}
