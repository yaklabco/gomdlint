//go:build !unix && !windows && !js && !wasip1

package fsutil

import "os"

// openNoFollow is not available on these platforms.
const openNoFollow = 0

// syncDirectories is false where directories cannot be synced.
const syncDirectories = false

// hasMultipleLinks reports false: the remaining platforms (Plan 9) have no
// hard links, so replacing a file cannot split one.
func hasMultipleLinks(_ *os.File) bool {
	return false
}

// restoreOwner is a no-op where files have no owner to restore.
func restoreOwner(_ ownerFile, _ os.FileInfo) (bool, error) {
	return true, nil
}
