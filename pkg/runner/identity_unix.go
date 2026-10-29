//go:build unix

package runner

import (
	"io/fs"
	"syscall"
)

// fileID returns the device and inode numbers of the file that info
// describes, which only the paths of one file share.
func fileID(info fs.FileInfo) (uint64, uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	//nolint:unconvert // Dev is signed on some systems and Ino narrower on others.
	return uint64(stat.Dev), uint64(stat.Ino), true
}
