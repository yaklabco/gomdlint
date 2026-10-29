//go:build !unix && !windows

package runner

import "io/fs"

// fileID reports false: these systems give no file identity,
// so os.SameFile tells files apart.
func fileID(_ string, _ fs.FileInfo) (uint64, uint64, bool) {
	return 0, 0, false
}
