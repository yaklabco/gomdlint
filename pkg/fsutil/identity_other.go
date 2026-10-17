//go:build !unix && !windows

package fsutil

import "os"

// hasMultipleLinks always reports false where link counts are unavailable.
func hasMultipleLinks(_ string, _ os.FileInfo) bool {
	return false
}

// restoreOwner is a no-op where files have no owner to restore.
func restoreOwner(_ ownerFile, _ os.FileInfo) (bool, error) {
	return true, nil
}

// syncDir is a no-op where directories cannot be synced.
func syncDir(_ string) {}
