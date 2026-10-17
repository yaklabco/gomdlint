//go:build !linux

package fsutil

import (
	"fmt"
	"os"
)

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

// renameNoReplace is only used after an exchange, which this platform
// cannot do.
func renameNoReplace(_ *os.File, from, to string) error {
	return fmt.Errorf("rename %s to %s: %w", from, to, errExchangeUnsupported)
}
