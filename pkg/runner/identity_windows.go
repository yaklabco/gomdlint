//go:build windows

package runner

import (
	"io/fs"
	"syscall"
)

// fileID returns the volume serial number and file index of the file at
// path, which only the paths of one file share. It opens the file as
// os.Stat finds it, following links, and reports false when it cannot, as
// os.SameFile then does.
func fileID(path string, _ fs.FileInfo) (uint64, uint64, bool) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	handle, err := syscall.CreateFile(name, 0, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return 0, 0, false
	}
	defer syscall.CloseHandle(handle) //nolint:errcheck // Nothing was written through the handle.
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &info); err != nil {
		return 0, 0, false
	}
	return uint64(info.VolumeSerialNumber), uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow), true
}
