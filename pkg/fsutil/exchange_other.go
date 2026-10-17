//go:build !linux

package fsutil

import "os"

// openExchangeDir returns no directory: only Linux can exchange two names
// atomically.
func openExchangeDir(_ *os.Root) (*os.File, error) {
	return nil, nil //nolint:nilnil // No handle is needed where exchanging is unavailable.
}

// exchangeFiles reports that exchanging two names atomically is not
// available on this platform.
func exchangeFiles(_ *os.File, _, _ string) error {
	return errExchangeUnsupported
}
