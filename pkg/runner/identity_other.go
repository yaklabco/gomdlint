//go:build !unix

package runner

import "io/fs"

// fileID reports false: os.Stat gives no file identity on these systems,
// so os.SameFile tells files apart.
func fileID(_ fs.FileInfo) (uint64, uint64, bool) {
	return 0, 0, false
}
